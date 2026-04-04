package orchestration

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"claude-conductor/pkg/accounts"
)

const conductorSessionPrefix = "claudeconductor_"

// DispatchError is a structured error with an exit code for CLI usage.
type DispatchError struct {
	Code int
	Msg  string
}

func (e *DispatchError) Error() string {
	return e.Msg
}

// DispatchResult holds the outcome of a successful dispatch.
type DispatchResult struct {
	TaskID       string
	InstanceName string
}

// RunDispatch dispatches a task prompt to a named worker instance.
// If redispatchTaskID is non-empty, it re-dispatches an existing task.
func RunDispatch(instanceName string, taskPrompt string, redispatchTaskID string) (*DispatchResult, error) {
	// 1. Load conductor config
	cfg, err := accounts.LoadConductorConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load conductor config: %w", err)
	}
	if cfg == nil {
		return nil, fmt.Errorf("no conductor config found; run 'claude-conductor setup' first")
	}

	// 2. Load registry
	reg, err := LoadRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to load registry: %w", err)
	}

	// 3. Validate instance exists
	entry, ok := reg.GetInstance(instanceName)
	if !ok {
		return nil, &DispatchError{Code: 1, Msg: fmt.Sprintf("instance %q not found in registry", instanceName)}
	}

	// 4. Validate tmux session alive
	if !tmuxHasSession(entry.TmuxSession) {
		return nil, &DispatchError{Code: 2, Msg: fmt.Sprintf("tmux session %q is not running", entry.TmuxSession)}
	}

	// 5. Validate session prefix
	if !strings.HasPrefix(entry.TmuxSession, conductorSessionPrefix) {
		return nil, &DispatchError{Code: 3, Msg: fmt.Sprintf("tmux session %q does not have required prefix %q", entry.TmuxSession, conductorSessionPrefix)}
	}

	// 6. Check worker readiness
	store, err := NewTaskStore()
	if err != nil {
		return nil, fmt.Errorf("failed to create task store: %w", err)
	}

	ready, err := isWorkerReady(store, instanceName, entry.TmuxSession)
	if err != nil {
		return nil, err
	}
	if !ready {
		return nil, &DispatchError{Code: 4, Msg: fmt.Sprintf("worker %q is not idle/ready", instanceName)}
	}

	// Validate prompt is provided for new dispatches
	if redispatchTaskID == "" && strings.TrimSpace(taskPrompt) == "" {
		return nil, &DispatchError{Code: 1, Msg: "task prompt is required when not re-dispatching"}
	}

	statusFilePath := store.StatusFilePath(instanceName)

	var taskID string

	if redispatchTaskID != "" {
		// 8. Re-dispatch existing task
		task, err := store.Get(redispatchTaskID)
		if err != nil {
			return nil, fmt.Errorf("failed to load task %q: %w", redispatchTaskID, err)
		}
		if task.Attempts >= MaxAttempts {
			return nil, &DispatchError{Code: 4, Msg: fmt.Sprintf("task %q has reached max attempts (%d)", redispatchTaskID, MaxAttempts)}
		}

		previousStatus := task.Status

		task.WorkerInstance = instanceName
		task.WorkerAccount = entry.Account
		task.Status = StatusDispatched
		task.Attempts++
		if err := store.Update(task); err != nil {
			return nil, fmt.Errorf("failed to update task: %w", err)
		}

		// Update the prompt file status line
		if err := updatePromptStatusFile(task.PromptFile, statusFilePath); err != nil {
			return nil, fmt.Errorf("failed to update prompt status file: %w", err)
		}

		// Append failure context if there was a previous error
		if previousStatus == StatusFailed && task.Error != nil && *task.Error != "" {
			if err := appendFailureContext(task.PromptFile, *task.Error); err != nil {
				return nil, fmt.Errorf("failed to append failure context: %w", err)
			}
		}

		taskID = redispatchTaskID
		taskPrompt = task.PromptFile
	} else {
		// 7. New dispatch
		var idErr error
		taskID, idErr = GenerateTaskID()
		if idErr != nil {
			return nil, fmt.Errorf("failed to generate task ID: %w", idErr)
		}
		task, err := store.Create(taskID, taskPrompt, instanceName, entry.Account, statusFilePath, "cli")
		if err != nil {
			return nil, fmt.Errorf("failed to create task: %w", err)
		}
		taskPrompt = task.PromptFile
	}

	// 9. Build instruction and retry loop
	instruction := fmt.Sprintf("Read and execute task: %s", taskPrompt)

	tmuxName := entry.TmuxSession

	// Send to tmux and poll for ack (up to MaxAttempts send attempts within this dispatch)
	for sendAttempt := 1; sendAttempt <= MaxAttempts; sendAttempt++ {
		if err := tmuxSendKeys(tmuxName, instruction); err != nil {
			return nil, fmt.Errorf("failed to send keys to tmux: %w", err)
		}

		if pollForAck(store, taskID, PollTimeout, PollInterval) {
			return &DispatchResult{TaskID: taskID, InstanceName: instanceName}, nil
		}

		// Log retry but don't mutate task.Attempts
		if sendAttempt < MaxAttempts {
			// Re-send the instruction on next iteration
		}
	}

	// All attempts exhausted
	task, getErr := store.Get(taskID)
	if getErr == nil {
		task.Status = StatusTimedOut
		_ = store.Update(task)
	}
	return nil, &DispatchError{Code: 5, Msg: fmt.Sprintf("Worker '%s' did not acknowledge task %s after %d attempts. Task marked as timed_out.", instanceName, taskID, MaxAttempts)}
}

// isWorkerReady checks if a worker is idle and ready to accept a task.
func isWorkerReady(store *TaskStore, instanceName, tmuxSession string) (bool, error) {
	ws, err := store.ReadStatus(instanceName)
	if err == nil {
		return ws.State == StateIdle, nil
	}

	// Status file missing — fallback to tmux capture-pane check
	content, captureErr := tmuxCapturePaneContent(tmuxSession)
	if captureErr != nil {
		return false, fmt.Errorf("failed to capture pane: %w", captureErr)
	}

	// Check for the Claude idle prompt string
	return strings.Contains(content, "No, and tell Claude what to do differently"), nil
}

// pollForAck polls the task file until status changes to in_progress.
func pollForAck(store *TaskStore, taskID string, timeout, interval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		task, err := store.Get(taskID)
		if err == nil && task.Status == StatusInProgress {
			return true
		}
		time.Sleep(interval)
	}
	return false
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
	return os.WriteFile(promptPath, []byte(strings.Join(lines, "\n")), 0600)
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
	return os.WriteFile(promptPath, []byte(newContent), 0600)
}

// --- tmux helpers ---

// tmuxHasSession checks if a tmux session with the given name is alive.
func tmuxHasSession(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t="+name)
	return cmd.Run() == nil
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

// tmuxCapturePaneContent captures the visible pane content of a tmux session.
func tmuxCapturePaneContent(session string) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-t", session, "-p")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("capture-pane failed: %w", err)
	}
	return string(out), nil
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
