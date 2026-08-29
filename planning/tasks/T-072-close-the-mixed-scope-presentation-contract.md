---
id: T-072-close-the-mixed-scope-presentation-contract
title: Close the mixed-Scope presentation contract
status: todo
priority: high
spec_ref: specs/v0.2.0.md#rg-012-mixed-scope-presentation-contract
dependencies:
    - T-032-close-the-path-matcher-contract
    - T-036-close-the-unknown-membership-product-policy
    - T-034-close-the-capacity-and-verification-budget
    - T-038-close-the-rain-field-contract
updated_at: "2026-08-29T09:55:21Z"
---

# T-072-close-the-mixed-scope-presentation-contract Close the mixed-Scope presentation contract

## Description

Close RG-012 with one deterministic presentation contract for mixed repository and path Scopes across Overview, Stream, and constrained Rain layouts.

## Acceptance

- The spec defines mixed-Scope row ordering and tie-breaks, compact multi-Scope Stream context, unambiguous label shortening, and non-Rain overflow navigation/indicators.
- Every hidden Scope, row, membership label, or Rain column is reachable or explicitly accounted for without starving a Scope.
- The contract composes with RG-004 unknown disclosure, RG-006 Rain columns, and RG-009 capacity bounds without moving matching or aggregation into rendering.

## Test Expectations

- Add normative vectors for repository/path ordering ties, overlapping membership, long labels, wide runes, scrolling positions, repeated resize, and `40x10`.
- Specify exact compact outputs and overflow indicators so implementation tests need no renderer-side policy choices.

## Verification Notes

- Record every RG-012 clause and its normative acceptance vector.
- Record `taskrail validate` and focused advisory coverage.

## Implementation Notes

- This is readiness work only; do not change production code.
- Keep presentation identity separate from enrichment evidence identity and consume prepared Scope membership state.
