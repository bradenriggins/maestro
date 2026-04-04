package ui

import (
	"fmt"
	"strings"

	"claude-conductor/pkg/orchestration"

	"github.com/charmbracelet/lipgloss"
)

// StatusBar renders a progress/status bar showing task counts at the bottom of the TUI.
type StatusBar struct {
	width int
}

// NewStatusBar creates a new StatusBar.
func NewStatusBar() *StatusBar {
	return &StatusBar{}
}

// SetWidth updates the width used when rendering the status bar.
func (s *StatusBar) SetWidth(w int) {
	s.width = w
}

// Render returns the rendered status bar string, or an empty string when there
// are no tasks to display.
func (s *StatusBar) Render() string {
	store, err := orchestration.NewTaskStore()
	if err != nil {
		return ""
	}

	allTasks, err := store.List("")
	if err != nil || len(allTasks) == 0 {
		return ""
	}

	total := len(allTasks)
	completed := 0
	inProgress := 0
	failed := 0
	for _, t := range allTasks {
		switch t.Status {
		case orchestration.StatusCompleted:
			completed++
		case orchestration.StatusInProgress:
			inProgress++
		case orchestration.StatusFailed, orchestration.StatusTimedOut:
			failed++
		}
	}

	// Progress bar
	barWidth := 12
	if total > 0 {
		filled := (completed * barWidth) / total
		if filled > barWidth {
			filled = barWidth
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

		barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

		status := fmt.Sprintf("[%s] %d/%d tasks", barStyle.Render(bar), completed, total)
		if inProgress > 0 {
			status += fmt.Sprintf("  %d active", inProgress)
		}
		if failed > 0 {
			failStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
			status += fmt.Sprintf("  %s", failStyle.Render(fmt.Sprintf("%d failed", failed)))
		}

		return dimStyle.Render(status)
	}

	return ""
}
