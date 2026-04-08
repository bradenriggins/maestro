package orchestration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTask_IsTerminal(t *testing.T) {
	tests := []struct {
		status   string
		terminal bool
	}{
		{StatusDispatched, false},
		{StatusInProgress, false},
		{StatusCompleted, true},
		{StatusFailed, true},
		{StatusTimedOut, true},
		{StatusStale, false},
		{StatusPending, false},
		{StatusBlocked, false},
	}
	for _, tt := range tests {
		task := Task{Status: tt.status}
		assert.Equal(t, tt.terminal, task.IsTerminal(), "status %s", tt.status)
	}
}

func TestGenerateTaskID(t *testing.T) {
	id, err := GenerateTaskID()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(id, "task-"))
	parts := strings.SplitN(id, "-", 3)
	assert.Equal(t, 3, len(parts))
	assert.Len(t, parts[2], 8) // now 8 hex chars (4 bytes)
	id2, err := GenerateTaskID()
	require.NoError(t, err)
	assert.NotEqual(t, id, id2)
}

func TestNowISO(t *testing.T) {
	ts := NowISO()
	assert.Contains(t, ts, "T")
	assert.True(t, strings.HasSuffix(ts, "Z"))
}

func TestAtomicWriteJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := tmpDir + "/test.json"
	data := map[string]string{"key": "value"}
	err := AtomicWriteJSON(path, data)
	assert.NoError(t, err)
	var result map[string]string
	err = ReadJSONFile(path, &result)
	assert.NoError(t, err)
	assert.Equal(t, "value", result["key"])
}
