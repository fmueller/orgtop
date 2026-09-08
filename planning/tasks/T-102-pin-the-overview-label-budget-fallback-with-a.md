---
id: T-102-pin-the-overview-label-budget-fallback-with-a
title: Pin the Overview label budget fallback with a direct table test
status: todo
priority: low
spec_ref: specs/v0.2.0.md#scope-aware-overview-and-stream
dependencies:
    - T-098-bound-overview-scope-labels-by-the-row-budget-the
updated_at: "2026-09-08T19:38:49Z"
---

# T-102-pin-the-overview-label-budget-fallback-with-a Pin the Overview label budget fallback with a direct table test

## Description

The T-098 review found that `TestOverviewShortensLongLabelsIntoTheRowBudget`
(`internal/tui/overview_scope_test.go:260`) never drives `labelBudget`
(`internal/tui/overview.go:211`) to its `budget <= 0` fallback: at both rendered
widths the budget stays positive, so the branch that hands the label the whole
view is only covered incidentally by `TestOverviewMarksRowsTheWidthCannotHold`
(`internal/tui/overview_test.go:537`), a test written for the FR-009 body cut.
No rendered case composes a wide-grapheme label against the counts-derived
budget either.

## Acceptance

- A table-driven test of `labelBudget` covers a positive budget, an exactly-zero
  budget, and a negative one, and fails if the fallback stops returning the view
  width.
- A rendered Overview case composes a wide-grapheme label against the
  counts-derived budget and holds the row inside the view width.

TODO: describe the work and link the spec section.

Follow-up derived from T-098-bound-overview-scope-labels-by-the-row-budget-the's verification or discovery.

## Acceptance

- TODO: define acceptance criteria.

## Verification Notes

- TODO: record verification evidence paths.

## Implementation Notes
