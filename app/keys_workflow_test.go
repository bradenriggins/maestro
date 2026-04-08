package app

import (
	"maestro/pkg/accounts"
	"maestro/session"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func newWorkflowKeyTestHome(t *testing.T) *home {
	t.Helper()

	h := newAutomationHomeForTest(t)
	h.conductorConfig = &accounts.ConductorConfig{}

	instance, err := session.NewInstance(session.InstanceOptions{
		Title:   "worker-1",
		Path:    t.TempDir(),
		Program: "claude",
		Account: "worker-1",
		Role:    string(accounts.RoleWorker),
	})
	require.NoError(t, err)
	instance.Branch = "feat/review-target"

	h.list.AddInstance(instance)()
	h.list.SetSelectedInstance(0)
	_ = h.instanceChanged()
	h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 100, Height: 28})

	return h
}

func TestSlashOpensDispatchWorkflowPanel(t *testing.T) {
	h := newWorkflowKeyTestHome(t)

	model, _ := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	updated, ok := model.(*home)
	require.True(t, ok)

	require.Equal(t, stateDefault, updated.state)
	require.Equal(t, workflowDispatch, updated.currentWorkflow())
	require.Nil(t, updated.quickDispatchOverlay)
}

func TestReviewEditTransitionsIntoDispatchWorkflowPanel(t *testing.T) {
	h := newWorkflowKeyTestHome(t)

	model, _ := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	updated, ok := model.(*home)
	require.True(t, ok)

	require.Equal(t, stateDefault, updated.state)
	require.Equal(t, workflowReview, updated.currentWorkflow())
	require.Nil(t, updated.reviewOverlay)

	model, _ = updated.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	updated, ok = model.(*home)
	require.True(t, ok)

	require.Equal(t, stateDefault, updated.state)
	require.Equal(t, workflowDispatch, updated.currentWorkflow())
	require.Nil(t, updated.quickDispatchOverlay)
}

func TestHistoryKeySelectsHistoryWorkflowPanel(t *testing.T) {
	h := newWorkflowKeyTestHome(t)

	model, _ := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	updated, ok := model.(*home)
	require.True(t, ok)

	require.Equal(t, stateDefault, updated.state)
	require.Equal(t, workflowHistory, updated.currentWorkflow())
	require.True(t, updated.orchestrationOverlay == nil || !updated.orchestrationOverlay.IsVisible())
}
