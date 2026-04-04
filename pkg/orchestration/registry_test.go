package orchestration

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestRegistry() Registry {
	return Registry{
		UpdatedAt: NowISO(),
		Instances: map[string]RegistryEntry{
			"orch-1": {
				Account:      "alice",
				Role:         "orchestrator",
				TmuxSession:  "conductor-orch-1",
				WorktreePath: "/tmp/orch-1",
				Branch:       "main",
				Status:       StateIdle,
				CreatedAt:    NowISO(),
				LastOutputAt: NowISO(),
			},
			"worker-1": {
				Account:      "bob",
				Role:         "worker",
				TmuxSession:  "conductor-worker-1",
				WorktreePath: "/tmp/worker-1",
				Branch:       "feat/foo",
				Status:       StateWorking,
				CreatedAt:    NowISO(),
				LastOutputAt: NowISO(),
			},
			"worker-2": {
				Account:      "carol",
				Role:         "worker",
				TmuxSession:  "conductor-worker-2",
				WorktreePath: "/tmp/worker-2",
				Branch:       "feat/bar",
				Status:       StateIdle,
				CreatedAt:    NowISO(),
				LastOutputAt: NowISO(),
			},
		},
	}
}

func TestLoadRegistryFromPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	original := makeTestRegistry()
	require.NoError(t, AtomicWriteJSON(path, original))

	reg, err := LoadRegistryFromPath(path)
	require.NoError(t, err)
	require.NotNil(t, reg)

	assert.Equal(t, original.UpdatedAt, reg.UpdatedAt)
	assert.Len(t, reg.Instances, 3)

	w1, ok := reg.Instances["worker-1"]
	assert.True(t, ok)
	assert.Equal(t, "bob", w1.Account)
	assert.Equal(t, "worker", w1.Role)
	assert.Equal(t, StateWorking, w1.Status)
}

func TestLoadRegistryFromPath_NilInstancesInitialized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	// Write a registry with no instances field (null after unmarshal).
	emptyReg := struct {
		UpdatedAt string `json:"updated_at"`
	}{UpdatedAt: NowISO()}
	require.NoError(t, AtomicWriteJSON(path, emptyReg))

	reg, err := LoadRegistryFromPath(path)
	require.NoError(t, err)
	assert.NotNil(t, reg.Instances, "Instances should be initialized even when absent in JSON")
	assert.Len(t, reg.Instances, 0)
}

func TestLoadRegistryFromPath_NotFound(t *testing.T) {
	_, err := LoadRegistryFromPath("/nonexistent/path/registry.json")
	assert.Error(t, err)
}

func TestListWorkers(t *testing.T) {
	reg := makeTestRegistry()

	workers := reg.ListWorkers()

	assert.Len(t, workers, 2)
	_, hasWorker1 := workers["worker-1"]
	_, hasWorker2 := workers["worker-2"]
	_, hasOrch := workers["orch-1"]
	assert.True(t, hasWorker1)
	assert.True(t, hasWorker2)
	assert.False(t, hasOrch, "orchestrator should not appear in ListWorkers")
}

func TestListAll(t *testing.T) {
	reg := makeTestRegistry()

	all := reg.ListAll()

	assert.Len(t, all, 3)
	_, hasOrch := all["orch-1"]
	assert.True(t, hasOrch)
}

func TestGetInstance_Found(t *testing.T) {
	reg := makeTestRegistry()

	entry, ok := reg.GetInstance("worker-1")
	assert.True(t, ok)
	require.NotNil(t, entry)
	assert.Equal(t, "bob", entry.Account)
}

func TestGetInstance_NotFound(t *testing.T) {
	reg := makeTestRegistry()

	entry, ok := reg.GetInstance("does-not-exist")
	assert.False(t, ok)
	assert.Nil(t, entry)
}
