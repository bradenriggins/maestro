package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	highlightColor    = lipgloss.AdaptiveColor{Light: "#4C7DFF", Dark: "#6B94FF"}
	primaryTextColor  = lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#dddddd"}
	mutedTextColor    = lipgloss.AdaptiveColor{Light: "#808080", Dark: "#808080"}
	subtleTextColor   = lipgloss.AdaptiveColor{Light: "#A49FA5", Dark: "#777777"}
	selectedBgColor   = lipgloss.AdaptiveColor{Light: "#dde4f0", Dark: "#2b3342"}
	selectedTextColor = lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#f4f7fb"}
	successColor      = lipgloss.AdaptiveColor{Light: "#51bd73", Dark: "#51bd73"}
	dangerColor       = lipgloss.AdaptiveColor{Light: "#de613e", Dark: "#f87171"}
	pausedColor       = lipgloss.AdaptiveColor{Light: "#888888", Dark: "#888888"}
)

var (
	readyStyle = lipgloss.NewStyle().Foreground(successColor)

	addedLinesStyle = lipgloss.NewStyle().Foreground(successColor)

	removedLinesStyle = lipgloss.NewStyle().Foreground(dangerColor)

	pausedStyle = lipgloss.NewStyle().Foreground(pausedColor)

	titleStyle = lipgloss.NewStyle().
			Padding(1, 1, 0, 1).
			Foreground(primaryTextColor)

	listDescStyle = lipgloss.NewStyle().
			Padding(0, 1, 1, 1).
			Foreground(subtleTextColor)

	selectedTitleStyle = lipgloss.NewStyle().
				Padding(1, 1, 0, 1).
				Background(selectedBgColor).
				Foreground(selectedTextColor)

	selectedDescStyle = lipgloss.NewStyle().
				Padding(0, 1, 1, 1).
				Background(selectedBgColor).
				Foreground(selectedTextColor)

	mainTitle = lipgloss.NewStyle().
			Background(highlightColor).
			Foreground(lipgloss.Color("230"))

	autoYesStyle = lipgloss.NewStyle().
			Background(selectedBgColor).
			Foreground(selectedTextColor)

	accountBadgeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	modelBadgeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))

	orchStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	previewPaneStyle = lipgloss.NewStyle().
				Foreground(primaryTextColor)

	terminalPaneStyle = lipgloss.NewStyle().
				Foreground(primaryTextColor)

	terminalFooterStyle = lipgloss.NewStyle().
				Foreground(mutedTextColor)

	inactiveTabBorder = tabBorderWithBottom("┴", "─", "┴")
	activeTabBorder   = tabBorderWithBottom("┘", " ", "└")
	inactiveTabStyle  = lipgloss.NewStyle().
				Border(inactiveTabBorder, true).
				BorderForeground(highlightColor).
				Foreground(mutedTextColor).
				AlignHorizontal(lipgloss.Center)
	activeTabStyle = lipgloss.NewStyle().
			Border(activeTabBorder, true).
			BorderForeground(highlightColor).
			Foreground(primaryTextColor).
			Bold(true).
			AlignHorizontal(lipgloss.Center)
	windowStyle = lipgloss.NewStyle().
			BorderForeground(highlightColor).
			Border(lipgloss.NormalBorder(), false, true, true, true)
)

func clampDimension(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func renderSurfaceContent(style lipgloss.Style, width, height int, content string) string {
	width = clampDimension(width)
	height = clampDimension(height)
	if width == 0 || height == 0 {
		return strings.Repeat("\n", height)
	}

	return style.
		Width(width).
		MaxWidth(width).
		Height(height).
		MaxHeight(height).
		Render(content)
}

func renderSurfaceFallback(style lipgloss.Style, width, height int, content string) string {
	return renderSurfaceContent(style.Align(lipgloss.Center, lipgloss.Center), width, height, content)
}
