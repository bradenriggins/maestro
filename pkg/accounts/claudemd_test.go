package accounts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCLAUDEMD_Orchestrator(t *testing.T) {
	dir := t.TempDir()

	ctx := CLAUDEMDContext{
		InstanceName:   "orchestrator",
		AccountName:    "main",
		Role:           string(RoleOrchestrator),
		ConductorDir:   "/home/user/.claude-conductor",
		StatusFilePath: "/home/user/.claude-conductor/status/orchestrator.json",
		WorkerInstances: []WorkerInfo{
			{Title: "Worker 1", Account: "worker-1", Status: "idle"},
			{Title: "Worker 2", Account: "worker-2", Status: "in-progress"},
		},
	}

	err := GenerateCLAUDEMD(dir, ctx)
	require.NoError(t, err)

	destPath := filepath.Join(dir, ".claude", "CLAUDE.md")
	data, err := os.ReadFile(destPath)
	require.NoError(t, err)

	content := string(data)

	assert.True(t, strings.Contains(content, "You Are the Conductor Orchestrator"),
		"should contain orchestrator header")
	assert.True(t, strings.Contains(content, "claude-conductor dispatch"),
		"should contain dispatch command")
	assert.True(t, strings.Contains(content, "claude-conductor status"),
		"should contain status command")
	assert.True(t, strings.Contains(content, "claude-conductor workers"),
		"should contain workers command")
	assert.True(t, strings.Contains(content, "claude-conductor tasks"),
		"should contain tasks command")
	assert.True(t, strings.Contains(content, "claude-conductor output"),
		"should contain output command")
	assert.True(t, strings.Contains(content, "claude-conductor recall"),
		"should contain recall command")
	assert.True(t, strings.Contains(content, "/home/user/.claude-conductor"),
		"should contain conductor dir path")
	assert.True(t, strings.Contains(content, "Worker 1"),
		"should contain first worker title")
	assert.True(t, strings.Contains(content, "worker-1"),
		"should contain first worker account")
	assert.True(t, strings.Contains(content, "Worker 2"),
		"should contain second worker title")
	assert.True(t, strings.Contains(content, "worker-2"),
		"should contain second worker account")
	assert.False(t, strings.Contains(content, "No workers currently active"),
		"should not show no-workers message when workers exist")
}

func TestGenerateCLAUDEMD_Worker(t *testing.T) {
	dir := t.TempDir()

	ctx := CLAUDEMDContext{
		InstanceName:    "worker-1",
		AccountName:     "work-account",
		Role:            string(RoleWorker),
		ConductorDir:    "/home/user/.claude-conductor",
		StatusFilePath:  "/home/user/.claude-conductor/status/worker-1.json",
		WorkerInstances: nil,
	}

	err := GenerateCLAUDEMD(dir, ctx)
	require.NoError(t, err)

	destPath := filepath.Join(dir, ".claude", "CLAUDE.md")
	data, err := os.ReadFile(destPath)
	require.NoError(t, err)

	content := string(data)

	assert.True(t, strings.Contains(content, "You Are a Conductor Worker"),
		"should contain worker header")
	assert.True(t, strings.Contains(content, "worker-1"),
		"should contain instance name")
	assert.True(t, strings.Contains(content, "work-account"),
		"should contain account name")
	assert.True(t, strings.Contains(content, "/home/user/.claude-conductor/status/worker-1.json"),
		"should contain status file path")
	assert.True(t, strings.Contains(content, "/home/user/.claude-conductor"),
		"should contain conductor dir path")
	assert.True(t, strings.Contains(content, "Acknowledgment Protocol"),
		"should contain acknowledgment protocol section")
	assert.True(t, strings.Contains(content, "Task Completion Protocol"),
		"should contain task completion protocol section")
	assert.True(t, strings.Contains(content, "Error Handling"),
		"should contain error handling section")
	assert.True(t, strings.Contains(content, "Rate Limit Handling"),
		"should contain rate limit handling section")
	assert.True(t, strings.Contains(content, "File Write Protocol"),
		"should contain file write protocol section")
}

func TestGenerateCLAUDEMD_OrchestratorNoWorkers(t *testing.T) {
	dir := t.TempDir()

	ctx := CLAUDEMDContext{
		InstanceName:    "orchestrator",
		AccountName:     "main",
		Role:            string(RoleOrchestrator),
		ConductorDir:    "/home/user/.claude-conductor",
		StatusFilePath:  "/home/user/.claude-conductor/status/orchestrator.json",
		WorkerInstances: nil,
	}

	err := GenerateCLAUDEMD(dir, ctx)
	require.NoError(t, err)

	destPath := filepath.Join(dir, ".claude", "CLAUDE.md")
	data, err := os.ReadFile(destPath)
	require.NoError(t, err)

	content := string(data)

	assert.True(t, strings.Contains(content, "You Are the Conductor Orchestrator"),
		"should contain orchestrator header")
	assert.True(t, strings.Contains(content, "No workers currently active"),
		"should show no-workers message when worker list is empty")
}
