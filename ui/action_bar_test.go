package ui

import (
	"maestro/keys"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"
)

func TestActionBarRendersPrimaryActions(t *testing.T) {
	bar := NewActionBar()
	bar.SetSize(80, 1)

	rendered := bar.Render([]ActionBarAction{
		{Key: keys.KeyEnter, Label: "open"},
		{Key: keys.KeyQuickDispatch, Label: "dispatch"},
		{Key: keys.KeyHelp, Label: "system"},
	}, "3 active", -1)

	require.Contains(t, rendered, "Enter open")
	require.Contains(t, rendered, "/ dispatch")
	require.Contains(t, rendered, "? system")
}

func TestActionBarFitsRequestedWidth(t *testing.T) {
	bar := NewActionBar()
	bar.SetSize(32, 1)

	rendered := bar.Render([]ActionBarAction{
		{Key: keys.KeyEnter, Label: "open"},
		{Key: keys.KeyQuickDispatch, Label: "dispatch"},
		{Key: keys.KeyHelp, Label: "system"},
	}, "12 active 4 blocked 1 failed", -1)

	require.Equal(t, 1, strings.Count(rendered, "\n")+1)
	require.LessOrEqual(t, lipgloss.Width(rendered), 32)
}
