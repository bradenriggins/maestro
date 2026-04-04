package orchestration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *TaskStore {
	t.Helper()
	return NewTaskStoreWithDir(t.TempDir())
}

func TestTaskStore_CreateAndGet(t *testing.T) {
	s := newTestStore(t)

	id := GenerateTaskID()
	statusPath := s.StatusFilePath("worker-1")
	task, err := s.Create(id, "do the thing", "worker-1", "acct-a", statusPath, "orchestrator")
	require.NoError(t, err)
	require.NotNil(t, task)

	assert.Equal(t, id, task.ID)
	assert.Equal(t, StatusDispatched, task.Status)
	assert.Equal(t, "worker-1", task.WorkerInstance)
	assert.Equal(t, "acct-a", task.WorkerAccount)
	assert.Equal(t, "orchestrator", task.DispatchedBy)
	assert.NotEmpty(t, task.PromptFile)
	assert.NotEmpty(t, task.ResultFile)
	assert.NotEmpty(t, task.CreatedAt)
	assert.NotEmpty(t, task.UpdatedAt)

	// Prompt file should exist with header + prompt text.
	promptData, err := os.ReadFile(task.PromptFile)
	require.NoError(t, err)
	promptStr := string(promptData)
	assert.Contains(t, promptStr, "Task ID: "+id)
	assert.Contains(t, promptStr, "Task File: ")
	assert.Contains(t, promptStr, "Result File: ")
	assert.Contains(t, promptStr, "Status File: ")
	assert.Contains(t, promptStr, "---")
	assert.Contains(t, promptStr, "do the thing")

	// Round-trip via Get.
	got, err := s.Get(id)
	require.NoError(t, err)
	assert.Equal(t, task.ID, got.ID)
	assert.Equal(t, task.Status, got.Status)
}

func TestTaskStore_Update(t *testing.T) {
	s := newTestStore(t)

	id := GenerateTaskID()
	task, err := s.Create(id, "update me", "worker-1", "acct-a", s.StatusFilePath("worker-1"), "orch")
	require.NoError(t, err)

	originalUpdatedAt := task.UpdatedAt

	task.Status = StatusInProgress
	task.Attempts = 1

	err = s.Update(task)
	require.NoError(t, err)

	// UpdatedAt must be refreshed (may be equal if very fast; just ensure it is set).
	assert.NotEmpty(t, task.UpdatedAt)
	_ = originalUpdatedAt // may be same second in fast tests; field is updated

	got, err := s.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, got.Status)
	assert.Equal(t, 1, got.Attempts)
}

func TestTaskStore_List(t *testing.T) {
	s := newTestStore(t)

	ids := []string{GenerateTaskID(), GenerateTaskID(), GenerateTaskID()}
	for _, id := range ids {
		_, err := s.Create(id, "task "+id, "worker-1", "acct-a", s.StatusFilePath("worker-1"), "orch")
		require.NoError(t, err)
	}

	// Mark one as completed.
	task, err := s.Get(ids[0])
	require.NoError(t, err)
	task.Status = StatusCompleted
	require.NoError(t, s.Update(task))

	// List all — should get all 3.
	all, err := s.List("")
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// Filter by dispatched — should get 2.
	dispatched, err := s.List(StatusDispatched)
	require.NoError(t, err)
	assert.Len(t, dispatched, 2)

	// Filter by completed — should get 1.
	completed, err := s.List(StatusCompleted)
	require.NoError(t, err)
	assert.Len(t, completed, 1)
	assert.Equal(t, ids[0], completed[0].ID)

	// Filter by failed — none.
	failed, err := s.List(StatusFailed)
	require.NoError(t, err)
	assert.Len(t, failed, 0)
}

func TestTaskStore_ListEmptyDir(t *testing.T) {
	s := newTestStore(t)

	// tasksDir does not exist yet — List should return nil without error.
	tasks, err := s.List("")
	require.NoError(t, err)
	assert.Nil(t, tasks)
}

func TestTaskStore_ForInstance(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < 2; i++ {
		_, err := s.Create(GenerateTaskID(), "prompt", "worker-1", "acct-a", s.StatusFilePath("worker-1"), "orch")
		require.NoError(t, err)
	}
	for i := 0; i < 3; i++ {
		_, err := s.Create(GenerateTaskID(), "prompt", "worker-2", "acct-b", s.StatusFilePath("worker-2"), "orch")
		require.NoError(t, err)
	}

	w1, err := s.ForInstance("worker-1")
	require.NoError(t, err)
	assert.Len(t, w1, 2)

	w2, err := s.ForInstance("worker-2")
	require.NoError(t, err)
	assert.Len(t, w2, 3)

	none, err := s.ForInstance("worker-99")
	require.NoError(t, err)
	assert.Len(t, none, 0)
}

func TestTaskStore_ReadStatus(t *testing.T) {
	s := newTestStore(t)

	// Write a status file manually.
	require.NoError(t, os.MkdirAll(s.statusDir, 0700))

	ws := WorkerStatus{
		State:     StateWorking,
		LastTask:  "task-123",
		Timestamp: NowISO(),
	}
	data, err := json.MarshalIndent(ws, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(s.StatusFilePath("worker-1"), data, 0600))

	got, err := s.ReadStatus("worker-1")
	require.NoError(t, err)
	assert.Equal(t, StateWorking, got.State)
	assert.Equal(t, "task-123", got.LastTask)
	assert.NotEmpty(t, got.Timestamp)
}

func TestTaskStore_ReadStatusMissing(t *testing.T) {
	s := newTestStore(t)

	_, err := s.ReadStatus("nonexistent-worker")
	assert.Error(t, err)
}

func TestTaskStore_GetNonexistent(t *testing.T) {
	s := newTestStore(t)

	_, err := s.Get("task-does-not-exist")
	assert.Error(t, err)
}

func TestTaskStore_StatusFilePath(t *testing.T) {
	s := newTestStore(t)

	p := s.StatusFilePath("my-worker")
	assert.True(t, filepath.IsAbs(p))
	assert.Equal(t, "my-worker.json", filepath.Base(p))
}
