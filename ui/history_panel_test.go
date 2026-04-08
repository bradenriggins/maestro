package ui

import (
	"testing"

	"maestro/pkg/orchestration"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestHistoryPanelSortsTasksNewestFirst(t *testing.T) {
	panel := NewHistoryPanel()
	panel.SetTasks([]*orchestration.Task{
		{ID: "older", CreatedAt: "2026-04-01T00:00:00Z", Status: orchestration.StatusCompleted, WorkerInstance: "worker-1"},
		{ID: "newer", CreatedAt: "2026-04-02T00:00:00Z", Status: orchestration.StatusFailed, WorkerInstance: "worker-2"},
	})

	require.Equal(t, "newer", panel.SelectedTask().ID)
	require.Equal(t, HistoryPanelNone, panel.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown}))
	require.Equal(t, "older", panel.SelectedTask().ID)
}

func TestHistoryPanelEscClosesWorkflow(t *testing.T) {
	panel := NewHistoryPanel()

	require.Equal(t, HistoryPanelClose, panel.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc}))
}
