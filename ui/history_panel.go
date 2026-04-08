package ui

import (
	"fmt"
	"sort"
	"strings"

	"maestro/pkg/orchestration"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type HistoryPanelEvent int

const (
	HistoryPanelNone HistoryPanelEvent = iota
	HistoryPanelClose
)

type HistoryPanel struct {
	width   int
	height  int
	loading bool
	loadErr string
	tasks   []*orchestration.Task
	cursor  int
}

func NewHistoryPanel() *HistoryPanel {
	return &HistoryPanel{}
}

func (p *HistoryPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *HistoryPanel) SetLoading(loading bool) {
	p.loading = loading
	if loading {
		p.loadErr = ""
	}
}

func (p *HistoryPanel) SetError(err error) {
	p.loading = false
	if err == nil {
		p.loadErr = ""
		return
	}
	p.loadErr = err.Error()
}

func (p *HistoryPanel) SetTasks(tasks []*orchestration.Task) {
	p.loading = false
	p.loadErr = ""
	p.tasks = append([]*orchestration.Task(nil), tasks...)
	sort.SliceStable(p.tasks, func(i, j int) bool {
		return p.tasks[i].CreatedAt > p.tasks[j].CreatedAt
	})
	if len(p.tasks) == 0 {
		p.cursor = 0
		return
	}
	if p.cursor >= len(p.tasks) {
		p.cursor = len(p.tasks) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
}

func (p *HistoryPanel) SelectedTask() *orchestration.Task {
	if len(p.tasks) == 0 || p.cursor >= len(p.tasks) {
		return nil
	}
	return p.tasks[p.cursor]
}

func (p *HistoryPanel) HandleKeyPress(msg tea.KeyMsg) HistoryPanelEvent {
	switch msg.String() {
	case "esc", "ctrl+c":
		return HistoryPanelClose
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
		}
	case "down", "j":
		if p.cursor < len(p.tasks)-1 {
			p.cursor++
		}
	}
	return HistoryPanelNone
}

func (p *HistoryPanel) Render() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	lines := []string{
		titleStyle.Render("History Workflow"),
		"",
		"Browse recent orchestration work and inspect the latest artifacts.",
		"",
	}

	switch {
	case p.loading:
		lines = append(lines, dimStyle.Render("Loading task history..."))
	case p.loadErr != "":
		lines = append(lines, errorStyle.Render(p.loadErr))
	case len(p.tasks) == 0:
		lines = append(lines, dimStyle.Render("No task history yet. Dispatch work to populate this panel."))
	default:
		lines = append(lines, "Recent tasks")
		for idx, task := range p.tasks {
			line := fmt.Sprintf("%s  %s  %s", task.ID, task.Status, task.WorkerInstance)
			if idx == p.cursor {
				lines = append(lines, selectedStyle.Render("> "+line))
			} else {
				lines = append(lines, "  "+line)
			}
			if idx >= 7 {
				break
			}
		}

		if task := p.SelectedTask(); task != nil {
			lines = append(lines,
				"",
				"Selected task",
				fmt.Sprintf("Status: %s", task.Status),
				fmt.Sprintf("Worker: %s", task.WorkerInstance),
				fmt.Sprintf("Prompt: %s", task.PromptFile),
				fmt.Sprintf("Result: %s", task.ResultFile),
			)
			if task.Error != nil && *task.Error != "" {
				lines = append(lines, errorStyle.Render("Error: "+*task.Error))
			}
		}
	}

	lines = append(lines, "", dimStyle.Render("[Esc] sessions  [↑/↓] browse"))

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
