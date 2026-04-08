package accounts

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// agentsOrchestratorTemplate is the AGENTS.md template rendered for orchestrator instances
// running under Codex. Same workflow as CLAUDE.md but with generic language and no
// subagent-spawning section (Codex does not support inline subagents).
var agentsOrchestratorTemplate = strings.Join([]string{
	"<!-- CONDUCTOR ORCHESTRATOR v1 — AUTO-GENERATED, DO NOT EDIT -->",
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
	"- Delegate implementation work to workers whenever possible — preserve your context window for coordination.",
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
	"| All workers ≥ 75% AND your usage < 40% | Handle the task directly |",
	"| All workers ≥ 75% AND your usage ≥ 40% | Wait, then retry; tell user if prolonged |",
	"",
	"## Model-Aware Routing",
	"",
	"Your workers run different AI models with different strengths. Consider model fit when dispatching:",
	"",
	"### Current Worker Models",
	"{{- range .WorkerInstances}}",
	"- **{{.Title}}** [{{.Account}}] — {{.Program}}/{{.Model}} — Best at: {{.ModelStrengths}}",
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
	"{{range .WorkerInstances}}- **{{.Title}}** [{{.Account}}] — {{.Status}}",
	"{{else}}No workers currently active. Ask the user to create worker instances from the TUI.",
	"{{end}}",
	"",
	"<!-- END CONDUCTOR ORCHESTRATOR -->",
}, "\n")

// agentsWorkerTemplate is the AGENTS.md template rendered for worker instances
// running under Codex. Same task protocol as the CLAUDE.md worker template but
// with generic language and AGENTS.md references.
var agentsWorkerTemplate = strings.Join([]string{
	"<!-- CONDUCTOR WORKER v1 — AUTO-GENERATED, DO NOT EDIT -->",
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
	"   - " + bt("Task ID:") + " — the task identifier",
	"   - " + bt("Task File:") + " — path to the task JSON file",
	"   - " + bt("Result File:") + " — where to write your result",
	"   - " + bt("Status File:") + " — your status file path (same as " + bt("{{.StatusFilePath}}") + ")",
	"   - Below the " + bt("---") + " separator is your actual task description",
	"3. **IMMEDIATELY** acknowledge by updating the task JSON file. This must happen within 30 seconds or the system assumes delivery failed. Use one of these methods (Acknowledgment Protocol, in order of preference):",
	"",
	"   **Method A — jq (preferred):**",
	"   ```bash",
	"   TASK_FILE=\"<Task File path from prompt header>\"",
	"   jq --arg ts \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\" '.status = \"in_progress\" | .updated_at = $ts' \"$TASK_FILE\" > \"${TASK_FILE}.tmp\" && mv \"${TASK_FILE}.tmp\" \"$TASK_FILE\"",
	"   ```",
	"",
	"   **Method B — Python one-liner (fallback):**",
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
	"   **Important:** Do NOT use " + bt("sed") + " on JSON files — the spacing in JSON output varies and sed patterns will silently fail to match. Always use a JSON-aware tool (jq or Python).",
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
	"   - " + bt("path/to/file.ts") + " — <what changed>",
	"   ",
	"   ## Branch",
	"   <your current branch name>",
	"   ",
	"   ## Commit",
	"   <commit SHA> — \"<commit message>\"",
	"   ",
	"   ## Notes",
	"   <Any concerns, follow-ups, or assumptions>",
	"   ```",
	"3. **Update the task JSON** — change " + bt("status") + " to " + bt(`"completed"`) + ", set " + bt("completed_at") + " to the current timestamp, update " + bt("updated_at") + ". Use a JSON-safe method (jq or Python — never sed):",
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
	"1. **Update the task JSON** — change " + bt("status") + " to " + bt(`"failed"`) + ", set " + bt("error") + " to a description, update " + bt("updated_at") + ". Use a JSON-safe method (jq or Python — never sed):",
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
	"- **Use JSON-aware tools** — " + bt("jq") + " or Python. Never use " + bt("sed") + " on JSON (spacing varies and sed patterns will silently fail).",
	"- **Always write atomically** — write to a " + bt(".tmp") + " file, then " + bt("mv") + " to the final path:",
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
	"- Do not push to remote — the user handles that from the TUI.",
	"- Do not modify the AGENTS.md file.",
	"",
	"### Your Identity",
	"",
	"- Instance name: **{{.InstanceName}}**",
	"- Account: **{{.AccountName}}**",
	"- Status file: " + bt("{{.StatusFilePath}}"),
	"",
	"<!-- END CONDUCTOR WORKER -->",
}, "\n")

// GenerateAGENTSMD writes an AGENTS.md file into <worktreePath>/
// using the orchestrator or worker template based on ctx.Role.
// Unlike CLAUDE.md (which goes into .claude/), AGENTS.md is written
// to the repository root because Codex reads it from there.
func GenerateAGENTSMD(worktreePath string, ctx CLAUDEMDContext) error {
	var tmplText string
	switch ctx.Role {
	case string(RoleOrchestrator):
		tmplText = agentsOrchestratorTemplate
	case string(RoleWorker):
		tmplText = agentsWorkerTemplate
	default:
		return fmt.Errorf("unknown role %q: must be %q or %q", ctx.Role, RoleOrchestrator, RoleWorker)
	}

	tmpl, err := template.New("agents.md").Parse(tmplText)
	if err != nil {
		return fmt.Errorf("failed to parse AGENTS.md template: %w", err)
	}

	// AGENTS.md goes in the repo root, not a subdirectory.
	destPath := filepath.Join(worktreePath, "AGENTS.md")

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return fmt.Errorf("failed to render AGENTS.md template: %w", err)
	}

	tmp := destPath + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write AGENTS.md: %w", err)
	}
	if err := os.Rename(tmp, destPath); err != nil {
		os.Remove(tmp) // clean up
		return fmt.Errorf("failed to finalize AGENTS.md: %w", err)
	}

	return nil
}
