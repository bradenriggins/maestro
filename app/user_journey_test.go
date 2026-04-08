package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"maestro/pkg/accounts"
	"maestro/ui/overlay"
)

func TestUserJourneyPromptResizeCancelRetryRemainsRecoverable(t *testing.T) {
	h := newAutomationHome(t)

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	require.Equal(t, stateHelp, h.state)
	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	require.Equal(t, stateDefault, h.state)

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	for _, r := range "journey-one" {
		pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEnter})
	require.Equal(t, statePrompt, h.state)

	sendWindowSize(t, h, 96, 24)
	view := renderPlain(h)
	require.Contains(t, view, "Enter prompt")
	require.Contains(t, view, "Branch")

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEsc})
	require.Equal(t, stateDefault, h.state)
	require.Equal(t, 0, h.list.NumInstances())

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	for _, r := range "journey-two" {
		pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEnter})
	require.Equal(t, statePrompt, h.state)
	view = renderPlain(h)
	require.Contains(t, view, "journey-two")
	require.NotContains(t, view, "journey-one")
}

func TestUserJourneyReviewEditResizeCancelReopenHasNoStaleTask(t *testing.T) {
	h := newAutomationHome(t)
	h.conductorConfig = &accounts.ConductorConfig{}
	addAutomationInstance(t, h, automationInstanceOpts{title: "worker-1", branch: "feat/review", role: string(accounts.RoleWorker), account: "acct-1"})

	h.reviewOverlay = overlay.NewReviewOverlay("worker-1", "feat/review", "main")
	h.reviewOverlay.SetSize(140, 40)
	h.state = stateReview

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	require.Equal(t, stateQuickDispatch, h.state)

	for _, r := range "tighten copy" {
		if r == ' ' {
			pressKey(t, h, tea.KeyMsg{Type: tea.KeySpace})
			continue
		}
		pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	sendWindowSize(t, h, 104, 28)
	view := renderPlain(h)
	require.Contains(t, view, "Quick Dispatch")
	require.Contains(t, view, "tighten copy")

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEsc})
	require.Equal(t, stateDefault, h.state)
	require.Nil(t, h.quickDispatchOverlay)

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	require.Equal(t, stateQuickDispatch, h.state)
	view = renderPlain(h)
	require.Contains(t, view, "worker-1")
	require.NotContains(t, view, "tighten copy")
}

func TestUserJourneyOrchestrationPreviewFlowSurvivesResizeAndBacktracksCleanly(t *testing.T) {
	h := newAutomationHome(t)
	h.conductorConfig = &accounts.ConductorConfig{}

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	require.Equal(t, stateOrchestration, h.state)
	h.orchestrationOverlay.SetPreviewContent("task-2-preview", strings.Join([]string{
		"# Failed task replay",
		"",
		"Observed flaky UI after resize and overlay churn.",
		"",
		"Suggested fixes:",
		"- preserve last good render during refresh",
		"- tighten empty-state guidance",
		"- add resize coverage",
	}, "\n"))

	view := renderPlain(h)
	require.Contains(t, view, "Task Result: task-2-preview")
	require.Contains(t, view, "Failed task replay")

	sendWindowSize(t, h, 100, 26)
	view = renderPlain(h)
	require.Contains(t, view, "Task Result: task-2-preview")
	require.Contains(t, view, "tighten empty-state guidance")

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	view = renderPlain(h)
	require.Contains(t, view, "add resize coverage")

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEsc})
	view = renderPlain(h)
	require.Contains(t, view, "Orchestration Panel")

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEsc})
	require.Equal(t, stateDefault, h.state)
}

func TestUserJourneyChaosNavigationLeavesAppRecoverable(t *testing.T) {
	h := newAutomationHome(t)
	h.conductorConfig = &accounts.ConductorConfig{}
	addAutomationInstance(t, h, automationInstanceOpts{title: "worker-1", branch: "feat/chaos", role: string(accounts.RoleWorker), account: "acct-1"})

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	sendWindowSize(t, h, 110, 30)
	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEsc})

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	sendWindowSize(t, h, 92, 24)
	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEsc})

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEsc})

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEsc})

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	pressKey(t, h, tea.KeyMsg{Type: tea.KeyTab})
	sendWindowSize(t, h, 140, 40)

	view := renderPlain(h)
	require.Equal(t, stateDefault, h.state)
	require.Contains(t, view, "worker-1")
	require.Contains(t, view, "Terminal")
}

func sendWindowSize(t *testing.T, h *home, width, height int) {
	t.Helper()
	_, cmd := h.Update(tea.WindowSizeMsg{Width: width, Height: height})
	if cmd != nil {
		applyCmd(t, h, cmd)
	}
}
