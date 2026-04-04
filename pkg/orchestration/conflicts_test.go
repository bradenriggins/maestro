package orchestration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectConflicts_NoConflicts(t *testing.T) {
	// Empty worktrees = no conflicts
	conflicts := DetectConflicts(map[string]string{})
	assert.Empty(t, conflicts)
}

func TestDetectConflicts_WithNonexistentPaths(t *testing.T) {
	// Non-existent paths should gracefully return no conflicts
	conflicts := DetectConflicts(map[string]string{
		"worker-1": "/nonexistent/path1",
		"worker-2": "/nonexistent/path2",
	})
	assert.Empty(t, conflicts)
}
