package accounts

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCLAUDEMD_Orchestrator(t *testing.T) {
	dir := t.TempDir()

	ctx := CLAUDEMDContext{
		InstanceName:   "orchestrator",
		AccountName:    "main",
		Role:           string(RoleOrchestrator),
		ConductorDir:   "/home/user/.maestro",
		StatusFilePath: "/home/user/.maestro/status/orchestrator.json",
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
	assert.True(t, strings.Contains(content, "maestro dispatch"),
		"should contain dispatch command")
	assert.True(t, strings.Contains(content, "maestro status"),
		"should contain status command")
	assert.True(t, strings.Contains(content, "maestro workers"),
		"should contain workers command")
	assert.True(t, strings.Contains(content, "maestro tasks"),
		"should contain tasks command")
	assert.True(t, strings.Contains(content, "maestro output"),
		"should contain output command")
	assert.True(t, strings.Contains(content, "maestro recall"),
		"should contain recall command")
	assert.True(t, strings.Contains(content, "/home/user/.maestro"),
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
		ConductorDir:    "/home/user/.maestro",
		StatusFilePath:  "/home/user/.maestro/status/worker-1.json",
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
	assert.True(t, strings.Contains(content, "/home/user/.maestro/status/worker-1.json"),
		"should contain status file path")
	assert.True(t, strings.Contains(content, "/home/user/.maestro"),
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
		ConductorDir:    "/home/user/.maestro",
		StatusFilePath:  "/home/user/.maestro/status/orchestrator.json",
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

// --- Template rendering tests with realistic/edge-case data ---

// renderTemplate is a helper that executes a Go text/template string with the
// given data and returns the rendered output.  It fails the test if parsing or
// execution fails.
func renderTemplate(t *testing.T, name, tmplText string, data interface{}) string {
	t.Helper()
	tmpl, err := template.New(name).Parse(tmplText)
	require.NoError(t, err, "template %q should parse without error", name)
	var buf bytes.Buffer
	require.NoError(t, tmpl.Execute(&buf, data), "template %q should execute without panic or error", name)
	return buf.String()
}

// TestOrchestratorTemplate_MultipleWorkersWithMixedModels exercises the
// orchestratorTemplate (used by CLAUDE.md) with workers that have a mix of
// program/model settings — some fully set, some with empty model, some with
// empty program.
func TestOrchestratorTemplate_MultipleWorkersWithMixedModels(t *testing.T) {
	ctx := CLAUDEMDContext{
		InstanceName: "orchestrator",
		AccountName:  "main",
		Role:         string(RoleOrchestrator),
		ConductorDir: "/home/user/.maestro",
		WorkerInstances: []WorkerInfo{
			{Title: "Worker 1", Account: "worker-1", Status: "idle", Program: "claude", Model: "sonnet-4.6", ModelStrengths: "implementation, refactoring"},
			{Title: "Worker 2", Account: "worker-2", Status: "working", Program: "codex", Model: "gpt-5.3-codex", ModelStrengths: "long-running projects"},
			{Title: "Worker 3", Account: "worker-3", Status: "idle", Program: "claude", Model: "", ModelStrengths: "general"},
			{Title: "Worker 4", Account: "worker-4", Status: "idle", Program: "", Model: "", ModelStrengths: ""},
		},
	}

	out := renderTemplate(t, "orchestrator", orchestratorTemplate, ctx)

	// Rendered content checks
	assert.Contains(t, out, "Worker 1", "should contain Worker 1 title")
	assert.Contains(t, out, "worker-1", "should contain worker-1 account")
	assert.Contains(t, out, "claude/sonnet-4.6", "should render program/model for worker-1")
	assert.Contains(t, out, "Worker 2", "should contain Worker 2 title")
	assert.Contains(t, out, "codex/gpt-5.3-codex", "should render program/model for worker-2")

	// Empty model: renders as "claude/" — verify no panic and worker appears
	assert.Contains(t, out, "Worker 3", "should contain Worker 3 with empty model")

	// Both program and model empty: renders as "/" — verify no panic
	assert.Contains(t, out, "Worker 4", "should contain Worker 4 with empty program and model")

	// Should not show no-workers message when workers are present
	assert.NotContains(t, out, "No workers currently active")

	// ConductorDir should be interpolated
	assert.Contains(t, out, "/home/user/.maestro")
}

// TestOrchestratorTemplate_EmptyWorkers verifies that an empty WorkerInstances
// slice renders the {{else}} branch without panicking.
func TestOrchestratorTemplate_EmptyWorkers(t *testing.T) {
	ctx := CLAUDEMDContext{
		InstanceName:    "orchestrator",
		AccountName:     "main",
		Role:            string(RoleOrchestrator),
		ConductorDir:    "/home/user/.maestro",
		WorkerInstances: []WorkerInfo{}, // empty slice, not nil
	}

	out := renderTemplate(t, "orchestrator-empty", orchestratorTemplate, ctx)
	assert.Contains(t, out, "No workers currently active", "empty slice should trigger else branch")
}

// TestOrchestratorTemplate_NilWorkers verifies that a nil WorkerInstances slice
// also renders the {{else}} branch without panicking.
func TestOrchestratorTemplate_NilWorkers(t *testing.T) {
	ctx := CLAUDEMDContext{
		InstanceName:    "orchestrator",
		AccountName:     "main",
		Role:            string(RoleOrchestrator),
		ConductorDir:    "/home/user/.maestro",
		WorkerInstances: nil,
	}

	out := renderTemplate(t, "orchestrator-nil", orchestratorTemplate, ctx)
	assert.Contains(t, out, "No workers currently active", "nil slice should trigger else branch")
}

// TestWorkerTemplate_Rendering verifies the workerTemplate renders correctly
// with all context fields populated.
func TestWorkerTemplate_Rendering(t *testing.T) {
	ctx := CLAUDEMDContext{
		InstanceName:   "worker-1",
		AccountName:    "work-account",
		Role:           string(RoleWorker),
		ConductorDir:   "/home/user/.maestro",
		StatusFilePath: "/home/user/.maestro/status/worker-1.json",
	}

	out := renderTemplate(t, "worker", workerTemplate, ctx)

	assert.Contains(t, out, "You Are a Conductor Worker")
	assert.Contains(t, out, "worker-1")
	assert.Contains(t, out, "work-account")
	assert.Contains(t, out, "/home/user/.maestro/status/worker-1.json")
	assert.Contains(t, out, "/home/user/.maestro")
	assert.Contains(t, out, "Acknowledgment Protocol")
	assert.Contains(t, out, "Task Completion Protocol")
	assert.Contains(t, out, "Error Handling")
	assert.Contains(t, out, "Rate Limit Handling")
}

// TestWorkerTemplate_EmptyStatusFilePath verifies the worker template renders
// even when StatusFilePath is empty (edge case: should not panic).
func TestWorkerTemplate_EmptyStatusFilePath(t *testing.T) {
	ctx := CLAUDEMDContext{
		InstanceName:   "worker-x",
		AccountName:    "acct",
		Role:           string(RoleWorker),
		ConductorDir:   "/tmp/conductor",
		StatusFilePath: "",
	}

	out := renderTemplate(t, "worker-empty-status", workerTemplate, ctx)
	assert.Contains(t, out, "worker-x", "instance name should appear even with empty status file path")
}

// --- AGENTS.md template tests ---

// TestGenerateAGENTSMD_Orchestrator verifies the full GenerateAGENTSMD path
// for an orchestrator with multiple workers.
func TestGenerateAGENTSMD_Orchestrator(t *testing.T) {
	dir := t.TempDir()

	ctx := CLAUDEMDContext{
		InstanceName: "orchestrator",
		AccountName:  "main",
		Role:         string(RoleOrchestrator),
		ConductorDir: "/home/user/.maestro",
		WorkerInstances: []WorkerInfo{
			{Title: "Worker 1", Account: "worker-1", Status: "idle", Program: "claude", Model: "sonnet-4.6", ModelStrengths: "implementation"},
			{Title: "Worker 2", Account: "worker-2", Status: "working", Program: "codex", Model: "gpt-5.3-codex", ModelStrengths: "long tasks"},
		},
	}

	err := GenerateAGENTSMD(dir, ctx)
	require.NoError(t, err)

	destPath := filepath.Join(dir, "AGENTS.md")
	data, err := os.ReadFile(destPath)
	require.NoError(t, err)

	content := string(data)

	assert.Contains(t, content, "You Are the Conductor Orchestrator")
	assert.Contains(t, content, "maestro dispatch")
	assert.Contains(t, content, "maestro status")
	assert.Contains(t, content, "/home/user/.maestro")
	assert.Contains(t, content, "Worker 1")
	assert.Contains(t, content, "claude/sonnet-4.6")
	assert.Contains(t, content, "Worker 2")
	assert.Contains(t, content, "codex/gpt-5.3-codex")
	assert.NotContains(t, content, "No workers currently active")
}

// TestGenerateAGENTSMD_OrchestratorNoWorkers verifies the AGENTS.md else-branch
// for an orchestrator with no workers.
func TestGenerateAGENTSMD_OrchestratorNoWorkers(t *testing.T) {
	dir := t.TempDir()

	ctx := CLAUDEMDContext{
		InstanceName:    "orchestrator",
		AccountName:     "main",
		Role:            string(RoleOrchestrator),
		ConductorDir:    "/home/user/.maestro",
		WorkerInstances: nil,
	}

	err := GenerateAGENTSMD(dir, ctx)
	require.NoError(t, err)

	destPath := filepath.Join(dir, "AGENTS.md")
	data, err := os.ReadFile(destPath)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "No workers currently active")
}

// TestGenerateAGENTSMD_Worker verifies the AGENTS.md worker template.
func TestGenerateAGENTSMD_Worker(t *testing.T) {
	dir := t.TempDir()

	ctx := CLAUDEMDContext{
		InstanceName:   "codex-worker",
		AccountName:    "codex-acct",
		Role:           string(RoleWorker),
		ConductorDir:   "/home/user/.maestro",
		StatusFilePath: "/home/user/.maestro/status/codex-worker.json",
	}

	err := GenerateAGENTSMD(dir, ctx)
	require.NoError(t, err)

	destPath := filepath.Join(dir, "AGENTS.md")
	data, err := os.ReadFile(destPath)
	require.NoError(t, err)

	content := string(data)

	assert.Contains(t, content, "You Are a Conductor Worker")
	assert.Contains(t, content, "codex-worker")
	assert.Contains(t, content, "codex-acct")
	assert.Contains(t, content, "/home/user/.maestro/status/codex-worker.json")
	assert.Contains(t, content, "Task Completion Protocol")
	assert.Contains(t, content, "Error Handling")
	assert.Contains(t, content, "Rate Limit Handling")
}

// TestAgentsOrchestratorTemplate_MixedModels exercises the agentsOrchestratorTemplate
// directly with workers that have mixed program/model settings.
func TestAgentsOrchestratorTemplate_MixedModels(t *testing.T) {
	ctx := CLAUDEMDContext{
		InstanceName: "orchestrator",
		AccountName:  "main",
		Role:         string(RoleOrchestrator),
		ConductorDir: "/home/user/.maestro",
		WorkerInstances: []WorkerInfo{
			{Title: "Worker A", Account: "wa", Status: "idle", Program: "claude", Model: "opus-4.6", ModelStrengths: "architecture"},
			{Title: "Worker B", Account: "wb", Status: "idle", Program: "codex", Model: "", ModelStrengths: "coding"},
			{Title: "Worker C", Account: "wc", Status: "idle", Program: "", Model: "", ModelStrengths: ""},
		},
	}

	out := renderTemplate(t, "agents-orchestrator", agentsOrchestratorTemplate, ctx)

	assert.Contains(t, out, "Worker A")
	assert.Contains(t, out, "claude/opus-4.6", "fully populated worker should render program/model")
	assert.Contains(t, out, "Worker B", "worker with empty model should appear without panic")
	assert.Contains(t, out, "Worker C", "worker with empty program and model should appear without panic")
	assert.NotContains(t, out, "No workers currently active")
	assert.Contains(t, out, "/home/user/.maestro")
}

// TestAgentsWorkerTemplate_Rendering exercises the agentsWorkerTemplate directly.
func TestAgentsWorkerTemplate_Rendering(t *testing.T) {
	ctx := CLAUDEMDContext{
		InstanceName:   "codex-w",
		AccountName:    "codex-account",
		Role:           string(RoleWorker),
		ConductorDir:   "/home/user/.maestro",
		StatusFilePath: "/home/user/.maestro/status/codex-w.json",
	}

	out := renderTemplate(t, "agents-worker", agentsWorkerTemplate, ctx)

	assert.Contains(t, out, "You Are a Conductor Worker")
	assert.Contains(t, out, "codex-w")
	assert.Contains(t, out, "codex-account")
	assert.Contains(t, out, "/home/user/.maestro/status/codex-w.json")
	assert.Contains(t, out, "Task Completion Protocol")
	assert.Contains(t, out, "Error Handling")
}
