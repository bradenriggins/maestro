package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"maestro/pkg/accounts"
	"maestro/session"
	"maestro/ui/overlay"
)

func TestUISnapshots(t *testing.T) {
	testCases := []struct {
		name   string
		render func(t *testing.T) string
	}{
		{
			name: "empty_state",
			render: func(t *testing.T) string {
				return snapshotRender(newAutomationHome(t))
			},
		},
		{
			name: "help_overlay",
			render: func(t *testing.T) string {
				h := newAutomationHome(t)
				pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
				return snapshotRender(h)
			},
		},
		{
			name: "prompt_overlay",
			render: func(t *testing.T) string {
				h := newAutomationHome(t)
				pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
				for _, r := range "snapshot-flow" {
					pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				}
				pressKey(t, h, tea.KeyMsg{Type: tea.KeyEnter})
				return snapshotRender(h)
			},
		},
		{
			name: "quick_dispatch_empty",
			render: func(t *testing.T) string {
				h := newAutomationHome(t)
				h.conductorConfig = &accounts.ConductorConfig{}
				pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
				pressKey(t, h, tea.KeyMsg{Type: tea.KeyEnter})
				return snapshotRender(h)
			},
		},
		{
			name: "quick_dispatch_with_worker",
			render: func(t *testing.T) string {
				h := newAutomationHome(t)
				h.conductorConfig = &accounts.ConductorConfig{}
				addAutomationInstance(t, h, automationInstanceOpts{title: "worker-1", role: string(accounts.RoleWorker), account: "acct-1"})
				pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
				for _, r := range "run audit" {
					if r == ' ' {
						pressKey(t, h, tea.KeyMsg{Type: tea.KeySpace})
						continue
					}
					pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				}
				return snapshotRender(h)
			},
		},
		{
			name: "review_overlay",
			render: func(t *testing.T) string {
				h := newAutomationHome(t)
				h.reviewOverlay = overlay.NewReviewOverlay("worker-1", "feat/ui-audit", "main")
				h.reviewOverlay.SetSize(140, 40)
				h.state = stateReview
				return snapshotRender(h)
			},
		},
		{
			name: "log_viewer",
			render: func(t *testing.T) string {
				h := newAutomationHome(t)
				pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
				return snapshotRender(h)
			},
		},
		{
			name: "orchestration_overlay",
			render: func(t *testing.T) string {
				h := newAutomationHome(t)
				pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
				return snapshotRender(h)
			},
		},
		{
			name: "diff_tab_no_changes",
			render: func(t *testing.T) string {
				h := newAutomationHome(t)
				addAutomationInstance(t, h, automationInstanceOpts{title: "worker-1", account: "acct-1"})
				pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
				return snapshotRender(h)
			},
		},
		{
			name: "paused_terminal_tab",
			render: func(t *testing.T) string {
				h := newAutomationHome(t)
				inst := addAutomationInstance(t, h, automationInstanceOpts{title: "worker-1", status: session.Paused, branch: "feat/pause"})
				h.tabbedWindow.SetInstance(inst)
				h.tabbedWindow.SetActiveTab(2)
				h.menu.SetActiveTab(2)
				require.NoError(t, h.tabbedWindow.UpdateTerminal(inst))
				return snapshotRender(h)
			},
		},
		{
			name: "status_banners_and_error",
			render: func(t *testing.T) string {
				h := newAutomationHome(t)
				addAutomationInstance(t, h, automationInstanceOpts{title: "worker-1", account: "acct-1"})
				h.setupNeeded = true
				h.conflictBanner = "⚠ branch drift detected"
				h.wakeBanner = "System resumed after sleep — reconciling state. Check workers with 'o'."
				h.errBox.SetMessage("dispatch failed")
				return snapshotRender(h)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.render(t)
			assertSnapshot(t, tc.name, got)
		})
	}
}

func snapshotRender(h *home) string {
	return normalizeSnapshotText(renderSansANSI(h.View()))
}

func normalizeSnapshotText(in string) string {
	in = strings.ReplaceAll(in, "\r", "")
	if !strings.HasSuffix(in, "\n") {
		in += "\n"
	}
	return in
}

func assertSnapshot(t *testing.T, name, got string) {
	t.Helper()
	root := os.Getenv("MAESTRO_TEST_SOURCE_ROOT")
	if root == "" {
		_, thisFile, _, ok := runtime.Caller(0)
		require.True(t, ok)
		root = filepath.Dir(thisFile)
	}
	path := filepath.Join(root, "testdata", "ui_snapshots", name+".snap")
	if os.Getenv("UPDATE_UI_SNAPSHOTS") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}
