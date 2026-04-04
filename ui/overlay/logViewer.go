package overlay

import (
	"os"
	"path/filepath"
	"strings"

	"claude-conductor/pkg/accounts"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LogViewerOverlay displays the last 50 lines of the conductor log file.
type LogViewerOverlay struct {
	width   int
	height  int
	visible bool
}

// NewLogViewerOverlay creates a new LogViewerOverlay.
func NewLogViewerOverlay() *LogViewerOverlay {
	return &LogViewerOverlay{}
}

// SetSize updates the overlay dimensions based on the terminal size.
func (l *LogViewerOverlay) SetSize(w, h int) {
	l.width = int(float32(w) * 0.8)
	l.height = int(float32(h) * 0.7)
}

// Toggle flips the visibility of the overlay.
func (l *LogViewerOverlay) Toggle() { l.visible = !l.visible }

// IsVisible reports whether the overlay is currently shown.
func (l *LogViewerOverlay) IsVisible() bool { return l.visible }

// Close hides the overlay.
func (l *LogViewerOverlay) Close() { l.visible = false }

// HandleKeyPress processes a key press and returns true if the overlay should close.
func (l *LogViewerOverlay) HandleKeyPress(msg tea.KeyMsg) (shouldClose bool) {
	switch msg.String() {
	case "esc", "l":
		return true
	}
	return false
}

// Render returns the rendered log viewer overlay as a string.
func (l *LogViewerOverlay) Render() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	logStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("Conductor Logs"))
	b.WriteString(dimStyle.Render("  (last 50 lines)"))
	b.WriteString("\n\n")

	base, err := accounts.ConductorDir()
	if err != nil {
		b.WriteString(dimStyle.Render("  Error: " + err.Error()))
	} else {
		logPath := filepath.Join(base, "logs", "conductor.log")
		data, err := os.ReadFile(logPath)
		if err != nil {
			b.WriteString(dimStyle.Render("  (no log file found)"))
		} else {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			start := 0
			if len(lines) > 50 {
				start = len(lines) - 50
			}
			for _, line := range lines[start:] {
				b.WriteString(logStyle.Render("  " + line))
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("[l/Esc] close"))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(l.width).
		MaxHeight(l.height)

	return boxStyle.Render(b.String())
}
