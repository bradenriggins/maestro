# UI Investigation

## Scope

This document captures the findings from a repo-level investigation into why the Maestro TUI can feel illogical, clipped, or nonfunctional despite the automated UI suite passing.

## Resolution Status

The layout, overlay sizing, overlay placement, and UI automation gaps described here were addressed on 2026-04-08 by introducing a viewport-driven layout contract in `app/`, exact-bound rendering in `ui/`, a shared viewport overlay contract in `ui/overlay/`, and stricter composed-render regression coverage in `app/*_test.go`.

## Validation Performed

Commands run during the investigation:

```bash
go test ./...
go test ./app -run 'TestUIAudit|TestUISnapshots|TestTUIAutomation|TestUserJourney'
```

Both commands passed during investigation. The issues below are therefore mostly structural layout/rendering problems plus coverage gaps, not currently failing test assertions.

## Executive Summary

The strongest root causes are:

1. Height budgeting in the layout phase does not match what `View()` actually renders.
2. Width is reduced multiple times across the list and tabbed panes, creating dead space and an off-balance layout.
3. The prompt overlay is given less space than it actually consumes.
4. Overlay centering is based on the rendered app tree instead of the viewport.
5. The automated UI tests normalize away many of the exact terminal artifacts users would notice.

The result is a TUI that can pass text-based tests while still looking clipped, shifted, padded incorrectly, or visually incoherent in real terminals.

## Findings

### 1. Height Budgeting Is Internally Inconsistent

Relevant code:

- `app/app.go:248-275`
- `app/app.go:1442-1504`

`updateHandleWindowSizeEvent()` allocates terminal space as if the screen is:

- content area
- menu
- error box

But `View()` later adds additional rows that were never included in that budget:

- top padding above the list
- top padding above the tabbed pane
- optional setup banner
- optional conflict banner
- optional wake banner
- optional status bar

This means the final render tree can exceed the terminal height even when the initial math says it should fit. Likely user-visible effects:

- clipped bottom controls
- status bar disappearing
- overlays that appear vertically off-center
- panes that feel like they jump or compress unpredictably

### 2. Width Is Being Shrunk Twice

Relevant code:

- `app/app.go:250-262`
- `ui/tabbed_window.go:76-83`
- `ui/list.go:115-117`
- `ui/list.go:274`

The app already splits width at roughly 30% list / 70% content. After that:

- `TabbedWindow.SetSize()` applies `AdjustPreviewWidth(width)` and keeps only 90% of the width it was given.
- `InstanceRenderer.setWidth()` also applies `AdjustPreviewWidth(width)`.
- the list title width also uses `AdjustPreviewWidth(l.width)`.

This compounds the shrinkage and creates unnecessary empty space. Because the composed UI is later centered, the whole screen can look spatially wrong even when it technically fits.

Likely user-visible effects:

- too much dead margin
- content pane feels narrower than expected
- list and preview area do not feel aligned
- overlays appear to float relative to oddly centered content

### 3. Prompt Overlay Height Is Oversubscribed

Relevant code:

- `app/app.go:264-266`
- `ui/overlay/textInput.go:98-113`
- `ui/overlay/textInput.go:304-353`

The prompt overlay gets `40%` of terminal height. Inside `TextInputOverlay.SetSize()`, the textarea is given that full height directly. Then `Render()` adds more vertical structure on top:

- title
- divider(s)
- optional profile picker
- optional branch picker
- button row
- footer hint
- border and padding

So the overlay uses more vertical space than it was budgeted to have.

Likely user-visible effects:

- prompt controls pushed offscreen
- branch/program selectors only partially visible
- modal that looks broken on smaller terminals

### 4. Overlay Placement Depends on the Rendered Background, Not the Viewport

Relevant code:

- `app/app.go:1502-1541`

All overlays are centered with `overlay.PlaceOverlay(..., mainView, true, true)`.

That means the background used for centering is the fully rendered `mainView`, whose height already varies based on banners, padding, status bar, and layout overflow. If `mainView` is taller than the terminal, overlay placement becomes content-dependent instead of viewport-dependent.

Likely user-visible effects:

- overlays look vertically shifted
- modal placement changes when banners appear
- help and orchestration overlays feel unstable after state changes

### 5. Help Overlay Sizing Depends on a Later Resize Pass

Relevant code:

- `app/help.go:168-173`
- `app/app.go:267-269`
- `ui/overlay/textOverlay.go:45-65`

`showHelpScreen()` creates the overlay and switches state immediately. Width is set elsewhere during window-size handling. Until that happens, `TextOverlay.Render()` uses whatever width is currently stored, including zero or stale values.

This is especially risky for first-render correctness and helps explain why the help overlay snapshots already look awkward.

### 6. Tabbed Window Height Uses Brittle Magic Numbers

Relevant code:

- `ui/tabbed_window.go:85-95`
- `ui/tabbed_window.go:283-288`

The tabbed pane height calculation relies on manually synchronized constants like:

- `- 2` for spacing/newlines
- an explicit `"\n"` row inserted between tabs and content

This is fragile. Minor styling changes can break the layout because the render math and the actual rendered structure are maintained separately.

### 7. Existing Snapshots Already Show Broken-Looking Composition

Relevant files:

- `app/testdata/ui_snapshots/help_overlay.snap`
- `app/testdata/ui_snapshots/orchestration_overlay.snap`

The checked-in snapshots already preserve visible signs of layout problems:

- borders intersect awkwardly
- background panes remain visibly malformed behind overlays
- alignment looks inconsistent

This is useful evidence that the current test suite is accepting visually poor renders as correct.

## Test Coverage Gaps

### 1. Tests Bypass the Real Key Handling Path

Relevant code:

- `app/tui_automation_test.go:62-68`

`pressKey()` forces `h.keySent = true` before calling `handleKeyPress()`. That bypasses the real menu highlight / resend path used in production.

Consequence:

- key timing and redispatch bugs can ship undetected

### 2. Snapshot Normalization Hides Real Terminal Problems

Relevant code:

- `app/ui_snapshot_test.go:148-160`
- `app/ui_audit_test.go:243-247`

The tests strip:

- ANSI sequences
- carriage returns
- block characters
- trailing spaces

For a Bubble Tea / Lipgloss TUI, those are not cosmetic details. They are part of the real layout. Removing them hides:

- border alignment issues
- overflow
- clipped rendering
- spacing drift
- terminal-specific visual corruption

### 3. Test Viewports Are Too Friendly

Relevant code:

- `app/ui_audit_test.go:210-220`
- `app/user_journey_test.go`

Most tests start at `140x40`, and the resize coverage only probes moderate sizes. The suite does not meaningfully stress:

- narrow terminals
- short terminals
- overlays plus banners plus status bar together
- pathological combinations of dynamic content

### 4. Many Assertions Only Check for Text Presence

Relevant files:

- `app/ui_audit_test.go`
- `app/user_journey_test.go`

The suite often checks that certain strings are present, but does not verify:

- focus behavior
- actual selection movement
- scroll usability
- overlay fit
- visual clipping
- end-to-end overlay placement within the composed app view

### 5. Some Snapshots Bypass Full App Composition

Relevant code:

- `app/ui_snapshot_test.go:49-56`

`quick_dispatch_empty` snapshots `h.quickDispatchOverlay.Render()` directly instead of snapshotting the full `home.View()` composition. That misses:

- centering issues
- interaction with banners and status bar
- background dimming behavior
- full-screen overlay placement defects

## Most Likely Root Cause Chain

The current UI problems are best explained by this chain:

1. The app allocates space with simple percentage-based math.
2. The actual render path adds more rows and padding than the allocator accounts for.
3. Both the list and tabbed pane shrink width again after the main split.
4. Overlays render against an already unstable background tree.
5. Tests normalize away the visual evidence and run mostly in friendly viewports.

That combination can easily produce a UI that is technically "passing" but feels broken to use.

## Recommended Fix Order

1. Fix vertical budgeting in `app/app.go` so `updateHandleWindowSizeEvent()` and `View()` account for the same rows.
2. Remove the second round of width shrinkage from the list and tabbed window.
3. Rework `TextInputOverlay` so textarea height is derived from the remaining overlay height, not the full container height.
4. Center overlays against the viewport dimensions, not the dynamically rendered `mainView`.
5. Add tests for small terminals and for the real key resend/highlight path.
6. Add snapshot coverage that preserves more terminal layout detail.

## Notes

No code changes were made as part of this investigation. This file records the findings only.
