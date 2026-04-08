package ui

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
	ReviewCancel
)

type ReviewPanel struct {
	width         int
	height        int
	instanceTitle string
	branch        string
	baseBranch    string
}

func NewReviewPanel() *ReviewPanel {
	return &ReviewPanel{}
}

func (p *ReviewPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *ReviewPanel) Configure(instanceTitle, branch, baseBranch string) {
	p.instanceTitle = instanceTitle
	p.branch = branch
	p.baseBranch = baseBranch
}

func (p *ReviewPanel) HandleKeyPress(msg tea.KeyMsg) ReviewAction {
	switch msg.String() {
	case "a":
		return ReviewApprove
	case "e":
		return ReviewEdit
	case "s":
		return ReviewSkip
	case "esc", "ctrl+c":
		return ReviewCancel
	}
	return ReviewNone
}

func (p *ReviewPanel) Render() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	actionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))

	lines := []string{
		titleStyle.Render(fmt.Sprintf("Review Workflow: %s", p.instanceTitle)),
		"",
		"Close out worker work without dropping back into the sessions view.",
		"",
		fmt.Sprintf("Branch: %s -> %s", branchStyle.Render(p.branch), branchStyle.Render(p.baseBranch)),
		"",
		actionStyle.Render("[a] approve") + " merge the branch back",
		actionStyle.Render("[e] edit") + " re-dispatch follow-up work",
		actionStyle.Render("[s] skip") + " leave the branch as-is",
		"",
		dimStyle.Render("[Esc] sessions"),
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	width := p.width
	if width <= 0 {
		width = 60
	}
	return box.Width(width - box.GetHorizontalFrameSize()).Render(strings.Join(lines, "\n"))
}
