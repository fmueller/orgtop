---
id: T-102-pin-the-overview-label-budget-fallback-with-a
title: Pin the Overview label budget fallback with a direct table test
status: completed
priority: low
spec_ref: specs/v0.2.0.md#scope-aware-overview-and-stream
dependencies:
    - T-098-bound-overview-scope-labels-by-the-row-budget-the
updated_at: "2026-09-08T20:29:36Z"
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

- `internal/tui/overview_scope_test.go:290` `TestLabelBudgetFallsBackToTheViewWidth`
  table-drives `labelBudget` (`internal/tui/overview.go:211`) over a positive
  remainder, an exactly-zero one, a negative one, and an unbounded width.
  Deliberate regression `return width` -> `return 0` fails 3 of the 4 cases
  (`labelBudget(12, 10) = 0, want 12`); restored, the table passes.
- `internal/tui/overview_scope_test.go:319`
  `TestOverviewHoldsWideGraphemeLabelsInsideTheView` renders a path Scope whose
  pattern carries twelve wide graphemes at widths 40 and 60, holding every line
  inside the view through `assertFits` and keeping the layout's complete push
  count behind the shortened label. Deliberate regression measuring
  `widestWidth` with `len` fails both widths (`row ... does not end in the
  complete push count of its layout`); restored, both pass.
- `internal/tui/overview_test.go:295` comment corrected: a path Scope's
  arbitrary UTF-8 pattern does carry wide graphemes into a rendered row.
- `go vet ./...`, `golangci-lint run ./internal/tui/` (0 issues),
  `gofmt -l internal/tui` (clean), `go test ./...` all pass.
- go-reviewer subagent: approve, no CRITICAL/HIGH findings; three low notes
  (hard-coded budget literals by design, deliberate overlap with
  `TestOverviewShortensLongLabelsIntoTheRowBudget`, both extra table cases
  load-bearing under mutation).

## Implementation Notes

- 2026-09-08T20:29:32Z: verification pass
