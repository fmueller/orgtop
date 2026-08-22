---
id: T-009-implement-the-overview-activity-view
title: Implement the Overview activity view
status: todo
priority: medium
spec_ref: specs/v0.1.0.md#overview-view
dependencies:
    - T-007-implement-the-asynchronous-refresh-lifecycle
updated_at: "2026-08-21T22:39:23Z"
---

# T-009-implement-the-overview-activity-view Implement the Overview activity view

## Description

Replace the Overview placeholder through its existing renderer seam with responsive,
direct repository aggregates. Limit changes to Overview state/rendering/tests; do not
modify root navigation or refresh orchestration.

## Acceptance

- Overview is initial and shows resolved identity, event count, PR activity, and
  pushes in precomputed row order.
- Zero-event repositories remain represented and an all-zero success has an explicit
  no-recent-activity state.
- Loading, first error, stale-with-data, and current states compose with shared chrome.
- `40x10` retains required context/content and narrower positive sizes do not panic.
- No baselines, percentages, signals, productivity measures, invented metrics, or
  data calculations appear.

## Test Expectations

- Assert populated ordering/counts/identity, all-zero, loading, error, stale, current,
  and `40x10` rendering.
- Run focused Overview tests and `task check`.

## Verification Notes

- Record exact commands, exit status, and named Overview rendering cases.

## Implementation Notes

- T-006 handoff: `overviewLines` in `internal/tui/overview.go` returns a
  placeholder body ("Repository activity is not rendered yet"). Replace it with
  the aggregate rows; the placeholder must not survive into a release.
- T-006 handoff: `overview.offset` exists as a preserved state slot but nothing
  moves it yet. Clamp and drive it here if Overview scrolls.
