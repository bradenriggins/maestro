package overlay

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"claude-conductor/pkg/accounts"
	"claude-conductor/pkg/orchestration"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// OrchestrationOverlay displays workers, tasks, and a plan preview in a modal panel.
type OrchestrationOverlay struct {
	width   int
	height  int
	visible bool
}

// NewOrchestrationOverlay creates a new orchestration overlay.
func NewOrchestrationOverlay() *OrchestrationOverlay {
	return &OrchestrationOverlay{}
}

// SetSize calculates the overlay dimensions as 80% width and 60% height of the
// terminal dimensions provided.
func (o *OrchestrationOverlay) SetSize(w, h int) {
	o.width = int(float32(w) * 0.8)
	o.height = int(float32(h) * 0.6)
}

// Toggle flips the visibility of the overlay.
func (o *OrchestrationOverlay) Toggle() {
	o.visible = !o.visible
}

// IsVisible reports whether the overlay is currently shown.
func (o *OrchestrationOverlay) IsVisible() bool {
	return o.visible
}

// Close hides the overlay.
func (o *OrchestrationOverlay) Close() {
	o.visible = false
}

// HandleKeyPress processes a key event. Returns true when the overlay should be
// closed (Esc or "o").
func (o *OrchestrationOverlay) HandleKeyPress(msg tea.KeyMsg) (shouldClose bool) {
	switch msg.String() {
	case "esc", "o":
		return true
	}
	return false
}

// Render builds and returns the styled overlay string.
func (o *OrchestrationOverlay) Render(opts ...WhitespaceOption) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	b.WriteString(titleStyle.Render("Orchestration Panel"))
	b.WriteString("\n\n")

	// --- Workers section ---
	b.WriteString(titleStyle.Render("Workers:"))
	b.WriteString("\n")

	reg, regErr := orchestration.LoadRegistry()
	var store *orchestration.TaskStore
	if regErr == nil {
		store, _ = orchestration.NewTaskStore()
	}

	if regErr != nil || reg == nil {
		b.WriteString(dimStyle.Render("  (registry unavailable)"))
		b.WriteString("\n")
	} else {
		workers := reg.ListWorkers()
		if len(workers) == 0 {
			b.WriteString(dimStyle.Render("  (no workers)"))
			b.WriteString("\n")
		}
		for name, entry := range workers {
			state := "unknown"
			taskInfo := ""
			if store != nil {
				ws, wsErr := store.ReadStatus(name)
				if wsErr == nil {
					state = ws.State
					if ws.LastTask != "" {
						task, tErr := store.Get(ws.LastTask)
						if tErr == nil {
							if ws.State == orchestration.StateWorking {
								taskInfo = fmt.Sprintf("Task: %s", orchTruncateStr(task.ID, 20))
							} else {
								taskInfo = fmt.Sprintf("Last: %s", orchTruncateStr(task.ID, 20))
							}
						}
					}
				}
			}

			statusColor := "242"
			switch state {
			case orchestration.StateIdle:
				statusColor = "242"
			case orchestration.StateWorking:
				statusColor = "42"
			case orchestration.StateRateLimited:
				statusColor = "214"
			}

			stateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor))
			badge := dimStyle.Render(fmt.Sprintf("[%s]", entry.Account))
			b.WriteString(fmt.Sprintf("  %-16s %s  %s  %s\n",
				name, badge, stateStyle.Render(state), dimStyle.Render(taskInfo)))
		}
	}

	// --- Tasks section ---
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("Tasks:"))
	b.WriteString("\n")

	if store != nil {
		tasks, tErr := store.List("")
		if tErr == nil && len(tasks) > 0 {
			// Show the most recent 10 tasks.
			start := 0
			if len(tasks) > 10 {
				start = len(tasks) - 10
			}
			for _, task := range tasks[start:] {
				statusColor := "242"
				switch task.Status {
				case orchestration.StatusCompleted:
					statusColor = "42"
				case orchestration.StatusFailed, orchestration.StatusTimedOut:
					statusColor = "196"
				case orchestration.StatusInProgress:
					statusColor = "33"
				case orchestration.StatusDispatched:
					statusColor = "214"
				}
				sStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor))
				b.WriteString(fmt.Sprintf("  %-20s  %s  %-16s\n",
					orchTruncateStr(task.ID, 20),
					sStyle.Render(fmt.Sprintf("%-12s", task.Status)),
					task.WorkerInstance))
			}
		} else {
			b.WriteString(dimStyle.Render("  (no tasks)"))
			b.WriteString("\n")
		}
	} else {
		b.WriteString(dimStyle.Render("  (task store unavailable)"))
		b.WriteString("\n")
	}

	// --- Plan section ---
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("Plan:"))
	b.WriteString("\n")

	base, err := accounts.ConductorDir()
	if err == nil {
		planPath := filepath.Join(base, "plan.md")
		data, err := os.ReadFile(planPath)
		if err == nil {
			lines := strings.Split(string(data), "\n")
			maxLines := 10
			if len(lines) < maxLines {
				maxLines = len(lines)
			}
			for _, line := range lines[:maxLines] {
				b.WriteString(dimStyle.Render("  " + line))
				b.WriteString("\n")
			}
		} else {
			b.WriteString(dimStyle.Render("  (no plan.md found)"))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("[o/Esc] close"))

	// Wrap everything in a rounded bordered box.
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(o.width).
		Height(o.height)

	return boxStyle.Render(b.String())
}

// orchTruncateStr shortens s to at most max runes, appending "..." when truncated.
func orchTruncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
