package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestDispatchPanelSubmitsSelectedWorkerAndTask(t *testing.T) {
	panel := NewDispatchPanel()
	panel.Configure([]string{"worker-1", "worker-2"}, 0, map[string]string{"worker-2": "gpt-5"})
	panel.SetTask("Investigate failing build")

	require.Equal(t, DispatchPanelNone, panel.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRight}))
	require.Equal(t, DispatchPanelSubmit, panel.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter}))
	require.Equal(t, "worker-2", panel.Worker())
	require.Equal(t, "Investigate failing build", panel.Task())
}

func TestDispatchPanelRequiresTaskBeforeSubmitting(t *testing.T) {
	panel := NewDispatchPanel()
	panel.Configure([]string{"worker-1"}, 0, nil)

	require.Equal(t, DispatchPanelNone, panel.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter}))
	require.Contains(t, panel.Render(), "Enter a task before dispatching.")
}
