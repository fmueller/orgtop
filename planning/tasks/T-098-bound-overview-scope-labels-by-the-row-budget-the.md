---
id: T-098-bound-overview-scope-labels-by-the-row-budget-the
title: Bound Overview Scope labels by the row budget the counts leave
status: completed
priority: low
spec_ref: specs/v0.2.0.md#scope-aware-overview-and-stream
dependencies:
    - T-085-implement-cell-safe-scope-label-shortening
updated_at: "2026-09-08T19:39:00Z"
---

# T-098-bound-overview-scope-labels-by-the-row-budget-the Bound Overview Scope labels by the row budget the counts leave

## Description

T-085 bounds each Overview Scope label by the whole view width
(`internal/tui/overview.go:155`), but `layoutRows` (`internal/tui/overview.go:174`)
appends `rowGap` and the counts behind the label, so a label close to the terminal
width still yields a row wider than the view, which the shared body cut then marks.
The label budget should be the cells the selected layout's counts leave, so the
label is shortened by RG-012's rule rather than by the body cut.

## Acceptance

- An Overview row is never wider than the view width while any layout can render
  its counts.
- The label budget is derived from the selected layout's counts, and the row order,
  Scope membership, and the layout ladder are unchanged.
- A Scope whose label the budget shortens still renders its token first.

## Verification Notes

- Record the rendered Overview rows and their cell widths at the minimum terminal
  and at a width a long label overflows.

## Implementation Notes

- 2026-09-08T19:38:34Z: verification pass
