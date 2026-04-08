package app

import (
	"os/exec"
	"sort"

	tea "github.com/charmbracelet/bubbletea"

	"maestro/pkg/orchestration"
	"maestro/session"
	"maestro/ui"
)

type workflowID string

const (
	workflowSessions workflowID = "sessions"
	workflowDispatch workflowID = "dispatch"
	workflowReview   workflowID = "review"
	workflowHistory  workflowID = "history"
	workflowSystem   workflowID = "system"
)

type workflowDescriptor struct {
	ID    workflowID
	Label string
	Hint  string
}

var workflows = []workflowDescriptor{
	{ID: workflowSessions, Label: "Sessions", Hint: "inspect workspaces and terminals"},
	{ID: workflowDispatch, Label: "Dispatch", Hint: "send work with routing context"},
	{ID: workflowReview, Label: "Review", Hint: "triage completed or failed work"},
	{ID: workflowHistory, Label: "History", Hint: "browse prior work and artifacts"},
	{ID: workflowSystem, Label: "System", Hint: "usage, logs, help, and orchestration"},
}

func (m *home) currentWorkflow() workflowID {
	switch {
	case m.state == stateQuickDispatch:
		return workflowDispatch
	case m.state == stateReview:
		return workflowReview
	case m.state == stateHelp || m.state == stateOrchestration || m.state == stateLogViewer:
		return workflowSystem
	case m.activeWorkflow != "":
		return m.activeWorkflow
	default:
		return workflowSessions
	}
}

func (m *home) setActiveWorkflow(workflow workflowID) {
	if workflow == "" {
		workflow = workflowSessions
	}
	m.activeWorkflow = workflow
}

func (m *home) setWorkflowSessions() {
	m.setActiveWorkflow(workflowSessions)
}

func (m *home) dispatchWorkers() ([]string, int, map[string]string) {
	var workers []string
	models := make(map[string]string)
	busyCount := 0

	for _, inst := range m.list.GetInstances() {
		if inst.Role != "worker" {
			continue
		}
		if inst.Status == session.Paused {
			busyCount++
			continue
		}
		workers = append(workers, inst.Title)
		if inst.Model != "" {
			models[inst.Title] = inst.Model
		}
	}

	sort.Strings(workers)
	return workers, busyCount, models
}

func (m *home) showDispatchWorkflow(selectedWorker string) {
	workers, busyCount, models := m.dispatchWorkers()
	if m.dispatchPanel == nil {
		m.dispatchPanel = ui.NewDispatchPanel()
	}
	m.dispatchPanel.SetSize(m.windowWidth, m.windowHeight)
	m.dispatchPanel.Configure(workers, busyCount, models)
	m.dispatchPanel.SetSelectedWorker(selectedWorker)
	m.dispatchPanel.SetSubmitting(false)
	m.setActiveWorkflow(workflowDispatch)
}

func (m *home) showReviewWorkflow(instanceTitle, branch, baseBranch string) {
	if m.reviewPanel == nil {
		m.reviewPanel = ui.NewReviewPanel()
	}
	m.reviewPanel.SetSize(m.windowWidth, m.windowHeight)
	m.reviewPanel.Configure(instanceTitle, branch, baseBranch)
	m.setActiveWorkflow(workflowReview)
}

func (m *home) showHistoryWorkflow() tea.Cmd {
	if m.historyPanel == nil {
		m.historyPanel = ui.NewHistoryPanel()
	}
	m.historyPanel.SetSize(m.windowWidth, m.windowHeight)
	m.historyPanel.SetLoading(true)
	m.setActiveWorkflow(workflowHistory)
	return m.loadHistoryWorkflow()
}

func (m *home) loadHistoryWorkflow() tea.Cmd {
	return func() tea.Msg {
		store, err := orchestration.NewTaskStore()
		if err != nil {
			return historyLoadedMsg{err: err}
		}
		tasks, err := store.List("")
		if err != nil {
			return historyLoadedMsg{err: err}
		}
		sort.SliceStable(tasks, func(i, j int) bool {
			return tasks[i].CreatedAt > tasks[j].CreatedAt
		})
		return historyLoadedMsg{tasks: tasks}
	}
}

func (m *home) handleDispatchWorkflowKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.dispatchPanel == nil {
		m.setWorkflowSessions()
		return m, nil
	}

	switch m.dispatchPanel.HandleKeyPress(msg) {
	case ui.DispatchPanelCancel:
		m.setWorkflowSessions()
		return m, nil
	case ui.DispatchPanelSubmit:
		worker := m.dispatchPanel.Worker()
		task := m.dispatchPanel.Task()
		m.dispatchPanel.SetSubmitting(true)
		return m, func() tea.Msg {
			result, err := orchestration.RunDispatch(worker, task, "", nil)
			if err != nil {
				return quickDispatchResultMsg{err: err}
			}
			return quickDispatchResultMsg{taskID: result.TaskID}
		}
	default:
		return m, nil
	}
}

func (m *home) handleReviewWorkflowKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.reviewPanel == nil {
		m.setWorkflowSessions()
		return m, nil
	}

	switch m.reviewPanel.HandleKeyPress(msg) {
	case ui.ReviewApprove:
		selected := m.list.GetSelectedInstance()
		m.setWorkflowSessions()
		if selected == nil {
			return m, nil
		}
		branch := selected.Branch
		title := selected.Title
		return m, func() tea.Msg {
			cmd := exec.Command("git", "merge", "--no-ff", branch,
				"-m", "maestro: merge "+title)
			out, err := cmd.CombinedOutput()
			return mergeResultMsg{branch: branch, output: string(out), err: err}
		}
	case ui.ReviewEdit:
		selected := m.list.GetSelectedInstance()
		if selected != nil {
			m.showDispatchWorkflow(selected.Title)
		}
		return m, nil
	case ui.ReviewSkip, ui.ReviewCancel:
		m.setWorkflowSessions()
		return m, nil
	default:
		return m, nil
	}
}

func (m *home) handleHistoryWorkflowKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.historyPanel == nil {
		m.setWorkflowSessions()
		return m, nil
	}

	switch msg.String() {
	case "/":
		m.showDispatchWorkflow("")
		return m, nil
	}

	if m.historyPanel.HandleKeyPress(msg) == ui.HistoryPanelClose {
		m.setWorkflowSessions()
	}
	return m, nil
}

func (m *home) workflowNavItems() []ui.WorkflowNavItem {
	selected := m.currentWorkflow()
	items := make([]ui.WorkflowNavItem, 0, len(workflows))
	for _, workflow := range workflows {
		items = append(items, ui.WorkflowNavItem{
			ID:       string(workflow.ID),
			Label:    workflow.Label,
			Hint:     workflow.Hint,
			Selected: workflow.ID == selected,
		})
	}
	return items
}
