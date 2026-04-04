package orchestration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTmux struct {
	alive map[string]bool
}

func (m *mockTmux) HasSession(name string) bool {
	return m.alive[name]
}

// makeReconcileRegistry writes a registry JSON to dir and returns its path.
func makeReconcileRegistry(t *testing.T, dir string, instances map[string]RegistryEntry) string {
	t.Helper()
	reg := Registry{
		UpdatedAt: NowISO(),
		Instances: instances,
	}
	path := filepath.Join(dir, "registry.json")
	require.NoError(t, AtomicWriteJSON(path, reg))
	return path
}

// TestReconcile_DetectsDeadSession verifies that when a registry entry has status
// "running" but tmux reports the session as dead, the entry becomes "dead" (with
// DiedAt set) and any in_progress tasks for that instance become stale.
func TestReconcile_DetectsDeadSession(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStoreWithDir(dir)

	regPath := makeReconcileRegistry(t, dir, map[string]RegistryEntry{
		"worker-1": {
			Account:     "alice",
			Role:        "worker",
			TmuxSession: "conductor-worker-1",
			Status:      "running",
			CreatedAt:   NowISO(),
		},
	})

	// Create an in_progress task for worker-1
	id := GenerateTaskID()
	task, err := store.Create(id, "do work", "worker-1", "alice", store.StatusFilePath("worker-1"), "orch")
	require.NoError(t, err)
	task.Status = StatusInProgress
	require.NoError(t, store.Update(task))

	tmux := &mockTmux{alive: map[string]bool{
		"conductor-worker-1": false, // session is dead
	}}

	result, err := Reconcile(tmux, regPath, store)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Dead session detected
	assert.Contains(t, result.DeadSessions, "worker-1")

	// Registry entry updated to "dead" with DiedAt set
	reg, err := LoadRegistryFromPath(regPath)
	require.NoError(t, err)
	entry := reg.Instances["worker-1"]
	assert.Equal(t, "dead", entry.Status)
	assert.NotNil(t, entry.DiedAt)
	assert.NotEmpty(t, *entry.DiedAt)

	// In-progress task should now be stale
	assert.Contains(t, result.StaleTasks, id)
	updated, err := store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusStale, updated.Status)
	require.NotNil(t, updated.Error)
	assert.Contains(t, *updated.Error, "died")
}

// TestReconcile_CorrectsStaleStatusFile verifies that when a worker's status file
// says "working" but the last task is already completed, the status file is
// corrected to "idle".
func TestReconcile_CorrectsStaleStatusFile(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStoreWithDir(dir)

	regPath := makeReconcileRegistry(t, dir, map[string]RegistryEntry{
		"worker-1": {
			Account:     "alice",
			Role:        "worker",
			TmuxSession: "conductor-worker-1",
			Status:      "running",
			CreatedAt:   NowISO(),
		},
	})

	// Create a completed task for worker-1
	id := GenerateTaskID()
	task, err := store.Create(id, "finished work", "worker-1", "alice", store.StatusFilePath("worker-1"), "orch")
	require.NoError(t, err)
	task.Status = StatusCompleted
	require.NoError(t, store.Update(task))

	// Write a status file that still claims the worker is working on that task
	require.NoError(t, os.MkdirAll(store.statusDir, 0700))
	ws := &WorkerStatus{
		State:     StateWorking,
		LastTask:  id,
		Timestamp: NowISO(),
	}
	require.NoError(t, AtomicWriteJSON(store.StatusFilePath("worker-1"), ws))

	tmux := &mockTmux{alive: map[string]bool{
		"conductor-worker-1": true, // session is alive
	}}

	result, err := Reconcile(tmux, regPath, store)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Contains(t, result.StatusCorrected, "worker-1")

	// Status file should now report idle
	corrected, err := store.ReadStatus("worker-1")
	require.NoError(t, err)
	assert.Equal(t, StateIdle, corrected.State)
}

// TestReconcile_PromotesStaleTasks verifies that tasks stuck in "stale" status
// for more than 30 seconds are promoted to "failed".
func TestReconcile_PromotesStaleTasks(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStoreWithDir(dir)

	regPath := makeReconcileRegistry(t, dir, map[string]RegistryEntry{
		"worker-1": {
			Account:     "alice",
			Role:        "worker",
			TmuxSession: "conductor-worker-1",
			Status:      "running",
			CreatedAt:   NowISO(),
		},
	})

	// Create a task and manually set it to stale with an old timestamp
	id := GenerateTaskID()
	task, err := store.Create(id, "stale work", "worker-1", "alice", store.StatusFilePath("worker-1"), "orch")
	require.NoError(t, err)

	// Set status to stale with an UpdatedAt more than 30 seconds in the past
	task.Status = StatusStale
	task.UpdatedAt = time.Now().UTC().Add(-60 * time.Second).Format(time.RFC3339)
	require.NoError(t, AtomicWriteJSON(store.taskPath(id), task))

	tmux := &mockTmux{alive: map[string]bool{
		"conductor-worker-1": true,
	}}

	result, err := Reconcile(tmux, regPath, store)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Contains(t, result.FailedTasks, id)

	promoted, err := store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, promoted.Status)
}

// TestReconcile_LeavesHealthyInstancesAlone verifies that a running instance with
// an alive tmux session and no stale tasks is left untouched.
func TestReconcile_LeavesHealthyInstancesAlone(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStoreWithDir(dir)

	regPath := makeReconcileRegistry(t, dir, map[string]RegistryEntry{
		"worker-1": {
			Account:     "alice",
			Role:        "worker",
			TmuxSession: "conductor-worker-1",
			Status:      "running",
			CreatedAt:   NowISO(),
		},
	})

	// Create a currently in_progress task (recently updated)
	id := GenerateTaskID()
	task, err := store.Create(id, "ongoing work", "worker-1", "alice", store.StatusFilePath("worker-1"), "orch")
	require.NoError(t, err)
	task.Status = StatusInProgress
	require.NoError(t, store.Update(task))

	tmux := &mockTmux{alive: map[string]bool{
		"conductor-worker-1": true, // session is alive
	}}

	result, err := Reconcile(tmux, regPath, store)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Empty(t, result.DeadSessions)
	assert.Empty(t, result.StaleTasks)
	assert.Empty(t, result.FailedTasks)
	assert.Empty(t, result.StatusCorrected)

	// Task should still be in_progress
	unchanged, err := store.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, unchanged.Status)

	// Registry entry status unchanged
	reg, err := LoadRegistryFromPath(regPath)
	require.NoError(t, err)
	assert.Equal(t, "running", reg.Instances["worker-1"].Status)
}

// TestReconcile_HandlesEmptyRegistry verifies that an empty registry produces no
// errors and no changes.
func TestReconcile_HandlesEmptyRegistry(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStoreWithDir(dir)

	regPath := makeReconcileRegistry(t, dir, map[string]RegistryEntry{})

	tmux := &mockTmux{alive: map[string]bool{}}

	result, err := Reconcile(tmux, regPath, store)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Empty(t, result.DeadSessions)
	assert.Empty(t, result.StaleTasks)
	assert.Empty(t, result.FailedTasks)
	assert.Empty(t, result.StatusCorrected)
}
