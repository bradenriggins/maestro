package orchestration

import (
	"fmt"
	"time"
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
}

// Reconcile checks registry against tmux reality and fixes state.
// This is the core reconciliation logic from spec Section 12.1.
func Reconcile(tmux TmuxChecker, registryPath string, store *TaskStore) (*ReconcileResult, error) {
	result := &ReconcileResult{}

	reg, err := LoadRegistryFromPath(registryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load registry: %w", err)
	}

	for name, entry := range reg.Instances {
		tmuxAlive := tmux.HasSession(entry.TmuxSession)

		// 1. Detect dead sessions
		if !tmuxAlive && entry.Status == "running" {
			entry.Status = "dead"
			now := NowISO()
			entry.DiedAt = &now
			reg.Instances[name] = entry
			result.DeadSessions = append(result.DeadSessions, name)

			// Mark in_progress tasks for this instance as stale
			tasks, err := store.ForInstance(name)
			if err == nil {
				for _, task := range tasks {
					if task.Status == StatusInProgress {
						task.Status = StatusStale
						task.UpdatedAt = NowISO()
						errMsg := "Worker tmux session died during execution"
						task.Error = &errMsg
						store.Update(task)
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
						AtomicWriteJSON(statusPath, ws)
						result.StatusCorrected = append(result.StatusCorrected, name)
					}
				}
			}
		}

		// 3. Detect stale tasks (in stale state for > 30 seconds) → promote to failed
		tasks, err := store.ForInstance(name)
		if err == nil {
			for _, task := range tasks {
				if task.Status == StatusStale {
					updatedAt, parseErr := time.Parse(time.RFC3339, task.UpdatedAt)
					if parseErr == nil && time.Since(updatedAt) > 30*time.Second {
						task.Status = StatusFailed
						store.Update(task)
						result.FailedTasks = append(result.FailedTasks, task.ID)
					}
				}
			}
		}
	}

	// Write updated registry
	reg.UpdatedAt = NowISO()
	AtomicWriteJSON(registryPath, reg)

	return result, nil
}
