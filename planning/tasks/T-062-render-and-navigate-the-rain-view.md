---
id: T-062-render-and-navigate-the-rain-view
title: Render and navigate the Rain view
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#ambient-rain-view
dependencies:
    - T-061-implement-deterministic-bounded-rain-state
    - T-057-render-mixed-scopes-in-overview
    - T-058-render-scope-context-in-stream
updated_at: "2026-08-29T09:22:04Z"
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
