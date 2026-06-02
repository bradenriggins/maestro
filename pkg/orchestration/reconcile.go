package orchestration

import (
	"fmt"
	"sort"
	"time"

	"maestro/log"
)

// TmuxChecker abstracts tmux session existence checking for testability.
type TmuxChecker interface {
	HasSession(name string) bool
}

// RealTmuxChecker uses actual tmux commands.
type RealTmuxChecker struct{}

func (r RealTmuxChecker) HasSession(name string) bool {
	return tmuxHasSession(name)
}

// ReconcileResult reports what the reconciliation found.
type ReconcileResult struct {
	DeadSessions    []string // instance names with dead tmux sessions
	StaleTasks      []string // task IDs marked stale
	FailedTasks     []string // task IDs promoted from stale to failed
	StatusCorrected []string // instance names where status file was corrected
	DAGDispatched   []string // task IDs dispatched by DAG pass
	DAGBlocked      []string // task IDs blocked by DAG pass
	DAGUnblocked    []string // task IDs unblocked by DAG pass
}

// Reconcile checks registry against tmux reality and fixes state.
// This is the core reconciliation logic from spec Section 12.1.
// It returns the updated registry so the caller can write it, avoiding
// concurrent writes to registry.json.
func Reconcile(tmux TmuxChecker, registryPath string, store *TaskStore) (*ReconcileResult, *Registry, error) {
	result := &ReconcileResult{}

	reg, err := LoadRegistryFromPath(registryPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load registry: %w", err)
	}

	for name, entry := range reg.Instances {
		tmuxAlive := tmux.HasSession(entry.TmuxSession)

		// Fetch all tasks for this instance once; reuse across all blocks below.
		tasks, tasksErr := store.ForInstance(name)

		// 1. Detect dead sessions
		sessionExpectedAlive := entry.Status == RegistryStatusRunning ||
			entry.Status == RegistryStatusPaused ||
			entry.Status == RegistryStatusStarting
		if !tmuxAlive && sessionExpectedAlive {
			entry.Status = RegistryStatusDead
			now := NowISO()
			entry.DiedAt = &now
			reg.Instances[name] = entry
			result.DeadSessions = append(result.DeadSessions, name)

			// Mark in_progress tasks for this instance as stale
			if tasksErr == nil {
				for _, task := range tasks {
					if task.Status == StatusInProgress {
						// Re-read immediately before writing: the worker process
						// writes the same file concurrently, and our `tasks`
						// snapshot is up to ~5s old. Without this, a task the
						// worker just completed gets clobbered back to stale
						// (and later failed), silently discarding the result.
						fresh, getErr := store.Get(task.ID)
						if getErr != nil || fresh.Status != StatusInProgress {
							continue
						}
						fresh.Status = StatusStale
						errMsg := "Worker tmux session died during execution"
						fresh.Error = &errMsg
						if err := store.Update(fresh); err != nil {
							log.WarningLog.Printf("reconcile: failed to mark task %s as stale: %v", fresh.ID, err)
							continue
						}
						result.StaleTasks = append(result.StaleTasks, fresh.ID)
					}
				}
			}
		}

		// 2. Reconcile task vs. status file state
		if tmuxAlive {
			ws, err := store.ReadStatus(name)
			if err == nil && ws.State == StateWorking {
				if ws.LastTask != "" {
					lastTask, taskErr := store.Get(ws.LastTask)
					if taskErr == nil && lastTask.IsTerminal() {
						// Worker finished but forgot to update status file. Re-read
						// the status immediately before correcting it: the worker
						// may have already moved on to a new task (its own write
						// would be clobbered, and we'd flip a genuinely-working
						// worker to idle → a second task gets double-dispatched
						// on top of the first).
						cur, curErr := store.ReadStatus(name)
						if curErr != nil || cur.State != StateWorking || cur.LastTask != ws.LastTask {
							// Worker changed its status since our read; leave it alone.
						} else {
							cur.State = StateIdle
							cur.Timestamp = NowISO()
							statusPath := store.StatusFilePath(name)
							if writeErr := AtomicWriteJSON(statusPath, cur); writeErr != nil {
								// Log but don't fail the whole reconciliation; fall through to block 3
								log.WarningLog.Printf("reconcile: failed to correct status file for %s: %v", name, writeErr)
							} else {
								result.StatusCorrected = append(result.StatusCorrected, name)
							}
						}
					}
				}
			}
		}

		// 3. Detect stale tasks (in stale state for > DefaultStallThreshold) → promote to failed.
		// Note: tasks newly marked stale in block 1 above have UpdatedAt == NowISO(), so
		// they will not cross the threshold here and will only be promoted on a future
		// reconciliation pass — this is intentional (gives operators a window to inspect).
		if tasksErr == nil {
			for _, task := range tasks {
				if task.Status == StatusStale {
					updatedAt, parseErr := ParseISO(task.UpdatedAt)
					if parseErr != nil {
						log.WarningLog.Printf("reconcile: task %s has unparseable UpdatedAt %q, skipping stale promotion: %v", task.ID, task.UpdatedAt, parseErr)
						continue
					}
					if time.Since(updatedAt) > DefaultStallThreshold {
						// Re-read before promoting: the task may have been
						// redispatched (stale→dispatched) since our snapshot.
						fresh, getErr := store.Get(task.ID)
						if getErr != nil || fresh.Status != StatusStale {
							continue
						}
						fresh.Status = StatusFailed
						if err := store.Update(fresh); err != nil {
							log.WarningLog.Printf("reconcile: failed to promote task %s to failed: %v", fresh.ID, err)
							continue
						}
						result.FailedTasks = append(result.FailedTasks, fresh.ID)
					}
				}
			}
		}
	}

	// 4. DAG pass: dispatch pending tasks whose deps completed, block tasks whose deps failed
	dagResult, dagErr := RunDAGPass(store)
	if dagErr != nil {
		log.WarningLog.Printf("reconcile: DAG pass error: %v", dagErr)
	}
	if dagResult != nil {
		result.DAGDispatched = dagResult.Dispatched
		result.DAGBlocked = dagResult.Blocked
		result.DAGUnblocked = dagResult.Unblocked
	}

	// Sort result slices for deterministic output (map iteration order is non-deterministic).
	sort.Strings(result.DeadSessions)
	sort.Strings(result.StaleTasks)
	sort.Strings(result.FailedTasks)
	sort.Strings(result.StatusCorrected)

	// Return the updated registry for the caller to write (avoids write contention)
	reg.UpdatedAt = NowISO()

	return result, reg, nil
}
