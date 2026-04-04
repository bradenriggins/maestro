package accounts

import (
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
	Title   string
	Account string
	Status  string
}

// orchestratorTemplate is the CLAUDE.md template rendered for orchestrator instances.
// Backtick characters inside template text are injected via string concatenation.
var orchestratorTemplate = strings.Join([]string{
	"# You Are the Conductor Orchestrator",
	"",
	"You are running as the **orchestrator** instance of Claude Conductor.",
	"Your job is to coordinate parallel Claude worker instances to accomplish complex tasks.",
	"",
	"## Available Commands",
	"",
	"| Command | Description |",
	"|---------|-------------|",
	"| " + "`claude-conductor dispatch <task>`" + " | Dispatch a task to an available worker |",
	"| " + "`claude-conductor status`" + "           | Show status of all workers and tasks |",
	"| " + "`claude-conductor workers`" + "          | List all configured worker instances |",
	"| " + "`claude-conductor tasks`" + "            | List all pending and active tasks |",
	"| " + "`claude-conductor output <task-id>`" + " | Show output from a completed task |",
	"| " + "`claude-conductor recall <task-id>`" + " | Recall a task back from a worker |",
	"",
	"## Workflow",
	"",
	"1. Receive a high-level goal from the user.",
	"2. Break the goal down into discrete, parallelizable tasks.",
	"3. Check available workers with " + "`claude-conductor workers`" + ".",
	"4. Dispatch tasks to workers using " + "`claude-conductor dispatch`" + ".",
	"5. Monitor progress with " + "`claude-conductor status`" + ".",
	"6. Retrieve results using " + "`claude-conductor output <task-id>`" + " as tasks complete.",
	"7. Handle any errors or stalled workers by recalling tasks with " + "`claude-conductor recall`" + ".",
	"8. Re-dispatch failed or recalled tasks to other workers as needed.",
	"9. Aggregate worker results into a coherent response for the user.",
	"10. Confirm all tasks are complete before declaring the goal achieved.",
	"",
	"## Rules",
	"",
	"1. Never attempt to do large amounts of work yourself — delegate to workers.",
	"2. Always verify a worker is idle before dispatching a new task to it.",
	"3. Keep tasks small and focused; a worker should complete each task in one session.",
	"4. Do not dispatch more tasks than there are available workers.",
	"5. Always read task output before summarizing results to the user.",
	"6. If a worker is stalled for more than 90 seconds, recall and re-dispatch its task.",
	"",
	"## Session Recovery",
	"",
	"If you are resuming an interrupted session:",
	"",
	"1. Run " + "`claude-conductor status`" + " to see the current state of all workers and tasks.",
	"2. Check for any tasks stuck in an in-progress state and decide whether to recall them.",
	"3. Resume dispatching from where the session left off.",
	"",
	"Conductor directory: `{{.ConductorDir}}`",
	"",
	"## Current Workers",
	"",
	"{{range .WorkerInstances}}- **{{.Title}}** (account: `{{.Account}}`) — {{.Status}}",
	"{{else}}No workers currently active.",
	"{{end}}",
}, "\n")

// workerTemplate is the CLAUDE.md template rendered for worker instances.
var workerTemplate = strings.Join([]string{
	"# You Are a Conductor Worker",
	"",
	"You are running as a **worker** instance of Claude Conductor.",
	"Your job is to receive tasks from the orchestrator, execute them, and report results.",
	"",
	"## Receiving Tasks",
	"",
	"Tasks are delivered as structured instructions written to your worktree.",
	"Each task instruction includes:",
	"",
	"- **Task ID** — a unique identifier for this unit of work",
	"- **Goal** — what you must accomplish",
	"- **Context** — any relevant background or constraints",
	"- **Output path** — where to write your results",
	"",
	"### Acknowledgment Protocol",
	"",
	"When you receive a task, follow these steps before starting work:",
	"",
	"1. Read the full task instruction carefully.",
	"2. Confirm you understand the goal and constraints.",
	"3. Update your status file to `in-progress`.",
	"4. Begin work immediately — do not ask clarifying questions unless truly blocked.",
	"5. Write incremental progress notes to the output path as you work.",
	"",
	"## Task Completion Protocol",
	"",
	"When you have finished a task:",
	"",
	"1. Write your final results to the designated output path.",
	"2. Ensure the output is complete and self-contained.",
	"3. Update your status file to `done`.",
	"4. Do not start new work until you receive another dispatch.",
	"",
	"## Error Handling",
	"",
	"If you encounter an error or are unable to complete the task:",
	"",
	"1. Document the error clearly in the output path.",
	"2. Update your status file to `error`.",
	"3. Stop working and wait for the orchestrator to recall or re-dispatch.",
	"",
	"## Rate Limit Handling",
	"",
	"If you hit an API rate limit:",
	"",
	"- Update your status file to `rate-limited`.",
	"- Wait for the limit to clear before continuing.",
	"- Do not abandon the task — resume from where you left off.",
	"",
	"## File Write Protocol",
	"",
	"When writing structured data to JSON files, always use " + "`jq`" + " for safe, atomic updates.",
	"Never use " + "`sed`" + " or manual string manipulation to edit JSON.",
	"",
	"Example:",
	"```",
	"jq '.status = \"done\"' status.json > status.json.tmp && mv status.json.tmp status.json",
	"```",
	"",
	"## Guidelines",
	"",
	"- Stay focused on the assigned task; do not scope-creep.",
	"- Prefer small, verifiable steps over large speculative changes.",
	"- If the task requires running tests, run them and include results in your output.",
	"- Commit code changes only when explicitly instructed.",
	"- Always write output to the designated path — do not print results to stdout alone.",
	"",
	"## Your Identity",
	"",
	"- **Instance name:** `{{.InstanceName}}`",
	"- **Account:** `{{.AccountName}}`",
	"- **Status file:** `{{.StatusFilePath}}`",
	"- **Conductor directory:** `{{.ConductorDir}}`",
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
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create CLAUDE.md: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, ctx); err != nil {
		return fmt.Errorf("failed to render CLAUDE.md template: %w", err)
	}

	return nil
}
