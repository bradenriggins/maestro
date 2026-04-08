package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"maestro/config"
	"maestro/pkg/accounts"
	"maestro/session"
	"maestro/ui/overlay"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func TestUIAuditEmptyStateHasClearNextSteps(t *testing.T) {
	h := newAutomationHome(t)

	view := renderPlain(h)
	require.Contains(t, view, "No instances yet")
	require.Contains(t, view, "Press 'n' to create your first instance")
	require.Contains(t, view, "Press '?' for help")
	require.Contains(t, view, "Press 'q' to quit")
}

func TestUIAuditHelpOverlayHasDismissHint(t *testing.T) {
	h := newAutomationHome(t)

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	view := renderPlain(h)
	require.Equal(t, stateHelp, h.state)
	require.Contains(t, view, "Create a new session")
	require.Contains(t, view, "Orchestration overlay")
	require.Contains(t, view, "[Any key] close")

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	require.Equal(t, stateDefault, h.state)
}

func TestUIAuditPromptOverlayIsAutomatableAndCancelable(t *testing.T) {
	h := newAutomationHome(t)

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	require.Equal(t, stateNew, h.state)
	require.True(t, h.promptAfterName)

	for _, r := range "ux-pass" {
		pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEnter})

	require.Equal(t, statePrompt, h.state)
	view := renderPlain(h)
	require.Contains(t, view, "Enter prompt")
	require.Contains(t, view, "Branch")
	require.Contains(t, view, "[Esc] cancel")
	require.Contains(t, view, "[Tab] next")

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEsc})
	require.Equal(t, stateDefault, h.state)
	require.Equal(t, 0, h.list.NumInstances())
}

func TestUIAuditQuickDispatchNoWorkersShowsActionableRecovery(t *testing.T) {
	h := newAutomationHome(t)
	h.conductorConfig = &accounts.ConductorConfig{}

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	require.Equal(t, stateQuickDispatch, h.state)

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEnter})
	view := ansiRegex.ReplaceAllString(h.quickDispatchOverlay.Render(), "")
	require.Contains(t, view, "Quick Dispatch")
	require.Contains(t, view, "No workers available. Press Esc, then 'n' to create an instance")
	require.Contains(t, view, "or 'o' for")

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEsc})
	require.Equal(t, stateDefault, h.state)
	require.Nil(t, h.quickDispatchOverlay)
}

func TestUIAuditQuickDispatchListsWorkerAndAcceptsTaskInput(t *testing.T) {
	h := newAutomationHome(t)
	h.conductorConfig = &accounts.ConductorConfig{}
	addAutomationInstance(t, h, automationInstanceOpts{title: "worker-1", role: string(accounts.RoleWorker), account: "acct-1"})

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	require.Equal(t, stateQuickDispatch, h.state)

	for _, r := range "check linting" {
		if r == ' ' {
			pressKey(t, h, tea.KeyMsg{Type: tea.KeySpace})
			continue
		}
		pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	view := renderPlain(h)
	require.Contains(t, view, "worker-1")
	require.Contains(t, view, "check linting")
	require.Contains(t, view, "[Enter] dispatch  [Esc] cancel")
}

func TestUIAuditQuickDispatchFitsSmallViewportWithLongNames(t *testing.T) {
	h := newAutomationHome(t)
	h.conductorConfig = &accounts.ConductorConfig{}
	addAutomationInstance(t, h, automationInstanceOpts{
		title:   "worker-with-a-very-long-name-for-overflow-checking",
		role:    string(accounts.RoleWorker),
		account: "account-with-a-very-long-name",
	})

	sendWindowSize(t, h, 80, 20)
	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "dispatch a very long task description for viewport fit coverage" {
		if r == ' ' {
			pressKey(t, h, tea.KeyMsg{Type: tea.KeySpace})
			continue
		}
		pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	require.Equal(t, stateQuickDispatch, h.state)
	assertRenderFitsViewport(t, h.View(), 80, 20)
	require.Contains(t, renderPlain(h), "Quick Dispatch")
}

func TestUIAuditLogViewerToggleIsAutomatable(t *testing.T) {
	h := newAutomationHome(t)

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, stateLogViewer, h.state)
	view := renderPlain(h)
	require.Contains(t, view, "Maestro Logs")
	require.Contains(t, view, "[l/Esc] close")

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEsc})
	require.Equal(t, stateDefault, h.state)
}

func TestUIAuditOrchestrationToggleIsAutomatable(t *testing.T) {
	h := newAutomationHome(t)

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	require.Equal(t, stateOrchestration, h.state)

	view := renderPlain(h)
	require.Contains(t, view, "Orchestration")

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyEsc})
	require.Equal(t, stateDefault, h.state)
}

func TestUIAuditReviewEditTransitionsIntoQuickDispatch(t *testing.T) {
	h := newAutomationHome(t)
	h.conductorConfig = &accounts.ConductorConfig{}
	addAutomationInstance(t, h, automationInstanceOpts{title: "worker-1", branch: "feat/ui-audit", role: string(accounts.RoleWorker), account: "acct-1"})

	h.reviewOverlay = overlay.NewReviewOverlay("worker-1", "feat/ui-audit", "main")
	h.reviewOverlay.SetSize(140, 40)
	h.state = stateReview

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	require.Equal(t, stateQuickDispatch, h.state)
	require.NotNil(t, h.quickDispatchOverlay)

	view := renderPlain(h)
	require.Contains(t, view, "Quick Dispatch")
	require.Contains(t, view, "worker-1")
}

func TestUIAuditNilHelpOverlayFallsBackSafely(t *testing.T) {
	h := newAutomationHome(t)
	h.state = stateHelp
	h.textOverlay = nil

	view := renderPlain(h)
	require.Contains(t, view, "n new")
	require.Contains(t, view, "q quit")
}

func TestUIAuditDiffTabShowsNoChangesFallback(t *testing.T) {
	h := newAutomationHome(t)
	addAutomationInstance(t, h, automationInstanceOpts{title: "worker-1", account: "acct-1"})

	pressKey(t, h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	view := renderPlain(h)

	require.Contains(t, view, "Diff")
	require.Contains(t, view, "No changes")
}

func TestUIAuditPausedTerminalShowsRecoveryGuidance(t *testing.T) {
	h := newAutomationHome(t)
	inst := addAutomationInstance(t, h, automationInstanceOpts{title: "worker-1", status: session.Paused, branch: "feat/pause"})
	h.tabbedWindow.SetInstance(inst)
	h.tabbedWindow.SetActiveTab(2)
	h.menu.SetActiveTab(2)
	require.NoError(t, h.tabbedWindow.UpdateTerminal(inst))

	view := renderPlain(h)
	require.Contains(t, view, "Terminal")
	require.Contains(t, view, "Session is paused. Resume to use terminal.")
}

func TestUIAuditStatusBannersAndErrorsAreVisible(t *testing.T) {
	h := newAutomationHome(t)
	addAutomationInstance(t, h, automationInstanceOpts{title: "worker-1", account: "acct-1"})
	h.setupNeeded = true
	h.conflictBanner = "⚠ branch drift detected"
	h.wakeBanner = "System resumed after sleep — reconciling state. Check workers with 'o'."
	h.errBox.SetMessage("dispatch failed")

	view := renderPlain(h)
	require.Contains(t, view, "Run `maestro setup` to enable it")
	require.Contains(t, view, "branch drift detected")
	require.Contains(t, view, "System resumed after sleep")
	require.Contains(t, view, "dispatch failed")
}

type automationInstanceOpts struct {
	title   string
	branch  string
	status  session.Status
	account string
	role    string
}

func newAutomationHome(t *testing.T) *home {
	t.Helper()
	sourceRoot, err := os.Getwd()
	require.NoError(t, err)
	t.Setenv("MAESTRO_TEST_SOURCE_ROOT", sourceRoot)

	repo := initAutomationRepo(t)
	t.Chdir(repo)
	t.Setenv("HOME", t.TempDir())

	h, err := newHome(context.Background(), "claude", false, true, true)
	require.NoError(t, err)
	h.appConfig = &config.Config{DefaultProgram: "claude"}
	h.conductorConfig = nil
	h.setupNeeded = false
	h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 140, Height: 40})
	return h
}

func initAutomationRepo(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	runTestGit(t, repo, "init", "-b", "main")

	readmePath := filepath.Join(repo, "README.md")
	require.NoError(t, os.WriteFile(readmePath, []byte("automation test repo\n"), 0o644))
	runTestGit(t, repo, "add", "README.md")
	runTestGit(t, repo,
		"-c", "user.name=Automation Tests",
		"-c", "user.email=automation-tests@example.com",
		"commit", "-m", "initial commit",
	)

	return repo
}

func runTestGit(t *testing.T, repo string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s failed: %s", strings.Join(args, " "), string(output))
}

func addAutomationInstance(t *testing.T, h *home, opts automationInstanceOpts) *session.Instance {
	t.Helper()

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   opts.title,
		Path:    t.TempDir(),
		Program: "claude",
		Account: opts.account,
		Role:    opts.role,
	})
	require.NoError(t, err)
	inst.Branch = opts.branch
	if opts.status != 0 {
		inst.Status = opts.status
	}
	h.list.AddInstance(inst)()
	h.list.SetSelectedInstance(h.list.NumInstances() - 1)
	return inst
}

func renderPlain(h *home) string {
	return renderSansANSI(h.View())
}

func renderSansANSI(s string) string {
	out := ansiRegex.ReplaceAllString(s, "")
	return strings.ReplaceAll(out, "\r", "")
}
