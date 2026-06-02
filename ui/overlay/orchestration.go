package overlay

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"maestro/pkg/accounts"
	"maestro/pkg/orchestration"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// orchFilterMode controls which tasks are shown in the task list.
type orchFilterMode int

const (
	filterAll orchFilterMode = iota
	filterCompleted
	filterFailed
	filterInProgress
	filterByWorker
)

// OrchCachedData holds a snapshot of the data loaded from disk for the overlay.
// It is exported so that app.go can carry it in a message type and apply it
// on the BubbleTea main loop via SetCachedData(), eliminating data races.
type OrchCachedData struct {
	registry        *orchestration.Registry
	tasks           []*orchestration.Task
	planContent     string
	usageReport     *orchestration.UsageReport
	workerStatuses  map[string]*orchestration.WorkerStatus
	workerLastTasks map[string]*orchestration.Task
}

// orchCachedData is the internal alias kept for backward compat within this file.
type orchCachedData = OrchCachedData

// OrchestrationOverlay displays workers, tasks, and a plan preview in a modal panel.
type OrchestrationOverlay struct {
	width           int
	height          int
	visible         bool
	lastDataRefresh time.Time
	cached          orchCachedData

	// Task navigation and result preview
	taskCursor     int
	taskNavActive  bool
	previewActive  bool
	previewTaskID  string
	previewContent string
	previewScroll  int

	// Filtering and search
	filterMode       orchFilterMode
	filterWorkerIdx  int
	filterWorkerList []string
	searchActive     bool
	searchQuery      string

	// pendingAction is set by HandleKeyPress when the caller should perform a
	// follow-up action after closing the overlay (e.g. "retry-all").
	pendingAction string
}

// NewOrchestrationOverlay creates a new orchestration overlay.
func NewOrchestrationOverlay() *OrchestrationOverlay {
	return &OrchestrationOverlay{}
}

// SetSize calculates the overlay dimensions as 80% width and 60% height of the
// terminal dimensions provided.
func (o *OrchestrationOverlay) SetSize(w, h int) {
	o.SetViewport(w, h)
}

// SetViewport sizes the orchestration overlay directly from viewport dimensions.
func (o *OrchestrationOverlay) SetViewport(viewW, viewH int) {
	box := ComputeModalBox(viewW, viewH, 120, 60)
	o.width = box.OuterWidth
	o.height = box.OuterHeight
}

// Toggle flips the visibility of the overlay.
func (o *OrchestrationOverlay) Toggle() {
	o.visible = !o.visible
}

// IsVisible reports whether the overlay is currently shown.
func (o *OrchestrationOverlay) IsVisible() bool {
	return o.visible
}

// Close hides the overlay and resets all interactive state.
func (o *OrchestrationOverlay) Close() {
	o.visible = false
	o.taskNavActive = false
	o.taskCursor = 0
	o.previewActive = false
	o.previewContent = ""
	o.previewScroll = 0
	o.searchActive = false
	o.searchQuery = ""
	o.filterMode = filterAll
}

// PendingAction returns and clears any pending action from the overlay.
func (o *OrchestrationOverlay) PendingAction() string {
	action := o.pendingAction
	o.pendingAction = ""
	return action
}

// filteredTasks applies the current filter mode and search query to the cached
// task list. It returns the matching tasks and the total (unfiltered) count.
func (o *OrchestrationOverlay) filteredTasks() (filtered []*orchestration.Task, totalCount int) {
	all := o.cached.tasks
	totalCount = len(all)

	// Phase 1: status / worker filter.
	var phase1 []*orchestration.Task
	for _, t := range all {
		switch o.filterMode {
		case filterAll:
			phase1 = append(phase1, t)
		case filterCompleted:
			if t.Status == orchestration.StatusCompleted {
				phase1 = append(phase1, t)
			}
		case filterFailed:
			if t.Status == orchestration.StatusFailed || t.Status == orchestration.StatusTimedOut {
				phase1 = append(phase1, t)
			}
		case filterInProgress:
			if t.Status == orchestration.StatusInProgress || t.Status == orchestration.StatusDispatched ||
				t.Status == orchestration.StatusPending || t.Status == orchestration.StatusBlocked {
				phase1 = append(phase1, t)
			}
		case filterByWorker:
			if len(o.filterWorkerList) > 0 && o.filterWorkerIdx < len(o.filterWorkerList) {
				if t.WorkerInstance == o.filterWorkerList[o.filterWorkerIdx] {
					phase1 = append(phase1, t)
				}
			}
		}
	}

	// Phase 2: keyword search.
	if o.searchQuery == "" {
		return phase1, totalCount
	}
	q := strings.ToLower(o.searchQuery)
	for _, t := range phase1 {
		if strings.Contains(strings.ToLower(t.ID), q) ||
			strings.Contains(strings.ToLower(t.WorkerInstance), q) ||
			strings.Contains(strings.ToLower(t.Status), q) {
			filtered = append(filtered, t)
		}
	}
	return filtered, totalCount
}

// visibleTasks returns the most recent (up to 10) tasks from the filtered list.
func (o *OrchestrationOverlay) visibleTasks() []*orchestration.Task {
	filtered, _ := o.filteredTasks()
	if len(filtered) <= 10 {
		return filtered
	}
	return filtered[len(filtered)-10:]
}

// moveTaskCursor shifts the cursor by delta, clamping to valid bounds.
func (o *OrchestrationOverlay) moveTaskCursor(delta int) {
	tasks := o.visibleTasks()
	o.taskCursor += delta
	if o.taskCursor < 0 {
		o.taskCursor = 0
	}
	if max := len(tasks) - 1; max >= 0 && o.taskCursor > max {
		o.taskCursor = max
	} else if len(tasks) == 0 {
		o.taskCursor = 0
	}
}

// SelectedTaskResultFile returns the ResultFile path for the task at the
// current cursor position, or ("", "") if there is no selection.
// The caller (app.go) uses this to load the file in a tea.Cmd and deliver
// the content via SetPreviewContent — keeping file I/O off the main loop.
func (o *OrchestrationOverlay) SelectedTaskResultFile() (taskID, resultFile string) {
	tasks := o.visibleTasks()
	if len(tasks) == 0 || o.taskCursor >= len(tasks) {
		return "", ""
	}
	t := tasks[o.taskCursor]
	return t.ID, t.ResultFile
}

// SetPreviewContent stores pre-loaded result content and activates the preview
// view. Called from app.go after loading the file in a background tea.Cmd.
func (o *OrchestrationOverlay) SetPreviewContent(taskID, content string) {
	o.previewTaskID = taskID
	o.previewContent = content
	o.previewActive = true
	o.previewScroll = 0
}

// renderResultPreview renders a scrollable text view of a task result.
func (o *OrchestrationOverlay) renderResultPreview() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("Task Result: " + orchTruncateStr(o.previewTaskID, 40)))
	b.WriteString("\n\n")

	lines := strings.Split(o.previewContent, "\n")
	totalLines := len(lines)

	// Reserve ~8 lines for chrome (title, blank, separator, position, footer, borders/padding).
	viewportHeight := o.height - 8
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	// Clamp scroll.
	maxScroll := totalLines - viewportHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if o.previewScroll > maxScroll {
		o.previewScroll = maxScroll
	}
	if o.previewScroll < 0 {
		o.previewScroll = 0
	}

	end := o.previewScroll + viewportHeight
	if end > totalLines {
		end = totalLines
	}
	for _, line := range lines[o.previewScroll:end] {
		b.WriteString("  " + line + "\n")
	}

	// Position indicator.
	b.WriteString("\n")
	startLine := o.previewScroll + 1
	pct := 0
	if totalLines > 0 {
		pct = end * 100 / totalLines
	}
	b.WriteString(dimStyle.Render(fmt.Sprintf("Line %d-%d of %d (%d%%)", startLine, end, totalLines, pct)))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("[Esc] back  [j/k] scroll  [g/G] top/bottom"))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(o.width).
		Height(o.height)

	return boxStyle.Render(b.String())
}

// cycleFilterMode advances to the next filter mode, wrapping around.
func (o *OrchestrationOverlay) cycleFilterMode() {
	o.filterMode++
	if o.filterMode > filterByWorker {
		o.filterMode = filterAll
	}
	if o.filterMode == filterByWorker {
		o.filterWorkerList = nil
		if o.cached.registry != nil {
			workers := o.cached.registry.ListWorkers()
			for name := range workers {
				o.filterWorkerList = append(o.filterWorkerList, name)
			}
			sort.Strings(o.filterWorkerList)
		}
		o.filterWorkerIdx = 0
	}
}

// HandleKeyPress processes a key event. Returns true when the overlay should be
// closed (Esc or "o").
func (o *OrchestrationOverlay) HandleKeyPress(msg tea.KeyMsg) (shouldClose bool) {
	// Search input mode: capture all keys for the search field.
	if o.searchActive {
		switch msg.Type {
		case tea.KeyEscape:
			o.searchActive = false
			o.searchQuery = ""
			return false
		case tea.KeyEnter:
			o.searchActive = false // commit search, keep query
			return false
		case tea.KeyBackspace:
			runes := []rune(o.searchQuery)
			if len(runes) > 0 {
				o.searchQuery = string(runes[:len(runes)-1])
			}
			return false
		case tea.KeyRunes:
			o.searchQuery += string(msg.Runes)
			return false
		case tea.KeySpace:
			o.searchQuery += " "
			return false
		}
		return false
	}

	// Result preview mode.
	if o.previewActive {
		switch msg.String() {
		case "esc":
			o.previewActive = false
			o.previewContent = ""
			o.previewScroll = 0
			return false
		case "j", "down":
			o.previewScroll++
			return false
		case "k", "up":
			o.previewScroll--
			if o.previewScroll < 0 {
				o.previewScroll = 0
			}
			return false
		case "g":
			o.previewScroll = 0
			return false
		case "G":
			o.previewScroll = 999999 // clamped in render
			return false
		}
		return false // consume all keys while in preview
	}

	// Main view keys.
	switch msg.String() {
	case "j", "down":
		o.taskNavActive = true
		o.moveTaskCursor(1)
		return false
	case "k", "up":
		o.taskNavActive = true
		o.moveTaskCursor(-1)
		return false
	case "enter":
		if o.taskNavActive {
			// Signal to the caller that it should load the result file in a tea.Cmd.
			// Actual file I/O is performed by app.go via SelectedTaskResultFile() +
			// SetPreviewContent() so that os.ReadFile never blocks Update().
			o.pendingAction = "open-preview"
		}
		return false
	case "tab", "Tab":
		o.cycleFilterMode()
		o.taskCursor = 0
		return false
	case "/":
		o.searchActive = true
		o.searchQuery = ""
		return false
	case "left":
		if o.filterMode == filterByWorker && len(o.filterWorkerList) > 0 {
			o.filterWorkerIdx--
			if o.filterWorkerIdx < 0 {
				o.filterWorkerIdx = len(o.filterWorkerList) - 1
			}
		}
		return false
	case "right":
		if o.filterMode == filterByWorker && len(o.filterWorkerList) > 0 {
			o.filterWorkerIdx++
			if o.filterWorkerIdx >= len(o.filterWorkerList) {
				o.filterWorkerIdx = 0
			}
		}
		return false
	case "R":
		o.pendingAction = "retry-all"
		return true // close overlay, caller checks pendingAction
	case "esc", "o":
		return true
	}
	return false
}

// CollectOrchData performs all disk I/O needed by the overlay (registry, tasks,
// plan file, usage report) and returns the result as a value. It is a pure
// function that reads external state but does not mutate the overlay.
// The caller must run it inside a tea.Cmd goroutine and apply the result via
// SetCachedData() in Update() — this eliminates data races between the goroutine
// and Render() on the main loop.
func CollectOrchData() OrchCachedData {
	var data OrchCachedData

	reg, regErr := orchestration.LoadRegistry()
	if regErr == nil {
		data.registry = reg
	}

	store, storeErr := orchestration.NewTaskStore()
	if storeErr == nil {
		tasks, tErr := store.List("")
		if tErr == nil {
			data.tasks = tasks
		}

		// Load worker statuses and their last tasks into the cache so Render()
		// does not need to create a second TaskStore.
		if data.registry != nil {
			workers := data.registry.ListWorkers()
			statuses := make(map[string]*orchestration.WorkerStatus, len(workers))
			lastTasks := make(map[string]*orchestration.Task, len(workers))
			for name := range workers {
				ws, wsErr := store.ReadStatus(name)
				if wsErr == nil {
					statuses[name] = ws
					if ws.LastTask != "" {
						task, tErr := store.Get(ws.LastTask)
						if tErr == nil {
							lastTasks[name] = task
						}
					}
				}
			}
			data.workerStatuses = statuses
			data.workerLastTasks = lastTasks
		}
	}

	base, err := accounts.ConductorDir()
	if err == nil {
		planPath := filepath.Join(base, "plan.md")
		fileData, err := os.ReadFile(planPath)
		if err == nil {
			data.planContent = string(fileData)
		}
	}

	data.usageReport, _ = orchestration.LoadUsageReport()
	return data
}

// SetCachedData applies a freshly-collected OrchCachedData snapshot to the overlay.
// Must be called from Update() on the BubbleTea main loop, not from a goroutine,
// to avoid data races with Render().
func (o *OrchestrationOverlay) SetCachedData(data OrchCachedData) {
	o.cached = data
	o.lastDataRefresh = time.Now()
}

// NeedsRefresh reports whether the cache is stale (older than 1.5 seconds).
// App.go's statusBarTickCmd fires every 2 seconds; using a threshold strictly
// less than the tick interval ensures at least one refresh fires per tick even
// with scheduler jitter.
func (o *OrchestrationOverlay) NeedsRefresh() bool {
	return time.Since(o.lastDataRefresh) > 1500*time.Millisecond
}

// Render builds and returns the styled overlay string. Render is intentionally
// side-effect free and reads only from the cache last populated by RefreshCache.
func (o *OrchestrationOverlay) Render(opts ...WhitespaceOption) string {

	// Short-circuit to preview if active.
	if o.previewActive {
		return o.renderResultPreview()
	}

	// Compute a read-only cursor for rendering without mutating o.taskCursor.
	// Clamping is done in moveTaskCursor (Update path), not here (View path),
	// to keep Render side-effect free as BubbleTea requires.
	tasks := o.visibleTasks()
	taskCursor := o.taskCursor
	if taskCursor >= len(tasks) {
		taskCursor = max(0, len(tasks)-1)
	}

	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	b.WriteString(titleStyle.Render("Orchestration Panel"))
	b.WriteString("\n\n")

	// --- Workers section ---
	b.WriteString(titleStyle.Render("Workers:"))
	b.WriteString("\n")

	reg := o.cached.registry

	if reg == nil {
		b.WriteString(dimStyle.Render("  (registry unavailable)"))
		b.WriteString("\n")
	} else {
		workers := reg.ListWorkers()
		if len(workers) == 0 {
			b.WriteString(dimStyle.Render("  (no workers)"))
			b.WriteString("\n")
		}
		// Iterate in a stable order — ranging a map directly reshuffles the
		// worker table on every redraw, making it flicker.
		workerNames := make([]string, 0, len(workers))
		for name := range workers {
			workerNames = append(workerNames, name)
		}
		sort.Strings(workerNames)
		for _, name := range workerNames {
			entry := workers[name]
			state := "unknown"
			taskInfo := ""
			if o.cached.workerStatuses != nil {
				if ws, ok := o.cached.workerStatuses[name]; ok {
					state = ws.State
					if task, ok := o.cached.workerLastTasks[name]; ok {
						if ws.State == orchestration.StateWorking {
							taskInfo = fmt.Sprintf("Task: %s", orchTruncateStr(task.ID, 20))
						} else {
							taskInfo = fmt.Sprintf("Last: %s", orchTruncateStr(task.ID, 20))
						}
					}
				}
			}

			statusColor := "242"
			switch state {
			case orchestration.StateIdle:
				statusColor = "242"
			case orchestration.StateWorking:
				statusColor = "42"
			case orchestration.StateRateLimited:
				statusColor = "214"
			}

			stateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor))
			badge := dimStyle.Render(fmt.Sprintf("[%s]", entry.Account))

			// Program/model indicator.
			programModel := ""
			if entry.Program != "" || entry.Model != "" {
				prog := entry.Program
				if prog == "" {
					prog = "claude"
				}
				model := entry.Model
				if model == "" {
					model = "auto"
				}
				programModel = dimStyle.Render(fmt.Sprintf(" %s/%s", prog, model))
			}

			b.WriteString(fmt.Sprintf("  %-16s %s%s  %s  %s\n",
				name, badge, programModel, stateStyle.Render(state), dimStyle.Render(taskInfo)))
		}
	}

	// --- Tasks section ---
	b.WriteString("\n")

	filtered, total := o.filteredTasks()
	if len(filtered) > 0 || total > 0 {
		filterLabel := "all"
		switch o.filterMode {
		case filterCompleted:
			filterLabel = "completed"
		case filterFailed:
			filterLabel = "failed"
		case filterInProgress:
			filterLabel = "in progress"
		case filterByWorker:
			if len(o.filterWorkerList) > 0 && o.filterWorkerIdx < len(o.filterWorkerList) {
				filterLabel = o.filterWorkerList[o.filterWorkerIdx]
			} else {
				filterLabel = "by worker"
			}
		}
		header := fmt.Sprintf("Tasks (%d %s of %d total):", len(filtered), filterLabel, total)
		b.WriteString(titleStyle.Render(header))

		// Search indicator.
		if o.searchActive {
			searchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
			b.WriteString("  " + searchStyle.Render("/"+o.searchQuery+"\u2588"))
		} else if o.searchQuery != "" {
			b.WriteString("  " + dimStyle.Render("search: "+o.searchQuery))
		}
		b.WriteString("\n")

		cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
		for i, task := range tasks {
			statusColor := "242"
			switch task.Status {
			case orchestration.StatusCompleted:
				statusColor = "42"
			case orchestration.StatusFailed, orchestration.StatusTimedOut:
				statusColor = "196"
			case orchestration.StatusInProgress:
				statusColor = "33"
			case orchestration.StatusDispatched:
				statusColor = "214"
			case orchestration.StatusPending:
				statusColor = "245"
			case orchestration.StatusBlocked:
				statusColor = "208"
			case orchestration.StatusStale:
				statusColor = "226"
			}
			sStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor))

			prefix := "  "
			if o.taskNavActive && i == taskCursor {
				prefix = "> "
				line := fmt.Sprintf("%-20s  %s  %-16s",
					orchTruncateStr(task.ID, 20),
					sStyle.Render(fmt.Sprintf("%-12s", task.Status)),
					task.WorkerInstance)
				b.WriteString(cursorStyle.Render(prefix) + line + "\n")
			} else {
				b.WriteString(fmt.Sprintf("%s%-20s  %s  %-16s\n",
					prefix,
					orchTruncateStr(task.ID, 20),
					sStyle.Render(fmt.Sprintf("%-12s", task.Status)),
					task.WorkerInstance))
			}
		}
	} else {
		b.WriteString(dimStyle.Render("  (no tasks)"))
		b.WriteString("\n")
	}

	// --- Plan section ---
	b.WriteString("\n")

	if o.cached.planContent != "" {
		lines := strings.Split(o.cached.planContent, "\n")
		totalLines := len(lines)
		maxLines := 10
		if totalLines < maxLines {
			maxLines = totalLines
		}
		if totalLines > maxLines {
			b.WriteString(titleStyle.Render(fmt.Sprintf("Plan (%d of %d lines):", maxLines, totalLines)))
		} else {
			b.WriteString(titleStyle.Render("Plan:"))
		}
		b.WriteString("\n")
		for _, line := range lines[:maxLines] {
			b.WriteString(dimStyle.Render("  " + line))
			b.WriteString("\n")
		}
	} else {
		b.WriteString(titleStyle.Render("Plan:"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  (no plan.md found)"))
		b.WriteString("\n")
	}

	// --- Usage section ---
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("Usage:"))
	b.WriteString("\n")
	if o.cached.usageReport != nil {
		b.WriteString(orchestration.FormatUsageSummary(o.cached.usageReport))
	} else {
		b.WriteString(dimStyle.Render("  (run `maestro usage` to collect data)"))
		b.WriteString("\n")
	}

	// After usage summary, show routing advice if data is available
	if o.cached.usageReport != nil {
		advice := orchestration.GetRoutingAdvice(o.cached.usageReport)
		adviceStr := orchestration.FormatRoutingAdvice(advice)
		b.WriteString("\n" + adviceStr + "\n")
	}

	// --- Freshness footer ---
	freshnessStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	freshness := "Waiting for first refresh"
	if !o.lastDataRefresh.IsZero() {
		elapsed := time.Since(o.lastDataRefresh)
		seconds := int(elapsed.Seconds())
		if seconds <= 0 {
			freshness = "Updated just now"
		} else {
			freshness = fmt.Sprintf("Updated %ds ago", seconds)
		}
	}
	b.WriteString("\n")
	b.WriteString(freshnessStyle.Render(freshness))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("[o/Esc] close  [Tab] filter  [/] search  [j/k] navigate  [Enter] preview  [R] retry failed"))

	// Wrap everything in a rounded bordered box.
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(o.width).
		Height(o.height)

	return boxStyle.Render(b.String())
}

// orchTruncateStr shortens s to at most maxLen runes, appending "..." when truncated.
func orchTruncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}
