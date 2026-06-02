# Known Issues

Issues identified during the bug-hunt audit that were **intentionally deferred**
rather than fixed, with the reasoning and a suggested approach for each. They're
ordered roughly by value. Contributions welcome — each entry names the file(s)
to start from.

The critical and high-priority bugs from that audit have been fixed; see
[CHANGELOG.md](../CHANGELOG.md).

---

## Deferred

### 1. Terminal-tab preview captures on the UI thread (perf)
**Where:** `app/app.go` (`instanceChanged` → `tabbedWindow.UpdateTerminal`),
`ui/terminal.go`, `session/tmux/tmux.go` (`CapturePaneContent`)

When the Terminal tab is active, `instanceChanged()` synchronously runs
`tmux capture-pane` (a subprocess) on the Bubble Tea main loop, and the preview
tick re-fires it ~10×/sec. That spawns a tmux process on the UI goroutine
several times a second, causing keypress lag while the Terminal tab is open.

**Why deferred:** the fix moves terminal capture off the main loop (the way the
*preview* pane is already handled, via `tickUpdateMetadataCmd` →
`metadataUpdateDoneMsg`). That touches the render/update flow and needs
interactive validation on a real TTY, which the automated suite can't provide.

**Suggested fix:** capture terminal content in the background metadata goroutine
and deliver it via a message applied in `Update()`, or throttle terminal capture
to the slower metadata cadence instead of the 100 ms preview tick.

### 2. `branch_prefix` in the conductor config is a dead knob
**Where:** `session/git/worktree.go` (`NewGitWorktree`), `config/config.go`,
`pkg/accounts/config.go`

Worktree branch names are built from `config.Config.BranchPrefix` (default
`<username>/`). The conductor config (`pkg/accounts/ConductorConfig`) *also* has
a `BranchPrefix` (default `maestro/`) that `maestro setup` writes — but the
worktree code never reads it. So editing `branch_prefix` in the conductor config
has no effect, and branches are named `<username>/<title>` instead of
`maestro/<title>`.

**Why deferred:** there are two competing config structs and picking a single
source of truth changes branch-naming for existing users (their current branches
are `<username>/...`). Wanted an explicit decision before changing naming
behavior.

**Suggested fix:** choose one source of truth — thread
`ConductorConfig.BranchPrefix` into `NewGitWorktree`, or have
`config.LoadConfig()` default its `BranchPrefix` from the conductor config — and
remove the now-dead field from the other struct.

### 3. Task-duration learning is inert (scheduler uses a flat default)
**Where:** `pkg/orchestration/duration_stats.go`,
`pkg/orchestration/scheduler.go` (`ResolveTaskDuration`)

`UpdateDurationStats` has no production caller, so `duration_stats.json` is never
written, and `ResolveTaskDuration` always falls back to the 15-minute default
(`DefaultTaskDurationMin`) unless `--duration` is passed. The scheduler's
message-consumption projection is therefore always computed from a flat
assumption. The `--duration` flag itself works.

**Why deferred:** wiring this end-to-end needs an idempotent "task just
completed" hook (so a duration isn't recorded twice across reconcile passes),
which means a new `Task` field and a write at the completion-detection point —
feature work beyond a surgical bug fix. The scheduler still functions on the
default; nothing claims duration-learning works, so it's not a correctness or
honesty issue.

**Suggested fix:** add a `DurationRecorded` flag to `Task`; in the reconcile
completion path, call `UpdateDurationStats(task)` once when a terminal
`completed` task with both timestamps is first observed; load the stats in
`RunScheduledDispatch` and pass them to `ResolveTaskDuration`, keyed by account.

---

## Lower-priority / smaller items

These are real but minor; listed so they're not lost.

- **Conflict detection misses untracked files.** `getModifiedFiles`
  (`pkg/orchestration/conflicts.go`) only checks `git diff` (tracked) and
  `--cached` (staged); two workers each *creating* the same new file aren't
  flagged. Use `git status --porcelain` instead, and log (don't silently skip)
  when a worktree's git query errors.

- **Codex usage only reads the primary rate-limit window.**
  `pkg/orchestration/usage_codex.go` parses only `RateLimits.Primary`, so an
  account near its weekly/secondary cap looks fresh and gets routed work that
  then fails. Add a `Secondary` field and route on
  `max(primary, secondary)`.

- **`*.tmp` files accumulate in `~/.maestro/`.** Atomic writes leave orphaned
  `*.tmp` files if a process crashes mid-write (`pkg/orchestration/ipc.go`). Add
  startup cleanup of stale `*.tmp` in the tasks/results/status dirs. (The
  worker-protocol temp name is also a fixed `${TASK_FILE}.tmp` — make it
  pid-suffixed.)

- **`TaskStore.List` silently drops corrupt task files.**
  `pkg/orchestration/taskstore.go` `continue`s on any `Get` error, hiding
  genuine JSON corruption (the task just vanishes from every view). Distinguish
  `os.IsNotExist` (benign) from a parse error (log a warning).

- **tmux session-name sanitization can collapse distinct titles.**
  `toMaestroTmuxName` (`session/tmux/tmux.go`) maps `foo.bar`, `foo:bar`,
  `foo#bar` to the same `maestro_foo_bar`. Title uniqueness is now enforced on
  exact match (`app/keys.go`), but a punctuation-only collision can still slip
  through; unconditionally suffix a short hash of the original title.

- **Diff base for existing-branch worktrees uses `HEAD`.**
  `setupFromExistingBranch` (`session/git/worktree_ops.go`) never sets
  `baseCommitSHA`, so `Diff()` shows only the delta since the last commit and
  under-counts untracked files. Record a base SHA at checkout.

- **Locale-dependent git error matching.** `worktree_ops.go` matches English
  substrings like `"not found"` / `"ambiguous argument 'HEAD'"`; these break
  under non-English `LC_ALL`. Test exit status (e.g.
  `git rev-parse --verify HEAD`) or run git with `LC_ALL=C`.

- **Dead overlay code.** `stateReview` / `stateQuickDispatch` and their
  overlays (`ui/overlay/review.go`, `ui/overlay/quickDispatch.go`) are
  unreachable — the workflow-panel system superseded them. Safe to delete.

- **Notification bundling drops messages.** `pkg/notify/macos.go` discards any
  notification within 5 s of the last instead of coalescing, so bursts lose all
  but the first. (Urgent failure notifications bypass this and are fine.)

---

## Repository housekeeping

- `docs/superpowers/specs/2026-04-08-maestro-tui-cleanup-design.md` is a
  leftover internal design spec unrelated to the public tool; consider removing.
- The `.worktrees/` directory in a local checkout holds stale duplicate copies
  of the source created by maestro itself; it's gitignored and should not be
  committed.
- `app/ui_audit_test.go` uses `testing.Chdir` (Go 1.24 API) while `go.mod`
  declares `go 1.23.0`; `go vet` warns. Bump the `go` directive to 1.24 (the
  toolchain is already pinned to 1.24.1) or use `os.Chdir` with cleanup.
