# Changelog

All notable changes to Maestro are documented here. Format loosely follows
[Keep a Changelog](https://keepachangelog.com/); this project is pre-1.0 and
not yet versioning releases, so changes land under **Unreleased** until tagged.

## [Unreleased]

### Fixed — critical + high-priority bug-hunt sprint

A multi-subsystem audit surfaced ~20 real defects behind general instability
("buggy / didn't work well"). All fixes are surgical and the full test suite
passes under the race detector. Two regression tests were added
(`TestResolveScheduledInstance`, the config-backfill case in `TestLoadConfig`).

**Routing / scheduling**
- Scheduled dispatch (`--schedule`) was computing the best account and then
  discarding the decision — it dispatched to the originally-named instance, so
  load never actually balanced. It now resolves the chosen account to its
  backing worker and dispatches there. (`pkg/orchestration/scheduler.go`)
- After waiting for a rate-limit reset, usage is now re-collected and the
  schedule recomputed instead of dispatching blind on a stale estimate.

**State / IPC races** — task and status JSON files are written by both the TUI
and the worker process with no lock; several blind overwrites were losing
updates:
- `reconcile` re-reads a task before marking it stale/failed, so a worker's
  freshly-`completed` task is no longer clobbered and its result discarded.
  (`pkg/orchestration/reconcile.go`)
- `reconcile` re-reads a worker's status before the working→idle correction,
  preventing a busy worker from being double-dispatched.
- `updateRegistry` now merges with the existing registry, derives death from
  tmux reality, and preserves `DiedAt`, instead of rebuilding from the live UI
  list and flapping dead workers back to "running". (`app/app.go`)
- The dispatcher persists its `SendAttempts` bump via a compare-and-set
  (`TaskStore.UpdateIfStatus`) so it can't clobber the worker's `in_progress`
  write (which previously caused false "timed out" on running tasks).
- DAG dispatch gained a `MaxAttempts` ceiling and only bumps
  `Attempts`/`DispatchedAt` after the worker accepts delivery — fixing infinite
  retries and attempt-counter churn. (`pkg/orchestration/dag.go`)
- Worker-written timestamps are parsed tolerantly (`ParseISO`); strict RFC3339
  parsing was stranding tasks in `stale` and skipping staleness checks.

**Sessions / git**
- `CleanupWorktrees` no longer force-deletes worktrees belonging to *other*
  repos from the shared `~/.maestro/worktrees` directory (it was destroying
  uncommitted work across repos). (`session/git/worktree_ops.go`)
- Credential isolation: on session restore the account's config-dir env is
  reconstructed if it wasn't persisted, so a worker can no longer inherit the
  launching shell's `CLAUDE_CONFIG_DIR`/`CODEX_HOME`. (`session/instance.go`)
- tmux: fixed a `monitor` data race; `Detach()` no longer panics (and crashes
  the whole TUI) when the session/server died while attached; the PTY opened
  during a timed-out `start()` is now closed; the stdin reader goroutine no
  longer leaks and steals keystrokes after a detach. (`session/tmux/tmux.go`)
- `PushChanges` uses `git push` instead of a malformed `gh repo sync`.
  (`session/git/worktree_git.go`)

**TUI / daemon**
- The daemon backfills config defaults, so a config missing
  `daemon_poll_interval` no longer panics `time.NewTicker(0)` — AutoYes was
  silently broken. (`config/config.go`, `daemon/daemon.go`)
- `List.GetSelectedInstance` clamps its index (no more out-of-range panic) and
  `RemoveByName` fixes selection drift after an async removal. (`ui/list.go`)
- AutoYes no longer presses Enter into a session the user is attached to.
- Duplicate instance titles are rejected (they collided on tmux session and
  git branch names). (`app/keys.go`)
- The orchestration overlay's worker table is sorted instead of reshuffling
  every frame. (`ui/overlay/orchestration.go`)
- The daemon now runs reconciliation, so `--after` dependency chains advance
  even when the interactive TUI isn't open. (`daemon/daemon.go`)

### Changed
- Documentation clarified: usage-aware auto-selection happens on the
  `--schedule` path; plain `maestro dispatch <worker>` targets the named worker.

See [docs/KNOWN_ISSUES.md](docs/KNOWN_ISSUES.md) for issues identified during
the audit that were intentionally deferred.
