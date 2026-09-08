---
id: T-099-account-for-clipped-detail-graphemes-at-one-cell
title: Account for clipped detail graphemes at one-cell width
status: completed
priority: low
spec_ref: specs/v0.2.0.md#scope-aware-overview-and-stream
dependencies: []
updated_at: "2026-09-08T19:57:35Z"
---

# T-099-account-for-clipped-detail-graphemes-at-one-cell Account for clipped detail graphemes at one-cell width

## Description

A-077 gives bounded Stream event detail one clipping exception: at `W=1` a
two-cell grapheme renders as a one-cell placeholder and increments a
`clipped detail graphemes` counter, and the original prepared grapheme reappears
after resize rather than being split.

T-086 implemented the placeholder half. `leadingChunk` in
`internal/tui/stream_detail.go` consumes a grapheme wider than the whole width
as `wideGraphemePlaceholder` (`?`), which is what keeps the greedy wrap from
either splitting a cluster or looping forever. The counter half is
unimplemented and owned by no task: nothing counts the replaced graphemes and
no chrome reports them, so a one-column terminal silently substitutes text
without disclosing that it did.

Follow-up derived from T-086-implement-bounded-stream-event-detail's review.

## Acceptance

- Wrapping detail at a positive width counts each grapheme the width replaced
  with the placeholder, exactly once per replacement.
- The count is reported in existing chrome rather than by consuming a detail
  row, and stays disjoint from the viewport, capacity, omission, and
  compact-label counters RG-012 already keeps apart.
- A width that replaces nothing reports no count at all.
- Restoring a width the grapheme fits renders the original prepared grapheme
  and drops the count, without re-deriving anything from the source.

## Test Expectations

- Cover a one-cell width over a wide grapheme, a zero-width cluster, a width
  that clips nothing, and the resize round trip that restores the original.

## Verification Notes

- Record the rendered detail line and the reported count at `W=1`, and the same
  line after restoring a width the grapheme fits.

## Implementation Notes

- The placeholder itself already exists as `wideGraphemePlaceholder` in
  `internal/tui/stream_detail.go`; this task adds only the accounting.
- `overflowRange` and `Model.overflow` already carry the detail's own
  `lines A-B of N` range, so the count belongs beside it rather than in a
  mechanism of its own.
- 2026-09-08T19:57:32Z: verification pass
