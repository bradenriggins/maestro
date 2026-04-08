# Maestro TUI Cleanup Design

## Summary

Maestro should feel like a workflow-first operator console rather than a collection of panes and overlays stitched together by implementation details. This cleanup redesign will make the entire TUI more logical, more reliable, and more polished by introducing a stable application shell, reducing overlay dependence, rebuilding navigation around operator jobs, and treating layout correctness as a product requirement rather than a rendering detail.

## Goals

- Make the entire UI/UX feel coherent to a first-time operator.
- Eliminate broken-feeling or surprising behaviors.
- Make primary flows obvious, fast, and keyboard-native.
- Raise visual quality so the app feels restrained, trustworthy, and deliberate.
- Strengthen automated coverage so layout and interaction regressions are caught before shipping.

## Non-Goals

- Preserve existing keybindings or layout patterns for compatibility.
- Add major new product scope unrelated to operator experience cleanup.
- Pursue a full visual reinvention that delays core usability and correctness work.

## Design Principles

1. The user should always know where they are, what is selected, and what they can do next.
2. Core work belongs in stable surfaces, not in surprising modal stacks.
3. The UI structure should mirror operator workflows, not package boundaries.
4. Visual restraint beats ornamental complexity; polish comes from clarity, spacing, hierarchy, and consistency.
5. Layout correctness, focus behavior, and flow predictability are functional requirements.

## Product Shape

The redesigned TUI should use a stable shell with three persistent regions:

- a left rail for the current workflow's navigable list or object set
- a main pane for the currently selected item's detail or active workflow surface
- a slim status/action region that communicates system state and the most important contextual actions

The shell should remain visually consistent while the content changes. The operator should not feel like the screen keeps changing rules as state changes.

Overlays should become rare and deliberate. They should be reserved for:

- destructive confirmations
- short, bounded input
- temporary supporting context that does not deserve a full workflow surface

Primary work such as dispatching, triaging results, inspecting sessions, and checking system health should move into first-class surfaces inside the shell.

## Information Architecture

The app should be reorganized around operator workflows:

- `Sessions`: active workspaces, terminals, live state, and session actions
- `Dispatch`: send work, inspect routing decisions, configure dependencies, and confirm submission
- `Review`: triage completed, failed, paused, or conflicted work with obvious next actions
- `History`: browse prior tasks, outcomes, and relevant artifacts
- `System`: diagnostics, usage data, configuration health, and environment problems

`Sessions` should remain the default landing workflow because it best represents the live operational state of Maestro.

Within each workflow:

- the left side presents the current list or index
- the main pane presents structured detail and the primary actions for the selected entity
- the status/action area advertises the few shortcuts that matter in the current context

The architecture should remove hidden toggles and reduce view modes that exist only because the current code is split across separate rendering components.

## Navigation Model

The keyboard model should be normalized across the app:

- arrows and `j/k` move selection
- `Enter` drills in, confirms, or executes the primary contextual action
- `Esc` backs out, cancels, or closes transient UI
- clearly advertised single-key shortcuts handle the most important secondary actions

The app should prefer shallow navigation over layered modal depth. Modal depth should effectively never exceed one layer.

Whenever focus changes, the user should be able to identify the active surface immediately from visual emphasis alone.

## Interaction Rules

The redesign should enforce these interaction rules:

- empty states explain the next useful action
- loading states explain what is happening
- error states explain impact and recovery
- destructive actions state consequences explicitly
- selection focus is visually unmistakable
- shortcut hints are contextual and visible, not assumed knowledge
- the same interaction should behave the same way in similar contexts

Any behavior that relies on the user remembering hidden app state, invisible mode switches, or inconsistent key semantics should be treated as a defect.

## Visual Direction

The interface should feel calm, premium, and operational. The target is not maximal decoration; it is disciplined clarity with enough personality to feel purpose-built.

Visual rules:

- strict spacing rhythm across shell, lists, panes, headers, and footers
- strong hierarchy for section headings, metadata, and actions
- consistent pane sizing without double-shrinking content
- overlays sized from the viewport and centered against the viewport, not against unstable rendered content
- restrained but meaningful use of color for state and severity
- cleaner borders, alignment, and text density so the app reads as an instrument rather than a toy

## Functional Architecture Requirements

The redesign must address the structural issues already identified in the repo investigation:

- viewport budgeting must match the composed `View()` tree
- width allocation must happen once per layout decision, not repeatedly downstream
- overlay sizing must account for all internal chrome before assigning content height
- overlay placement must be viewport-relative
- help and other overlays must render correctly on first open without relying on a later resize pass
- render math and rendered structure must stop drifting apart through brittle magic numbers

These are not implementation niceties. They are foundational to whether the UI feels trustworthy.

## Testing Strategy

The quality bar must rise alongside the redesign. Required coverage direction:

- composed app viewport-contract tests that validate full-screen layout behavior
- narrow and short terminal stress cases, not only comfortable sizes
- overlay-fit assertions for all major transient surfaces
- user-journey tests that exercise real key handling paths
- snapshot coverage that preserves terminal details which reveal clipping, spacing, or border corruption
- stronger assertions around focus movement, selection state, and visible next actions

The guiding rule is that text presence alone is not enough to prove the UI works.

## Delivery Strategy

The cleanup should ship in three coordinated waves:

### Wave 1: Shell and Layout Stability

- establish the stable shell contract
- fix viewport budgeting, pane sizing, and overlay placement
- remove brittle layout behaviors that make the UI feel physically broken

### Wave 2: Workflow-First Rebuild

- restructure the major workflows around operator jobs
- reduce overlay dependence
- normalize navigation and contextual actions

### Wave 3: Finish and Harden

- refine copy, affordances, and empty/loading/error states
- tighten hierarchy, spacing, and visual consistency
- expand regression coverage until the redesign is trustworthy to maintain

## Success Criteria

This redesign is successful when:

- a first-time operator can understand the current area, selection, and next action without guesswork
- the app no longer looks clipped, unstable, or spatially incoherent under realistic terminal sizes
- primary flows such as session inspection, dispatch, review, and recovery feel obvious and fast
- overlays feel exceptional instead of routine
- automated tests fail when real visual or flow regressions are introduced
- the interface feels deliberate enough that a strong engineer would describe it as clean, logical, and obviously cared for

## Risks and Mitigations

### Risk: the redesign becomes an open-ended reinvention

Mitigation: keep the scope centered on workflow clarity, functional correctness, and finish quality rather than adding unrelated product ambition.

### Risk: visual polish outruns structural cleanup

Mitigation: land shell and layout fixes before workflow and finish work, and treat coverage upgrades as part of each wave.

### Risk: implementation spreads across too many files without clear ownership

Mitigation: create a written implementation plan that decomposes the redesign into bounded tasks with explicit file ownership, tests, and checkpoints.
