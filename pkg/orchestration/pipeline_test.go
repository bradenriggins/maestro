package orchestration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePipelineYAML(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "pipeline.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	return path
}

func TestLoadPipeline_Valid(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: ci-pipeline
tasks:
  - id: lint
    worker: worker-1
    prompt: "Run linting"
  - id: unit-tests
    worker: worker-2
    depends_on: [lint]
    prompt: "Run unit tests"
  - id: integration-tests
    worker: worker-3
    depends_on: [lint]
    prompt: "Run integration tests"
  - id: deploy
    worker: worker-1
    depends_on: [unit-tests, integration-tests]
    prompt: "Deploy to staging"
`
	path := writePipelineYAML(t, dir, yamlContent)
	p, err := LoadPipeline(path)
	require.NoError(t, err)
	assert.Equal(t, "ci-pipeline", p.Name)
	assert.Len(t, p.Tasks, 4)
}

func TestLoadPipeline_CycleDetected(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: cycle-pipeline
tasks:
  - id: a
    worker: worker-1
    depends_on: [b]
    prompt: "Task A"
  - id: b
    worker: worker-1
    depends_on: [a]
    prompt: "Task B"
`
	path := writePipelineYAML(t, dir, yamlContent)
	p, err := LoadPipeline(path)
	require.NoError(t, err)

	store := NewTaskStoreWithDir(t.TempDir())
	_, err = RunPipeline(p, store)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestLoadPipeline_MissingName(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
tasks:
  - id: a
    worker: worker-1
    prompt: "Task A"
`
	path := writePipelineYAML(t, dir, yamlContent)
	_, err := LoadPipeline(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestLoadPipeline_UnknownDep(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: bad-dep-pipeline
tasks:
  - id: a
    worker: worker-1
    depends_on: [nonexistent]
    prompt: "Task A"
`
	path := writePipelineYAML(t, dir, yamlContent)
	_, err := LoadPipeline(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown task")
}

func TestRunPipeline_FourTaskDiamond(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: ci-pipeline
tasks:
  - id: lint
    worker: worker-1
    prompt: "Run linting"
  - id: unit-tests
    worker: worker-2
    depends_on: [lint]
    prompt: "Run unit tests"
  - id: integration-tests
    worker: worker-3
    depends_on: [lint]
    prompt: "Run integration tests"
  - id: deploy
    worker: worker-1
    depends_on: [unit-tests, integration-tests]
    prompt: "Deploy to staging"
`
	path := writePipelineYAML(t, dir, yamlContent)
	p, err := LoadPipeline(path)
	require.NoError(t, err)

	store := NewTaskStoreWithDir(t.TempDir())
	result, err := RunPipeline(p, store)
	require.NoError(t, err)

	assert.Equal(t, "ci-pipeline", result.PipelineName)
	assert.Len(t, result.TaskIDs, 4)
	assert.Empty(t, result.Errors)

	// lint should be dispatched (no deps)
	lintID := result.TaskIDs["lint"]
	lintTask, err := store.Get(lintID)
	require.NoError(t, err)
	assert.Equal(t, StatusDispatched, lintTask.Status)
	assert.Equal(t, "ci-pipeline", lintTask.PipelineID)

	// unit-tests should be pending (depends on lint)
	utID := result.TaskIDs["unit-tests"]
	utTask, err := store.Get(utID)
	require.NoError(t, err)
	assert.Equal(t, StatusPending, utTask.Status)
	assert.Contains(t, utTask.DependsOn, lintID)

	// integration-tests should be pending
	itID := result.TaskIDs["integration-tests"]
	itTask, err := store.Get(itID)
	require.NoError(t, err)
	assert.Equal(t, StatusPending, itTask.Status)
	assert.Contains(t, itTask.DependsOn, lintID)

	// deploy should be pending (depends on unit-tests and integration-tests)
	deployID := result.TaskIDs["deploy"]
	deployTask, err := store.Get(deployID)
	require.NoError(t, err)
	assert.Equal(t, StatusPending, deployTask.Status)
	assert.Len(t, deployTask.DependsOn, 2)
}

func TestRunPipeline_TaskNoDeps_DispatchedImmediately(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: simple-pipeline
tasks:
  - id: standalone
    worker: worker-1
    prompt: "Do something"
`
	path := writePipelineYAML(t, dir, yamlContent)
	p, err := LoadPipeline(path)
	require.NoError(t, err)

	store := NewTaskStoreWithDir(t.TempDir())
	result, err := RunPipeline(p, store)
	require.NoError(t, err)

	taskID := result.TaskIDs["standalone"]
	task, err := store.Get(taskID)
	require.NoError(t, err)
	assert.Equal(t, StatusDispatched, task.Status)
	assert.Equal(t, 1, task.Attempts)
}

func TestRunPipeline_ThreeNodeCycle(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: cycle3
tasks:
  - id: a
    worker: worker-1
    depends_on: [c]
    prompt: "A"
  - id: b
    worker: worker-1
    depends_on: [a]
    prompt: "B"
  - id: c
    worker: worker-1
    depends_on: [b]
    prompt: "C"
`
	path := writePipelineYAML(t, dir, yamlContent)
	p, err := LoadPipeline(path)
	require.NoError(t, err)

	store := NewTaskStoreWithDir(t.TempDir())
	_, err = RunPipeline(p, store)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestLoadPipeline_DuplicateID(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: dup
tasks:
  - id: a
    worker: worker-1
    prompt: "A"
  - id: a
    worker: worker-2
    prompt: "A again"
`
	path := writePipelineYAML(t, dir, yamlContent)
	_, err := LoadPipeline(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

func TestLoadPipeline_SelfDependency(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: self-dep
tasks:
  - id: a
    worker: worker-1
    depends_on: [a]
    prompt: "Task A"
`
	path := writePipelineYAML(t, dir, yamlContent)
	_, err := LoadPipeline(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "depends on itself")
}

func TestLoadPipeline_DuplicateDep(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: dup-dep
tasks:
  - id: a
    worker: worker-1
    prompt: "Task A"
  - id: b
    worker: worker-1
    depends_on: [a, a]
    prompt: "Task B"
`
	path := writePipelineYAML(t, dir, yamlContent)
	_, err := LoadPipeline(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate dependency")
}

func TestLoadPipeline_EmptyPrompt(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: empty-prompt
tasks:
  - id: a
    worker: worker-1
    prompt: ""
`
	path := writePipelineYAML(t, dir, yamlContent)
	_, err := LoadPipeline(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing prompt")
}

func TestLoadPipeline_MissingWorker(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: no-worker
tasks:
  - id: a
    prompt: "Task A"
`
	path := writePipelineYAML(t, dir, yamlContent)
	_, err := LoadPipeline(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing worker")
}

func TestLoadPipeline_NoTasks(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: empty
tasks: []
`
	path := writePipelineYAML(t, dir, yamlContent)
	_, err := LoadPipeline(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one task")
}

func TestDAGPass_DuplicateDeps_NoPanic(t *testing.T) {
	store := NewTaskStoreWithDir(t.TempDir())

	createTaskDirect(t, store, "dep-1", StatusCompleted, "worker-1", nil)
	// Task with duplicate deps (should not cause double-dispatch or panic)
	createTaskDirect(t, store, "pending-task", StatusPending, "worker-1", []string{"dep-1", "dep-1"})

	result, err := RunDAGPass(store)
	require.NoError(t, err)
	assert.Contains(t, result.Dispatched, "pending-task")
}

func TestDAGPass_SelfDependency_Blocks(t *testing.T) {
	store := NewTaskStoreWithDir(t.TempDir())

	// A pending task that depends on itself should block (it's pending, not completed)
	createTaskDirect(t, store, "self-dep", StatusPending, "worker-1", []string{"self-dep"})

	result, err := RunDAGPass(store)
	require.NoError(t, err)
	// Self-dep: the task is pending, so dep check sees it as "still running" -> stays pending
	// It will never complete because it depends on itself completing first
	assert.Empty(t, result.Dispatched)
}

func TestDetectCycle_SelfLoop(t *testing.T) {
	store := NewTaskStoreWithDir(t.TempDir())

	err := DetectCycle(store, "task-a", []string{"task-a"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}
