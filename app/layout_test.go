package app

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"
)

func assertRenderedViewFitsViewport(t *testing.T, rendered string, width, height int) {
	t.Helper()

	require.LessOrEqual(t, lipgloss.Height(rendered), height, "rendered height exceeds viewport")
	for idx, line := range strings.Split(rendered, "\n") {
		require.LessOrEqualf(t, lipgloss.Width(line), width, "line %d exceeds viewport width", idx+1)
	}
}

func TestRenderedViewFitsViewportUnderStress(t *testing.T) {
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
