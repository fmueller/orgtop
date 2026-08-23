---
id: T-022-give-stream-sticky-column-headings
title: Give Stream sticky column headings
status: todo
priority: medium
spec_ref: specs/v0.1.0.md#stream-view
dependencies: []
updated_at: "2026-08-23T20:14:11Z"
---

# T-022-give-stream-sticky-column-headings Give Stream sticky column headings

## Description

Stream renders four unlabeled columns. Nothing tells a first-time user that the
second column is the repository, the third the category, or where the actor ends
and the description begins:

```
14:38:00  acme/backend   push          alice · pushed 3 commits to main
```

Column headings fix that, but only if they stay put. A heading row that scrolls
away with the content is worse than none, because it is visible exactly while the
user still remembers the layout and gone once they have scrolled far enough to
need it. So the headings belong to the view's chrome, not to its scrollable body.

That is the invasive part: Stream's content area becomes the shared budget minus
one further line, and the scroll clamp has to be computed against that remainder.
`chromeLines` in `internal/tui/model.go` is currently a single shared constant for
both views.

## Acceptance

- Stream renders a heading row naming the rendered columns, positioned directly
  under the shared header.
- The heading row does not scroll: any scroll position, at any height, keeps it
  visible.
- Stream's scroll clamp is computed against the content area that remains after
  the headings, so the last event stays reachable and paging past the end still
  clamps to the last row.
- Overview's content area and scroll behavior are unchanged; the extra line is
  Stream's alone.
- Headings are subordinate to content: a height that cannot hold both the headings
  and at least one event row drops the headings and keeps the row.
- At `40x10`, Stream retains the active view, transport and freshness state, at
  least one primary content or explicit state line, and the quit hint, so A-010
  continues to hold with the headings present.
- The headings follow the same widest-layout-that-fits ladder as the rows, so they
  stay aligned with the columns they name at every width.
- The explicit loading, error, and no-recent-activity states are not given column
  headings, since they have no columns.

## Test Expectations

- A `internal/tui` test scrolling to the bottom of a long snapshot and asserting
  the heading row is still the first body line.
- A test at the height where the headings must give way, proving the event row
  survives and the headings do not.
- A wired flow in `cmd/orgtop/integration_test.go` at `40x10` covering both the
  headings and last-row reachability, extending the existing narrow-terminal work
  rather than duplicating it.
- Existing A-010 and viewport clamp tests updated where the extra line changes the
  expected budget, with each change explained.

## Verification Notes

- Record `task check`, `go test ./... -race`, and `git diff --check` exit statuses.
- Name every existing assertion whose expected body height changed and why.
- Regression proof: making the headings part of the scrollable body must fail the
  scrolled-to-bottom test.

## Implementation Notes

- `chromeLines` in `internal/tui/model.go` is shared by both views today.
  Per-view chrome is the change; FR-007 was amended to allow a view to reserve a
  further fixed line of its own.
- `Model.scroll` and `Model.body` both compute `height - chromeLines`. Both need
  the per-view remainder, and they must agree, or the clamp and the render will
  disagree about how many rows exist.
- Highest-risk task of the Stream legibility set: it moves the arithmetic the
  A-010 contract rests on. Prefer deriving the per-view reservation from the view
  rather than branching on the mode in several places.
