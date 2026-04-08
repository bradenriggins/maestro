package overlay

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TextOverlay represents a text screen overlay
type TextOverlay struct {
	// Whether the overlay has been dismissed
	Dismissed bool
	// Callback function to be called when the overlay is dismissed.
	// It returns a tea.Cmd so that async work (e.g. waiting for tmux detach)
	// is run off the BubbleTea main loop.
	OnDismiss func() tea.Cmd
	// Content to display in the overlay
	content string

	width int
}

// NewTextOverlay creates a new text screen overlay with the given title and content
func NewTextOverlay(content string) *TextOverlay {
	return &TextOverlay{
		Dismissed: false,
		content:   content,
	}
}

// HandleKeyPress processes a key press and updates the state.
// Returns (shouldClose, cmd) where cmd is the tea.Cmd returned by OnDismiss (may be nil).
func (t *TextOverlay) HandleKeyPress(msg tea.KeyMsg) (bool, tea.Cmd) {
	// Close on any key
	t.Dismissed = true
	// Call the OnDismiss callback if it exists and collect the returned cmd
	var cmd tea.Cmd
	if t.OnDismiss != nil {
		cmd = t.OnDismiss()
	}
	return true, cmd
}

// Render renders the text overlay
func (t *TextOverlay) Render(opts ...WhitespaceOption) string {
	// Create styles
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(t.width)

	// Apply the border style and return
	return style.Render(t.content)
}

func (t *TextOverlay) SetWidth(width int) {
	t.width = width
}
