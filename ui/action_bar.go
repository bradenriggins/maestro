package ui

import (
	"maestro/keys"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
)

type ActionBarAction struct {
	Key   keys.KeyName
	Label string
}

type ActionBar struct {
	width  int
	height int
}

var actionBarKeyStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#655F5F",
	Dark:  "#B8B2B2",
}).Bold(true)

var actionBarLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#7A7474",
	Dark:  "#9C9494",
})

var actionBarSummaryStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#8A8484",
	Dark:  "#7F7A7A",
})

func NewActionBar() *ActionBar {
	return &ActionBar{}
}

func (a *ActionBar) SetSize(width, height int) {
	a.width = width
	a.height = height
}

func (a *ActionBar) Render(actions []ActionBarAction, summary string, keyDown keys.KeyName) string {
	if a.width <= 0 || a.height <= 0 {
		return ""
	}

	left := a.renderActions(actions, keyDown)
	right := strings.TrimSpace(summary)

	content := left
	if right != "" {
		renderedRight := actionBarSummaryStyle.Render(right)
		gap := a.width - lipgloss.Width(left) - lipgloss.Width(renderedRight)
		if gap >= 2 {
			content = left + strings.Repeat(" ", gap) + renderedRight
		}
	}

	if lipgloss.Width(content) > a.width {
		content = truncate.String(content, uint(a.width))
	}

	return lipgloss.Place(a.width, a.height, lipgloss.Left, lipgloss.Center, content)
}

func (a *ActionBar) renderActions(actions []ActionBarAction, keyDown keys.KeyName) string {
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		label := action.Label
		if label == "" {
			label = keys.GlobalKeyBindings[action.Key].Help().Desc
		}

		keyStyle := actionBarKeyStyle
		labelStyle := actionBarLabelStyle
		if keyDown == action.Key {
			keyStyle = keyStyle.Underline(true)
			labelStyle = labelStyle.Underline(true)
		}

		parts = append(parts, keyStyle.Render(displayActionKey(action.Key))+" "+labelStyle.Render(label))
	}

	return strings.Join(parts, "  ")
}

func displayActionKey(name keys.KeyName) string {
	switch name {
	case keys.KeyEnter, keys.KeySubmitName:
		return "Enter"
	case keys.KeyTab:
		return "Tab"
	case keys.KeyShiftUp:
		return "Shift+↑"
	case keys.KeyShiftDown:
		return "Shift+↓"
	default:
		return keys.GlobalKeyBindings[name].Help().Key
	}
}
