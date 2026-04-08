package app

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/ansi"
	"github.com/stretchr/testify/require"

	"maestro/pkg/accounts"
)

func TestComputeLayoutKeepsRowsInsideViewport(t *testing.T) {
	layout := computeLayout(96, 24, layoutFlags{
		setupBanner:    true,
		conflictBanner: true,
		wakeBanner:     true,
		statusBar:      true,
		errBox:         true,
	})

	require.Equal(t, 96, layout.viewport.W)
	require.Equal(t, 24, layout.viewport.H)
	require.Equal(t, 24, layout.totalHeight())
	require.Equal(t, 1, layout.menu.H)
	require.Equal(t, 1, layout.err.H)
	require.Greater(t, layout.content.H, 0)
}

func TestRenderedViewFitsViewportUnderStress(t *testing.T) {
	h := newAutomationHome(t)
	h.conductorConfig = &accounts.ConductorConfig{}
	addAutomationInstance(t, h, automationInstanceOpts{title: "worker-1", account: "acct-1"})
	h.setupNeeded = true
	h.conflictBanner = "branch drift detected"
	h.wakeBanner = "resumed after sleep"
	h.errBox.SetMessage("dispatch failed")

	sendWindowSize(t, h, 96, 24)
	assertRenderFitsViewport(t, h.View(), 96, 24)
}

func TestHelpOverlayFitsViewportImmediatelyAfterOpen(t *testing.T) {
	h := newAutomationHome(t)
	sendWindowSize(t, h, 80, 20)

	pressKey(t, h, teaKeyRunes('?'))
	require.Equal(t, stateHelp, h.state)

	assertRenderFitsViewport(t, h.View(), 80, 20)
}

func TestPromptOverlayFitsViewportImmediatelyAfterOpen(t *testing.T) {
	h := newAutomationHome(t)
	sendWindowSize(t, h, 80, 20)

	pressKey(t, h, teaKeyRunes('N'))
	for _, r := range "resize-check" {
		pressKey(t, h, teaKeyRunes(r))
	}
	pressKey(t, h, teaKeyEnter())
	require.Equal(t, statePrompt, h.state)

	assertRenderFitsViewport(t, h.View(), 80, 20)
}

func TestWorkflowShellRenderedViewFitsViewportUnderStress(t *testing.T) {
	testCases := []struct {
		width  int
		height int
	}{
		{width: 80, height: 20},
		{width: 100, height: 28},
		{width: 120, height: 32},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%dx%d", tc.width, tc.height), func(t *testing.T) {
			h := newWorkflowShellTestHome(t)
			h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: tc.width, Height: tc.height})
			rendered := h.View()
			assertRenderedViewFitsViewport(t, rendered, tc.width, tc.height)
		})
	}
}

func TestRenderedViewFitsViewportAfterBannerStateChangeWithoutResize(t *testing.T) {
	h := newWorkflowShellTestHome(t)

	h.conflictBanner = strings.Repeat("conflict ", 40)
	_, _ = h.Update(hideErrMsg{})
	rendered := h.View()

	assertRenderedViewFitsViewport(t, rendered, 100, 28)
	require.Contains(t, rendered, "conflict")
}

func assertRenderFitsViewport(t *testing.T, rendered string, width, height int) {
	t.Helper()

	lines := strings.Split(strings.ReplaceAll(rendered, "\r", ""), "\n")
	require.Len(t, lines, height)
	for _, line := range lines {
		require.LessOrEqual(t, ansi.PrintableRuneWidth(line), width)
	}
}

func assertRenderedViewFitsViewport(t *testing.T, rendered string, width, height int) {
	t.Helper()

	require.LessOrEqual(t, lipgloss.Height(rendered), height, "rendered height exceeds viewport")
	for idx, line := range strings.Split(rendered, "\n") {
		require.LessOrEqualf(t, lipgloss.Width(line), width, "line %d exceeds viewport width", idx+1)
	}
}

func teaKeyRunes(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func teaKeyEnter() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEnter}
}
