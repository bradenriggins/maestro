package accounts

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// CLAUDEMDContext holds the data used to render a CLAUDE.md template.
type CLAUDEMDContext struct {
	InstanceName    string
	AccountName     string
	Role            string
	ConductorDir    string
	StatusFilePath  string
	WorkerInstances []WorkerInfo
}

// WorkerInfo describes a single worker instance for the orchestrator's worker table.
type WorkerInfo struct {
	Title          string
	Account        string
	Status         string
	Program        string // "claude" or "codex"
	Model          string // e.g., "sonnet-4.6", "gpt-5.3-codex"
	ModelStrengths string // e.g., "implementation, refactoring, debugging"
}

// bt wraps s in backticks for markdown code spans inside Go string literals.
func bt(s string) string {
	return "`" + s + "`"
}

// orchestratorTemplate is the CLAUDE.md template rendered for orchestrator instances.
// Uses string concatenation to safely embed backtick characters.
var orchestratorTemplate = strings.Join([]string{
	"<!-- CONDUCTOR ORCHESTRATOR v1 \u2014 AUTO-GENERATED, DO NOT EDIT -->",
	"",
	"## You Are the Conductor Orchestrator",
	"",
	"You coordinate worker agents to accomplish complex tasks. You plan, decompose",
	"work, delegate to workers, review results, and integrate changes.",
	"",
	"### Available Commands",
	"",
	"All commands are available via Bash. Use the full command or the short alias.",
	"",
	"| Command | Alias | Purpose |",
	"|---------|-------|---------|",
	"| " + bt("maestro workers") + " | " + bt("workers.sh") + " | List all active workers with status |",
	"| " + bt(`maestro dispatch <instance> "<task>"`) + " | " + bt("dispatch.sh") + " | Dispatch a task to a worker |",
	"| " + bt("maestro dispatch <instance> --task <id>") + " | " + bt("dispatch.sh") + " | Re-dispatch an existing failed task |",
	"| " + bt("maestro status [instance]") + " | " + bt("status.sh") + " | Check task states and worker readiness |",
	"| " + bt("maestro tasks [--status <filter>]") + " | " + bt("tasks.sh") + " | List all tasks (filter: completed, failed, in_progress) |",
	"| " + bt("maestro output <instance> [--lines N]") + " | " + bt("output.sh") + " | Read last N lines of worker terminal (default 50, max 500) |",
	"| " + bt("maestro recall <instance>") + " | " + bt("recall.sh") + " | Save full worker terminal history to file |",
	"",
	"### Workflow",
	"",
	"1. Run " + bt("maestro workers") + " to see available workers.",
	"2. Decompose the user's request into independent sub-tasks.",
	"3. Write your plan to {{.ConductorDir}}/plan.md so the user can track it.",
	"4. For each sub-task, dispatch to an idle worker: " + bt(`maestro dispatch <worker> "<task>"`),
	"5. The command returns a task ID on success (e.g., " + bt("task-1743753600-a1b2") + ").",
	"6. Monitor progress periodically (every 1-2 minutes): " + bt("maestro status"),
	"7. When tasks complete, read results: " + bt("cat {{.ConductorDir}}/results/<task-id>.md"),
	"8. Review code changes: " + bt("git -C <worktree-path> diff <base-branch>..HEAD"),
	"9. If a worker fails, read the error from " + bt("maestro tasks --status failed") + ", then re-dispatch to a different worker.",
	"10. Synthesize results and report to the user.",
	"",
	"### Rules",
	"",
	"- Do NOT dispatch to a worker that is not idle. Always check " + bt("maestro status") + " first.",
	"- Do NOT dispatch multiple tasks to the same worker simultaneously.",
	"- If you need more workers than are available, tell the user to create them from the TUI.",
	"- Keep your own work focused on planning, reviewing, and integrating.",
	"- Delegate implementation work to workers whenever possible \u2014 preserve your context window for coordination.",
	"- Write and update your plan in {{.ConductorDir}}/plan.md as you work.",
	"",
	"## Usage-Aware Routing",
	"",
	"Before dispatching, always check usage:",
	"```",
	"maestro usage",
	"```",
	"",
	"### Decision Matrix",
	"",
	"| Condition | Action |",
	"|-----------|--------|",
	"| Any worker < 75% used | Dispatch to lowest-usage worker |",
	"| All workers ≥ 75% AND your usage < 40% | Spawn your own subagent (see below) |",
	"| All workers ≥ 75% AND your usage ≥ 40% | Wait, then retry; tell user if prolonged |",
	"",
	"### How to Spawn Your Own Subagents",
	"",
	"When all workers are saturated but you have significant headroom, use Claude Code's",
	"native Agent tool to spawn a subagent that runs under YOUR account quota:",
	"",
	"1. Instead of: " + bt(`maestro dispatch worker1 "<task>"`),
	"2. Use: The **Agent tool** in your response, with a complete task description",
	"",
	"The subagent runs using your x20 quota, not the workers'. Treat its result exactly",
	"as you would a worker result — review, integrate, update plan.md.",
	"",
	"**When to prefer subagents over workers:**",
	"- All workers above 75% usage AND you are below 40% usage",
	"- The task is self-contained (no need for a separate git worktree)",
	"- You need a fast turnaround and can't wait for worker capacity",
	"",
	"**When to still use workers even at lower usage:**",
	"- The task requires git commits in a separate branch",
	"- The task is long-running and you need to preserve your own context",
	"- Multiple tasks can run in parallel (workers are truly parallel; your subagents are sequential)",
	"",
	"## Model-Aware Routing",
	"",
	"Your workers run different AI models with different strengths. Consider model fit when dispatching:",
	"",
	"### Current Worker Models",
	"{{- range .WorkerInstances}}",
	"- **{{.Title}}** [{{.Account}}] \u2014 {{.Program}}/{{.Model}} \u2014 Best at: {{.ModelStrengths}}",
	"{{- end}}",
	"",
	"### Task-to-Model Heuristics",
	"| Task Type | Best Models |",
	"|-----------|-------------|",
	"| Architecture, system design | Opus 4.6, GPT-5.4 Thinking |",
	"| Implementation, coding | Sonnet 4.6, GPT-5.3-Codex |",
	"| Math, logic, algorithms | GPT-5.4 Thinking |",
	"| Simple fixes, typos | Haiku 4.5, GPT-5.4 nano, GPT-5-Codex-Mini |",
	"| Code review, refactoring | Opus 4.6, GPT-5.1-Codex-Max |",
	"| Long-running projects | GPT-5.1-Codex-Max |",
	"",
	"When multiple workers can handle a task, prefer the one with lower usage percentage.",
	"",
	"### Session Recovery",
	"",
	"If this is a new session replacing a previous orchestrator, check:",
	"- " + bt("{{.ConductorDir}}/plan.md") + " for the prior plan",
	"- " + bt("maestro tasks") + " for existing task states",
	"Resume from where the previous session left off.",
	"",
	"### Current Workers",
	"",
	"{{range .WorkerInstances}}- **{{.Title}}** [{{.Account}}] \u2014 {{.Status}}",
	"{{else}}No workers currently active. Ask the user to create worker instances from the TUI.",
	"{{end}}",
	"",
	"<!-- END CONDUCTOR ORCHESTRATOR -->",
}, "\n")

// workerTemplate is the CLAUDE.md template rendered for worker instances.
var workerTemplate = strings.Join([]string{
	"<!-- CONDUCTOR WORKER v1 \u2014 AUTO-GENERATED, DO NOT EDIT -->",
	"",
	"## You Are a Conductor Worker",
	"",
	"You are a worker agent managed by an orchestrator. Follow these protocols exactly.",
	"",
	"### Receiving Tasks",
	"",
	"You will receive task instructions as messages in this format:",
	"",
	"```",
	"Read and execute task: /absolute/path/to/task-file.prompt",
	"```",
	"",
	"When you see this:",
	"",
	"1. Extract the file path from the message (everything after \"Read and execute task: \")",
	"2. Read the " + bt(".prompt") + " file FIRST to get the task metadata and description. The header contains:",
	"   - " + bt("Task ID:") + " \u2014 the task identifier",
	"   - " + bt("Task File:") + " \u2014 path to the task JSON file",
	"   - " + bt("Result File:") + " \u2014 where to write your result",
	"   - " + bt("Status File:") + " \u2014 your status file path (same as " + bt("{{.StatusFilePath}}") + ")",
	"   - Below the " + bt("---") + " separator is your actual task description",
	"3. **IMMEDIATELY** acknowledge by updating the task JSON file. This must happen within 30 seconds or the system assumes delivery failed. Use one of these methods (Acknowledgment Protocol, in order of preference):",
	"",
	"   **Method A \u2014 Edit tool (preferred):** Read the task JSON file with the Read tool, then use the Edit tool to change " + bt(`"status": "dispatched"`) + " to " + bt(`"status": "in_progress"`) + " and update the " + bt(`"updated_at"`) + " timestamp.",
	"",
	"   **Method B \u2014 jq (if available):**",
	"   ```bash",
	"   TASK_FILE=\"<Task File path from prompt header>\"",
	"   jq --arg ts \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\" '.status = \"in_progress\" | .updated_at = $ts' \"$TASK_FILE\" > \"${TASK_FILE}.tmp\" && mv \"${TASK_FILE}.tmp\" \"$TASK_FILE\"",
	"   ```",
	"",
	"   **Method C \u2014 Python one-liner (fallback):**",
	"   ```bash",
	"   TASK_FILE=\"<Task File path from prompt header>\"",
	"   python3 -c \"",
	"   import json, datetime",
	"   with open('$TASK_FILE') as f: d = json.load(f)",
	"   d['status'] = 'in_progress'",
	"   d['updated_at'] = datetime.datetime.utcnow().strftime('%Y-%m-%dT%H:%M:%SZ')",
	"   with open('${TASK_FILE}.tmp', 'w') as f: json.dump(d, f, indent=2)",
	"   \" && mv \"${TASK_FILE}.tmp\" \"$TASK_FILE\"",
	"   ```",
	"",
	"   **Important:** Do NOT use " + bt("sed") + " on JSON files \u2014 the spacing in JSON output varies and sed patterns will silently fail to match. Always use a JSON-aware tool (Edit, jq, or Python).",
	"4. Update your status file to " + bt("\"working\"") + ":",
	"   ```bash",
	"   echo '{\"state\":\"working\",\"last_task\":\"<Task ID>\",\"timestamp\":\"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'\"}' > \"{{.StatusFilePath}}.tmp\" && mv \"{{.StatusFilePath}}.tmp\" \"{{.StatusFilePath}}\"",
	"   ```",
	"5. Execute the task as described in the prompt",
	"",
	"### Task Completion Protocol",
	"",
	"When you finish a task:",
	"",
	"1. **Commit all changes** to your current branch with a descriptive commit message.",
	"2. **Write a result file** to " + bt("{{.ConductorDir}}/results/<task-id>.md") + ":",
	"   ```markdown",
	"   # Task Result: <task-id>",
	"   ",
	"   ## Summary",
	"   <One paragraph describing what was done>",
	"   ",
	"   ## Changes",
	"   - " + bt("path/to/file.ts") + " \u2014 <what changed>",
	"   ",
	"   ## Branch",
	"   <your current branch name>",
	"   ",
	"   ## Commit",
	"   <commit SHA> \u2014 \"<commit message>\"",
	"   ",
	"   ## Notes",
	"   <Any concerns, follow-ups, or assumptions>",
	"   ```",
	"3. **Update the task JSON** \u2014 change " + bt("status") + " to " + bt(`"completed"`) + ", set " + bt("completed_at") + " to the current timestamp, update " + bt("updated_at") + ". Use the same JSON-safe method as the acknowledgment step (Edit tool, jq, or Python \u2014 never sed):",
	"   ```bash",
	"   TASK_FILE=\"<Task File path>\"",
	"   jq --arg ts \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\" '.status = \"completed\" | .completed_at = $ts | .updated_at = $ts' \"$TASK_FILE\" > \"${TASK_FILE}.tmp\" && mv \"${TASK_FILE}.tmp\" \"$TASK_FILE\"",
	"   ```",
	"4. **Update your status file**:",
	"   ```bash",
	"   echo '{\"state\":\"idle\",\"last_task\":\"<task-id>\",\"timestamp\":\"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'\"}' > \"{{.StatusFilePath}}.tmp\" && mv \"{{.StatusFilePath}}.tmp\" \"{{.StatusFilePath}}\"",
	"   ```",
	"",
	"### Error Handling",
	"",
	"If you encounter an error you cannot resolve:",
	"",
	"1. **Update the task JSON** \u2014 change " + bt("status") + " to " + bt(`"failed"`) + ", set " + bt("error") + " to a description, update " + bt("updated_at") + ". Use a JSON-safe method (Edit tool, jq, or Python \u2014 never sed):",
	"   ```bash",
	"   TASK_FILE=\"<Task File path>\"",
	"   jq --arg ts \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\" --arg err \"<description of what went wrong>\" '.status = \"failed\" | .error = $err | .updated_at = $ts' \"$TASK_FILE\" > \"${TASK_FILE}.tmp\" && mv \"${TASK_FILE}.tmp\" \"$TASK_FILE\"",
	"   ```",
	"2. **Update your status file** to idle:",
	"   ```bash",
	"   echo '{\"state\":\"idle\",\"last_task\":\"<task-id>\",\"timestamp\":\"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'\"}' > \"{{.StatusFilePath}}.tmp\" && mv \"{{.StatusFilePath}}.tmp\" \"{{.StatusFilePath}}\"",
	"   ```",
	"3. Do NOT continue working. Wait for new instructions.",
	"",
	"### Rate Limit Handling",
	"",
	"If you hit a rate limit, update your status file:",
	"```bash",
	"echo '{\"state\":\"rate_limited\",\"last_task\":\"<task-id>\",\"timestamp\":\"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'\"}' > \"{{.StatusFilePath}}.tmp\" && mv \"{{.StatusFilePath}}.tmp\" \"{{.StatusFilePath}}\"",
	"```",
	"",
	"### File Write Protocol",
	"",
	"When updating JSON files:",
	"- **Use JSON-aware tools** \u2014 the Edit tool, " + bt("jq") + ", or Python. Never use " + bt("sed") + " on JSON (spacing varies and sed patterns will silently fail).",
	"- **Always write atomically** \u2014 write to a " + bt(".tmp") + " file, then " + bt("mv") + " to the final path:",
	"  ```bash",
	"  jq '.field = \"value\"' file.json > file.json.tmp && mv file.json.tmp file.json",
	"  ```",
	"- For the status file (simple, small), a full overwrite with " + bt("echo") + " is acceptable since you control the entire content.",
	"",
	"### Guidelines",
	"",
	"- Stay focused on the assigned task. Do not modify files outside the task scope.",
	"- Commit frequently with clear, descriptive messages.",
	"- If the task is ambiguous, do your best interpretation and note assumptions in the result file.",
	"- Do not push to remote \u2014 the user handles that from the TUI.",
	"- Do not modify any files in the " + bt(".claude/") + " directory.",
	"",
	"### Your Identity",
	"",
	"- Instance name: **{{.InstanceName}}**",
	"- Account: **{{.AccountName}}**",
	"- Status file: " + bt("{{.StatusFilePath}}"),
	"",
	"<!-- END CONDUCTOR WORKER -->",
}, "\n")

// GenerateCLAUDEMD writes a CLAUDE.md file into <worktreePath>/.claude/
// using the orchestrator or worker template based on ctx.Role.
func GenerateCLAUDEMD(worktreePath string, ctx CLAUDEMDContext) error {
	var tmplText string
	switch ctx.Role {
	case string(RoleOrchestrator):
		tmplText = orchestratorTemplate
	case string(RoleWorker):
		tmplText = workerTemplate
	default:
		return fmt.Errorf("unknown role %q: must be %q or %q", ctx.Role, RoleOrchestrator, RoleWorker)
	}

	tmpl, err := template.New("claude.md").Parse(tmplText)
	if err != nil {
		return fmt.Errorf("failed to parse CLAUDE.md template: %w", err)
	}

	destDir := filepath.Join(worktreePath, ".claude")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w", err)
	}

	destPath := filepath.Join(destDir, "CLAUDE.md")

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return fmt.Errorf("failed to render CLAUDE.md template: %w", err)
	}

	tmp := destPath + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write CLAUDE.md: %w", err)
	}
	if err := os.Rename(tmp, destPath); err != nil {
		os.Remove(tmp) // clean up
		return fmt.Errorf("failed to finalize CLAUDE.md: %w", err)
	}

	return nil
}
