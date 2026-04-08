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
						task.Status = StatusStale
						task.UpdatedAt = NowISO()
						errMsg := "Worker tmux session died during execution"
						task.Error = &errMsg
						if err := store.Update(task); err != nil {
							log.WarningLog.Printf("reconcile: failed to mark task %s as stale: %v", task.ID, err)
							continue
						}
						result.StaleTasks = append(result.StaleTasks, task.ID)
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
					if taskErr == nil && (lastTask.Status == StatusCompleted || lastTask.Status == StatusFailed || lastTask.Status == StatusTimedOut) {
						// Worker finished but forgot to update status file
						ws.State = StateIdle
						ws.Timestamp = NowISO()
						statusPath := store.StatusFilePath(name)
						if writeErr := AtomicWriteJSON(statusPath, ws); writeErr != nil {
							// Log but don't fail the whole reconciliation; fall through to block 3
							log.WarningLog.Printf("reconcile: failed to correct status file for %s: %v", name, writeErr)
						} else {
							result.StatusCorrected = append(result.StatusCorrected, name)
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
					updatedAt, parseErr := time.Parse(time.RFC3339, task.UpdatedAt)
					if parseErr != nil {
						log.WarningLog.Printf("reconcile: task %s has unparseable UpdatedAt %q, skipping stale promotion: %v", task.ID, task.UpdatedAt, parseErr)
						continue
					}
					if time.Since(updatedAt) > DefaultStallThreshold {
						task.Status = StatusFailed
						if err := store.Update(task); err != nil {
							log.WarningLog.Printf("reconcile: failed to promote task %s to failed: %v", task.ID, err)
							continue
						}
						result.FailedTasks = append(result.FailedTasks, task.ID)
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
