package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"maestro/keys"
	"maestro/log"
	"maestro/pkg/accounts"
	"maestro/pkg/orchestration"
	"maestro/pkg/programs"
	"maestro/session"
	"maestro/session/git"
	"maestro/ui"
	"maestro/ui/overlay"
)

func (m *home) handleMenuHighlighting(msg tea.KeyMsg) (cmd tea.Cmd, returnEarly bool) {
	// Handle footer action highlighting when you press a button. We intercept it here
	// and immediately return to update the UI while re-sending the keypress. Then, on
	// the next call to this, we actually handle the keypress.
	if m.keySent {
		m.keySent = false
		return nil, false
	}
	if m.state == statePrompt || m.state == stateHelp || m.state == stateConfirm ||
		m.state == stateOrchestration || m.state == stateQuickDispatch || m.state == stateLogViewer ||
		m.state == stateReview {
		return nil, false
	}
	if m.state == stateDefault && m.currentWorkflow() != workflowSessions {
		return nil, false
	}
	// If it's in the global keymap, we should try to highlight it.
	name, ok := keys.GlobalKeyNamesByString[msg.String()]
	if !ok {
		return nil, false
	}
	if name == keys.KeyQuickDispatch || name == keys.KeyHistory {
		return nil, false
	}
	if name == keys.KeyResume && m.conductorConfig != nil {
		selected := m.list.GetSelectedInstance()
		if selected != nil && !selected.Paused() && selected.Account != "" &&
			selected.Role == string(accounts.RoleWorker) {
			return nil, false
		}
	}

	if m.list.GetSelectedInstance() != nil && m.list.GetSelectedInstance().Paused() && name == keys.KeyEnter {
		return nil, false
	}
	if name == keys.KeyShiftDown || name == keys.KeyShiftUp {
		return nil, false
	}

	// Promote Enter to the "submit name" action while the new-instance overlay
	// is active so the footer highlights the correct action.
	if name == keys.KeyEnter && m.state == stateNew {
		name = keys.KeySubmitName
	}
	m.keySent = true
	return tea.Batch(
		func() tea.Msg { return msg },
		m.keydownCallback(name)), true
}

func (m *home) handleKeyPress(msg tea.KeyMsg) (mod tea.Model, cmd tea.Cmd) {
	// Dismiss wake-from-sleep banner on any keypress
	if m.wakeBanner != "" {
		m.wakeBanner = ""
		// Don't return — let the keypress propagate to its normal handler
	}

	cmd, returnEarly := m.handleMenuHighlighting(msg)
	if returnEarly {
		return m, cmd
	}

	if m.state == stateHelp {
		return m.handleHelpState(msg)
	}

	if m.state == stateNew {
		// Guard against state desync: if the list is empty, return to default.
		if m.list.NumInstances() == 0 {
			m.state = stateDefault
			return m, nil
		}

		// Handle quit commands first. Don't handle q because the user might want to type that.
		if msg.String() == "ctrl+c" {
			m.state = stateDefault
			m.promptAfterName = false
			m.list.Kill()
			return m, tea.Batch(
				tea.WindowSize(),
				func() tea.Msg { return menuStateMsg{state: ui.StateDefault} },
			)
		}

		instance := m.list.GetInstances()[m.list.NumInstances()-1]
		switch msg.Type {
		// Start the instance (enable previews etc) and go back to the main menu state.
		case tea.KeyEnter:
			if len(instance.Title) == 0 {
				return m, m.handleError(fmt.Errorf("title cannot be empty"))
			}

			// If promptAfterName, show prompt+branch overlay before starting
			if m.promptAfterName {
				m.promptAfterName = false
				m.state = statePrompt
				m.menu.SetState(ui.StatePrompt)
				m.textInputOverlay = m.newPromptOverlay()
				// Trigger initial branch search (no debounce, version 0)
				initialSearch := m.runBranchSearch("", m.textInputOverlay.BranchFilterVersion())
				return m, tea.Batch(tea.WindowSize(), initialSearch)
			}

			// Set Loading status and finalize into the list immediately
			instance.SetStatus(session.Loading)
			m.newInstanceFinalizer()
			m.promptAfterName = false
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)

			// Snapshot worker infos on the main loop before goroutine launch.
			workerInfos := m.currentWorkerInfos()

			// Return a tea.Cmd that runs instance.Start in the background
			startCmd := func() tea.Msg {
				err := instance.Start(true)
				if err == nil && instance.Account != "" {
					m.regenerateInstructions(instance, workerInfos)
				}
				return instanceStartedMsg{
					instance:        instance,
					err:             err,
					promptAfterName: false,
				}
			}

			return m, tea.Batch(tea.WindowSize(), m.instanceChanged(), startCmd)
		case tea.KeyRunes:
			if runewidth.StringWidth(instance.Title) >= 32 {
				return m, m.handleError(fmt.Errorf("title cannot be longer than 32 characters"))
			}
			if err := instance.SetTitle(instance.Title + string(msg.Runes)); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeyBackspace:
			runes := []rune(instance.Title)
			if len(runes) == 0 {
				return m, nil
			}
			if err := instance.SetTitle(string(runes[:len(runes)-1])); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeySpace:
			if err := instance.SetTitle(instance.Title + " "); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeyEsc:
			m.list.Kill()
			m.state = stateDefault
			instanceChangedCmd := m.instanceChanged()

			return m, tea.Batch(
				instanceChangedCmd,
				tea.WindowSize(),
				func() tea.Msg { return menuStateMsg{state: ui.StateDefault} },
			)
		default:
		}
		return m, nil
	} else if m.state == statePrompt {
		// Handle cancel via ctrl+c before delegating to the overlay
		if msg.String() == "ctrl+c" {
			return m, m.cancelPromptOverlay()
		}

		// Use the new TextInputOverlay component to handle all key events
		shouldClose, branchFilterChanged := m.textInputOverlay.HandleKeyPress(msg)

		// Check if the form was submitted or canceled
		if shouldClose {
			selected := m.list.GetSelectedInstance()
			if selected == nil {
				return m, nil
			}

			if m.textInputOverlay.IsCanceled() {
				return m, m.cancelPromptOverlay()
			}

			if m.textInputOverlay.IsSubmitted() {
				prompt := m.textInputOverlay.GetValue()
				selectedBranch := m.textInputOverlay.GetSelectedBranch()
				selectedProgram := m.textInputOverlay.GetSelectedProgram()

				if !selected.Started() {
					// Shift+N flow: instance not started yet — set branch, start, then send prompt
					if selectedBranch != "" {
						selected.SetSelectedBranch(selectedBranch)
					}
					if selectedProgram != "" {
						selected.Program = selectedProgram
					}
					selected.Prompt = prompt

					// Finalize into list and start
					selected.SetStatus(session.Loading)
					m.newInstanceFinalizer()
					m.textInputOverlay = nil
					m.state = stateDefault
					m.menu.SetState(ui.StateDefault)

					// Snapshot worker infos on the main loop before goroutine launch.
					workerInfos := m.currentWorkerInfos()

					startCmd := func() tea.Msg {
						err := selected.Start(true)
						if err == nil && selected.Account != "" {
							m.regenerateInstructions(selected, workerInfos)
						}
						return instanceStartedMsg{
							instance:        selected,
							err:             err,
							promptAfterName: false,
							selectedBranch:  selectedBranch,
						}
					}

					return m, tea.Batch(tea.WindowSize(), m.instanceChanged(), startCmd)
				}

				// Regular flow: instance already running, just send prompt via a tea.Cmd.
				inst := selected
				promptToSend := prompt
				sendCmd := func() tea.Msg {
					if err := inst.SendPrompt(promptToSend); err != nil {
						log.ErrorLog.Printf("failed to send prompt: %v", err)
					}
					return promptSentMsg{name: inst.Title}
				}
				// Close the overlay and reset state
				m.textInputOverlay = nil
				m.state = stateDefault
				m.menu.SetState(ui.StateDefault)
				return m, tea.Batch(
					tea.WindowSize(),
					func() tea.Msg {
						return helpTriggerMsg{helpType: helpStart(selected), onDismiss: nil}
					},
					sendCmd,
				)
			}

			// Close the overlay and reset state
			m.textInputOverlay = nil
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, tea.Batch(
				tea.WindowSize(),
				func() tea.Msg {
					return helpTriggerMsg{helpType: helpStart(selected), onDismiss: nil}
				},
			)
		}

		// Schedule a debounced branch search if the filter changed
		if branchFilterChanged {
			filter := m.textInputOverlay.BranchFilter()
			version := m.textInputOverlay.BranchFilterVersion()
			return m, m.scheduleBranchSearch(filter, version)
		}

		return m, nil
	}

	// Handle confirmation state
	if m.state == stateConfirm {
		if m.confirmationOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.confirmationOverlay.HandleKeyPress(msg)
		if shouldClose {
			confirmed := m.confirmationOverlay.Confirmed
			m.state = stateDefault
			m.confirmationOverlay = nil
			if confirmed && m.pendingConfirmAction != nil {
				action := m.pendingConfirmAction
				m.pendingConfirmAction = nil
				return m, action
			}
			m.pendingConfirmAction = nil
			return m, nil
		}
		return m, nil
	}

	if m.state == stateOrchestration {
		if m.orchestrationOverlay.HandleKeyPress(msg) {
			// HandleKeyPress returns true when the overlay should close (Esc, "o", or "R").
			// Consume the pending action before closing so we don't operate on a stale state.
			action := m.orchestrationOverlay.PendingAction()
			m.orchestrationOverlay.Close()
			m.state = stateDefault
			if action == "retry-all" {
				return m.handleBulkRetry()
			}
			// No action or unknown action — return to default state.
			return m, nil
		}
		// HandleKeyPress returned false — overlay stays open.
		// Check for "open-preview" action set by the Enter key handler.
		// Load the result file in a background goroutine so os.ReadFile never blocks Update().
		if action := m.orchestrationOverlay.PendingAction(); action == "open-preview" {
			orch := m.orchestrationOverlay
			taskID, resultFile := orch.SelectedTaskResultFile()
			return m, func() tea.Msg {
				var content string
				if resultFile == "" {
					content = "No result file found."
				} else {
					data, err := os.ReadFile(resultFile)
					if err != nil {
						content = "No result file found."
					} else {
						content = string(data)
					}
				}
				return orchPreviewLoadedMsg{taskID: taskID, content: content}
			}
		}
		return m, nil
	}

	if m.state == stateLogViewer {
		if m.logViewerOverlay.HandleKeyPress(msg) {
			m.logViewerOverlay.Close()
			m.state = stateDefault
		}
		return m, nil
	}

	if m.state == stateReview {
		if m.reviewOverlay == nil {
			m.state = stateDefault
			return m, nil
		}
		if m.reviewOverlay.HandleKeyPress(msg) {
			action := m.reviewOverlay.GetAction()
			m.reviewOverlay = nil
			m.state = stateDefault

			switch action {
			case overlay.ReviewApprove:
				// Merge the worker's branch as a proper tea.Cmd so result is reported on the main loop.
				selected := m.list.GetSelectedInstance()
				if selected != nil {
					branch := selected.Branch
					title := selected.Title
					mergeCmd := func() tea.Msg {
						cmd := exec.Command("git", "merge", "--no-ff", branch,
							"-m", fmt.Sprintf("maestro: merge %s", title))
						out, err := cmd.CombinedOutput()
						return mergeResultMsg{branch: branch, output: string(out), err: err}
					}
					return m, mergeCmd
				}
			case overlay.ReviewEdit:
				// Open quick-dispatch to re-dispatch to the same worker
				selected := m.list.GetSelectedInstance()
				if selected != nil {
					workers := []string{selected.Title}
					m.quickDispatchOverlay = overlay.NewQuickDispatchOverlay(workers)
					m.quickDispatchOverlay.SetWidth(m.windowWidth)
					// Populate worker model info from the orchestration registry.
					models := make(map[string]string)
					if reg, regErr := orchestration.LoadRegistry(); regErr == nil {
						for name, entry := range reg.Instances {
							if entry.Model != "" {
								models[name] = entry.Model
							}
						}
					}
					m.quickDispatchOverlay.SetWorkerModels(models)
					m.state = stateQuickDispatch
					return m, nil
				}
			case overlay.ReviewSkip:
				// Intentional no-op — return to default state
			}
			return m, nil
		}
		return m, nil
	}

	if m.state == stateQuickDispatch {
		if msg.String() == "ctrl+c" {
			m.quickDispatchOverlay = nil
			m.state = stateDefault
			return m, nil
		}
		shouldClose := m.quickDispatchOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.quickDispatchOverlay.IsSubmitted() {
				dispatchWorker := m.quickDispatchOverlay.GetWorker()
				dispatchTask := m.quickDispatchOverlay.GetTask()
				m.quickDispatchOverlay = nil
				m.state = stateDefault
				return m, func() tea.Msg {
					result, err := orchestration.RunDispatch(dispatchWorker, dispatchTask, "", nil)
					if err != nil {
						return quickDispatchResultMsg{err: err}
					}
					return quickDispatchResultMsg{taskID: result.TaskID}
				}
			}
			m.quickDispatchOverlay = nil
			m.state = stateDefault
		}
		return m, nil
	}

	if m.state == stateDefault {
		switch m.currentWorkflow() {
		case workflowDispatch:
			return m.handleDispatchWorkflowKey(msg)
		case workflowReview:
			return m.handleReviewWorkflowKey(msg)
		case workflowHistory:
			return m.handleHistoryWorkflowKey(msg)
		}
	}

	// Exit scrolling mode when ESC is pressed and preview pane is in scrolling mode
	// Check if Escape key was pressed and we're not in the diff tab (meaning we're in preview tab)
	// Always check for escape key first to ensure it doesn't get intercepted elsewhere
	if msg.Type == tea.KeyEsc {
		// If in preview tab and in scroll mode, exit scroll mode
		if m.tabbedWindow.IsInPreviewTab() && m.tabbedWindow.IsPreviewInScrollMode() {
			// Use the selected instance from the list
			selected := m.list.GetSelectedInstance()
			err := m.tabbedWindow.ResetPreviewToNormalMode(selected)
			if err != nil {
				return m, m.handleError(err)
			}
			return m, m.instanceChanged()
		}
		// If in terminal tab and in scroll mode, exit scroll mode
		if m.tabbedWindow.IsInTerminalTab() && m.tabbedWindow.IsTerminalInScrollMode() {
			m.tabbedWindow.ResetTerminalToNormalMode()
			return m, m.instanceChanged()
		}
	}

	// Handle quit commands first
	if msg.String() == "ctrl+c" || msg.String() == "q" {
		return m.handleQuit()
	}

	name, ok := keys.GlobalKeyNamesByString[msg.String()]
	if !ok {
		return m, nil
	}

	switch name {
	case keys.KeyHelp:
		return m.showHelpScreen(helpTypeGeneral{}, nil)
	case keys.KeyPrompt:
		if m.list.NumInstances() >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
		}

		// Start a background fetch so branches are up to date by the time the picker opens
		fetchCmd := func() tea.Msg {
			currentDir, _ := os.Getwd()
			git.FetchBranches(currentDir)
			return nil
		}

		promptOpts := session.InstanceOptions{
			Title:   "",
			Path:    ".",
			Program: m.program,
		}
		if m.conductorConfig != nil {
			acct := m.nextAvailableAccount(accounts.RoleWorker)
			if acct == nil {
				return m, m.handleError(fmt.Errorf("all accounts have active instances — pause or kill one first"))
			}
			promptOpts.Account = acct.Name
			promptOpts.Role = string(acct.Role)
			acctSpec, specOk := programs.Get(acct.Program)
			if !specOk {
				return m, m.handleError(fmt.Errorf("unknown program %q for account %q", acct.Program, acct.Name))
			}
			promptOpts.Env = map[string]string{acctSpec.ConfigDirEnvVar: acct.ConfigDir}
		}

		instance, err := session.NewInstance(promptOpts)
		if err != nil {
			return m, m.handleError(err)
		}

		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)
		m.promptAfterName = true

		return m, fetchCmd
	case keys.KeyNew:
		if m.list.NumInstances() >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
		}

		opts := session.InstanceOptions{
			Title:   "",
			Path:    ".",
			Program: m.program,
		}
		if m.conductorConfig != nil {
			acct := m.nextAvailableAccount(accounts.RoleWorker)
			if acct == nil {
				return m, m.handleError(fmt.Errorf("all accounts have active instances — pause or kill one first"))
			}
			opts.Account = acct.Name
			opts.Role = string(acct.Role)
			acctSpec, specOk := programs.Get(acct.Program)
			if !specOk {
				return m, m.handleError(fmt.Errorf("unknown program %q for account %q", acct.Program, acct.Name))
			}
			opts.Env = map[string]string{acctSpec.ConfigDirEnvVar: acct.ConfigDir}
		}

		instance, err := session.NewInstance(opts)
		if err != nil {
			return m, m.handleError(err)
		}

		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)

		return m, nil
	case keys.KeyUp:
		m.list.Up()
		return m, m.instanceChanged()
	case keys.KeyDown:
		m.list.Down()
		return m, m.instanceChanged()
	case keys.KeyShiftUp:
		m.tabbedWindow.ScrollUp()
		return m, m.instanceChanged()
	case keys.KeyShiftDown:
		m.tabbedWindow.ScrollDown()
		return m, m.instanceChanged()
	case keys.KeyTab:
		m.tabbedWindow.Toggle()
		m.menu.SetActiveTab(m.tabbedWindow.GetActiveTab())
		return m, m.instanceChanged()
	case keys.KeyKill:
		selected := m.list.GetSelectedInstance()
		if selected == nil || selected.Status == session.Loading {
			return m, nil
		}

		// Capture the selected instance before launching the goroutine so the
		// closure references a stable pointer even if the list changes.
		instanceToKill := selected

		// Create the kill action as a tea.Cmd.
		// IMPORTANT: all I/O work (tmux kill + storage delete) happens here (in the goroutine).
		// m.list.RemoveByName() and m.updateRegistry() are called on the main loop
		// in the instanceKilledMsg handler below.
		killAction := func() tea.Msg {
			// Get worktree and check if branch is checked out
			worktree, err := instanceToKill.GetGitWorktree()
			if err != nil {
				return err
			}

			checkedOut, err := worktree.IsBranchCheckedOut()
			if err != nil {
				return err
			}

			if checkedOut {
				return fmt.Errorf("instance %s is currently checked out", instanceToKill.Title)
			}

			// Kill the tmux session and git worktree (blocking subprocess — safe in goroutine).
			if err := instanceToKill.Kill(); err != nil {
				log.ErrorLog.Printf("could not kill instance %q: %v", instanceToKill.Title, err)
			}

			// Delete from storage (I/O only — safe in goroutine)
			if err := m.storage.DeleteInstance(instanceToKill.Title); err != nil {
				return err
			}

			return instanceKilledMsg{title: instanceToKill.Title}
		}

		// Show confirmation modal
		// Capture branch before the closure runs (selection could change).
		branch := selected.Branch
		message := fmt.Sprintf("Kill instance '%s'?\n\nBranch '%s' will be preserved.\nWorktree files will be removed.", selected.Title, branch)
		return m, m.confirmAction(message, killAction)
	case keys.KeySubmit:
		selected := m.list.GetSelectedInstance()
		if selected == nil || selected.Status == session.Loading {
			return m, nil
		}

		// Create the push action as a tea.Cmd
		pushAction := func() tea.Msg {
			// Default commit message with timestamp
			commitMsg := fmt.Sprintf("[maestro] update from '%s' on %s", selected.Title, time.Now().Format(time.RFC822))
			worktree, err := selected.GetGitWorktree()
			if err != nil {
				return err
			}
			if err = worktree.PushChanges(commitMsg, true); err != nil {
				return err
			}
			return nil
		}

		// Show confirmation modal
		message := fmt.Sprintf("[!] Push changes from session '%s'?", selected.Title)
		return m, m.confirmAction(message, pushAction)
	case keys.KeyCheckout:
		selected := m.list.GetSelectedInstance()
		if selected == nil || selected.Status == session.Loading {
			return m, nil
		}

		// Guard: do nothing if the instance is already paused.
		if selected.Paused() {
			return m, nil
		}

		// Show help screen before pausing. The onDismiss callback returns a tea.Cmd
		// so Pause() (which runs git + tmux subprocesses) runs in a goroutine.
		instToPause := selected
		return m.showHelpScreen(helpTypeInstanceCheckout{instance: instToPause}, func() tea.Cmd {
			return func() tea.Msg {
				err := instToPause.Pause()
				return pauseCompleteMsg{name: instToPause.Title, err: err}
			}
		})
	case keys.KeyResume:
		selected := m.list.GetSelectedInstance()
		if selected == nil || selected.Status == session.Loading {
			return m, nil
		}

		// If paused, resume via a tea.Cmd so blocking subprocess calls don't block Update().
		if selected.Paused() {
			instToResume := selected
			// Snapshot worker infos on the main loop before goroutine launch.
			workerInfos := m.currentWorkerInfos()
			return m, func() tea.Msg {
				if err := instToResume.Resume(); err != nil {
					return resumeCompleteMsg{name: instToResume.Title, err: err}
				}
				// Regenerate CLAUDE.md on resume (reflects current worker list).
				if instToResume.Account != "" {
					m.regenerateInstructions(instToResume, workerInfos)
				}
				return resumeCompleteMsg{name: instToResume.Title}
			}
		}

		// If running a worker in multi-account mode, open the review overlay.
		if m.conductorConfig != nil && selected.Account != "" && selected.Role == string(accounts.RoleWorker) {
			m.showReviewWorkflow(selected.Title, selected.Branch, "main")
			return m, nil
		}

		// Instance is running and no other action applies — show a brief hint.
		return m, m.handleError(fmt.Errorf("r: instance is running (use p to pause, x to kill)"))
	case keys.KeyOrchestration:
		if m.orchestrationOverlay.IsVisible() {
			m.orchestrationOverlay.Close()
			m.state = stateDefault
		} else {
			m.orchestrationOverlay.Toggle()
			m.state = stateOrchestration
		}
		return m, nil
	case keys.KeyQuickDispatch:
		if m.conductorConfig == nil {
			return m, nil
		}
		m.showDispatchWorkflow("")
		return m, nil
	case keys.KeyLogViewer:
		if m.logViewerOverlay.IsVisible() {
			m.logViewerOverlay.Close()
			m.state = stateDefault
		} else {
			m.logViewerOverlay.Toggle()
			m.state = stateLogViewer
		}
		return m, nil
	case keys.KeyDiff:
		// Toggle to diff tab in the tabbed window
		if m.list.GetSelectedInstance() != nil {
			m.tabbedWindow.SetActiveTab(1) // Diff tab
			m.menu.SetActiveTab(1)
		}
		return m, m.instanceChanged()
	case keys.KeyPreviewToggle:
		// Toggle between preview tabs
		if m.list.GetSelectedInstance() != nil {
			m.tabbedWindow.Toggle()
			m.menu.SetActiveTab(m.tabbedWindow.GetActiveTab())
		}
		return m, m.instanceChanged()
	case keys.KeyHistory:
		if m.conductorConfig != nil {
			return m, m.showHistoryWorkflow()
		}
		return m, nil
	case keys.KeyEnter:
		if m.list.NumInstances() == 0 {
			return m, nil
		}
		selected := m.list.GetSelectedInstance()
		if selected == nil || selected.Paused() || selected.Status == session.Loading || !selected.TmuxAlive() {
			return m, nil
		}
		// Terminal tab: attach to terminal session
		if m.tabbedWindow.IsInTerminalTab() {
			return m.showHelpScreen(helpTypeInstanceAttach{}, func() tea.Cmd {
				ch, err := m.tabbedWindow.AttachTerminal()
				if err != nil {
					return m.handleError(err)
				}
				return func() tea.Msg {
					<-ch // runs in a goroutine, not on the main loop
					return instanceDetachedMsg{}
				}
			})
		}
		// Show help screen before attaching
		return m.showHelpScreen(helpTypeInstanceAttach{}, func() tea.Cmd {
			ch, err := m.list.Attach()
			if err != nil {
				return m.handleError(err)
			}
			return func() tea.Msg {
				<-ch // runs in a goroutine, not on the main loop
				return instanceDetachedMsg{}
			}
		})
	default:
		return m, nil
	}
}

// regenerateInstructions builds a CLAUDEMDContext for the given instance and
// calls accounts.GenerateInstructions. workerInfos should be snapshotted on
// the main loop (via currentWorkerInfos) before being passed to goroutine
// closures that call this method.
func (m *home) regenerateInstructions(inst *session.Instance, workerInfos []accounts.WorkerInfo) error {
	conductorDir, err := accounts.ConductorDir()
	if err != nil {
		return err
	}
	worktreePath := inst.GetWorktreePath()
	if worktreePath == "" || conductorDir == "" {
		return nil
	}

	cmdCtx := accounts.CLAUDEMDContext{
		InstanceName:   inst.Title,
		AccountName:    inst.Account,
		Role:           inst.Role,
		ConductorDir:   conductorDir,
		StatusFilePath: filepath.Join(conductorDir, "status", inst.Title+".json"),
	}
	if inst.Role == string(accounts.RoleOrchestrator) {
		cmdCtx.WorkerInstances = workerInfos
	}
	return accounts.GenerateInstructions(worktreePath, accounts.InstructionsContext{
		CLAUDEMDContext: cmdCtx,
		Program:         inst.Program,
	})
}
