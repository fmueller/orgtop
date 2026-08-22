---
id: T-010-implement-the-scrollable-stream-activity-view
title: Implement the scrollable Stream activity view
status: completed
priority: medium
spec_ref: specs/v0.1.0.md#stream-view
dependencies:
    - T-007-implement-the-asynchronous-refresh-lifecycle
updated_at: "2026-08-22T13:35:15Z"
---

# T-010-implement-the-scrollable-stream-activity-view Implement the scrollable Stream activity view

## Description

Replace the Stream placeholder through its existing state/update/render seams with
reverse-chronological Event rendering and manual bounded scrolling. Limit changes to
Stream files/tests; do not modify root navigation, source, or snapshot calculation.

## Acceptance

- Each row contains timestamp, repository, non-color category encoding, optional
  actor, and domain description in snapshot order.
- Up/down/page scrolling clamps after input, refresh shrinkage, and resize.
- View switches preserve Stream position and loaded state.
- Empty, loading, first-error, stale-with-data, and current states compose with chrome.
- `40x10` retains required context/content; no filter, clustering, signals, Inspect,
  Rain, or unjustified Bubbles dependency appears.

## Test Expectations

- Cover row content/order, actor absence, category encoding, scroll/page bounds,
  refresh clamping, preserved position, degraded/empty states, and `40x10`.
- Run focused Stream tests and `task check`.

## Verification Notes

- Record exact commands, exit status, and named Stream update/render cases.

## Implementation Notes

- T-006 handoff: `streamLines` in `internal/tui/stream.go` returns a placeholder
  body ("Recent events are not rendered yet"). Replace it with the event rows;
  the placeholder must not survive into a release.
- T-006 handoff: `stream.offset` is preserved across mode switches but no key
  moves it and only `renderBody` windows on it. Add the scroll/page keys and the
  upper bound here, and cover clamping after refresh shrinkage and resize.
- 2026-08-22T13:35:11Z: verification pass
