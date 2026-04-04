package orchestration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"claude-conductor/pkg/accounts"
)

// TaskStore manages task files in a configurable base directory.
type TaskStore struct {
	tasksDir  string
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
	return filepath.Join(s.tasksDir, id+".prompt.txt")
}

// resultPath returns the expected path to a result file by ID.
func (s *TaskStore) resultPath(id string) string {
	return filepath.Join(s.resultsDir, id+".md")
}

// Create writes a new task JSON file and its companion prompt file.
// The prompt file has a header block followed by the prompt text.
func (s *TaskStore) Create(id, prompt, workerInstance, workerAccount, statusFilePath, dispatchedBy string) (*Task, error) {
	if err := os.MkdirAll(s.tasksDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create tasks dir: %w", err)
	}
	if err := os.MkdirAll(s.resultsDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create results dir: %w", err)
	}

	now := NowISO()
	taskFile := s.taskPath(id)
	promptFile := s.promptPath(id)
	resultFile := s.resultPath(id)

	task := &Task{
		ID:             id,
		Status:         StatusDispatched,
		WorkerInstance: workerInstance,
		WorkerAccount:  workerAccount,
		PromptFile:     promptFile,
		ResultFile:     resultFile,
		CreatedAt:      now,
		UpdatedAt:      now,
		Attempts:       0,
		DispatchedBy:   dispatchedBy,
	}

	if err := AtomicWriteJSON(taskFile, task); err != nil {
		return nil, fmt.Errorf("failed to write task file: %w", err)
	}

	header := fmt.Sprintf("Task ID: %s\nTask File: %s\nResult File: %s\nStatus File: %s\n---\n%s",
		id, taskFile, resultFile, statusFilePath, prompt)
	if err := os.WriteFile(promptFile, []byte(header), 0600); err != nil {
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
			return nil, err
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
