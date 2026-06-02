# Maestro

> Run multiple Claude Code and Codex CLI agents in parallel — across separate accounts — from a single terminal dashboard.

Maestro solves the rate-limit wall. When you're running several AI coding tasks at once, a single Anthropic or OpenAI account runs out of headroom fast. Maestro reads each account's real-time usage data, routes each task to the account with the most capacity, and manages all of it from one BubbleTea TUI. Workers get their own tmux sessions and git worktrees; the orchestrator sees everything.

> Maestro is built on [Claude Squad](https://github.com/smtg-ai/claude-squad) (AGPL-3.0) and extends it with multi-account credential isolation, usage-aware routing, model-aware dispatch, and Codex CLI support. See [Attribution](#attribution) for details.

## Features

- **Multi-account credential isolation** — each account runs in its own `CLAUDE_CONFIG_DIR` or `CODEX_HOME`, injected per tmux session so credentials never bleed across workers
- **Usage tracking and routing** — reads Claude Code `statusLine` data and Codex session JSONL files to track each account's 5-hour rolling usage, then routes tasks to the account with the most capacity
- **Model-aware dispatch** — uses the program-specific model defaults and available model families defined in code, then picks the right worker for each task type
- **Claude Code and Codex side by side** — both programs can be orchestrator or worker; mix and match freely
- **tmux session isolation** — each worker lives in a dedicated tmux session (`maestro_<name>`), so you can attach, inspect, and detach at any time
- **Git worktree isolation** — each worker gets its own branch and worktree, keeping in-progress changes separate
- **File-based IPC** — tasks, results, and worker status live in `~/.maestro/` as plain JSON files; no external services required
- **TUI dashboard** — BubbleTea interface shows all workers, tasks, diffs, and logs in one view

## How It Works

```
~/.maestro/
  config.json          <- Account definitions (name, program, role, config dir)
  registry.json        <- Live instance -> tmux session mapping
  accounts/{name}/     <- Per-account CLAUDE_CONFIG_DIR or CODEX_HOME
  tasks/*.json         <- Task dispatch files (IPC)
  status/*.json        <- Worker status files (idle / busy)
  results/*.md         <- Task output written by workers
  usage/*.json         <- Real-time usage from statusLine / JSONL
```

**Task dispatch:** `maestro dispatch <worker> "<prompt>"` writes a task file and sends a tmux instruction to the worker. The worker's instructions file (`CLAUDE.md` for Claude Code or `AGENTS.md` for Codex) tells it to read the file, acknowledge within 30 seconds, execute the task, write a result file, and set its status back to idle.

**Usage tracking:** During setup, each Claude Code account's `settings.json` gets a `statusLine` command that pipes rate limit data (five-hour and seven-day usage percentages) to a receiver script, which writes to `~/.maestro/usage/<account>.json`. Codex usage is tracked from session JSONL files.

**Routing:** `maestro dispatch <worker> --schedule` calls `CollectUsage`, ranks accounts by available headroom, and dispatches to the worker backing the account with the most capacity — waiting for a rate-limit reset first if that's faster (bounded by `--max-wait`). Plain `maestro dispatch <worker>` targets the named worker directly; the TUI and `maestro status` also surface routing advice (which account currently has the most headroom).

**Reconciliation:** The TUI reconciles state every 5 seconds — it checks that tmux sessions are alive, transitions stale in-progress tasks to failed if the worker is dead, and corrects status files when they diverge from actual task state.

## Prerequisites

- [tmux](https://github.com/tmux/tmux)
- [gh](https://cli.github.com) (GitHub CLI — used for branch push / PR creation)
- [jq](https://jqlang.github.io/jq/)
- [Claude Code](https://claude.ai/download) and/or [Codex CLI](https://github.com/openai/codex)

Maestro must be run from within a git repository.

## Installation

Prerequisites for local builds:

- Go 1.23+ (toolchain pinned to 1.24.1)
- tmux
- gh
- jq

**curl | bash (recommended):**

```bash
curl -fsSL https://raw.githubusercontent.com/bradenriggins/maestro/main/install.sh | bash
```

The installer detects your platform and architecture, downloads the correct binary, and adds it to `~/.local/bin`.

**Build from source:**

```bash
git clone https://github.com/bradenriggins/maestro.git
cd maestro
go build -o maestro ./
sudo mv maestro /usr/local/bin/
```

## Setup

Run the interactive setup wizard once per machine:

```bash
maestro setup
```

This walks you through adding one or more accounts, logging into Claude Code or Codex under each account's isolated config directory, and configuring roles (orchestrator / worker). Account config is written to `~/.maestro/config.json`.

To add more accounts later:

```bash
maestro add-account
```

## Automated Testing

Maestro now includes a layered automation harness so you can test CLI behavior and core TUI flows without manual input.

```bash
make test       # unit/package tests
make test-race  # race detector
make smoke      # isolated CLI smoke tests in a temp HOME
make e2e        # automated TUI flow tests
make ui-audit   # UX/interaction audit report + artifacts
make ui-snapshots # snapshot regression checks for major UI states
make ci         # full local validation pass
```

### What the automation covers

- **Unit/integration logic** via `go test ./...`
- **Concurrency issues** via `go test -race ./...`
- **CLI sanity checks** for commands like `version`, `debug`, `doctor`, and setup-required failure paths
- **Headless TUI interaction tests** that drive the Bubble Tea model with synthetic keypresses and assert rendered output for:
  - empty state
  - help overlay + dismiss affordance
  - prompt flow + cancellation
  - orchestration / log viewer / quick-dispatch toggles
  - review-to-redispatch transition
  - clean state restoration after cancellation or nil-overlay drift
- **UX audit artifacts** via `make ui-audit`, which writes machine-readable test output and a markdown summary under `artifacts/ui-audit/`
- **Snapshot regression coverage** via `make ui-snapshots`, which locks major UI states to checked-in baseline renders under `app/testdata/ui_snapshots/`

### Current limit

This gives you real automation for logic and interaction flow, but subjective UX quality still needs richer heuristics over time. The next layer after this is broader scenario coverage, snapshot-style render assertions, and failure-recovery scoring for orchestration workflows.

## CLI Reference

| Command | Description |
|---------|-------------|
| `maestro` | Open the TUI dashboard (must be run from a git repo) |
| `maestro setup` | Interactive wizard to configure accounts |
| `maestro add-account` | Add a new account to an existing configuration |
| `maestro dispatch <worker> [prompt]` | Dispatch a task to a named worker instance |
| `maestro workers` | List all registered workers and their status |
| `maestro status [worker]` | Show worker and task status (optionally filtered to one worker) |
| `maestro usage` | Show 5-hour rolling usage levels for all configured accounts |
| `maestro tasks [--status <filter>]` | List all tasks, optionally filtered by status |
| `maestro output <worker> [--lines N]` | Capture recent terminal output from a worker (default 50 lines) |
| `maestro recall <worker>` | Save the full terminal scrollback from a worker to a file |
| `maestro retry-failed [--max N] [--worker name]` | Retry all failed and timed-out tasks across idle workers |
| `maestro doctor` | Diagnose state issues (config, registry, tasks, binaries, permissions) |
| `maestro pipeline <file.yaml>` | Execute a task pipeline from a YAML definition |
| `maestro clean` | Archive completed/failed tasks and remove capture files |
| `maestro reset` | Destroy all instances, tasks, registry, and tmux sessions |
| `maestro debug` | Print config path and parsed config JSON |
| `maestro version` | Print the version number |

### dispatch flags

```
--task <id>         Re-dispatch an existing task by ID instead of creating a new one
--after <id,...>    Task IDs that must complete before this task runs
--schedule          Enable throughput-optimal scheduling (waits for rate-limit resets if beneficial)
--duration <min>    Estimated task duration in minutes (used by scheduler)
--max-wait <min>    Maximum wait time in minutes for a worker reset (default: 120)
```

### tasks flags

```
--status <filter>    Filter by status: dispatched, in_progress, completed, failed
```

### retry-failed flags

```
--max <n>        Maximum number of tasks to retry (default: all)
--worker <name>  Force all retries to a specific worker
```

## TUI Key Reference

### Managing sessions

| Key | Action |
|-----|--------|
| `n` | Create a new session (account picker) |
| `N` | Create a new session with a prompt |
| `x` | Kill (delete) the selected session |
| `up` / `j`, `down` / `k` | Navigate between sessions |
| `Enter` | Attach to the selected session |
| `Ctrl-Q` | Detach from the current session |

### Handoff

| Key | Action |
|-----|--------|
| `p` | Pause: commit changes and pause session |
| `P` | Commit and push branch to GitHub |
| `r` | Resume a paused session |

### Orchestration

| Key | Action |
|-----|--------|
| `o` | Orchestration overlay (workers, tasks, usage) |
| `/` | Quick-dispatch palette |
| `d` | Show diff tab |
| `l` | Log viewer |
| `h` | Task history |
| `f` | Toggle preview mode |

### Other

| Key | Action |
|-----|--------|
| `Tab` | Switch between preview, diff, and terminal tabs |
| `Shift-down` / `Shift-up` | Scroll in preview / diff / terminal view |
| `?` | Help |
| `q` | Quit (saves session state for resume) |

## Attribution

Maestro is based on [Claude Squad](https://github.com/smtg-ai/claude-squad) by smtg-ai and is licensed under the GNU Affero General Public License v3.0 (AGPL-3.0). Substantial modifications have been made, including multi-account credential management, usage tracking and routing, model-aware task dispatch, and Codex CLI integration.

See [NOTICE](NOTICE) for full attribution details.

## License

GNU Affero General Public License v3.0. See [LICENSE.md](LICENSE.md) for the full text.
