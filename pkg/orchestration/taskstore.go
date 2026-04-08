package orchestration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"maestro/pkg/accounts"
)

// TaskStore manages task files in a configurable base directory.
type TaskStore struct {
	tasksDir   string
	resultsDir string
	statusDir  string
}

// NewTaskStore creates a TaskStore rooted at the default ConductorDir.
func NewTaskStore() (*TaskStore, error) {
	base, err := accounts.ConductorDir()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve conductor dir: %w", err)
	}
	return NewTaskStoreWithDir(base), nil
}

// NewTaskStoreWithDir creates a TaskStore rooted at baseDir (useful for testing).
func NewTaskStoreWithDir(baseDir string) *TaskStore {
	return &TaskStore{
		tasksDir:   filepath.Join(baseDir, "tasks"),
		resultsDir: filepath.Join(baseDir, "results"),
		statusDir:  filepath.Join(baseDir, "status"),
	}
}

// taskPath returns the path to a task JSON file by ID.
func (s *TaskStore) taskPath(id string) string {
	return filepath.Join(s.tasksDir, id+".json")
}

// promptPath returns the path to a prompt text file by ID.
func (s *TaskStore) promptPath(id string) string {
	return filepath.Join(s.tasksDir, id+".prompt")
}

// resultPath returns the expected path to a result file by ID.
func (s *TaskStore) resultPath(id string) string {
	return filepath.Join(s.resultsDir, id+".md")
}

// CreateOptions configures optional task creation parameters such as dependencies.
type CreateOptions struct {
	DependsOn  []string
	PipelineID string
}

// Create writes a new task JSON file and its companion prompt file.
// The prompt file has a header block followed by the prompt text.
// If opts is non-nil and DependsOn is set, the task starts as StatusPending instead of StatusDispatched.
func (s *TaskStore) Create(id, prompt, workerInstance, workerAccount, statusFilePath, dispatchedBy string, opts *CreateOptions) (*Task, error) {
	if err := os.MkdirAll(s.tasksDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create tasks dir: %w", err)
	}
	if err := os.MkdirAll(s.resultsDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create results dir: %w", err)
	}
	if err := os.MkdirAll(s.statusDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create status dir: %w", err)
	}

	now := NowISO()
	taskFile := s.taskPath(id)
	promptFile := s.promptPath(id)
	resultFile := s.resultPath(id)

	status := StatusDispatched
	attempts := 1

	var dependsOn []string
	var pipelineID string
	if opts != nil {
		if len(opts.DependsOn) > 0 {
			dependsOn = opts.DependsOn
			status = StatusPending
			attempts = 0
		}
		pipelineID = opts.PipelineID
	}

	// Only set DispatchedAt for tasks that are actually being dispatched.
	// Pending tasks (with dependencies) haven't been dispatched yet; their
	// DispatchedAt should be set when they are actually dispatched later,
	// so that UpdateDurationStats computes the real execution duration
	// rather than including the wait-for-dependencies period.
	var dispatchedAt *string
	if status == StatusDispatched {
		ts := now
		dispatchedAt = &ts
	}

	task := &Task{
		ID:             id,
		Status:         status,
		WorkerInstance: workerInstance,
		WorkerAccount:  workerAccount,
		PromptFile:     promptFile,
		ResultFile:     resultFile,
		CreatedAt:      now,
		UpdatedAt:      now,
		DispatchedAt:   dispatchedAt,
		Attempts:       attempts,
		DispatchedBy:   dispatchedBy,
		DependsOn:      dependsOn,
		PipelineID:     pipelineID,
	}

	if err := AtomicWriteJSON(taskFile, task); err != nil {
		return nil, fmt.Errorf("failed to write task file: %w", err)
	}

	header := fmt.Sprintf("Task ID: %s\nTask File: %s\nResult File: %s\nStatus File: %s\n\n---\n\n%s",
		id, taskFile, resultFile, statusFilePath, prompt)
	if err := AtomicWritePrompt(promptFile, []byte(header)); err != nil {
		// Roll back the already-written task JSON to avoid an orphaned task record.
		os.Remove(taskFile)
		return nil, fmt.Errorf("failed to write prompt file: %w", err)
	}

	return task, nil
}

// Get reads and returns the task with the given ID.
func (s *TaskStore) Get(id string) (*Task, error) {
	var task Task
	if err := ReadJSONFile(s.taskPath(id), &task); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("task %q not found", id)
		}
		return nil, fmt.Errorf("failed to read task %q: %w", id, err)
	}
	return &task, nil
}

// Update writes the task back to disk, stamping UpdatedAt with the current time.
func (s *TaskStore) Update(task *Task) error {
	task.UpdatedAt = NowISO()
	if err := AtomicWriteJSON(s.taskPath(task.ID), task); err != nil {
		return fmt.Errorf("failed to update task %q: %w", task.ID, err)
	}
	return nil
}

// List returns all tasks in the tasks directory. If statusFilter is non-empty,
// only tasks whose Status matches are returned.
func (s *TaskStore) List(statusFilter string) ([]*Task, error) {
	entries, err := os.ReadDir(s.tasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read tasks dir: %w", err)
	}

	var tasks []*Task
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		task, err := s.Get(id)
		if err != nil {
			continue // skip missing or corrupt files (TOCTOU race)
		}
		if statusFilter != "" && task.Status != statusFilter {
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// ForInstance returns all tasks assigned to the given worker instance.
func (s *TaskStore) ForInstance(instanceName string) ([]*Task, error) {
	all, err := s.List("")
	if err != nil {
		return nil, err
	}
	var result []*Task
	for _, t := range all {
		if t.WorkerInstance == instanceName {
			result = append(result, t)
		}
	}
	return result, nil
}

// StatusFilePath returns the absolute path to the status JSON file for a worker instance.
func (s *TaskStore) StatusFilePath(instanceName string) string {
	return filepath.Join(s.statusDir, instanceName+".json")
}

// ReadStatus reads and returns the WorkerStatus for the given instance.
func (s *TaskStore) ReadStatus(instanceName string) (*WorkerStatus, error) {
	var ws WorkerStatus
	if err := ReadJSONFile(s.StatusFilePath(instanceName), &ws); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("status file for %q not found", instanceName)
		}
		return nil, fmt.Errorf("failed to read status for %q: %w", instanceName, err)
	}
	return &ws, nil
}
