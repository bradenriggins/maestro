package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestReviewPanelReturnsEditAction(t *testing.T) {
	panel := NewReviewPanel()
	panel.Configure("worker-1", "feat/task-1", "main")

	require.Equal(t, ReviewEdit, panel.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")}))
}

func TestReviewPanelRenderIncludesBranchContext(t *testing.T) {
	panel := NewReviewPanel()
	panel.SetSize(72, 12)
	panel.Configure("worker-1", "feat/task-1", "main")

	rendered := panel.Render()

	require.Contains(t, rendered, "Review Workflow: worker-1")
	require.Contains(t, rendered, "feat/task-1")
	require.Contains(t, rendered, "main")
}
