package app

import (
	"context"
	"maestro/config"
	"maestro/session"
	"maestro/ui"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func newWorkflowShellTestHome(t *testing.T) *home {
	t.Helper()

	h := newAutomationHomeForTest(t)

	instance, err := session.NewInstance(session.InstanceOptions{
		Title:   "shell-test",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	h.list.AddInstance(instance)()
	h.list.SetSelectedInstance(0)
	_ = h.instanceChanged()

	h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 100, Height: 28})
	return h
}

func newAutomationHomeForTest(t *testing.T) *home {
	t.Helper()

	spin := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	list := ui.NewList(&spin, false)

	return &home{
		ctx:          context.Background(),
		state:        stateDefault,
		appConfig:    config.DefaultConfig(),
		list:         list,
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewTerminalPane()),
		errBox:       ui.NewErrBox(),
		statusBar:    ui.NewStatusBar(),
		workflowNav:  ui.NewWorkflowNav(),
	}
}

func TestWorkflowShellShowsPrimaryWorkflowsAndFitsViewport(t *testing.T) {
	h := newWorkflowShellTestHome(t)

	rendered := h.View()

	require.Contains(t, rendered, "Sessions")
	require.Contains(t, rendered, "Dispatch")
	require.Contains(t, rendered, "Review")
	require.Contains(t, rendered, "History")
	require.Contains(t, rendered, "System")
	assertRenderedViewFitsViewport(t, rendered, 100, 28)
}

func TestActionBarShowsPrimaryActionsForSessionsWorkflow(t *testing.T) {
	h := newWorkflowShellTestHome(t)

	rendered := h.View()

	require.Contains(t, rendered, "Enter open")
	require.Contains(t, rendered, "/ dispatch")
	require.Contains(t, rendered, "? system")
	require.NotContains(t, rendered, "N new with prompt")
	require.NotContains(t, rendered, "q quit")
	require.False(t, strings.Contains(rendered, "↵ attach"), "legacy menu hint strip should not be rendered")
}

func TestCurrentWorkflowDefaultsToSessions(t *testing.T) {
	h := newAutomationHomeForTest(t)

	require.Equal(t, workflowSessions, h.currentWorkflow())
}

func TestViewDoesNotMutateWorkflowNavState(t *testing.T) {
	h := newWorkflowShellTestHome(t)
	h.workflowNav.SetItems([]ui.WorkflowNavItem{{
		ID:       "sentinel",
		Label:    "Sentinel",
		Hint:     "render state should stay untouched",
		Selected: true,
	}})
	h.workflowNav.SetSize(12, 4)
	initialRender := h.workflowNav.Render()

	_ = h.View()

	require.Equal(t, initialRender, h.workflowNav.Render())
}
