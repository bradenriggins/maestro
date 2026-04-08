package ui

import "github.com/charmbracelet/lipgloss"

func clampNonNegative(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// TabbedWindowChromeHeight returns the fixed vertical chrome consumed by the
// tab bar and bordered window shell.
func TabbedWindowChromeHeight() int {
	return activeTabStyle.GetVerticalFrameSize() + 1 + windowStyle.GetVerticalFrameSize() + 2
}

// TabbedWindowContentWidth returns the exact inner width available to the
// panes rendered inside the tabbed window frame.
func TabbedWindowContentWidth(totalWidth int) int {
	return clampNonNegative(totalWidth - windowStyle.GetHorizontalFrameSize())
}

// TabbedWindowContentHeight returns the exact inner height available to the
// panes rendered inside the tabbed window frame.
func TabbedWindowContentHeight(totalHeight int) int {
	return clampNonNegative(totalHeight - TabbedWindowChromeHeight())
}

func placeCentered(width, height int, content string) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}
