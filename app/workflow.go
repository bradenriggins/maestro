package app

import "maestro/ui"

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
	default:
		return workflowSessions
	}
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
