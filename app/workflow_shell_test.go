package app

import (
	"context"
	"maestro/config"
	"maestro/session"
	"maestro/ui"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func newWorkflowShellTestHome(t *testing.T) *home {
	t.Helper()

	spin := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	list := ui.NewList(&spin, false)
	instance, err := session.NewInstance(session.InstanceOptions{
		Title:   "shell-test",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	list.AddInstance(instance)()

	h := &home{
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

	h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 100, Height: 28})
	return h
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

func TestCurrentWorkflowDefaultsToSessions(t *testing.T) {
	h := &home{state: stateDefault}

	require.Equal(t, workflowSessions, h.currentWorkflow())
}
