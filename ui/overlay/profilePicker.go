package overlay

import (
	"maestro/config"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// ProfilePicker is an embeddable component for selecting a profile.
// It displays a horizontal selector with left/right arrow navigation.
type ProfilePicker struct {
	profiles []config.Profile
	cursor   int
	focused  bool
	width    int
}

// NewProfilePicker creates a new profile picker with the given profiles.
// The first profile is selected by default.
func NewProfilePicker(profiles []config.Profile) *ProfilePicker {
	return &ProfilePicker{
		profiles: profiles,
	}
}

// Focus gives the profile picker focus.
func (pp *ProfilePicker) Focus() {
	pp.focused = true
}

// Blur removes focus from the profile picker.
func (pp *ProfilePicker) Blur() {
	pp.focused = false
}

// SetWidth sets the rendering width.
func (pp *ProfilePicker) SetWidth(w int) {
	pp.width = w
}

// HandleKeyPress processes a key event. Returns true if consumed.
func (pp *ProfilePicker) HandleKeyPress(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyLeft:
		if pp.cursor > 0 {
			pp.cursor--
		}
		return true
	case tea.KeyRight:
		if pp.cursor < len(pp.profiles)-1 {
			pp.cursor++
		}
		return true
	}
	return false
}

// GetSelectedProfile returns the currently selected profile.
// Returns a zero-value config.Profile if the profiles slice is empty.
func (pp *ProfilePicker) GetSelectedProfile() config.Profile {
	if len(pp.profiles) == 0 {
		return config.Profile{} // zero value — caller must handle empty Program field
	}
	if pp.cursor < 0 || pp.cursor >= len(pp.profiles) {
		return pp.profiles[0]
	}
	return pp.profiles[pp.cursor]
}

// HasMultiple returns true if there is more than one profile to choose from.
func (pp *ProfilePicker) HasMultiple() bool {
	return len(pp.profiles) > 1
}

var (
	ppLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62")).
			Bold(true)

	ppSelectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("0"))

	ppDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
)

// Render renders the profile picker.
func (pp *ProfilePicker) Render() string {
	var s strings.Builder
	header := "Profile"

	if pp.HasMultiple() && pp.focused {
		header += "  ←/→ to change"
	}
	s.WriteString(ppLabelStyle.Render(truncateToWidth(header, pp.width)))
	s.WriteString("\n\n")

	remaining := pp.width
	for i, p := range pp.profiles {
		if i > 0 && remaining > 0 {
			sep := truncateToWidth(" | ", remaining)
			s.WriteString(ppDimStyle.Render(sep))
			remaining -= runewidth.StringWidth(sep)
			if remaining <= 0 {
				break
			}
		}

		label := truncateToWidth(" "+p.Name+" ", remaining)
		if label == "" {
			break
		}
		if i == pp.cursor && pp.focused {
			s.WriteString(ppSelectedStyle.Render(label))
		} else if i == pp.cursor {
			s.WriteString(label)
		} else {
			s.WriteString(ppDimStyle.Render(label))
		}
		remaining -= runewidth.StringWidth(label)
	}

	return s.String()
}
