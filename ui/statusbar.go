package ui

import (
	"fmt"
	"strings"
	"time"

	"maestro/pkg/accounts"
	"maestro/pkg/orchestration"

	"github.com/charmbracelet/lipgloss"
)

// TaskCounts holds a snapshot of task status counts used by the status bar.
// It is exported so that app.go can carry it in a message type and apply it
// on the BubbleTea main loop via SetCounts(), eliminating data races between
// the collection goroutine and Render() reads.
type TaskCounts struct {
	Total      int
	Completed  int
	InProgress int
	Failed     int
	Pending    int
	Blocked    int
	Stalled    int
}

// StatusBar renders a progress/status bar showing task counts at the bottom of the TUI.
type StatusBar struct {
	width           int
	cachedCounts    TaskCounts
	lastCountUpdate time.Time
}

// NewStatusBar creates a new StatusBar.
func NewStatusBar() *StatusBar {
	return &StatusBar{}
}

// SetWidth updates the width used when rendering the status bar.
func (s *StatusBar) SetWidth(w int) {
	s.width = w
}

// SetCounts applies a freshly-computed TaskCounts snapshot to the status bar.
// Must be called from Update() on the BubbleTea main loop, not from a goroutine,
// to avoid data races with Render().
func (s *StatusBar) SetCounts(counts TaskCounts) {
	s.cachedCounts = counts
	s.lastCountUpdate = time.Now()
}

// CollectStatusBarCounts performs all disk I/O needed to produce a TaskCounts
// snapshot. It is a pure function: it reads external state and returns a value
// without mutating any shared struct. The caller must run it inside a tea.Cmd
// goroutine and apply the result via SetCounts() in Update().
func CollectStatusBarCounts() (TaskCounts, error) {
	store, err := orchestration.NewTaskStore()
	if err != nil {
		return TaskCounts{}, err
	}
	allTasks, err := store.List("")
	if err != nil {
		return TaskCounts{}, err
	}
	counts := TaskCounts{Total: len(allTasks)}
	for _, t := range allTasks {
		switch t.Status {
		case orchestration.StatusCompleted:
			counts.Completed++
		case orchestration.StatusInProgress, orchestration.StatusDispatched, orchestration.StatusStale:
			counts.InProgress++
		case orchestration.StatusFailed, orchestration.StatusTimedOut:
			counts.Failed++
		case orchestration.StatusPending:
			counts.Pending++
		case orchestration.StatusBlocked:
			counts.Blocked++
		}
	}

	// Count stalled workers — only update the cached stall count
	// when both the task list and registry reads succeed, so a
	// transient task-read failure doesn't silently zero it out.
	stalledCount := 0
	if reg, regErr := orchestration.LoadRegistry(); regErr == nil {
		for name, entry := range reg.Instances {
			if entry.LastOutputAt != "" && entry.Role == string(accounts.RoleWorker) {
				if lastOut, parseErr := time.Parse(time.RFC3339, entry.LastOutputAt); parseErr == nil {
					if time.Since(lastOut) > orchestration.DefaultStallThreshold {
						// Verify worker has an active task
						instTasks, _ := store.ForInstance(name)
						for _, t := range instTasks {
							if t.Status == orchestration.StatusInProgress {
								stalledCount++
								break
							}
						}
					}
				}
			}
		}
	}
	counts.Stalled = stalledCount
	return counts, nil
}

// Render returns the rendered status bar string. It is intentionally
// side-effect free and only reads from the cache last set by SetCounts.
func (s *StatusBar) Render() string {
	total := s.cachedCounts.Total
	completed := s.cachedCounts.Completed
	inProgress := s.cachedCounts.InProgress
	failed := s.cachedCounts.Failed

	helpHint := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("[?] help")

	if total == 0 {
		// No tasks — still show the help hint right-aligned.
		gap := s.width - lipgloss.Width(helpHint)
		if gap < 0 {
			gap = 0
		}
		return strings.Repeat(" ", gap) + helpHint
	}

	// Progress bar
	barWidth := 12
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
	if s.cachedCounts.Pending > 0 {
		pendStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
		status += fmt.Sprintf("  %s", pendStyle.Render(fmt.Sprintf("%d pending", s.cachedCounts.Pending)))
	}
	if s.cachedCounts.Blocked > 0 {
		blockStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
		status += fmt.Sprintf("  %s", blockStyle.Render(fmt.Sprintf("%d blocked", s.cachedCounts.Blocked)))
	}
	if failed > 0 {
		failStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		status += fmt.Sprintf("  %s", failStyle.Render(fmt.Sprintf("%d failed", failed)))
	}
	if s.cachedCounts.Stalled > 0 {
		stallStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
		status += "  " + stallStyle.Render(fmt.Sprintf("%d stalled", s.cachedCounts.Stalled))
	}

	// Place the help hint at the right edge of the bar.
	statusRendered := dimStyle.Render(status)
	gap := s.width - lipgloss.Width(statusRendered) - lipgloss.Width(helpHint)
	if gap < 2 {
		gap = 2
	}
	return statusRendered + strings.Repeat(" ", gap) + helpHint
}
