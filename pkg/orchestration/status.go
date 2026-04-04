package orchestration

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// RunStatus prints a combined view of workers and tasks.
// If instanceFilter is non-empty, only that instance's data is shown.
func RunStatus(instanceFilter string) error {
	reg, err := LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	store, err := NewTaskStore()
	if err != nil {
		return fmt.Errorf("failed to create task store: %w", err)
	}

	// --- Workers section ---
	fmt.Println("=== Workers ===")
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tACCOUNT\tSTATE\tTASK")

	names := sortedKeys(reg.Instances)
	for _, name := range names {
		if instanceFilter != "" && name != instanceFilter {
			continue
		}
		entry := reg.Instances[name]

		state := entry.Status
		// Try to read live worker status
		ws, wsErr := store.ReadStatus(name)
		if wsErr == nil {
			state = ws.State
		}

		taskInfo := "-"
		if ws != nil && ws.LastTask != "" {
			taskInfo = truncateID(ws.LastTask)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, entry.Account, state, taskInfo)
	}
	w.Flush()

	// --- Tasks section ---
	fmt.Println()
	fmt.Println("=== Tasks ===")
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tWORKER\tPROMPT\tELAPSED")

	var tasks []*Task
	if instanceFilter != "" {
		tasks, err = store.ForInstance(instanceFilter)
	} else {
		tasks, err = store.List("")
	}
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	for _, t := range tasks {
		desc := readPromptDescription(t.PromptFile)
		elapsed := timeSince(t.CreatedAt)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			truncateID(t.ID), t.Status, t.WorkerInstance, truncate(desc, 40), elapsed)
	}
	tw.Flush()

	return nil
}

// RunWorkers prints a listing of all workers with state and worktree path.
func RunWorkers() error {
	reg, err := LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	store, err := NewTaskStore()
	if err != nil {
		return fmt.Errorf("failed to create task store: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tACCOUNT\tROLE\tSTATE\tWORKTREE")

	names := sortedKeys(reg.Instances)
	for _, name := range names {
		entry := reg.Instances[name]
		state := entry.Status
		ws, wsErr := store.ReadStatus(name)
		if wsErr == nil {
			state = ws.State
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", name, entry.Account, entry.Role, state, entry.WorktreePath)
	}
	w.Flush()

	return nil
}

// RunTasks prints a listing of all tasks, optionally filtered by status.
func RunTasks(statusFilter string) error {
	store, err := NewTaskStore()
	if err != nil {
		return fmt.Errorf("failed to create task store: %w", err)
	}

	tasks, err := store.List(statusFilter)
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tWORKER\tATTEMPTS\tPROMPT\tELAPSED")

	for _, t := range tasks {
		desc := readPromptDescription(t.PromptFile)
		elapsed := timeSince(t.CreatedAt)
		fmt.Fprintf(w, "%s\t%s\t%s\t%d/%d\t%s\t%s\n",
			truncateID(t.ID), t.Status, t.WorkerInstance, t.Attempts, MaxAttempts,
			truncate(desc, 40), elapsed)
	}
	w.Flush()

	return nil
}

// --- helpers ---

// truncateID returns a shortened task ID for display (e.g. "task-170...ab3f").
func truncateID(id string) string {
	if len(id) <= 16 {
		return id
	}
	return id[:9] + "..." + id[len(id)-4:]
}

// truncate shortens a string to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// timeSince parses an ISO timestamp and returns a human-readable duration like "3m 12s".
func timeSince(isoTimestamp string) string {
	t, err := time.Parse(time.RFC3339, isoTimestamp)
	if err != nil {
		return "?"
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// readPromptDescription reads the first line of text after the "---" separator
// in a prompt file, returning it as a brief description.
func readPromptDescription(promptPath string) string {
	data, err := os.ReadFile(promptPath)
	if err != nil {
		return "(unreadable)"
	}
	content := string(data)
	parts := strings.SplitN(content, "---\n", 2)
	if len(parts) < 2 {
		return "(no description)"
	}
	firstLine := strings.SplitN(strings.TrimSpace(parts[1]), "\n", 2)[0]
	if firstLine == "" {
		return "(empty)"
	}
	return firstLine
}

// sortedKeys returns map keys sorted alphabetically.
func sortedKeys(m map[string]RegistryEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
