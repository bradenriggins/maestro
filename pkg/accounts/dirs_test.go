package accounts

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnsureConductorDirs(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := EnsureConductorDirs()
	assert.NoError(t, err)

	base := filepath.Join(tmpDir, ".claude-conductor")
	for _, dir := range RequiredDirs {
		info, err := os.Stat(filepath.Join(base, dir))
		assert.NoError(t, err, "directory %s should exist", dir)
		assert.True(t, info.IsDir(), "%s should be a directory", dir)
		assert.Equal(t, os.FileMode(0700), info.Mode().Perm(), "%s should have 0700 permissions", dir)
	}

	gitignore := filepath.Join(base, ".gitignore")
	content, err := os.ReadFile(gitignore)
	assert.NoError(t, err, ".gitignore should exist")
	assert.Equal(t, "*\n", string(content))
}
