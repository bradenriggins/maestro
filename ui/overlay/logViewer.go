package overlay

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"maestro/pkg/accounts"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LogViewerOverlay displays the last 50 lines of the maestro log file.
type LogViewerOverlay struct {
	width       int
	height      int
	visible     bool
	lastRead    time.Time
	cachedLines []string
}

// NewLogViewerOverlay creates a new LogViewerOverlay.
func NewLogViewerOverlay() *LogViewerOverlay {
	return &LogViewerOverlay{}
}

// SetSize updates the overlay dimensions based on the terminal size.
func (l *LogViewerOverlay) SetSize(w, h int) {
	l.SetViewport(w, h)
}

// SetViewport sizes the log viewer overlay directly from viewport dimensions.
func (l *LogViewerOverlay) SetViewport(viewW, viewH int) {
	box := ComputeModalBox(viewW, viewH, 120, 60)
	l.width = box.OuterWidth
	l.height = box.OuterHeight
}

// Toggle flips the visibility of the overlay.
func (l *LogViewerOverlay) Toggle() { l.visible = !l.visible }

// IsVisible reports whether the overlay is currently shown.
func (l *LogViewerOverlay) IsVisible() bool { return l.visible }

// Close hides the overlay.
func (l *LogViewerOverlay) Close() { l.visible = false }

// NeedsRefresh reports whether the cached log lines are stale (older than 1 second).
func (l *LogViewerOverlay) NeedsRefresh() bool {
	return time.Since(l.lastRead) > 1*time.Second
}

// SetLines applies freshly-read log lines to the overlay. Must be called from
// Update() on the BubbleTea main loop, not from a goroutine.
func (l *LogViewerOverlay) SetLines(lines []string) {
	l.cachedLines = lines
	l.lastRead = time.Now()
}

// CollectLogLines reads the maestro log file and returns the last 50 lines.
// It is a pure function that only reads external state and returns a value.
// The caller must run it inside a tea.Cmd goroutine and apply the result via
// SetLines() in Update().
func CollectLogLines() []string {
	// Primary log location: the temp directory where log.Initialize writes.
	logPath := filepath.Join(os.TempDir(), "maestro.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		// Fallback: check the maestro logs directory.
		base, baseErr := accounts.ConductorDir()
		if baseErr != nil {
			return nil
		}
		data, err = os.ReadFile(filepath.Join(base, "logs", "maestro.log"))
		if err != nil {
			return nil
		}
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	start := 0
	if len(lines) > 50 {
		start = len(lines) - 50
	}
	return lines[start:]
}

// HandleKeyPress processes a key press and returns true if the overlay should close.
func (l *LogViewerOverlay) HandleKeyPress(msg tea.KeyMsg) (shouldClose bool) {
	switch msg.String() {
	case "esc", "l":
		return true
	}
	return false
}

// Render returns the rendered log viewer overlay as a string. Render is
// intentionally side-effect free and reads only from cachedLines.
func (l *LogViewerOverlay) Render() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	logStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("Maestro Logs"))
	b.WriteString(dimStyle.Render("  (last 50 lines)"))
	b.WriteString("\n\n")

	if len(l.cachedLines) == 0 {
		b.WriteString(dimStyle.Render("  (no log file found)"))
	} else {
		for _, line := range l.cachedLines {
			b.WriteString(logStyle.Render("  " + line))
			b.WriteString("\n")
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
