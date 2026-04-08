package overlay

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"maestro/pkg/accounts"
	"maestro/pkg/orchestration"
)

func TestOrchestrationOverlay_FreshnessDoesNotExplodeBeforeFirstRefresh(t *testing.T) {
	o := NewOrchestrationOverlay()
	o.SetSize(140, 40)

	view := o.Render()
	require.Contains(t, view, "Waiting for first refresh")
	require.NotContains(t, view, "9223372036s ago")
}

func TestOrchestrationOverlay_TaskNavigationFilterSearchAndPreview(t *testing.T) {
	o := NewOrchestrationOverlay()
	o.SetSize(140, 40)
	o.lastDataRefresh = time.Now()

	resultPath := filepath.Join(t.TempDir(), "task-2.md")
	o.SetCachedData(OrchCachedData{
		registry: &orchestration.Registry{Instances: map[string]orchestration.RegistryEntry{
			"worker-1": {Role: string(accounts.RoleWorker), Account: "acct-a", Program: "claude", Model: "sonnet"},
			"worker-2": {Role: string(accounts.RoleWorker), Account: "acct-b", Program: "codex", Model: "gpt-5"},
		}},
		tasks: []*orchestration.Task{
			{ID: "task-1-lint", Status: orchestration.StatusCompleted, WorkerInstance: "worker-1", ResultFile: filepath.Join(t.TempDir(), "task-1.md")},
			{ID: "task-2-preview", Status: orchestration.StatusFailed, WorkerInstance: "worker-2", ResultFile: resultPath},
			{ID: "task-3-build", Status: orchestration.StatusInProgress, WorkerInstance: "worker-1", ResultFile: filepath.Join(t.TempDir(), "task-3.md")},
		},
		workerStatuses: map[string]*orchestration.WorkerStatus{
			"worker-1": {State: orchestration.StateIdle},
			"worker-2": {State: orchestration.StateWorking, LastTask: "task-2-preview"},
		},
		workerLastTasks: map[string]*orchestration.Task{
			"worker-2": {ID: "task-2-preview", Status: orchestration.StatusFailed, WorkerInstance: "worker-2"},
		},
	})

	view := o.Render()
	require.Contains(t, view, "Orchestration Panel")
	require.Contains(t, view, "worker-1")
	require.Contains(t, view, "task-2-preview")

	// Navigate into the task list, then filter down to failed tasks.
	require.False(t, o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}))
	require.False(t, o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab}))
	require.False(t, o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab}))
	failedView := o.Render()
	require.Contains(t, failedView, "Tasks (1 failed of 3 total)")
	require.Contains(t, failedView, "task-2-preview")
	require.NotContains(t, failedView, "task-1-lint")

	// Search should keep the failed task visible and show the query state.
	require.False(t, o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}))
	for _, r := range "preview" {
		require.False(t, o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}))
	}
	require.False(t, o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter}))
	searchedView := o.Render()
	require.Contains(t, searchedView, "search: preview")
	require.Contains(t, searchedView, "task-2-preview")

	// Enter should request preview loading for the selected task.
	require.False(t, o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter}))
	require.Equal(t, "open-preview", o.PendingAction())
	taskID, selectedResultPath := o.SelectedTaskResultFile()
	require.Equal(t, "task-2-preview", taskID)
	require.Equal(t, resultPath, selectedResultPath)

	o.SetSize(100, 28)
	o.SetPreviewContent(taskID, strings.Join([]string{
		"# Failed task replay",
		"",
		"## Prompt",
		"Investigate flaky preview rendering after resize and quick tab switching.",
		"",
		"## Observed",
		"- diff pane briefly rendered blank",
		"- terminal recovered after second refresh",
		"- user had no inline guidance",
		"",
		"## Suggested fix",
		"1. keep last good preview while refresh is pending",
		"2. show explicit retry CTA when capture fails",
		"3. add resize regression coverage",
	}, "\n"))

	previewView := o.Render()
	require.Contains(t, previewView, "Task Result: task-2-preview")
	require.Contains(t, previewView, "Investigate flaky preview rendering")
	require.Contains(t, previewView, "[Esc] back  [j/k] scroll  [g/G] top/bottom")

	require.False(t, o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}))
	scrolledPreview := o.Render()
	require.Contains(t, scrolledPreview, "add resize regression coverage")

	require.False(t, o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc}))
	backToList := o.Render()
	require.Contains(t, backToList, "Tasks (1 failed of 3 total)")
	require.NotContains(t, backToList, "Task Result:")
}
