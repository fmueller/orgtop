---
id: T-090-move-rain-page-range-and-hidden-counts-into-the
title: Move Rain page range and hidden counts into the shared header
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#ambient-rain-view
dependencies:
    - T-065-implement-deterministic-responsive-overflow
    - T-062-render-and-navigate-the-rain-view
updated_at: "2026-09-04T22:41:45Z"
---

# T-090-move-rain-page-range-and-hidden-counts-into-the Move Rain page range and hidden counts into the shared header

## Description

RG-006's "Columns, Paging, and Resize" states that the header shows the one-based
`scopes A-B of N` range, or the equivalent one-Scope form, plus the hidden Scope
and hidden item counts. T-062 renders that range and those counts in Rain's own
body context line instead, because the shared header's RG-012 overflow-range
segment and its closed priority ladder are owned by
T-065-implement-deterministic-responsive-overflow and did not exist yet.

Once T-065 lands the header range segment, Rain's range and its hidden Scope and
hidden item counts belong in the shared header, and the Rain context line keeps
only what RG-006 leaves to the view.

Follow-up derived from T-062-render-and-navigate-the-rain-view's verification or discovery.

## Acceptance

- The Rain page range and its hidden Scope and hidden item counts render in the
  shared header through T-065's closed segment ladder, at the RG-012 priority
  that segment already has.
- The Rain context line retains the selected window, the pause state, the
  hidden-legend indicator, RG-008's textual recency fallback, and the disjoint
  omission, collision, and clipped counters.
- No count is duplicated between the header and the Rain context line, and the
  RG-006 counters stay disjoint and are never merged or substituted.

## Test Expectations

- Cover the header range and hidden counts for a single-Scope, a multi-Scope
  single-page, and a multi-page selection, at the widths that select each rung
  of the ladder, and assert the Rain context line no longer repeats them.

## Verification Notes

- Record the rendered header segment and Rain context line per selection shape
  and per width rung.

## Implementation Notes

- The Rain-side source is `rainContextSegments` in `internal/tui/rain_render.go`;
  `rainScopeRange` and the `hiddenScopes`/`hiddenItems` entries of
  `rainDisjointCounts` are the parts that move.
- Rain is not a viewport view, so its range comes from the prepared fixed page in
  `rainField`, not from a scroll offset.
