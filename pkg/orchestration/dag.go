package orchestration

import (
	"fmt"
	"strings"

	"maestro/log"
)

// DAGPassResult holds the outcome of a single DAG evaluation pass.
type DAGPassResult struct {
	Dispatched []string // task IDs moved from pending -> dispatched
	Blocked    []string // task IDs moved to blocked
	Unblocked  []string // task IDs recovered from blocked -> pending
}

// RunDAGPass evaluates all pending and blocked tasks, advancing or blocking them
// based on the current state of their dependencies.
func RunDAGPass(store *TaskStore) (*DAGPassResult, error) {
	result := &DAGPassResult{}

	tasks, err := store.List("")
	if err != nil {
		return nil, fmt.Errorf("dag pass: failed to list tasks: %w", err)
	}

	for _, task := range tasks {
		if task.Status == StatusPending && len(task.DependsOn) > 0 {
			handlePendingTask(store, task, result)
		}
	}

	for _, task := range tasks {
		if task.Status == StatusBlocked {
			handleBlockedTask(store, task, result)
		}
	}

	return result, nil
}

// handlePendingTask checks all dependencies for a pending task and transitions it
// to dispatched (all deps complete), blocked (any dep failed/missing), or leaves
// it pending (deps still running).
func handlePendingTask(store *TaskStore, task *Task, result *DAGPassResult) {
	allComplete := true
	for _, depID := range task.DependsOn {
		dep, err := store.Get(depID)
		if err != nil {
			// Dependency not found -> block
			blockTask(store, task, fmt.Sprintf("dependency %q not found", depID))
			result.Blocked = append(result.Blocked, task.ID)
			return
		}

		switch dep.Status {
		case StatusFailed, StatusTimedOut, StatusStale, StatusBlocked:
			blockTask(store, task, fmt.Sprintf("dependency %q is %s", depID, dep.Status))
			result.Blocked = append(result.Blocked, task.ID)
			return
		case StatusCompleted:
			// This dep is done, continue checking others
		default:
			// Still running (dispatched, in_progress, pending)
			allComplete = false
		}
	}

	if !allComplete {
		return // still waiting on deps
	}

	// All deps completed -> dispatch this task
	task.Status = StatusDispatched
	task.Attempts++
	task.Error = nil
	now := NowISO()
	task.DispatchedAt = &now
	if err := store.Update(task); err != nil {
		log.WarningLog.Printf("dag: failed to dispatch task %s: %v", task.ID, err)
		return
	}
	result.Dispatched = append(result.Dispatched, task.ID)
}

// handleBlockedTask re-evaluates a blocked task's deps to see if it can be unblocked.
func handleBlockedTask(store *TaskStore, task *Task, result *DAGPassResult) {
	if len(task.DependsOn) == 0 {
		return
	}

	anyNonTerminalOrRetrying := false
	allComplete := true

	for _, depID := range task.DependsOn {
		dep, err := store.Get(depID)
		if err != nil {
			// Dep still not found, stay blocked
			return
		}

		switch dep.Status {
		case StatusCompleted:
			// Good
		case StatusDispatched, StatusInProgress, StatusPending:
			// Dep is being retried or still running
			anyNonTerminalOrRetrying = true
			allComplete = false
		default:
			// Still in a failure state
			allComplete = false
		}
	}

	if allComplete || anyNonTerminalOrRetrying {
		// All deps completed, or at least one dep is being retried -> move to pending
		task.Status = StatusPending
		task.Error = nil
		task.CompletedAt = nil
		if err := store.Update(task); err != nil {
			log.WarningLog.Printf("dag: failed to unblock task %s: %v", task.ID, err)
			return
		}
		result.Unblocked = append(result.Unblocked, task.ID)
	}
	// Otherwise all deps are still in a terminal failure state -> leave as blocked
}

// SendDAGDispatchedTasks delivers newly-dispatched DAG tasks to their workers' tmux sessions.
// It looks up each task from the store, resolves the worker's tmux session from the registry,
// and sends the "Read and execute task:" instruction. If the worker is busy, the task is
// reverted to pending so it can be retried on the next DAG pass. Errors are logged but do
// not stop delivery of subsequent tasks.
func SendDAGDispatchedTasks(taskIDs []string, store *TaskStore, reg *Registry) {
	for _, taskID := range taskIDs {
		task, err := store.Get(taskID)
		if err != nil {
			log.WarningLog.Printf("dag send: failed to load task %s: %v", taskID, err)
			continue
		}
		entry, ok := reg.GetInstance(task.WorkerInstance)
		if !ok {
			log.WarningLog.Printf("dag send: worker %q not found in registry for task %s — reverting to pending", task.WorkerInstance, taskID)
			revertToPending(store, task)
			continue
		}
		if !tmuxHasSession(entry.TmuxSession) {
			log.WarningLog.Printf("dag send: tmux session %q not alive for task %s — reverting to pending", entry.TmuxSession, taskID)
			revertToPending(store, task)
			continue
		}
		// Check worker readiness before sending to avoid double-dispatching a busy worker
		ready, readyErr := isWorkerReady(store, task.WorkerInstance, entry.TmuxSession)
		if readyErr != nil {
			log.WarningLog.Printf("dag send: failed to check worker readiness for %q (task %s): %v — reverting to pending", task.WorkerInstance, taskID, readyErr)
			revertToPending(store, task)
			continue
		}
		if !ready {
			log.InfoLog.Printf("dag send: worker %q is busy — deferring task %s to next pass", task.WorkerInstance, taskID)
			revertToPending(store, task)
			continue
		}
		instruction := fmt.Sprintf("Read and execute task: %s", task.PromptFile)
		if err := tmuxSendKeys(entry.TmuxSession, instruction); err != nil {
			log.WarningLog.Printf("dag send: failed to send task %s to tmux %q: %v", taskID, entry.TmuxSession, err)
		}
	}
}

// revertToPending moves a dispatched task back to pending so the next DAG pass can retry.
func revertToPending(store *TaskStore, task *Task) {
	task.Status = StatusPending
	task.Attempts--
	if task.Attempts < 0 {
		task.Attempts = 0
	}
	if err := store.Update(task); err != nil {
		log.WarningLog.Printf("dag send: failed to revert task %s to pending: %v", task.ID, err)
	}
}

// blockTask marks a task as blocked with an error message.
func blockTask(store *TaskStore, task *Task, reason string) {
	task.Status = StatusBlocked
	task.Error = &reason
	if err := store.Update(task); err != nil {
		log.WarningLog.Printf("dag: failed to block task %s: %v", task.ID, err)
	}
}

// DetectCycle checks whether adding a task with newTaskID and newTaskDeps would
// create a cycle in the dependency graph of all non-terminal tasks.
func DetectCycle(store *TaskStore, newTaskID string, newTaskDeps []string) error {
	tasks, err := store.List("")
	if err != nil {
		return fmt.Errorf("cycle detection: failed to list tasks: %w", err)
	}

	// Build adjacency list: task -> tasks it depends on
	adj := make(map[string][]string)
	for _, t := range tasks {
		if !t.IsTerminal() && len(t.DependsOn) > 0 {
			adj[t.ID] = t.DependsOn
		}
	}
	// Add the proposed new task
	if len(newTaskDeps) > 0 {
		adj[newTaskID] = newTaskDeps
	}

	// Collect all node IDs
	nodes := make(map[string]bool)
	for id, deps := range adj {
		nodes[id] = true
		for _, d := range deps {
			nodes[d] = true
		}
	}

	// DFS with coloring: 0=white (unvisited), 1=gray (in stack), 2=black (done)
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int)
	for id := range nodes {
		color[id] = white
	}

	var cyclePath []string

	var dfs func(node string) bool
	dfs = func(node string) bool {
		color[node] = gray
		cyclePath = append(cyclePath, node)

		for _, dep := range adj[node] {
			if color[dep] == gray {
				// Found a cycle - build the cycle description
				cyclePath = append(cyclePath, dep)
				return true
			}
			if color[dep] == white {
				if dfs(dep) {
					return true
				}
			}
		}

		cyclePath = cyclePath[:len(cyclePath)-1]
		color[node] = black
		return false
	}

	for id := range nodes {
		if color[id] == white {
			cyclePath = nil
			if dfs(id) {
				// Extract just the cycle portion from cyclePath
				cycleStart := cyclePath[len(cyclePath)-1]
				var cycle []string
				inCycle := false
				for _, n := range cyclePath {
					if n == cycleStart {
						inCycle = true
					}
					if inCycle {
						cycle = append(cycle, n)
					}
				}
				return fmt.Errorf("dependency cycle detected: %s", strings.Join(cycle, " -> "))
			}
		}
	}

	return nil
}
