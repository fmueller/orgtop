---
id: T-062-render-and-navigate-the-rain-view
title: Render and navigate the Rain view
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#ambient-rain-view
dependencies:
    - T-061-implement-deterministic-bounded-rain-state
    - T-057-render-mixed-scopes-in-overview
    - T-058-render-scope-context-in-stream
updated_at: "2026-09-04T22:42:05Z"
---

# T-062-render-and-navigate-the-rain-view Render and navigate the Rain view

## Description

Add Rain as the third primary Bubble Tea view using prepared deterministic Rain state.

## Acceptance

- `1`, `2`, `3`, and `tab` navigation, shared header, POLLING/freshness/degraded context, Scope columns, no-color usefulness, and per-view state retention implement the closed contracts.
- Rain remains bounded and responsive at closed dimensions and never claims severity or anomaly.
- Loaded data is retained while switching views.

## Test Expectations

- Add TUI tests for direct and tab navigation, state retention, tick messages, mixed Scope labels, no-color output, resize, and `40x10`.

## Verification Notes

- Record navigation sequences and deterministic renders.

## Implementation Notes

- Rendering consumes prepared state and performs no placement, recency, matching, or aggregation calculations.
- Draw category glyphs from the shared `categoryVocabulary` in
  `internal/tui/category.go`; do not add a Rain-local glyph or spelling
  (found while completing T-059).
- RG-008 puts two Rain-only obligations behind this task's "no-color
  usefulness": the complete glyph-to-text legend whenever dimensions permit,
  and the `recency: N new · R recent · A aging` context, shortened to
  `age N/R/A`, whenever the effective profile cannot render distinct intensity
  attributes. A constrained Rain that hides the legend keeps its hidden-legend
  indicator under RG-012.
- 2026-09-04T22:41:24Z: verification pass
