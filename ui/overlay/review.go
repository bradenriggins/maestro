package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type ReviewAction int

const (
	ReviewNone ReviewAction = iota
	ReviewApprove
	ReviewEdit
	ReviewSkip
)

type ReviewOverlay struct {
	instanceTitle string
	branch        string
	baseBranch    string
	action        ReviewAction
	width         int
	height        int
}

func NewReviewOverlay(instanceTitle, branch, baseBranch string) *ReviewOverlay {
	return &ReviewOverlay{
		instanceTitle: instanceTitle,
		branch:        branch,
		baseBranch:    baseBranch,
	}
}

func (r *ReviewOverlay) SetSize(w, h int) {
	r.SetViewport(w, h)
}

func (r *ReviewOverlay) SetViewport(viewW, viewH int) {
	box := ComputeModalBox(viewW, viewH, 80, 24)
	r.width = box.OuterWidth
	r.height = box.OuterHeight
}

func (r *ReviewOverlay) HandleKeyPress(msg tea.KeyMsg) (shouldClose bool) {
	switch msg.String() {
	case "a":
		r.action = ReviewApprove
		return true
	case "e":
		r.action = ReviewEdit
		return true
	case "s":
		r.action = ReviewSkip
		return true
	case "esc":
		r.action = ReviewNone
		return true
	}
	return false
}

func (r *ReviewOverlay) GetAction() ReviewAction {
	return r.action
}

func (r *ReviewOverlay) Render() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	actionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Review: %s", r.instanceTitle)))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("  Branch: %s → %s\n",
		branchStyle.Render(r.branch),
		branchStyle.Render(r.baseBranch)))
	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
		actionStyle.Render("[a]pprove (merge)"),
		actionStyle.Render("[e]dit (re-dispatch)"),
		actionStyle.Render("[s]kip")))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  [Esc] exit review"))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(r.width)
	if r.height > 0 {
		boxStyle = boxStyle.Height(r.height)
	}

	return boxStyle.Render(b.String())
}
