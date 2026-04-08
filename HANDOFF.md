# Maestro — Session Handoff

**Date:** 2026-04-04
**Tag:** v0.5.1
**Status:** Historical handoff; use `README.md`, CLI help, and the current code for live state

---

## What This Is

A multi-account orchestration tool for Claude Code and Codex. The orchestrator decomposes work and delegates tasks to worker accounts via file-based IPC.

Based on the original Claude Squad codebase (Go, BubbleTea, tmux, git worktrees).

## Project Location

Repository root.

## Key Files

| File | Purpose |
|------|---------|
| `README.md` | User-facing overview and setup |
| `main.go` | CLI entrypoint and command wiring |
| `app/app.go` | Bubble Tea TUI state machine |
| `pkg/accounts/` | Account config, setup, and instruction templates |
| `pkg/orchestration/` | Task store, dispatch, reconciliation, and usage routing |
| `session/` | Instance lifecycle, git worktrees, and tmux integration |
| `ui/` | List, menu, and overlays |

## Architecture

```
~/.maestro/                    <- Runtime data (created by `setup`)
  config.json                  <- Account definitions
  registry.json                <- Live instance -> tmux mapping
  session.json                 <- Session resume state
  accounts/{name}/             <- Per-account CLAUDE_CONFIG_DIR or CODEX_HOME
  tasks/*.json, *.prompt       <- Task files (IPC)
  status/*.json                <- Worker status files
  results/*.md                 <- Task results
  usage/*.json                 <- Real-time usage from statusLine
  bin/*.sh                     <- Shell script wrappers

repo root/                    <- Source code
  main.go                      <- Cobra CLI
  app/app.go                   <- Bubble Tea TUI
  pkg/accounts/                <- Account config, setup wizard, instruction file templates
  pkg/orchestration/           <- Task store, registry, dispatch, reconcile, usage
  pkg/notify/                  <- macOS notifications
  session/                     <- Instance model, tmux, git worktrees
  ui/                          <- List, menu, overlays
  keys/                        <- Keybindings
```

## How Auth Works

Each account gets an isolated `CLAUDE_CONFIG_DIR` or `CODEX_HOME` under `~/.maestro/accounts/{name}/`. OAuth tokens are stored in the macOS Keychain, namespaced by `sha256(config_dir)[:8]`. tmux sessions inject the env var via `tmux new-session -e CLAUDE_CONFIG_DIR=<path>`.

## CLI Commands

```
setup       <- Configure accounts (OAuth login per account)
add-account <- Add another account to an existing configuration
dispatch    <- Send task to worker (creates task files, sends tmux instruction, polls for ack)
status      <- Show workers + tasks + usage
workers     <- List active workers
tasks       <- List/filter tasks
output      <- Capture worker terminal (last N lines)
recall      <- Save full worker scrollback to file
usage       <- Show per-account usage percentages
doctor      <- Diagnose state issues
clean       <- Archive old tasks, remove captures
```

## TUI Keybindings

```
n           <- New instance (with account picker)
Enter       <- Attach to instance (Ctrl+Q to detach)
p           <- Pause instance
x           <- Kill instance
d           <- Show diff tab
f           <- Toggle preview mode
r           <- Resume (paused) / Review mode (completed worker)
o           <- Orchestration overlay (workers, tasks, plan, usage)
/           <- Quick-dispatch palette
l           <- Log viewer
h           <- Task history (opens orchestration overlay)
?           <- Help
q           <- Quit (saves session for resume)
```

## Orchestration Protocol

1. Orchestrator's instructions file tells it to use `maestro dispatch <worker> "<task>"`
2. Dispatch creates `tasks/<id>.json` + `tasks/<id>.prompt`, sends tmux instruction to worker
3. Worker's instructions file tells it to read the prompt file, update task JSON to `in_progress` within 30s
4. Dispatch polls for ack, retries up to 3 times
5. Worker executes task, writes `results/<id>.md`, updates task to `completed`, sets status to `idle`
6. Orchestrator reads results, reviews, integrates

## Usage Tracking

Each Claude Code account's `settings.json` gets a `statusLine` command configured during setup. Claude Code pipes real-time rate limit data (`five_hour.used_percentage`, `seven_day.used_percentage`) to the receiver script, which writes to `~/.maestro/usage/<account>.json`. The orchestrator, status command, and TUI overlay all display this data.

## Reconciliation

Every 5 seconds, the TUI:
- Checks all tmux sessions are alive (marks dead ones in registry)
- Transitions in_progress tasks on dead workers to stale -> failed (after 30s)
- Corrects status files when task state diverges from worker state
- Detects wake-from-sleep (tick gap > 30s)

## What's Been Audited

Parallel audit rounds covered:
- Type consistency across all packages
- instructions file template correctness (rewrote from scratch after first audit)
- Dispatch flow step-by-step trace against spec
- Data race detection (fixed lastReconcileTime race, registry contention)
- Env persistence across restarts
- File I/O correctness (atomic writes, TOCTOU handling, nil safety)
- Template rendering verification (backticks, jq commands, variable substitution)
- Instance lifecycle wiring (every create/start/pause/resume/kill path)
- Spec compliance (every section verified)
- Code quality (unused code, error handling, raw string constants)

## Known Remaining Items

- Wake-from-sleep TUI banner not implemented (only logs — spec Section 12.2)
- `--dry-run` flag not implemented
- Task history (`h` key) reuses orchestration overlay — no dedicated history view yet
- StatusLine JSON structure from Claude Code not yet validated against real data
- `Prompt` field on Instance is ephemeral (not persisted) — narrow crash window

## How to Build & Test

```bash
cd /path/to/repo
go build -o maestro ./
go test ./...
go vet ./...
./maestro version
```

## How to Use (First Time)

```bash
cd /path/to/repo
go build -o maestro ./
sudo cp maestro /usr/local/bin/

# Configure accounts
maestro setup

# Run in any git repo
cd ~/Projects/my-project
maestro
```

## Notes

- This handoff is a historical snapshot, not a source of truth for current file counts or version numbers.
- Prefer `README.md`, the CLI help output, and the code itself when this document and the repo diverge.
