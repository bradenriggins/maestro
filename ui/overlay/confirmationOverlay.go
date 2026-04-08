package overlay

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConfirmationOverlay represents a confirmation dialog overlay
type ConfirmationOverlay struct {
	// Whether the overlay has been dismissed
	Dismissed bool
	// Confirmed is true when the user pressed the confirm key (as opposed to cancel/esc).
	Confirmed bool
	// Message to display in the overlay
	message string
	// Width of the overlay
	width int
	// Height of the overlay
	height int
	// Callback function to be called when the user confirms (presses 'y')
	OnConfirm func()
	// Callback function to be called when the user cancels (presses 'n' or 'esc')
	OnCancel func()
	// Custom confirm key (defaults to 'y')
	ConfirmKey string
	// Custom cancel key (defaults to 'n')
	CancelKey string
	// Custom styling options
	borderColor lipgloss.Color
}

// NewConfirmationOverlay creates a new confirmation dialog overlay with the given message
func NewConfirmationOverlay(message string) *ConfirmationOverlay {
	return &ConfirmationOverlay{
		Dismissed:   false,
		message:     message,
		width:       50, // Default width
		ConfirmKey:  "y",
		CancelKey:   "n",
		borderColor: lipgloss.Color("#de613e"), // Red color for confirmations
	}
}

// HandleKeyPress processes a key press and updates the state
// Returns true if the overlay should be closed
func (c *ConfirmationOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	switch msg.String() {
	case c.ConfirmKey:
		c.Dismissed = true
		c.Confirmed = true
		if c.OnConfirm != nil {
			c.OnConfirm()
		}
		return true
	case c.CancelKey, "esc":
		c.Dismissed = true
		c.Confirmed = false
		if c.OnCancel != nil {
			c.OnCancel()
		}
		return true
	default:
		// Ignore other keys in confirmation state
		return false
	}
}

// Render renders the confirmation overlay
func (c *ConfirmationOverlay) Render(opts ...WhitespaceOption) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c.borderColor).
		Padding(1, 2).
		Width(c.width)
	if c.height > 0 {
		style = style.Height(c.height)
	}

	// Add the confirmation instructions
	content := c.message + "\n\n" +
		"Press " + lipgloss.NewStyle().Bold(true).Render(c.ConfirmKey) + " to confirm, " +
		lipgloss.NewStyle().Bold(true).Render(c.CancelKey) + " or " +
		lipgloss.NewStyle().Bold(true).Render("esc") + " to cancel"

	// Apply the border style and return
	return style.Render(content)
}

// SetWidth sets the width of the confirmation overlay
func (c *ConfirmationOverlay) SetWidth(width int) {
	c.width = width
}

// SetViewport sets the confirmation overlay size from viewport dimensions.
func (c *ConfirmationOverlay) SetViewport(viewW, viewH int) {
	box := ComputeModalBox(viewW, viewH, 50, 12)
	c.width = box.OuterWidth
	c.height = box.OuterHeight
}

// SetBorderColor sets the border color of the confirmation overlay
func (c *ConfirmationOverlay) SetBorderColor(color lipgloss.Color) {
	c.borderColor = color
}

// SetConfirmKey sets the key used to confirm the action
func (c *ConfirmationOverlay) SetConfirmKey(key string) {
	c.ConfirmKey = key
}

// SetCancelKey sets the key used to cancel the action
func (c *ConfirmationOverlay) SetCancelKey(key string) {
	c.CancelKey = key
}
