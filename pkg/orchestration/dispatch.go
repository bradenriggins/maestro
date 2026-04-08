package orchestration

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"maestro/log"
	"maestro/pkg/accounts"
)

const conductorSessionPrefix = "maestro_"

// DispatchError is a structured error with an exit code for CLI usage.
type DispatchError struct {
	Code int
	Msg  string
}

func (e *DispatchError) Error() string {
	return e.Msg
}

func (e *DispatchError) ExitCode() int {
	if e == nil || e.Code == 0 {
		return 1
	}
	return e.Code
}

// DispatchResult holds the outcome of a successful dispatch.
type DispatchResult struct {
	TaskID       string
	InstanceName string
	Pending      bool // true if task was created as pending (has dependencies)
}

// RunDispatch dispatches a task prompt to a named worker instance.
// If redispatchTaskID is non-empty, it re-dispatches an existing task.
// If dependsOn is non-empty, the task starts as pending and will be dispatched
// automatically when all dependencies complete.
func RunDispatch(instanceName string, taskPrompt string, redispatchTaskID string, dependsOn []string) (*DispatchResult, error) {
	cfg, err := accounts.LoadConductorConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load conductor config: %w", err)
	}
	if cfg == nil {
		return nil, fmt.Errorf("no config found; run 'maestro setup' first")
	}

	if redispatchTaskID == "" && strings.TrimSpace(taskPrompt) == "" {
		return nil, &DispatchError{Code: 1, Msg: "task prompt is required when not re-dispatching"}
	}

	reg, err := LoadRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to load registry: %w", err)
	}

	entry, ok := reg.GetInstance(instanceName)
	if !ok {
		return nil, &DispatchError{Code: 1, Msg: fmt.Sprintf("instance %q not found in registry. Run 'maestro workers' to see available instances.", instanceName)}
	}

	if !strings.HasPrefix(entry.TmuxSession, conductorSessionPrefix) {
		return nil, &DispatchError{Code: 3, Msg: fmt.Sprintf("tmux session %q does not have required prefix %q", entry.TmuxSession, conductorSessionPrefix)}
	}

	if !tmuxHasSession(entry.TmuxSession) {
		return nil, &DispatchError{Code: 2, Msg: fmt.Sprintf("tmux session %q is not running. The instance may have crashed. Check: maestro doctor", entry.TmuxSession)}
	}

	store, err := NewTaskStore()
	if err != nil {
		return nil, fmt.Errorf("failed to create task store: %w", err)
	}

	hasDeps := len(dependsOn) > 0

	if !hasDeps {
		ready, err := isWorkerReady(store, instanceName, entry.TmuxSession)
		if err != nil {
			return nil, err
		}
		if !ready {
			return nil, &DispatchError{Code: 4, Msg: fmt.Sprintf("worker %q is not idle. Check status: maestro status %s", instanceName, instanceName)}
		}
	}

	statusFilePath := store.StatusFilePath(instanceName)

	var taskID string
	var previousStatus string

	if redispatchTaskID != "" {
		task, err := store.Get(redispatchTaskID)
		if err != nil {
			return nil, fmt.Errorf("failed to load task %q: %w", redispatchTaskID, err)
		}
		if task.Attempts >= MaxAttempts {
			return nil, &DispatchError{Code: 4, Msg: fmt.Sprintf("task %q has reached max attempts (%d). Create a new task or check task history: maestro tasks --status failed", redispatchTaskID, MaxAttempts)}
		}

		previousStatus = task.Status
		previousAttempts := task.Attempts
		previousWorkerInstance := task.WorkerInstance
		previousWorkerAccount := task.WorkerAccount
		previousDispatchedAt := task.DispatchedAt
		previousCompletedAt := task.CompletedAt
		previousError := task.Error

		task.WorkerInstance = instanceName
		task.WorkerAccount = entry.Account
		task.Status = StatusDispatched
		task.Attempts++
		now := NowISO()
		task.DispatchedAt = &now
		task.CompletedAt = nil
		task.Error = nil
		if err := store.Update(task); err != nil {
			return nil, fmt.Errorf("failed to update task: %w", err)
		}

		if err := updatePromptStatusFile(task.PromptFile, statusFilePath); err != nil {
			task.Status = previousStatus
			task.Attempts = previousAttempts
			task.WorkerInstance = previousWorkerInstance
			task.WorkerAccount = previousWorkerAccount
			task.DispatchedAt = previousDispatchedAt
			task.CompletedAt = previousCompletedAt
			task.Error = previousError
			if rollbackErr := store.Update(task); rollbackErr != nil {
				log.ErrorLog.Printf("dispatch: rollback failed for task %s: %v", task.ID, rollbackErr)
			}
			return nil, fmt.Errorf("failed to update prompt status file: %w", err)
		}

		taskID = redispatchTaskID
		taskPrompt = task.PromptFile
	} else {
		var idErr error
		taskID, idErr = GenerateTaskID()
		if idErr != nil {
			return nil, fmt.Errorf("failed to generate task ID: %w", idErr)
		}

		if hasDeps {
			if cycleErr := DetectCycle(store, taskID, dependsOn); cycleErr != nil {
				return nil, &DispatchError{Code: 1, Msg: cycleErr.Error()}
			}
		}

		var opts *CreateOptions
		if hasDeps {
			opts = &CreateOptions{DependsOn: dependsOn}
		}

		task, err := store.Create(taskID, taskPrompt, instanceName, entry.Account, statusFilePath, "cli", opts)
		if err != nil {
			return nil, fmt.Errorf("failed to create task: %w", err)
		}

		inferredCategory := InferCategory(taskPrompt)
		task.InferredCategory = string(inferredCategory)
		if err := store.Update(task); err != nil {
			log.WarningLog.Printf("dispatch: failed to update task with inferred category: %v", err)
		}

		if task.Status == StatusPending {
			return &DispatchResult{TaskID: taskID, InstanceName: instanceName, Pending: true}, nil
		}

		taskPrompt = task.PromptFile
	}

	// Build the tmux instruction. The worker reads the prompt file itself, so
	// only the generated path is sent through tmux.
	instruction := fmt.Sprintf("Read and execute task: %s", taskPrompt)

	tmuxName := entry.TmuxSession

	// Send to tmux and poll for ack (up to MaxAttempts send attempts within this dispatch)
	for sendAttempt := 1; sendAttempt <= MaxAttempts; sendAttempt++ {
		if sendAttempt > 1 {
			currentTask, getErr := store.Get(taskID)
			if getErr == nil && currentTask.Status == StatusInProgress {
				return &DispatchResult{TaskID: taskID, InstanceName: instanceName}, nil
			}
		}

		if sendAttempt == 1 && redispatchTaskID != "" {
			reTask, reErr := store.Get(taskID)
			if reErr == nil && (previousStatus == StatusFailed || previousStatus == StatusTimedOut) && reTask.Error != nil && *reTask.Error != "" {
				if fcErr := appendFailureContext(reTask.PromptFile, *reTask.Error); fcErr != nil {
					log.WarningLog.Printf("dispatch: failed to append failure context for task %s: %v", taskID, fcErr)
				}
			}
		}

		if err := tmuxSendKeys(tmuxName, instruction); err != nil {
			return nil, fmt.Errorf("failed to send keys to tmux: %w", err)
		}

		sendTask, getErr := store.Get(taskID)
		if getErr != nil {
			log.ErrorLog.Printf("dispatch: failed to read task %s for SendAttempts update: %v", taskID, getErr)
		} else {
			if sendTask.Status == StatusInProgress {
				return &DispatchResult{TaskID: taskID, InstanceName: instanceName}, nil
			}
			sendTask.SendAttempts++
			if updateErr := store.Update(sendTask); updateErr != nil {
				log.ErrorLog.Printf("dispatch: failed to persist SendAttempts for task %s: %v", taskID, updateErr)
			}
		}

		if pollForAck(store, taskID, PollTimeout, PollInterval) {
			return &DispatchResult{TaskID: taskID, InstanceName: instanceName}, nil
		}
	}

	// All attempts exhausted.
	// Only mark timed_out if the task is still in "dispatched" state — the worker
	// may have acknowledged between the last pollForAck and this read.
	task, getErr := store.Get(taskID)
	if getErr != nil {
		log.ErrorLog.Printf("dispatch: failed to read task %s for stale-marking: %v", taskID, getErr)
	} else if task.Status == StatusDispatched {
		task.Status = StatusTimedOut
		timeoutMsg := "task timed out waiting for worker acknowledgment"
		task.Error = &timeoutMsg
		now := NowISO()
		task.CompletedAt = &now
		if updateErr := store.Update(task); updateErr != nil {
			log.ErrorLog.Printf("dispatch: failed to persist timed_out status for task %s: %v", taskID, updateErr)
		}
	} else if task.Status == StatusInProgress {
		// Worker acknowledged after poll deadline — treat as success.
		return &DispatchResult{TaskID: taskID, InstanceName: instanceName}, nil
	}
	return nil, &DispatchError{Code: 5, Msg: fmt.Sprintf("worker %q did not acknowledge task %s after %d attempts (marked timed_out). Check: maestro output %s", instanceName, taskID, MaxAttempts, instanceName)}
}

// IsWorkerIdle checks whether a worker instance is idle and ready for a task.
// Exported for use by bulk retry and other packages.
func IsWorkerIdle(instanceName string) (bool, error) {
	store, err := NewTaskStore()
	if err != nil {
		return false, err
	}
	return isWorkerReady(store, instanceName, "")
}

// isWorkerReady checks if a worker is idle and ready to accept a task.
func isWorkerReady(store *TaskStore, instanceName string, _ string) (bool, error) {
	ws, err := store.ReadStatus(instanceName)
	if err == nil {
		if ws.State != StateIdle {
			return false, nil
		}
		// Check staleness: if the idle status is old, verify via task state
		if ws.Timestamp != "" {
			if t, parseErr := time.Parse(time.RFC3339, ws.Timestamp); parseErr == nil {
				if time.Since(t) > 10*time.Minute {
					// Status file is stale — fall through to task-based check
					tasks, tasksErr := store.ForInstance(instanceName)
					if tasksErr != nil {
						return false, fmt.Errorf("failed to check active tasks for %q: %w", instanceName, tasksErr)
					}
					for _, task := range tasks {
						if task.Status == StatusDispatched || task.Status == StatusInProgress || task.Status == StatusStale || task.Status == StatusPending || task.Status == StatusBlocked {
							return false, nil
						}
					}
					// No active tasks, stale-but-idle status is acceptable
				}
			}
		}
		return true, nil
	}

	// Status file missing — check active tasks for this instance instead
	tasks, tasksErr := store.ForInstance(instanceName)
	if tasksErr != nil {
		// Disk error: we cannot determine worker state. Refuse dispatch rather
		// than risking a double-dispatch on a worker that may already be busy.
		return false, fmt.Errorf("failed to check active tasks for %q: %w", instanceName, tasksErr)
	}
	for _, task := range tasks {
		if task.Status == StatusDispatched || task.Status == StatusInProgress || task.Status == StatusStale || task.Status == StatusPending || task.Status == StatusBlocked {
			return false, nil
		}
	}
	return true, nil
}

// pollForAck polls the task file until status changes to in_progress.
// It checks for acknowledgment before each sleep so the deadline is honoured
// to within one poll interval.
func pollForAck(store *TaskStore, taskID string, timeout, interval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		task, err := store.Get(taskID)
		if err == nil && task.Status == StatusInProgress {
			return true
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false
		}
		sleep := interval
		if sleep > remaining {
			sleep = remaining
		}
		time.Sleep(sleep)
	}
}

// updatePromptStatusFile rewrites the Status File line in a prompt file.
func updatePromptStatusFile(promptPath, newStatusPath string) error {
	data, err := os.ReadFile(promptPath)
	if err != nil {
		return fmt.Errorf("failed to read prompt file: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, "Status File: ") {
			lines[i] = "Status File: " + newStatusPath
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("prompt file %q has no Status File line", promptPath)
	}
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Errorf("failed to generate temp file suffix: %w", err)
	}
	tmp := fmt.Sprintf("%s.%d-%x.tmp", promptPath, os.Getpid(), b)
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")), 0600); err != nil {
		return fmt.Errorf("failed to write temp prompt file: %w", err)
	}
	if err := os.Rename(tmp, promptPath); err != nil {
		os.Remove(tmp) // best-effort cleanup to avoid temp file leak
		return fmt.Errorf("failed to rename temp prompt file: %w", err)
	}
	return nil
}

// appendFailureContext inserts a failure notice after the --- separator in the prompt file.
func appendFailureContext(promptPath, errorMsg string) error {
	data, err := os.ReadFile(promptPath)
	if err != nil {
		return err
	}
	content := string(data)
	separator := "\n\n---\n\n"
	idx := strings.Index(content, separator)
	if idx == -1 {
		return nil
	}
	failureNote := fmt.Sprintf("Previous attempt failed: %s. Please try a different approach.\n\n", errorMsg)
	insertPos := idx + len(separator)
	newContent := content[:insertPos] + failureNote + content[insertPos:]
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Errorf("failed to generate temp file suffix: %w", err)
	}
	tmp := fmt.Sprintf("%s.%d-%x.tmp", promptPath, os.Getpid(), b)
	if err := os.WriteFile(tmp, []byte(newContent), 0600); err != nil {
		return fmt.Errorf("failed to write temp prompt file: %w", err)
	}
	if err := os.Rename(tmp, promptPath); err != nil {
		os.Remove(tmp) // best-effort cleanup to avoid temp file leak
		return fmt.Errorf("failed to rename temp prompt file: %w", err)
	}
	return nil
}

// --- tmux helpers ---

// tmuxHasSession checks if a tmux session with the given name is alive.
func tmuxHasSession(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t="+name)
	return cmd.Run() == nil
}

// TmuxHasSession is the exported version of tmuxHasSession.
func TmuxHasSession(name string) bool {
	return tmuxHasSession(name)
}

// tmuxSendKeys sends text to a tmux session followed by Enter.
func tmuxSendKeys(session, text string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", session, "-l", text)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("send-keys failed: %s: %w", string(out), err)
	}
	enterCmd := exec.Command("tmux", "send-keys", "-t", session, "Enter")
	if out, err := enterCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("send-keys Enter failed: %s: %w", string(out), err)
	}
	return nil
}

// tmuxCapturePaneLines captures the last N lines from a tmux session's pane.
func tmuxCapturePaneLines(session string, lines int) (string, error) {
	startLine := fmt.Sprintf("-%d", lines)
	cmd := exec.Command("tmux", "capture-pane", "-t", session, "-p", "-S", startLine)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("capture-pane failed: %w", err)
	}
	return string(out), nil
}

// tmuxCaptureFullHistory captures the full scrollback history of a tmux session.
func tmuxCaptureFullHistory(session string) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-t", session, "-p", "-S", "-")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("capture-pane full history failed: %w", err)
	}
	return string(out), nil
}
