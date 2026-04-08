package orchestration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionState_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create conductor dir
	conductorDir := filepath.Join(tmpDir, ".maestro")
	os.MkdirAll(conductorDir, 0700)

	state := &SessionState{
		RepoPath:  "/tmp/test-repo",
		StartedAt: NowISO(),
		StashRef:  "stash@{0}",
		StartTag:  "maestro/session-start/12345",
		Instances: []SessionInstance{
			{Title: "plan", Account: "main", Branch: "main"},
			{Title: "worker-1", Account: "w1", Branch: "feat/auth"},
		},
	}

	err := SaveSession(state)
	require.NoError(t, err)

	loaded, err := LoadSession()
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, "/tmp/test-repo", loaded.RepoPath)
	assert.Equal(t, "stash@{0}", loaded.StashRef)
	assert.Len(t, loaded.Instances, 2)
	assert.Equal(t, "plan", loaded.Instances[0].Title)
}

func TestSessionState_LoadNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, ".maestro"), 0700)

	loaded, err := LoadSession()
	assert.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestDeleteSession(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	conductorDir := filepath.Join(tmpDir, ".maestro")
	os.MkdirAll(conductorDir, 0700)

	// Create a session file
	state := &SessionState{RepoPath: "/tmp/test", StartedAt: NowISO()}
	SaveSession(state)

	// Delete it
	err := DeleteSession()
	assert.NoError(t, err)

	// Should be gone
	loaded, err := LoadSession()
	assert.NoError(t, err)
	assert.Nil(t, loaded)
}
