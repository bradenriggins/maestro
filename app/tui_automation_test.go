package app

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestAutomatedUIEmptyStateHelpFlow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	h, err := newHome(context.Background(), "claude", false, true, true)
	require.NoError(t, err)
	h.conductorConfig = nil
	h.setupNeeded = false
	h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 140, Height: 40})

	view := h.View()
	require.Contains(t, view, "No instances yet")
	require.Contains(t, view, "Press 'n' to create your first instance")

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	require.Equal(t, stateHelp, h.state)
	require.NotNil(t, h.textOverlay)
	require.Contains(t, h.View(), "Create a new session")
	require.Contains(t, h.View(), "Orchestration overlay")

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	require.Equal(t, stateDefault, h.state)
	require.Contains(t, h.View(), "No instances yet")
}

func TestAutomatedUINewSessionCancelFlow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	h, err := newHome(context.Background(), "claude", false, true, true)
	require.NoError(t, err)
	h.conductorConfig = nil
	h.setupNeeded = false
	h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 140, Height: 40})

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	require.Equal(t, stateNew, h.state)
	require.Equal(t, 1, h.list.NumInstances())

	for _, r := range "debug-flow" {
		pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	view := renderPlain(h)
	require.Contains(t, view, "debug-flow")

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEsc})
	require.Equal(t, stateDefault, h.state)
	require.Equal(t, 0, h.list.NumInstances())
	require.Contains(t, h.View(), "No instances yet")
}

func pressKey(t *testing.T, h *home, msg tea.KeyMsg) {
	t.Helper()
	model, cmd := h.Update(msg)
	updated, ok := model.(*home)
	require.True(t, ok)
	*h = *updated
	if cmd != nil {
		applyCmd(t, h, cmd)
	}
}

func applyCmd(t *testing.T, h *home, cmd tea.Cmd) {
	t.Helper()
	msg := cmd()
	if msg == nil {
		return
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			if child != nil {
				applyCmd(t, h, child)
			}
		}
		return
	}
	model, follow := h.Update(msg)
	updated, ok := model.(*home)
	require.True(t, ok)
	*h = *updated
	if follow != nil {
		applyCmd(t, h, follow)
	}
}
