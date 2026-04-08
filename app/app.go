package app

import (
	"context"
	"fmt"
	"maestro/config"
	"maestro/keys"
	"maestro/log"
	"maestro/pkg/accounts"
	"maestro/pkg/notify"
	"maestro/pkg/orchestration"
	"maestro/pkg/programs"
	"maestro/session"
	"maestro/session/git"
	"maestro/session/tmux"
	"maestro/ui"
	"maestro/ui/overlay"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const GlobalInstanceLimit = 10

// Run is the main entrypoint into the application.
func Run(ctx context.Context, program string, autoYes bool, fresh bool, noSafetyNet bool) error {
	model, err := newHome(ctx, program, autoYes, fresh, noSafetyNet)
	if err != nil {
		return err
	}
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(), // Mouse scroll
	)
	_, err = p.Run()
	return err
}

type state int

const (
	stateDefault state = iota
	// stateNew is the state when the user is creating a new instance.
	stateNew
	// statePrompt is the state when the user is entering a prompt.
	statePrompt
	// stateHelp is the state when a help screen is displayed.
	stateHelp
	// stateConfirm is the state when a confirmation modal is displayed.
	stateConfirm
	// stateOrchestration is the state when the orchestration overlay is displayed.
	stateOrchestration
	// stateQuickDispatch is the state when the quick dispatch overlay is displayed.
	stateQuickDispatch
	// stateLogViewer is the state when the log viewer overlay is displayed.
	stateLogViewer
	// stateReview is the state when the review overlay is displayed.
	stateReview
)

type home struct {
	ctx context.Context

	// -- Storage and Configuration --

	program string
	autoYes bool

	// storage is the interface for saving/loading data to/from the app's state
	storage *session.Storage
	// appConfig stores persistent application configuration
	appConfig *config.Config
	// appState stores persistent application state like seen help screens
	appState config.AppState

	// conductorConfig holds the multi-account configuration.
	// Nil if no account config exists (falls back to single-account behavior).
	conductorConfig *accounts.ConductorConfig

	// -- State --

	// state is the current discrete state of the application
	state state
	// newInstanceFinalizer is called when the state is stateNew and then you press enter.
	// It registers the new instance in the list after the instance has been started.
	newInstanceFinalizer func()

	// promptAfterName tracks if we should enter prompt mode after naming
	promptAfterName bool

	// keySent is used to manage underlining menu items
	keySent bool

	// -- UI Components --

	// list displays the list of instances
	list *ui.List
	// menu displays the bottom menu
	menu *ui.Menu
	// tabbedWindow displays the tabbed window with preview and diff panes
	tabbedWindow *ui.TabbedWindow
	// errBox displays error messages
	errBox *ui.ErrBox
	// global spinner instance. we plumb this down to where it's needed
	spinner spinner.Model
	// textInputOverlay handles text input with state
	textInputOverlay *overlay.TextInputOverlay
	// textOverlay displays text information
	textOverlay *overlay.TextOverlay
	// confirmationOverlay displays confirmation modals
	confirmationOverlay *overlay.ConfirmationOverlay

	// Maestro overlays
	orchestrationOverlay *overlay.OrchestrationOverlay
	quickDispatchOverlay *overlay.QuickDispatchOverlay
	logViewerOverlay     *overlay.LogViewerOverlay
	reviewOverlay        *overlay.ReviewOverlay
	statusBar            *ui.StatusBar

	// windowWidth stores the last known terminal width for overlay sizing
	windowWidth int
	// windowHeight stores the last known terminal height for layout calculations
	windowHeight int

	// Session persistence and git safety net
	sessionStartedAt string
	stashRef         string
	startTag         string
	conflictBanner   string

	// lastReconcileTime tracks the wall-clock time of the last reconciliation
	// tick so we can detect wake-from-sleep gaps.
	lastReconcileTime time.Time

	// lastOutputChange tracks the last time each instance's tmux output changed (for stall detection)
	lastOutputChange map[string]time.Time

	// Wake-from-sleep banner
	wakeBanner    string // non-empty = show banner
	wakeBannerSeq int    // prevents stale dismiss messages

	// Stall detection
	stalledInstances  map[string]time.Time // key=instance name, value=when stall first detected
	stallCheckCounter int                  // only check every 10th metadata tick (~5s)

	// registryMutationSeq is incremented every time updateRegistry() writes registry.json.
	// Used to detect stale reconcile snapshots.
	registryMutationSeq int

	// setupNeeded is true when no account config was found on startup.
	setupNeeded bool

	// pendingConfirmAction is the tea.Cmd to run when the user confirms a modal.
	// Set by confirmAction and consumed by the stateConfirm handler.
	pendingConfirmAction tea.Cmd
}

func newHome(ctx context.Context, program string, autoYes bool, fresh bool, noSafetyNet bool) (*home, error) {
	// Load application config
	appConfig := config.LoadConfig()

	// Load application state
	appState := config.LoadState()

	// Load account config (optional — nil means single-account mode)
	conductorCfg, err := accounts.LoadConductorConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load account config: %w", err)
	}
	// setupNeeded is true on first run before 'maestro setup' has been run.
	setupNeeded := conductorCfg == nil

	// Initialize storage
	storage, err := session.NewStorage(appState)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	orchOverlay := overlay.NewOrchestrationOverlay()
	logOverlay := overlay.NewLogViewerOverlay()
	statusBar := ui.NewStatusBar()

	h := &home{
		ctx:                  ctx,
		spinner:              spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		menu:                 ui.NewMenu(),
		tabbedWindow:         ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewTerminalPane()),
		errBox:               ui.NewErrBox(),
		storage:              storage,
		appConfig:            appConfig,
		program:              program,
		autoYes:              autoYes,
		state:                stateDefault,
		appState:             appState,
		conductorConfig:      conductorCfg,
		orchestrationOverlay: orchOverlay,
		logViewerOverlay:     logOverlay,
		statusBar:            statusBar,
		lastReconcileTime:    time.Now(),
		lastOutputChange:     make(map[string]time.Time),
		stalledInstances:     make(map[string]time.Time),
		setupNeeded:          setupNeeded,
	}
	h.list = ui.NewList(&h.spinner, autoYes)

	// Load saved instances
	instances, err := storage.LoadInstances()
	if err != nil {
		return nil, fmt.Errorf("failed to load instances: %w", err)
	}

	// Add loaded instances to the list
	for _, instance := range instances {
		// Set AutoYes before finalizing so the instance is fully configured
		// before it is registered in the list.
		if autoYes {
			instance.AutoYes = true
		}
		// Call the finalizer immediately.
		h.list.AddInstance(instance)()
	}

	// Git safety net
	if conductorCfg != nil && !noSafetyNet {
		repoDir, _ := filepath.Abs(".")
		safetyResult, err := orchestration.SetupGitSafetyNet(repoDir)
		if err != nil {
			log.ErrorLog.Printf("git safety net: %v", err)
		} else {
			h.stashRef = safetyResult.StashRef
			h.startTag = safetyResult.StartTag
			h.sessionStartedAt = orchestration.NowISO()
		}
	}

	return h, nil
}

// updateHandleWindowSizeEvent sets the sizes of the components.
// The components will try to render inside their bounds.
func (m *home) updateHandleWindowSizeEvent(msg tea.WindowSizeMsg) {
	m.windowWidth = msg.Width
	m.windowHeight = msg.Height

	layout := m.currentLayout()
	listWidth, tabsWidth := splitContentWidth(layout.content.W)

	m.tabbedWindow.SetSize(tabsWidth, layout.content.H)
	m.list.SetSize(listWidth, layout.content.H)

	m.sizeOverlays()

	previewWidth, previewHeight := m.tabbedWindow.GetPreviewSize()
	if err := m.list.SetSessionPreviewSize(previewWidth, previewHeight); err != nil {
		log.ErrorLog.Print(err)
	}
	m.menu.SetSize(layout.menu.W, layout.menu.H)
	m.errBox.SetSize(layout.err.W, layout.err.H)

	if m.orchestrationOverlay != nil {
		m.orchestrationOverlay.SetSize(msg.Width, msg.Height)
	}
	if m.logViewerOverlay != nil {
		m.logViewerOverlay.SetSize(msg.Width, msg.Height)
	}
	if m.statusBar != nil {
		m.statusBar.SetWidth(layout.status.W)
	}
}

func (m *home) Init() tea.Cmd {
	// Upon starting, we want to start the spinner. Whenever we get a spinner.TickMsg, we
	// update the spinner, which sends a new spinner.TickMsg. I think this lasts forever lol.
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			time.Sleep(100 * time.Millisecond)
			return previewTickMsg{}
		},
		tickUpdateMetadataCmd(m.snapshotActiveInstances()),
		reconcileTickCmd(),
		sessionSaveTickCmd(),
		conflictCheckTickCmd(),
		statusBarTickCmd(),
	)
}

func (m *home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Guard against state desync: if an overlay was supposed to be shown but its
	// backing field is nil, reset to stateDefault so View() renders correctly.
	// This is the right place for state mutation — never inside View().
	switch m.state {
	case statePrompt:
		if m.textInputOverlay == nil {
			m.state = stateDefault
		}
	case stateHelp:
		if m.textOverlay == nil {
			m.state = stateDefault
		}
	case stateConfirm:
		if m.confirmationOverlay == nil {
			m.state = stateDefault
		}
	case stateReview:
		if m.reviewOverlay == nil {
			m.state = stateDefault
		}
	case stateQuickDispatch:
		if m.quickDispatchOverlay == nil {
			m.state = stateDefault
		}
	}

	switch msg := msg.(type) {
	case hideErrMsg:
		m.errBox.Clear()
	case wakeBannerDismissMsg:
		if msg.seq == m.wakeBannerSeq {
			m.wakeBanner = ""
		}
		return m, nil
	case previewTickMsg:
		cmd := m.instanceChanged()
		return m, tea.Batch(
			cmd,
			func() tea.Msg {
				time.Sleep(100 * time.Millisecond)
				return previewTickMsg{}
			},
		)
	case keyupMsg:
		m.menu.ClearKeydown()
		return m, nil
	case reconcileTickMsg:
		// If setup was missing on startup, try loading the account config on each tick.
		// This lets the banner clear automatically after the user runs `maestro setup`
		// in another terminal, without requiring a restart.
		if m.setupNeeded {
			if cfg, err := accounts.LoadConductorConfig(); err == nil && cfg != nil {
				m.setupNeeded = false
				m.conductorConfig = cfg
			}
		}
		if m.conductorConfig != nil {
			capturedSeq := m.registryMutationSeq
			conductorCfgSnapshot := m.conductorConfig
			return m, tea.Batch(
				reconcileTickCmd(),
				func() tea.Msg {
					base, err := accounts.ConductorDir()
					if err != nil {
						return reconcileDoneMsg{err: err, capturedSeq: capturedSeq}
					}
					store, err := orchestration.NewTaskStore()
					if err != nil {
						return reconcileDoneMsg{err: err, capturedSeq: capturedSeq}
					}
					regPath := filepath.Join(base, "registry.json")
					result, reg, err := orchestration.Reconcile(orchestration.RealTmuxChecker{}, regPath, store)

					// Collect usage data alongside reconciliation
					usageReport, _ := orchestration.CollectUsage(conductorCfgSnapshot)
					if usageReport != nil {
						orchestration.SaveUsageReport(usageReport)
					}

					return reconcileDoneMsg{result: result, registry: reg, err: err, capturedSeq: capturedSeq}
				},
			)
		}
		return m, reconcileTickCmd()
	case reconcileDoneMsg:
		if msg.err != nil {
			log.ErrorLog.Printf("reconciliation: %v", msg.err)
		} else if msg.result != nil {
			if len(msg.result.DeadSessions) > 0 {
				log.InfoLog.Printf("reconciliation: detected dead sessions: %v", msg.result.DeadSessions)
			}
			if len(msg.result.FailedTasks) > 0 {
				log.InfoLog.Printf("reconciliation: promoted stale tasks to failed: %v", msg.result.FailedTasks)
			}
			if len(msg.result.StatusCorrected) > 0 {
				log.InfoLog.Printf("reconciliation: corrected status files: %v", msg.result.StatusCorrected)
			}
			if len(msg.result.DAGDispatched) > 0 {
				log.InfoLog.Printf("reconciliation: DAG dispatched tasks: %v", msg.result.DAGDispatched)
				// Deliver newly dispatched DAG tasks to their workers' tmux sessions.
				if msg.registry != nil {
					store, storeErr := orchestration.NewTaskStore()
					if storeErr == nil {
						orchestration.SendDAGDispatchedTasks(msg.result.DAGDispatched, store, msg.registry)
					} else {
						log.ErrorLog.Printf("reconciliation: failed to create task store for DAG dispatch: %v", storeErr)
					}
				}
			}
			if len(msg.result.DAGBlocked) > 0 {
				log.InfoLog.Printf("reconciliation: DAG blocked tasks: %v", msg.result.DAGBlocked)
			}
			if len(msg.result.DAGUnblocked) > 0 {
				log.InfoLog.Printf("reconciliation: DAG unblocked tasks: %v", msg.result.DAGUnblocked)
			}
			// Notify on dead sessions
			for _, name := range msg.result.DeadSessions {
				notify.NotifyTaskFailed(name, "worker session died")
			}
			// Notify on failed tasks
			for _, taskID := range msg.result.FailedTasks {
				notify.NotifyTaskFailed("", fmt.Sprintf("task %s failed", taskID))
			}
		}
		// Write updated registry on the main loop (no contention with updateRegistry).
		// If the registry changed after reconciliation started, skip the stale snapshot
		// to avoid overwriting user changes.
		if msg.registry != nil && msg.capturedSeq == m.registryMutationSeq {
			base, err := accounts.ConductorDir()
			if err == nil && base != "" {
				regPath := filepath.Join(base, "registry.json")
				if err := orchestration.AtomicWriteJSON(regPath, msg.registry); err != nil {
					log.ErrorLog.Printf("reconciliation: failed to write registry: %v", err)
				}
			}
		} else if msg.registry != nil {
			log.InfoLog.Printf("reconciliation: discarding stale registry snapshot (seq %d vs current %d)",
				msg.capturedSeq, m.registryMutationSeq)
		}
		// Wake detection (safe — on main loop now)
		if time.Since(m.lastReconcileTime) > 30*time.Second && !m.lastReconcileTime.IsZero() {
			gap := time.Since(m.lastReconcileTime)
			log.InfoLog.Printf("reconciliation: detected wake from sleep (gap: %v)", gap)
			m.wakeBanner = "System resumed after sleep — reconciling state. Check workers with 'o'."
			m.wakeBannerSeq++
			capturedSeq := m.wakeBannerSeq
			m.lastReconcileTime = time.Now()
			// Return with auto-dismiss timer AND the next reconcile tick
			return m, tea.Batch(
				reconcileTickCmd(),
				tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
					return wakeBannerDismissMsg{seq: capturedSeq}
				}),
			)
		}
		m.lastReconcileTime = time.Now()
		return m, nil
	case statusBarTickMsg:
		// Collect status bar counts, orchestration overlay data, and log lines in
		// a background goroutine so disk I/O never blocks the BubbleTea main loop.
		// Results are delivered back via uiCacheRefreshMsg and applied on the main
		// loop by SetCounts/SetCachedData/SetLines — eliminating data races.
		if m.conductorConfig != nil {
			sb := m.statusBar
			orch := m.orchestrationOverlay
			logV := m.logViewerOverlay
			orchVisible := orch != nil && orch.IsVisible() && orch.NeedsRefresh()
			logVisible := logV != nil && logV.IsVisible() && logV.NeedsRefresh()
			return m, tea.Batch(
				statusBarTickCmd(),
				func() tea.Msg {
					// Collect all data off the main loop.
					var sbCounts ui.TaskCounts
					if sb != nil {
						sbCounts, _ = ui.CollectStatusBarCounts()
					}
					var orchData overlay.OrchCachedData
					if orchVisible {
						orchData = overlay.CollectOrchData()
					}
					var logLines []string
					if logVisible {
						logLines = overlay.CollectLogLines()
					}
					// Return a function that applies the results on the main loop.
					return uiCacheRefreshMsg{apply: func() {
						if sb != nil {
							sb.SetCounts(sbCounts)
						}
						if orchVisible {
							orch.SetCachedData(orchData)
						}
						if logVisible && logLines != nil {
							logV.SetLines(logLines)
						}
					}}
				},
			)
		}
		return m, statusBarTickCmd()
	case sessionSaveTickMsg:
		if m.conductorConfig != nil {
			m.saveSessionState()
		}
		return m, sessionSaveTickCmd()
	case conflictCheckTickMsg:
		if m.conductorConfig != nil {
			worktrees := make(map[string]string)
			for _, inst := range m.list.GetInstances() {
				if inst.Account != "" && inst.GetWorktreePath() != "" {
					worktrees[inst.Title] = inst.GetWorktreePath()
				}
			}
			conflicts := orchestration.DetectConflicts(worktrees)
			if len(conflicts) > 0 {
				var parts []string
				for _, c := range conflicts {
					parts = append(parts, fmt.Sprintf("%s are both editing %s",
						strings.Join(c.Workers, " and "), c.FilePath))
				}
				m.conflictBanner = "⚠ " + strings.Join(parts, "; ")
			} else {
				m.conflictBanner = ""
			}
		}
		return m, conflictCheckTickCmd()
	case metadataUpdateDoneMsg:
		for _, r := range msg.results {
			if r.updated {
				r.instance.SetStatus(session.Running)
				if r.instance.Account != "" {
					m.lastOutputChange[r.instance.Title] = time.Now()
				}
			} else if r.hasPrompt {
				r.instance.TapEnter()
			} else {
				r.instance.SetStatus(session.Ready)
			}
			if r.diffStats != nil && r.diffStats.Error != nil {
				if !strings.Contains(r.diffStats.Error.Error(), "base commit SHA not set") {
					log.WarningLog.Printf("could not update diff stats: %v", r.diffStats.Error)
				}
				r.instance.SetDiffStats(nil)
			} else {
				r.instance.SetDiffStats(r.diffStats)
			}
			// Apply pre-fetched preview content (captured off the main loop) to the
			// selected instance's preview pane — no subprocess on the main goroutine.
			if r.previewFetched && r.instance == m.list.GetSelectedInstance() {
				m.tabbedWindow.SetPreviewContent(r.previewContent)
			}
		}

		// Stall detection — check every ~5s (every 10th metadata tick at 500ms)
		m.stallCheckCounter++
		var tasksToFail []string // task IDs to mark failed in background
		if m.stallCheckCounter >= 10 && m.conductorConfig != nil {
			m.stallCheckCounter = 0
			stallThreshold := time.Duration(m.conductorConfig.StallWarnSeconds) * time.Second
			if stallThreshold == 0 {
				stallThreshold = orchestration.DefaultStallThreshold
			}
			stallTimeout := time.Duration(m.conductorConfig.StallTimeoutSeconds) * time.Second

			store, storeErr := orchestration.NewTaskStore()
			if storeErr == nil {
				for _, r := range msg.results {
					instName := r.instance.Title
					if r.instance.Account == "" {
						continue // not a conductor instance
					}

					// Check if this instance has an in_progress task
					tasks, _ := store.ForInstance(instName)
					hasActiveTask := false
					for _, t := range tasks {
						if t.Status == orchestration.StatusInProgress {
							hasActiveTask = true
							break
						}
					}

					if !hasActiveTask {
						delete(m.stalledInstances, instName)
						continue
					}

					lastChange, ok := m.lastOutputChange[instName]
					if !ok {
						continue
					}

					sinceLast := time.Since(lastChange)

					// Check timeout first (more severe)
					if stallTimeout > 0 && sinceLast > stallTimeout {
						// Collect task IDs to mark failed in a background Cmd
						for _, t := range tasks {
							if t.Status == orchestration.StatusInProgress {
								tasksToFail = append(tasksToFail, t.ID)
							}
						}
						delete(m.stalledInstances, instName)
						continue
					}

					// Check stall warning
					if sinceLast > stallThreshold {
						if _, alreadyFlagged := m.stalledInstances[instName]; !alreadyFlagged {
							m.stalledInstances[instName] = time.Now()
							log.WarningLog.Printf("stall detected: %s has had no output change for %s", instName, sinceLast.Round(time.Second))
							if m.conductorConfig.Notifications.Enabled && m.conductorConfig.Notifications.WorkerStalled {
								notify.NotifyWorkerStalled(instName, sinceLast)
							}
						}
					} else {
						// Output resumed — clear stall flag
						if _, wasFlagged := m.stalledInstances[instName]; wasFlagged {
							log.InfoLog.Printf("stall cleared: %s output resumed", instName)
							delete(m.stalledInstances, instName)
						}
					}
				}
			}
		}

		if len(tasksToFail) > 0 {
			return m, tea.Batch(
				tickUpdateMetadataCmd(m.snapshotActiveInstances()),
				func() tea.Msg {
					store, err := orchestration.NewTaskStore()
					if err != nil {
						return nil
					}
					for _, id := range tasksToFail {
						task, err := store.Get(id)
						if err != nil {
							continue
						}
						if task.Status == orchestration.StatusInProgress {
							task.Status = orchestration.StatusFailed
							errMsg := "stalled: no output change exceeded timeout"
							task.Error = &errMsg
							if updateErr := store.Update(task); updateErr != nil {
								log.ErrorLog.Printf("stall timeout: failed to update task %s: %v", id, updateErr)
							} else {
								notify.NotifyTaskFailed(id, "stall timeout")
							}
						}
					}
					return stallTimeoutMsg{taskIDs: tasksToFail}
				},
			)
		}

		return m, tickUpdateMetadataCmd(m.snapshotActiveInstances())
	case tea.MouseMsg:
		// Handle mouse wheel events for scrolling the diff/preview pane
		if msg.Action == tea.MouseActionPress {
			if msg.Button == tea.MouseButtonWheelDown || msg.Button == tea.MouseButtonWheelUp {
				selected := m.list.GetSelectedInstance()
				if selected == nil || selected.Status == session.Paused {
					return m, nil
				}

				switch msg.Button {
				case tea.MouseButtonWheelUp:
					m.tabbedWindow.ScrollUp()
				case tea.MouseButtonWheelDown:
					m.tabbedWindow.ScrollDown()
				}
			}
		}
		return m, nil
	case branchSearchDebounceMsg:
		// Debounce timer fired — check if this is still the current filter version
		if m.textInputOverlay == nil {
			return m, nil
		}
		if msg.version != m.textInputOverlay.BranchFilterVersion() {
			return m, nil // stale, a newer debounce is pending
		}
		return m, m.runBranchSearch(msg.filter, msg.version)
	case branchSearchResultMsg:
		if m.textInputOverlay != nil {
			m.textInputOverlay.SetBranchResults(msg.branches, msg.version)
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	case tea.WindowSizeMsg:
		m.updateHandleWindowSizeEvent(msg)
		return m, nil
	case error:
		// Handle errors from confirmation actions
		return m, m.handleError(msg)
	case instanceKilledMsg:
		// I/O is done; mutate list and registry safely on the main loop.
		// Use RemoveByName (pure in-memory) — instance.Kill() already ran in the goroutine.
		m.tabbedWindow.CleanupTerminalForInstance(msg.title)
		m.list.RemoveByName(msg.title)
		m.updateRegistry()
		return m, m.instanceChanged()
	case mergeResultMsg:
		if msg.err != nil {
			return m, m.handleError(fmt.Errorf("merge failed for %s: %s: %w", msg.branch, msg.output, msg.err))
		}
		m.errBox.SetMessage(fmt.Sprintf("Merged %s", msg.branch))
		return m, func() tea.Msg {
			select {
			case <-m.ctx.Done():
			case <-time.After(3 * time.Second):
			}
			return hideErrMsg{}
		}
	case helpTriggerMsg:
		// showHelpScreen mutates model state and must run on the main loop.
		return m.showHelpScreen(msg.helpType, msg.onDismiss)
	case instanceChangedMsg:
		// Handle instance changed after confirmation action
		return m, m.instanceChanged()
	case uiCacheRefreshMsg:
		// Apply pre-collected UI cache data on the main loop to avoid races
		// between the collection goroutine and Render() reads.
		if msg.apply != nil {
			msg.apply()
		}
		return m, nil
	case instanceDetachedMsg:
		// Handle return from tmux attach — refresh state
		m.state = stateDefault
		return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
	case orchPreviewLoadedMsg:
		// Deliver background-loaded result file content to the orchestration overlay.
		if m.orchestrationOverlay != nil {
			m.orchestrationOverlay.SetPreviewContent(msg.taskID, msg.content)
		}
		return m, nil
	case promptSentMsg:
		// No-op: prompt was sent asynchronously; log for debugging.
		log.InfoLog.Printf("prompt sent to instance %q", msg.name)
		return m, nil
	case quickDispatchResultMsg:
		if msg.err != nil {
			return m, m.handleError(fmt.Errorf("quick dispatch failed: %w", msg.err))
		}
		log.InfoLog.Printf("quick dispatch succeeded: task %q", msg.taskID)
		return m, nil
	case bulkRetryCountMsg:
		if msg.err != nil {
			return m, m.handleError(msg.err)
		}
		if msg.totalFailed == 0 {
			return m, m.handleError(fmt.Errorf("no failed or timed-out tasks to retry"))
		}
		message := fmt.Sprintf("Retry %d failed/timed-out tasks across %d idle workers?", msg.totalFailed, msg.idleWorkers)
		retryAction := func() tea.Msg {
			result, err := orchestration.RunBulkRetry(orchestration.BulkRetryOptions{})
			return bulkRetryResultMsg{result: result, err: err}
		}
		return m, m.confirmAction(message, retryAction)
	case bulkRetryResultMsg:
		if msg.err != nil {
			return m, m.handleError(msg.err)
		}
		r := msg.result
		summary := fmt.Sprintf("Retried %d/%d tasks", r.Retried, r.Total)
		if r.Skipped > 0 {
			summary += fmt.Sprintf(", %d skipped", r.Skipped)
		}
		if r.Failed > 0 {
			summary += fmt.Sprintf(", %d errors", r.Failed)
		}
		m.errBox.SetMessage(summary)
		return m, nil
	case stallTimeoutMsg:
		for _, id := range msg.taskIDs {
			log.WarningLog.Printf("stall timeout: task %s marked failed", id)
		}
		return m, nil
	case pauseCompleteMsg:
		if msg.err != nil {
			return m, m.handleError(msg.err)
		}
		if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
			log.ErrorLog.Printf("failed to save instances after pause: %v", err)
		}
		m.updateRegistry()
		m.tabbedWindow.CleanupTerminalForInstance(msg.name)
		return m, m.instanceChanged()
	case resumeCompleteMsg:
		if msg.err != nil {
			return m, m.handleError(msg.err)
		}
		if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
			return m, m.handleError(err)
		}
		m.updateRegistry()
		return m, tea.WindowSize()
	case menuStateMsg:
		m.menu.SetState(msg.state)
		return m, nil
	case instanceStartedMsg:
		// Select the instance that just started (or failed)
		m.list.SelectInstance(msg.instance)

		if msg.err != nil {
			m.list.Kill()
			return m, tea.Batch(m.handleError(msg.err), m.instanceChanged())
		}

		// Save after successful start
		if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
			return m, m.handleError(err)
		}
		m.updateRegistry()
		if m.autoYes {
			msg.instance.AutoYes = true
		}

		if msg.promptAfterName {
			m.state = statePrompt
			m.menu.SetState(ui.StatePrompt)
			m.textInputOverlay = m.newPromptOverlay()
		} else {
			// If instance has a prompt (set from Shift+N flow), send it now via a tea.Cmd
			// so the sleep inside SendPrompt (or any startup delay) does not block Update().
			if msg.instance.Prompt != "" {
				inst := msg.instance
				prompt := inst.Prompt
				inst.Prompt = ""
				sendCmd := func() tea.Msg {
					// Brief pause to let Claude Code finish starting before injecting the prompt.
					time.Sleep(100 * time.Millisecond)
					if err := inst.SendPrompt(prompt); err != nil {
						log.ErrorLog.Printf("failed to send prompt: %v", err)
					}
					return promptSentMsg{name: inst.Title}
				}
				m.menu.SetState(ui.StateDefault)
				m.showHelpScreen(helpStart(msg.instance), nil)
				return m, tea.Batch(tea.WindowSize(), m.instanceChanged(), sendCmd)
			}
			m.menu.SetState(ui.StateDefault)
			m.showHelpScreen(helpStart(msg.instance), nil)
		}

		return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *home) handleQuit() (tea.Model, tea.Cmd) {
	if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
		// Log the error but still quit — blocking the quit on a save failure would
		// trap the user in the TUI with no way to exit except SIGKILL.
		log.ErrorLog.Printf("handleQuit: failed to save instances: %v", err)
	}
	m.updateRegistry()

	if m.conductorConfig != nil {
		m.saveSessionState()

		// Git safety teardown — log summary
		repoDir, _ := filepath.Abs(".")
		summary := orchestration.TeardownGitSafetyNet(repoDir, m.startTag, m.stashRef)
		if summary != "" {
			log.InfoLog.Printf("git safety net summary:\n%s", summary)
		}
	}

	return m, tea.Quit
}

// saveSessionState persists the current session state for auto-resume.
func (m *home) saveSessionState() {
	repoDir, _ := filepath.Abs(".")
	var instances []orchestration.SessionInstance
	for _, inst := range m.list.GetInstances() {
		if inst.Account != "" {
			instances = append(instances, orchestration.SessionInstance{
				Title:   inst.Title,
				Account: inst.Account,
				Branch:  inst.Branch,
				Env:     inst.Env,
			})
		}
	}
	state := &orchestration.SessionState{
		RepoPath:  repoDir,
		StartedAt: m.sessionStartedAt,
		StashRef:  m.stashRef,
		StartTag:  m.startTag,
		Instances: instances,
	}
	if err := orchestration.SaveSession(state); err != nil {
		log.ErrorLog.Printf("session save: %v", err)
	}
}

// updateRegistry writes the current instance state to registry.json for
// orchestration consumers. It is a best-effort operation; errors are logged
// but not surfaced to the user.
func (m *home) updateRegistry() {
	if m.conductorConfig == nil {
		return
	}

	baseDir, err := accounts.ConductorDir()
	if err != nil {
		log.ErrorLog.Printf("updateRegistry: failed to get conductor dir: %v", err)
		return
	}

	instances := m.list.GetInstances()
	entries := make(map[string]orchestration.RegistryEntry, len(instances))

	for _, inst := range instances {
		if inst.Account == "" {
			continue
		}

		var status string
		switch inst.Status {
		case session.Paused:
			status = orchestration.RegistryStatusPaused
		case session.Loading:
			status = orchestration.RegistryStatusStarting
		case session.Ready, session.Running:
			status = orchestration.RegistryStatusRunning
		default:
			status = orchestration.RegistryStatusRunning
		}

		lastOutput := orchestration.NowISO()
		if t, ok := m.lastOutputChange[inst.Title]; ok {
			lastOutput = t.UTC().Format(time.RFC3339)
		}

		entries[inst.Title] = orchestration.RegistryEntry{
			Account:      inst.Account,
			Role:         inst.Role,
			Program:      inst.Program,
			Model:        inst.Model,
			TmuxSession:  tmux.SanitizeTmuxName(inst.Title),
			WorktreePath: inst.GetWorktreePath(),
			Branch:       inst.Branch,
			Status:       status,
			CreatedAt:    inst.CreatedAt.Format(time.RFC3339),
			LastOutputAt: lastOutput,
		}
	}

	registry := orchestration.Registry{
		Instances: entries,
		UpdatedAt: orchestration.NowISO(),
	}

	registryPath := filepath.Join(baseDir, "registry.json")
	if err := orchestration.AtomicWriteJSON(registryPath, registry); err != nil {
		log.ErrorLog.Printf("updateRegistry: failed to write registry.json: %v", err)
		return
	}
	m.registryMutationSeq++
}

// handleMenuHighlighting, handleKeyPress, and regenerateInstructions live in keys.go.

// currentWorkerInfos returns a snapshot of WorkerInfo for all active non-orchestrator
// instances. Used to populate the orchestrator's worker table.
func (m *home) currentWorkerInfos() []accounts.WorkerInfo {
	var workers []accounts.WorkerInfo
	for _, inst := range m.list.GetInstances() {
		if inst.Account == "" || inst.Role == string(accounts.RoleOrchestrator) {
			continue
		}
		var statusStr string
		switch inst.Status {
		case session.Paused:
			statusStr = "paused"
		case session.Loading:
			statusStr = "starting"
		case session.Running:
			statusStr = "running"
		case session.Ready:
			statusStr = "idle"
		default:
			statusStr = "unknown"
		}
		progName := inst.Program
		if progName == "" {
			progName = "claude"
		}
		modelID := inst.Model
		modelStrengths := programs.FormatModelStrengths(modelID)

		workers = append(workers, accounts.WorkerInfo{
			Title:          inst.Title,
			Account:        inst.Account,
			Status:         statusStr,
			Program:        progName,
			Model:          modelID,
			ModelStrengths: modelStrengths,
		})
	}
	return workers
}

// nextAvailableAccount returns an available account that does not have an active
// (Running or Starting) instance. preferredRole controls which accounts are
// preferred: pass accounts.RoleWorker to skip orchestrator accounts (for generic
// worker creation), or accounts.RoleOrchestrator to prefer the orchestrator.
// Falls back to any available account if no match for the preferred role exists.
// Returns nil if all accounts are occupied.
func (m *home) nextAvailableAccount(preferredRole accounts.Role) *accounts.Account {
	if m.conductorConfig == nil {
		return nil
	}

	// Instance statuses: Running, Ready, Loading, Paused
	// Killed instances are removed from the list entirely.
	activeAccounts := make(map[string]bool)
	for _, inst := range m.list.GetInstances() {
		if inst.Account != "" && inst.Status != session.Paused {
			activeAccounts[inst.Account] = true
		}
	}

	// First pass: prefer accounts matching the requested role.
	for i := range m.conductorConfig.Accounts {
		acct := &m.conductorConfig.Accounts[i]
		if !activeAccounts[acct.Name] && acct.Role == preferredRole {
			return acct
		}
	}

	// Second pass: fall back to any available account.
	for i := range m.conductorConfig.Accounts {
		acct := &m.conductorConfig.Accounts[i]
		if !activeAccounts[acct.Name] {
			return acct
		}
	}

	return nil
}

// instanceChanged updates the preview pane, menu, and diff pane based on the selected instance.
// The tmux capture-pane subprocess is NOT called here; it is run in the background by
// tickUpdateMetadataCmd and the result is applied via metadataUpdateDoneMsg.SetPreviewContent.
// For non-running states (nil, Loading, Paused) UpdatePreview is still called because those
// paths do no subprocess I/O (they just set fallback text).
func (m *home) instanceChanged() tea.Cmd {
	// selected may be nil
	selected := m.list.GetSelectedInstance()

	m.tabbedWindow.UpdateDiff(selected)
	m.tabbedWindow.SetInstance(selected)
	// Update menu with current instance
	m.menu.SetInstance(selected)

	// Call UpdatePreview only for states that don't require subprocess I/O (nil / Loading / Paused).
	// For running instances the preview content is delivered by metadataUpdateDoneMsg to avoid
	// blocking the main loop with a tmux capture-pane call every 100 ms.
	if selected == nil || selected.Status == session.Loading || selected.Status == session.Paused {
		if err := m.tabbedWindow.UpdatePreview(selected); err != nil {
			return m.handleError(err)
		}
	}

	if err := m.tabbedWindow.UpdateTerminal(selected); err != nil {
		return m.handleError(err)
	}
	return nil
}

type keyupMsg struct{}

// keydownCallback clears the menu option highlighting after 500ms.
func (m *home) keydownCallback(name keys.KeyName) tea.Cmd {
	m.menu.Keydown(name)
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(500 * time.Millisecond):
		}

		return keyupMsg{}
	}
}

// uiCacheRefreshMsg carries a deferred function that applies freshly-collected
// data to one or more UI components on the main BubbleTea loop. Using a
// function avoids exposing internal data types across package boundaries.
type uiCacheRefreshMsg struct {
	apply func()
}

// wakeBannerDismissMsg auto-dismisses the wake-from-sleep banner.
type wakeBannerDismissMsg struct {
	seq int
}

// hideErrMsg implements tea.Msg and clears the error text from the screen.
type hideErrMsg struct{}

// previewTickMsg implements tea.Msg and triggers a preview update
type previewTickMsg struct{}

// reconcileTickMsg triggers a periodic reconciliation pass.
type reconcileTickMsg struct{}

// reconcileDoneMsg carries the results of a reconciliation pass back to the main loop.
type reconcileDoneMsg struct {
	result      *orchestration.ReconcileResult
	registry    *orchestration.Registry
	err         error
	capturedSeq int // registryMutationSeq at the time the goroutine was launched
}

// reconcileTickCmd returns a Cmd that fires reconcileTickMsg every 5 seconds.
func reconcileTickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return reconcileTickMsg{}
	})
}

// statusBarTickMsg triggers a periodic status bar cache refresh.
type statusBarTickMsg struct{}

// statusBarTickCmd returns a Cmd that fires statusBarTickMsg every 2 seconds.
func statusBarTickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return statusBarTickMsg{}
	})
}

// sessionSaveTickMsg triggers a periodic session state save.
type sessionSaveTickMsg struct{}

func sessionSaveTickCmd() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return sessionSaveTickMsg{}
	})
}

// conflictCheckTickMsg triggers a periodic conflict check.
type conflictCheckTickMsg struct{}

func conflictCheckTickCmd() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return conflictCheckTickMsg{}
	})
}

type instanceChangedMsg struct{}

// instanceKilledMsg is sent when killAction completes its I/O work successfully.
// The main loop then mutates m.list and calls m.updateRegistry() on the correct goroutine.
// title holds the instance name so the terminal pane can be cleaned up before Kill() removes it.
type instanceKilledMsg struct {
	title string
}

// instanceDetachedMsg is sent after the user detaches from a tmux session.
type instanceDetachedMsg struct{}

// orchPreviewLoadedMsg carries file content for the orchestration overlay's
// result preview, loaded in a background goroutine to keep os.ReadFile off
// the BubbleTea main loop.
type orchPreviewLoadedMsg struct {
	taskID  string
	content string
}

// promptSentMsg is sent after SendPrompt completes in a background goroutine.
type promptSentMsg struct{ name string }

// quickDispatchResultMsg carries the result of a quick-dispatch orchestration call.
type quickDispatchResultMsg struct {
	taskID string
	err    error
}

// pauseCompleteMsg is sent when instance.Pause() finishes in a background goroutine.
type pauseCompleteMsg struct {
	name string
	err  error
}

// resumeCompleteMsg is sent when instance.Resume() finishes in a background goroutine.
type resumeCompleteMsg struct {
	name string
	err  error
}

// menuStateMsg requests a m.menu.SetState() call on the main loop,
// replacing the anti-pattern of calling SetState inside a tea.Sequence closure (data race).
type menuStateMsg struct{ state ui.MenuState }

type instanceStartedMsg struct {
	instance        *session.Instance
	err             error
	promptAfterName bool
	selectedBranch  string
}

// helpTriggerMsg requests that showHelpScreen be called from the main Update loop,
// avoiding model mutation inside a tea.Sequence closure (which runs in a goroutine).
type helpTriggerMsg struct {
	helpType  helpText
	onDismiss func() tea.Cmd
}

// mergeResultMsg carries the result of a git merge back to the main loop.
type mergeResultMsg struct {
	branch string
	output string
	err    error
}

// bulkRetryCountMsg carries the async I/O results for the bulk retry confirmation modal.
type bulkRetryCountMsg struct {
	totalFailed int
	idleWorkers int
	err         error
}

// bulkRetryResultMsg carries the result of a bulk retry operation back to the main loop.
type bulkRetryResultMsg struct {
	result *orchestration.BulkRetryResult
	err    error
}

// stallTimeoutMsg is sent after stall-timeout tasks have been marked failed in the background.
type stallTimeoutMsg struct {
	taskIDs []string
}

// branchSearchDebounceMsg fires after the debounce interval to trigger a search.
type branchSearchDebounceMsg struct {
	filter  string
	version uint64
}

// branchSearchResultMsg carries search results back to Update.
type branchSearchResultMsg struct {
	branches []string
	version  uint64
}

const branchSearchDebounce = 150 * time.Millisecond

// scheduleBranchSearch returns a debounced tea.Cmd: sleeps, then triggers a search message.
func (m *home) scheduleBranchSearch(filter string, version uint64) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(branchSearchDebounce)
		return branchSearchDebounceMsg{filter: filter, version: version}
	}
}

// runBranchSearch returns a tea.Cmd that performs the git search in the background.
func (m *home) runBranchSearch(filter string, version uint64) tea.Cmd {
	return func() tea.Msg {
		currentDir, _ := os.Getwd()
		branches, err := git.SearchBranches(currentDir, filter)
		if err != nil {
			log.WarningLog.Printf("branch search failed: %v", err)
			return nil
		}
		return branchSearchResultMsg{branches: branches, version: version}
	}
}

// instanceMetaResult holds the results of a single instance's metadata update,
// computed in a background goroutine.
type instanceMetaResult struct {
	instance       *session.Instance
	updated        bool
	hasPrompt      bool
	diffStats      *git.DiffStats
	previewContent string // pre-fetched tmux pane content (empty string = no update)
	previewFetched bool   // true when previewContent was successfully fetched
}

// metadataUpdateDoneMsg is sent when the background metadata update completes.
type metadataUpdateDoneMsg struct {
	results []instanceMetaResult
}

// snapshotActiveInstances returns the currently active (started, not paused)
// instances. Called on the main thread so the filtering doesn't race with
// state mutations.
func (m *home) snapshotActiveInstances() []*session.Instance {
	var out []*session.Instance
	for _, inst := range m.list.GetInstances() {
		if inst.Started() && !inst.Paused() {
			out = append(out, inst)
		}
	}
	return out
}

// tickUpdateMetadataCmd returns a self-chaining Cmd that sleeps 500ms, then performs
// expensive metadata I/O (tmux capture, git diff) in parallel background goroutines.
// Because it only re-schedules after completing, overlapping ticks are impossible.
// The active instances slice should be snapshotted on the main thread via
// snapshotActiveInstances() before being passed here.
func tickUpdateMetadataCmd(active []*session.Instance) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(500 * time.Millisecond)

		if len(active) == 0 {
			return metadataUpdateDoneMsg{}
		}

		results := make([]instanceMetaResult, len(active))
		var wg sync.WaitGroup
		for idx, inst := range active {
			wg.Add(1)
			go func(i int, instance *session.Instance) {
				defer wg.Done()
				r := &results[i]
				r.instance = instance
				r.updated, r.hasPrompt = instance.HasUpdated()
				r.diffStats = instance.ComputeDiff()
				// Capture preview content off the main loop to avoid blocking the BubbleTea
				// goroutine with a tmux subprocess every tick.
				if content, err := instance.Preview(); err == nil {
					r.previewContent = content
					r.previewFetched = true
				}
			}(idx, inst)
		}
		wg.Wait()

		return metadataUpdateDoneMsg{results: results}
	}
}

// handleError handles all errors which get bubbled up to the app. sets the error message. We return a callback tea.Cmd that returns a hideErrMsg message
// which clears the error message after 3 seconds.
func (m *home) handleError(err error) tea.Cmd {
	log.ErrorLog.Printf("%v", err)
	m.errBox.SetError(err)
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(3 * time.Second):
		}

		return hideErrMsg{}
	}
}

func (m *home) newPromptOverlay() *overlay.TextInputOverlay {
	ti := overlay.NewTextInputOverlayWithBranchPicker("Enter prompt", "", m.appConfig.GetProfiles())
	ti.SetViewport(m.windowWidth, m.windowHeight)
	return ti
}

// cancelPromptOverlay cancels the prompt overlay, cleaning up unstarted instances.
func (m *home) cancelPromptOverlay() tea.Cmd {
	selected := m.list.GetSelectedInstance()
	if selected != nil && !selected.Started() {
		m.list.Kill()
	}
	m.textInputOverlay = nil
	m.promptAfterName = false
	m.state = stateDefault
	return tea.Batch(
		tea.WindowSize(),
		func() tea.Msg { return menuStateMsg{state: ui.StateDefault} },
	)
}

// confirmAction shows a confirmation modal and stores the action to execute on confirm.
// When the user presses the confirm key, the stateConfirm handler will run action as a
// tea.Cmd so its returned tea.Msg is dispatched back into the BubbleTea message bus.
func (m *home) confirmAction(message string, action tea.Cmd) tea.Cmd {
	m.state = stateConfirm
	m.pendingConfirmAction = action

	// Create and show the confirmation overlay using ConfirmationOverlay
	m.confirmationOverlay = overlay.NewConfirmationOverlay(message)
	// Set a fixed width for consistent appearance
	m.sizeOverlays()

	return nil
}

// handleBulkRetry launches background I/O to count failed tasks and idle workers,
// then returns a bulkRetryCountMsg so the confirmation modal is shown on the main loop.
func (m *home) handleBulkRetry() (tea.Model, tea.Cmd) {
	return m, func() tea.Msg {
		store, err := orchestration.NewTaskStore()
		if err != nil {
			return bulkRetryCountMsg{err: err}
		}
		failed, _ := store.List(orchestration.StatusFailed)
		timedOut, _ := store.List(orchestration.StatusTimedOut)
		totalFailed := len(failed) + len(timedOut)

		idleCount := 0
		reg, _ := orchestration.LoadRegistry()
		if reg != nil {
			for name := range reg.ListWorkers() {
				if idle, _ := orchestration.IsWorkerIdle(name); idle {
					idleCount++
				}
			}
		}
		return bulkRetryCountMsg{totalFailed: totalFailed, idleWorkers: idleCount}
	}
}

func (m *home) View() string {
	layout := m.currentLayout()
	listWidth, tabsWidth := splitContentWidth(layout.content.W)

	var content string
	if m.list.NumInstances() == 0 && m.state == stateDefault {
		emptyMsg := lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Align(lipgloss.Center).
			Render("No instances yet\n\nPress 'n' to create your first instance\nPress '?' for help\nPress 'q' to quit")
		content = fitBlockToRect(lipgloss.Place(layout.content.W, layout.content.H, lipgloss.Center, lipgloss.Center, emptyMsg), layout.content)
	} else {
		listBlock := fitBlock(m.list.String(), listWidth, layout.content.H)
		previewBlock := fitBlock(m.tabbedWindow.String(), tabsWidth, layout.content.H)
		content = fitBlockToRect(lipgloss.JoinHorizontal(lipgloss.Top, listBlock, previewBlock), layout.content)
	}

	// Add the status bar when multi-account mode is active.
	statusBarStr := ""
	if m.conductorConfig != nil && m.statusBar != nil {
		statusBarStr = m.statusBar.Render()
	}

	// Setup-needed banner shown when no account config exists on first run.
	setupBannerStr := ""
	if m.setupNeeded {
		setupBannerStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true).
			Padding(0, 1)
		setupBannerStr = setupBannerStyle.Render(
			"Multi-account orchestration is not configured. Run `maestro setup` to enable it.")
	}

	// Conflict banner
	conflictBannerStr := ""
	if m.conflictBanner != "" {
		conflictBannerStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).
			Bold(true).
			Padding(0, 1)
		conflictBannerStr = conflictBannerStyle.Render(m.conflictBanner)
	}

	// Wake-from-sleep banner
	wakeBannerStr := ""
	if m.wakeBanner != "" {
		wakeBannerStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true).
			Padding(0, 1)
		wakeBannerStr = wakeBannerStyle.Render(m.wakeBanner)
	}

	viewParts := []string{}
	if layout.setupBanner.H > 0 {
		viewParts = append(viewParts, fitBlockToRect(setupBannerStr, layout.setupBanner))
	}
	if layout.conflictBanner.H > 0 {
		viewParts = append(viewParts, fitBlockToRect(conflictBannerStr, layout.conflictBanner))
	}
	if layout.wakeBanner.H > 0 {
		viewParts = append(viewParts, fitBlockToRect(wakeBannerStr, layout.wakeBanner))
	}
	if layout.content.H > 0 {
		viewParts = append(viewParts, content)
	}
	if layout.menu.H > 0 {
		viewParts = append(viewParts, fitBlockToRect(m.menu.String(), layout.menu))
	}
	if layout.status.H > 0 {
		viewParts = append(viewParts, fitBlockToRect(statusBarStr, layout.status))
	}
	if layout.err.H > 0 {
		viewParts = append(viewParts, fitBlockToRect(m.errBox.String(), layout.err))
	}

	mainView := fitBlock(strings.Join(viewParts, "\n"), layout.viewport.W, layout.viewport.H)

	if m.state == statePrompt {
		if m.textInputOverlay == nil {
			// State desync: overlay is nil but state says prompt. Render safely without
			// mutating m.state (View must be side-effect free). Update() will correct state
			// on the next frame when the nil overlay is detected there.
			log.ErrorLog.Printf("text input overlay is nil in statePrompt — rendering default view")
			return mainView
		}
		return overlay.PlaceOverlayInViewport(m.windowWidth, m.windowHeight, m.textInputOverlay.Render(), mainView, true)
	} else if m.state == stateHelp {
		if m.textOverlay == nil {
			log.ErrorLog.Printf("text overlay is nil in stateHelp — rendering default view")
			return mainView
		}
		return overlay.PlaceOverlayInViewport(m.windowWidth, m.windowHeight, m.textOverlay.Render(), mainView, true)
	} else if m.state == stateConfirm {
		if m.confirmationOverlay == nil {
			log.ErrorLog.Printf("confirmation overlay is nil in stateConfirm — rendering default view")
			return mainView
		}
		return overlay.PlaceOverlayInViewport(m.windowWidth, m.windowHeight, m.confirmationOverlay.Render(), mainView, true)
	}

	// Render overlays
	if m.orchestrationOverlay != nil && m.orchestrationOverlay.IsVisible() {
		return overlay.PlaceOverlayInViewport(m.windowWidth, m.windowHeight, m.orchestrationOverlay.Render(), mainView, true)
	}
	if m.logViewerOverlay != nil && m.logViewerOverlay.IsVisible() {
		return overlay.PlaceOverlayInViewport(m.windowWidth, m.windowHeight, m.logViewerOverlay.Render(), mainView, true)
	}
	if m.quickDispatchOverlay != nil {
		return overlay.PlaceOverlayInViewport(m.windowWidth, m.windowHeight, m.quickDispatchOverlay.Render(), mainView, true)
	}
	if m.reviewOverlay != nil {
		return overlay.PlaceOverlayInViewport(m.windowWidth, m.windowHeight, m.reviewOverlay.Render(), mainView, true)
	}

	return mainView
}

func (m *home) currentLayout() layoutSpec {
	return computeLayout(m.windowWidth, m.windowHeight, layoutFlags{
		setupBanner:    m.setupNeeded,
		conflictBanner: m.conflictBanner != "",
		wakeBanner:     m.wakeBanner != "",
		statusBar:      m.conductorConfig != nil && m.statusBar != nil,
		errBox:         true,
	})
}

func (m *home) sizeOverlays() {
	if m.textInputOverlay != nil {
		m.textInputOverlay.SetViewport(m.windowWidth, m.windowHeight)
	}
	if m.textOverlay != nil {
		m.textOverlay.SetViewport(m.windowWidth, m.windowHeight)
	}
	if m.confirmationOverlay != nil {
		m.confirmationOverlay.SetViewport(m.windowWidth, m.windowHeight)
	}
	if m.quickDispatchOverlay != nil {
		m.quickDispatchOverlay.SetViewport(m.windowWidth, m.windowHeight)
	}
	if m.reviewOverlay != nil {
		m.reviewOverlay.SetViewport(m.windowWidth, m.windowHeight)
	}
}
