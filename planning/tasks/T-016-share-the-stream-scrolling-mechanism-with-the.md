---
id: T-016-share-the-stream-scrolling-mechanism-with-the
title: Share the Stream scrolling mechanism with the Overview rows
status: completed
priority: medium
spec_ref: specs/v0.1.0.md#overview-view
dependencies:
    - T-010-implement-the-scrollable-stream-activity-view
updated_at: "2026-08-22T14:27:54Z"
---

# T-016-share-the-stream-scrolling-mechanism-with-the Share the Stream scrolling mechanism with the Overview rows

## Description

T-009 renders one Overview row per selected repository but never moves
`overview.offset`, so rows past the body budget are dropped with no way to reach
them: 30 selected repositories at `40x10` render 8 rows and no indication that 22
exist. T-010 introduces clamped scrolling for `stream.offset`. Reuse that single
mechanism for `overview.offset` rather than growing a second, divergent copy, and
advertise the keys once in the shared footer.

## Acceptance

- Overview scrolls its aggregate rows with the same keys and the same clamping as
  Stream, and the clamp lives in one place used by both views.
- Each view's offset is clamped after a refresh that shrinks the snapshot and after
  a resize, and never leaves the rendered row range.
- Per-view offsets stay independent and survive mode switches, as they do today.
- The footer advertises the scroll controls where they fit and still advertises only
  implemented controls, never displacing the quit hint.
- `40x10` retains active view, transport/freshness state, content, and the quit hint;
  narrower positive sizes do not panic.
- Row content, row order, and the explicit no-recent-activity state are unchanged
  from T-009. No derived measure is introduced.

## Test Expectations

- Assert scrolling in both views, clamping at both ends, clamping after a snapshot
  shrink and after a resize, independent per-view offsets, footer advertising, and
  `40x10` rendering.
- Run focused Overview and Stream tests and `task check`.

## Verification Notes

- Record exact commands, exit status, and the named scrolling and clamping cases.

## Implementation Notes

- T-009 handoff: `overview.offset` exists in `internal/tui/overview.go` as a
  preserved state slot that nothing moves. `renderBody` in `internal/tui/model.go`
  already clamps with `min(max(offset, 0), len(lines))`, so the gap is key handling,
  not clamping.
- `handleKey` in `internal/tui/model.go` currently handles only `q`, `ctrl+c`, `1`,
  `2`, and `tab`; footer candidates live in `internal/tui/chrome.go`, and
  `TestFooterAdvertisesOnlyImplementedControls` fails today if the footer mentions
  scrolling, so update it together with the new controls.
- v0.1.0 scope is reachability through scrolling. The stronger no-silent-truncation
  rule with a position or overflow indicator is carried by v0.2.0 FR-011 and is not
  part of this task.
- 2026-08-22T14:27:50Z: verification pass
