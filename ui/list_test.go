package ui

import (
	"maestro/pkg/accounts"
	"maestro/session"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"
)

func TestInstanceRendererUsesAssignedWidthAndTruncatesLongTitles(t *testing.T) {
	spin := spinner.New()
	list := NewList(&spin, false)
	list.SetSize(32, 10)

	instance := &session.Instance{
		Title:   strings.Repeat("Long title ", 6),
		Branch:  "feature/very-long-branch-name-for-rendering",
		Status:  session.Ready,
		Account: "worker",
		Model:   "gpt-5",
		Role:    string(accounts.RoleOrchestrator),
	}

	rendered := list.renderer.Render(instance, 1, true, false)

	require.Equal(t, 32, lipgloss.Width(rendered))
	require.Contains(t, rendered, "...")
}
