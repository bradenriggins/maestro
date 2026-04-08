package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"
)

func TestTabbedWindowSetSizeUsesFullAssignedDimensions(t *testing.T) {
	window := NewTabbedWindow(NewPreviewPane(), NewDiffPane(), NewTerminalPane())

	window.SetSize(90, 20)

	expectedContentWidth := TabbedWindowContentWidth(90)
	expectedContentHeight := TabbedWindowContentHeight(20)

	require.Equal(t, 90, window.width)
	require.Equal(t, expectedContentWidth, window.preview.width)
	require.Equal(t, expectedContentWidth, window.diff.width)
	require.Equal(t, expectedContentWidth, window.terminal.width)
	require.Equal(t, expectedContentHeight, window.preview.height)
	require.Equal(t, expectedContentHeight, window.diff.height)
	require.Equal(t, expectedContentHeight, window.terminal.height)
}

func TestTabbedWindowStringMatchesAssignedSize(t *testing.T) {
	window := NewTabbedWindow(NewPreviewPane(), NewDiffPane(), NewTerminalPane())
	window.SetSize(90, 20)
	window.preview.SetContent("preview")

	rendered := window.String()

	require.Equal(t, 90, lipgloss.Width(rendered))
	require.Equal(t, 20, lipgloss.Height(rendered))
}
