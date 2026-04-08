package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type WorkflowNavItem struct {
	ID       string
	Label    string
	Hint     string
	Selected bool
}

type WorkflowNav struct {
	width  int
	height int
	items  []WorkflowNavItem
}

var workflowNavTitleStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("252")).
	Bold(true).
	Padding(0, 1)

var workflowNavItemStyle = lipgloss.NewStyle().
	Padding(0, 1).
	Foreground(lipgloss.Color("245"))

var workflowNavSelectedItemStyle = workflowNavItemStyle.Copy().
	Foreground(lipgloss.Color("230")).
	Background(lipgloss.Color("62"))

var workflowNavFrameStyle = lipgloss.NewStyle().
	BorderRight(true).
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("238"))

func NewWorkflowNav() *WorkflowNav {
	return &WorkflowNav{}
}

func (w *WorkflowNav) SetSize(width, height int) {
	w.width = width
	w.height = height
}

func (w *WorkflowNav) SetItems(items []WorkflowNavItem) {
	w.items = append(w.items[:0], items...)
}

func (w *WorkflowNav) Render() string {
	if w.width <= 0 || w.height <= 0 {
		return ""
	}

	contentWidth := maxInt(w.width-workflowNavFrameStyle.GetHorizontalFrameSize(), 0)
	lines := []string{workflowNavTitleStyle.Width(contentWidth).Render("Workflows"), ""}
	for _, item := range w.items {
		line := fmt.Sprintf("  %s", item.Label)
		if item.Selected {
			line = fmt.Sprintf("› %s", item.Label)
		}
		if item.Hint != "" {
			line += " - " + item.Hint
		}
		line = runewidth.Truncate(line, contentWidth, "...")

		style := workflowNavItemStyle
		if item.Selected {
			style = workflowNavSelectedItemStyle
		}
		lines = append(lines, style.Width(contentWidth).Render(line))
	}

	return workflowNavFrameStyle.Render(
		lipgloss.Place(
			contentWidth,
			w.height,
			lipgloss.Left,
			lipgloss.Top,
			strings.Join(lines, "\n"),
		),
	)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
