package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type DispatchPanelEvent int

const (
	DispatchPanelNone DispatchPanelEvent = iota
	DispatchPanelCancel
	DispatchPanelSubmit
)

type DispatchPanel struct {
	width        int
	height       int
	workers      []string
	workerModels map[string]string
	workerIdx    int
	taskInput    string
	busyCount    int
	submitting   bool
	statusMsg    string
	errorMsg     string
}

func NewDispatchPanel() *DispatchPanel {
	return &DispatchPanel{}
}

func (p *DispatchPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *DispatchPanel) Configure(workers []string, busyCount int, workerModels map[string]string) {
	p.workers = append([]string(nil), workers...)
	p.busyCount = busyCount
	p.workerModels = workerModels
	if len(p.workers) == 0 {
		p.workerIdx = 0
		return
	}
	if p.workerIdx >= len(p.workers) {
		p.workerIdx = len(p.workers) - 1
	}
	if p.workerIdx < 0 {
		p.workerIdx = 0
	}
}

func (p *DispatchPanel) SetSelectedWorker(name string) {
	for idx, worker := range p.workers {
		if worker == name {
			p.workerIdx = idx
			return
		}
	}
}

func (p *DispatchPanel) SetTask(task string) {
	p.taskInput = task
}

func (p *DispatchPanel) Worker() string {
	if len(p.workers) == 0 || p.workerIdx >= len(p.workers) {
		return ""
	}
	return p.workers[p.workerIdx]
}

func (p *DispatchPanel) Task() string {
	return strings.TrimSpace(p.taskInput)
}

func (p *DispatchPanel) SetSubmitting(submitting bool) {
	p.submitting = submitting
}

func (p *DispatchPanel) SetStatusMessage(msg string) {
	p.statusMsg = msg
	if msg != "" {
		p.errorMsg = ""
	}
}

func (p *DispatchPanel) SetErrorMessage(msg string) {
	p.errorMsg = msg
	if msg != "" {
		p.statusMsg = ""
	}
}

func (p *DispatchPanel) ClearTask() {
	p.taskInput = ""
}

func (p *DispatchPanel) HandleKeyPress(msg tea.KeyMsg) DispatchPanelEvent {
	if p.submitting {
		switch msg.String() {
		case "esc", "ctrl+c":
			p.submitting = false
			return DispatchPanelCancel
		}
		return DispatchPanelNone
	}

	switch msg.Type {
	case tea.KeyEsc:
		return DispatchPanelCancel
	case tea.KeyEnter:
		if len(p.workers) == 0 {
			if p.busyCount > 0 {
				p.SetErrorMessage("All workers are busy. Pause, wait, or open system status with 'o'.")
			} else {
				p.SetErrorMessage("No workers are available yet. Start a worker or try again later.")
			}
			return DispatchPanelNone
		}
		if p.Task() == "" {
			p.SetErrorMessage("Enter a task before dispatching.")
			return DispatchPanelNone
		}
		p.errorMsg = ""
		return DispatchPanelSubmit
	case tea.KeyLeft:
		if p.workerIdx > 0 {
			p.workerIdx--
		}
		return DispatchPanelNone
	case tea.KeyRight:
		if p.workerIdx < len(p.workers)-1 {
			p.workerIdx++
		}
		return DispatchPanelNone
	case tea.KeyBackspace:
		runes := []rune(p.taskInput)
		if len(runes) > 0 {
			p.taskInput = string(runes[:len(runes)-1])
		}
		return DispatchPanelNone
	case tea.KeyRunes:
		p.taskInput += string(msg.Runes)
		return DispatchPanelNone
	case tea.KeySpace:
		p.taskInput += " "
		return DispatchPanelNone
	}

	switch msg.String() {
	case "ctrl+c":
		return DispatchPanelCancel
	}

	return DispatchPanelNone
}

func (p *DispatchPanel) Render() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	inputStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))

	var workerLine strings.Builder
	if len(p.workers) == 0 {
		workerLine.WriteString(dimStyle.Render("No workers available"))
	} else {
		for idx, worker := range p.workers {
			label := worker
			if model, ok := p.workerModels[worker]; ok && model != "" {
				label = fmt.Sprintf("%s (%s)", worker, model)
			}
			if idx == p.workerIdx {
				workerLine.WriteString(selectedStyle.Render("[" + label + "]"))
			} else {
				workerLine.WriteString(dimStyle.Render(" " + label + " "))
			}
			if idx < len(p.workers)-1 {
				workerLine.WriteString(" ")
			}
		}
	}

	task := p.taskInput
	if task == "" {
		task = dimStyle.Render("Describe the work to route...")
	} else {
		task = inputStyle.Render(task)
	}

	statusLine := dimStyle.Render("[Enter] dispatch  [Esc] sessions  [←/→] worker")
	if p.submitting {
		statusLine = dimStyle.Render("Dispatching task...")
	}

	body := []string{
		titleStyle.Render("Dispatch Workflow"),
		"",
		"Route work onto an available worker without leaving the workflow shell.",
		"",
		"Worker",
		workerLine.String(),
		"",
		"Task",
		task + "█",
		"",
		statusLine,
	}

	if p.statusMsg != "" {
		body = append(body, "", statusStyle.Render(p.statusMsg))
	}
	if p.errorMsg != "" {
		body = append(body, "", errorStyle.Render(p.errorMsg))
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	width := p.width
	if width <= 0 {
		width = 60
	}
	return box.Width(width - box.GetHorizontalFrameSize()).Render(strings.Join(body, "\n"))
}
