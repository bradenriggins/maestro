package orchestration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper to create a task with explicit status and deps, bypassing Create() logic.
func createTaskDirect(t *testing.T, store *TaskStore, id, status, worker string, deps []string) {
	t.Helper()
	// Use Create to set up files, then override status/deps
	task, err := store.Create(id, "prompt for "+id, worker, "acct", store.StatusFilePath(worker), "test", nil)
	require.NoError(t, err)
	task.Status = status
	task.DependsOn = deps
	if status == StatusPending {
		task.Attempts = 0
	}
	require.NoError(t, store.Update(task))
}

func TestDAGPass_AllDepsComplete_TaskDispatched(t *testing.T) {
	store := NewTaskStoreWithDir(t.TempDir())

	// Create completed dependency
	createTaskDirect(t, store, "dep-1", StatusCompleted, "worker-1", nil)
	createTaskDirect(t, store, "dep-2", StatusCompleted, "worker-1", nil)

	// Create pending task that depends on dep-1 and dep-2
	createTaskDirect(t, store, "pending-task", StatusPending, "worker-1", []string{"dep-1", "dep-2"})

	result, err := RunDAGPass(store)
	require.NoError(t, err)

	assert.Contains(t, result.Dispatched, "pending-task")
	assert.Empty(t, result.Blocked)

	// Verify the task was actually updated
	task, err := store.Get("pending-task")
	require.NoError(t, err)
	assert.Equal(t, StatusDispatched, task.Status)
	// Attempts is NOT incremented by the DAG pass itself — it is only bumped
	// once the worker accepts delivery (SendDAGDispatchedTasks). This prevents
	// a busy worker that defers delivery from churning the attempt counter.
	assert.Equal(t, 0, task.Attempts)
}

func TestDAGPass_OneDepFailed_TaskBlocked(t *testing.T) {
	store := NewTaskStoreWithDir(t.TempDir())

	createTaskDirect(t, store, "dep-ok", StatusCompleted, "worker-1", nil)
	createTaskDirect(t, store, "dep-fail", StatusFailed, "worker-1", nil)

	createTaskDirect(t, store, "pending-task", StatusPending, "worker-1", []string{"dep-ok", "dep-fail"})

	result, err := RunDAGPass(store)
	require.NoError(t, err)

	assert.Contains(t, result.Blocked, "pending-task")
	assert.Empty(t, result.Dispatched)

	task, err := store.Get("pending-task")
	require.NoError(t, err)
	assert.Equal(t, StatusBlocked, task.Status)
	require.NotNil(t, task.Error)
	assert.Contains(t, *task.Error, "dep-fail")
}

func TestDAGPass_BlockedTaskRecovers_WhenDepRetried(t *testing.T) {
	store := NewTaskStoreWithDir(t.TempDir())

	// Dep starts as failed
	createTaskDirect(t, store, "dep-1", StatusFailed, "worker-1", nil)
	createTaskDirect(t, store, "blocked-task", StatusBlocked, "worker-1", []string{"dep-1"})

	// Simulate dep being retried: change to in_progress
	dep, err := store.Get("dep-1")
	require.NoError(t, err)
	dep.Status = StatusInProgress
	require.NoError(t, store.Update(dep))

	result, err := RunDAGPass(store)
	require.NoError(t, err)

	assert.Contains(t, result.Unblocked, "blocked-task")

	task, err := store.Get("blocked-task")
	require.NoError(t, err)
	assert.Equal(t, StatusPending, task.Status)
	assert.Nil(t, task.Error)
}

func TestDAGPass_DepNotFound_TaskBlocked(t *testing.T) {
	store := NewTaskStoreWithDir(t.TempDir())

	// Create pending task with a non-existent dependency
	createTaskDirect(t, store, "pending-task", StatusPending, "worker-1", []string{"nonexistent-dep"})

	result, err := RunDAGPass(store)
	require.NoError(t, err)

	assert.Contains(t, result.Blocked, "pending-task")

	task, err := store.Get("pending-task")
	require.NoError(t, err)
	assert.Equal(t, StatusBlocked, task.Status)
	require.NotNil(t, task.Error)
	assert.Contains(t, *task.Error, "not found")
}

func TestDAGPass_NoPendingTasks_EmptyResult(t *testing.T) {
	store := NewTaskStoreWithDir(t.TempDir())

	// Create only completed and dispatched tasks (no pending)
	createTaskDirect(t, store, "done-1", StatusCompleted, "worker-1", nil)
	createTaskDirect(t, store, "active-1", StatusDispatched, "worker-1", nil)

	result, err := RunDAGPass(store)
	require.NoError(t, err)

	assert.Empty(t, result.Dispatched)
	assert.Empty(t, result.Blocked)
	assert.Empty(t, result.Unblocked)
}

func TestDAGPass_DepsStillRunning_StaysPending(t *testing.T) {
	store := NewTaskStoreWithDir(t.TempDir())

	createTaskDirect(t, store, "dep-running", StatusInProgress, "worker-1", nil)
	createTaskDirect(t, store, "pending-task", StatusPending, "worker-1", []string{"dep-running"})

	result, err := RunDAGPass(store)
	require.NoError(t, err)

	assert.Empty(t, result.Dispatched)
	assert.Empty(t, result.Blocked)

	task, err := store.Get("pending-task")
	require.NoError(t, err)
	assert.Equal(t, StatusPending, task.Status)
}

func TestDAGPass_BlockedRecovery_AllDepsComplete(t *testing.T) {
	store := NewTaskStoreWithDir(t.TempDir())

	// Start with a blocked task whose dep is now completed
	createTaskDirect(t, store, "dep-1", StatusCompleted, "worker-1", nil)
	createTaskDirect(t, store, "blocked-task", StatusBlocked, "worker-1", []string{"dep-1"})

	result, err := RunDAGPass(store)
	require.NoError(t, err)

	assert.Contains(t, result.Unblocked, "blocked-task")

	task, err := store.Get("blocked-task")
	require.NoError(t, err)
	assert.Equal(t, StatusPending, task.Status)
}

// --- Cycle detection tests ---

func TestDetectCycle_LinearChain_NoCycle(t *testing.T) {
	store := NewTaskStoreWithDir(t.TempDir())

	// A -> B -> C (linear chain, no cycle)
	createTaskDirect(t, store, "task-a", StatusPending, "worker-1", []string{"task-b"})
	createTaskDirect(t, store, "task-b", StatusPending, "worker-1", []string{"task-c"})
	createTaskDirect(t, store, "task-c", StatusPending, "worker-1", nil)

	err := DetectCycle(store, "task-d", []string{"task-a"})
	assert.NoError(t, err)
}

func TestDetectCycle_Diamond_NoCycle(t *testing.T) {
	store := NewTaskStoreWithDir(t.TempDir())

	// Diamond: D depends on B and C, both depend on A
	createTaskDirect(t, store, "task-a", StatusPending, "worker-1", nil)
	createTaskDirect(t, store, "task-b", StatusPending, "worker-1", []string{"task-a"})
	createTaskDirect(t, store, "task-c", StatusPending, "worker-1", []string{"task-a"})

	err := DetectCycle(store, "task-d", []string{"task-b", "task-c"})
	assert.NoError(t, err)
}

func TestDetectCycle_SimpleCycle_AB(t *testing.T) {
	store := NewTaskStoreWithDir(t.TempDir())

	// A depends on B, adding B depends on A creates cycle
	createTaskDirect(t, store, "task-a", StatusPending, "worker-1", []string{"task-b"})
	createTaskDirect(t, store, "task-b", StatusPending, "worker-1", nil)

	err := DetectCycle(store, "task-b", []string{"task-a"})
	// Note: task-b already exists but we override its deps via the virtual node
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestDetectCycle_MultiHopCycle(t *testing.T) {
	store := NewTaskStoreWithDir(t.TempDir())

	// A -> B -> C, adding C -> A creates a cycle
	createTaskDirect(t, store, "task-a", StatusPending, "worker-1", []string{"task-b"})
	createTaskDirect(t, store, "task-b", StatusPending, "worker-1", []string{"task-c"})
	createTaskDirect(t, store, "task-c", StatusPending, "worker-1", nil)

	err := DetectCycle(store, "task-c", []string{"task-a"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestDetectCycle_NoExistingTasks(t *testing.T) {
	store := NewTaskStoreWithDir(t.TempDir())

	// New task with dep on non-existent task - no cycle, just a dangling ref
	err := DetectCycle(store, "task-a", []string{"task-b"})
	assert.NoError(t, err)
}
