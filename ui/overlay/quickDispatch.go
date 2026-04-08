package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	qdTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	qdDimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	qdSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	qdInputStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	qdErrorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	qdBoxStyle      = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2)
)

// QuickDispatchOverlay is a small modal that lets the user type a task and
// pick a worker to dispatch it to. Cycle workers with left/right arrows,
// type a task description, then press Enter to dispatch or Esc to cancel.
type QuickDispatchOverlay struct {
	workers      []string          // worker instance names
	workerIdx    int               // currently selected worker index
	workerModels map[string]string // worker name -> model name
	taskInput    string            // task text being typed
	submitted    bool
	canceled     bool
	width        int
	height       int
	errorMsg     string // set when Enter is pressed with no workers available
	busyCount    int    // number of workers that exist but were excluded (e.g. paused)
}

// NewQuickDispatchOverlay creates a new QuickDispatchOverlay with the given
// list of worker names and an optional count of busy/paused workers that were
// excluded from the available list.
func NewQuickDispatchOverlay(workers []string) *QuickDispatchOverlay {
	return &QuickDispatchOverlay{
		workers: workers,
	}
}

// SetBusyCount records how many workers exist but are not available (e.g. paused).
// This lets the overlay show a more specific hint when no workers are in the list.
func (q *QuickDispatchOverlay) SetBusyCount(n int) {
	q.busyCount = n
}

// SetWorkerModels stores a mapping of worker name to model name so the
// overlay can display which model each worker is running.
func (q *QuickDispatchOverlay) SetWorkerModels(m map[string]string) {
	q.workerModels = m
}

// SetWidth sets the rendered width of the overlay to 60% of the given
// terminal width, with a minimum of 40 columns.
func (q *QuickDispatchOverlay) SetWidth(w int) {
	q.width = int(float32(w) * 0.6)
	if q.width < 40 {
		q.width = 40
	}
}

// SetViewport sizes the quick dispatch overlay directly from the viewport dimensions.
func (q *QuickDispatchOverlay) SetViewport(viewW, viewH int) {
	box := ComputeModalBox(viewW, viewH, 80, 18)
	q.width = box.OuterWidth
	q.height = box.OuterHeight
}

// HandleKeyPress processes a key event and updates overlay state.
// Returns true when the overlay should be closed (submitted or canceled).
func (q *QuickDispatchOverlay) HandleKeyPress(msg tea.KeyMsg) (shouldClose bool) {
	switch msg.Type {
	case tea.KeyEsc:
		q.canceled = true
		return true
	case tea.KeyEnter:
		if len(q.workers) == 0 {
			if q.busyCount > 0 {
				q.errorMsg = "All workers busy. Press Esc, then 'o' to check status or wait for a worker to become idle."
			} else {
				q.errorMsg = "No workers available. Press Esc, then 'n' to create an instance or 'o' for status."
			}
			return false
		}
		if strings.TrimSpace(q.taskInput) != "" {
			q.submitted = true
			return true
		}
		return false
	case tea.KeyLeft:
		if q.workerIdx > 0 {
			q.workerIdx--
		}
		return false
	case tea.KeyRight:
		if q.workerIdx < len(q.workers)-1 {
			q.workerIdx++
		}
		return false
	case tea.KeyBackspace:
		runes := []rune(q.taskInput)
		if len(runes) > 0 {
			q.taskInput = string(runes[:len(runes)-1])
		}
		return false
	case tea.KeyRunes:
		q.taskInput += string(msg.Runes)
		return false
	case tea.KeySpace:
		q.taskInput += " "
		return false
	}
	return false
}

// IsSubmitted returns true if the user pressed Enter with a valid task and worker.
func (q *QuickDispatchOverlay) IsSubmitted() bool { return q.submitted }

// IsCanceled returns true if the user pressed Esc.
func (q *QuickDispatchOverlay) IsCanceled() bool { return q.canceled }

// GetWorker returns the name of the currently selected worker, or an empty
// string if no workers are available.
func (q *QuickDispatchOverlay) GetWorker() string {
	if len(q.workers) == 0 {
		return ""
	}
	return q.workers[q.workerIdx]
}

// GetTask returns the trimmed task description entered by the user.
func (q *QuickDispatchOverlay) GetTask() string {
	return strings.TrimSpace(q.taskInput)
}

// Render returns the rendered string for the overlay.
func (q *QuickDispatchOverlay) Render() string {
	var b strings.Builder
	b.WriteString(qdTitleStyle.Render("Quick Dispatch"))
	b.WriteString("\n\n")

	// Worker picker
	b.WriteString("  Worker: ")
	if len(q.workers) == 0 {
		b.WriteString(qdDimStyle.Render("(no workers available)"))
	} else {
		for i, w := range q.workers {
			displayName := w
			if q.workerModels != nil {
				if model, ok := q.workerModels[w]; ok && model != "" {
					displayName = fmt.Sprintf("%s (%s)", w, model)
				}
			}
			if i == q.workerIdx {
				b.WriteString(qdSelectedStyle.Render("[" + displayName + "]"))
			} else {
				b.WriteString(qdDimStyle.Render(" " + displayName + " "))
			}
			if i < len(q.workers)-1 {
				b.WriteString(" ")
			}
		}
		b.WriteString(qdDimStyle.Render("  ← / → to change"))
	}
	b.WriteString("\n\n")

	// Task input
	b.WriteString("  Task:   ")
	displayText := q.taskInput
	if displayText == "" {
		b.WriteString(qdDimStyle.Render("Type task description..."))
	} else {
		b.WriteString(qdInputStyle.Render(displayText))
	}
	b.WriteString("█")
	b.WriteString("\n\n")

	// Footer hint
	b.WriteString(qdDimStyle.Render("  [Enter] dispatch  [Esc] cancel"))

	// Error message (shown when Enter is pressed with no workers)
	if q.errorMsg != "" {
		b.WriteString("\n\n")
		b.WriteString(qdErrorStyle.Render("  " + q.errorMsg))
	}

	style := qdBoxStyle.Width(q.width)
	if q.height > 0 {
		style = style.Height(q.height)
	}
	return style.Render(b.String())
}

// Visible reports whether the overlay is currently active.
func (q *QuickDispatchOverlay) Visible() bool {
	return q != nil
}

// String implements fmt.Stringer for convenient debugging.
func (q *QuickDispatchOverlay) String() string {
	return fmt.Sprintf("QuickDispatchOverlay{worker=%q, task=%q, submitted=%v, canceled=%v}",
		q.GetWorker(), q.GetTask(), q.submitted, q.canceled)
}
