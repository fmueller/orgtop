---
id: T-110-qualify-activity-counts-and-empty-states-by
title: Qualify activity counts and empty states by snapshot coverage
status: todo
priority: high
spec_ref: specs/v0.2.0.md#scope-aware-overview-and-stream
dependencies:
    - T-054-implement-scoped-snapshots-and-direct-aggregation
    - T-087-report-mixed-scope-context-in-the-shared-header
updated_at: "2026-09-19T08:53:27Z"
---

# T-110-qualify-activity-counts-and-empty-states-by Qualify activity counts and empty states by snapshot coverage

## Description

A zero Overview row can result from newest-500 global retention rather than
repository inactivity. Counts also cover unequal source-history spans. Make
coverage explicit without inventing complete-history or activity-rate claims.

## Acceptance

- Update exact No activity wording in RG-004/A-037 and affected presentation
  contracts before implementing new copy.
- Zero confirmed events with no unknown evidence reads No activity in retained
  snapshot or an equally explicit bounded equivalent. Unknown-only scopes stay
  distinct and never appear confirmed quiet.
- Identify counts as retained events, not rates or fixed-window totals. Nearby
  coverage text explains the newest-100-per-repository fetch and newest-500
  global retention bounds without claiming the source returned a full page.
- Preserve separate selection omissions, snapshot truncation, viewport hiding,
  and membership uncertainty; do not invent per-scope omitted counts or history
  completeness. Preserve loading/error/stale states and bounded layouts.
- Explain that Rain windows filter available fetched activity, not guaranteed
  coverage of that duration. Update related docs and the Unreleased changelog.

## Test Expectations

- More than 500 mixed-repository events displace all events from a quieter
  repository, which keeps a qualified zero row. Contrast genuine empty data
  and unknown-only path membership.
- Cover truncated/non-truncated snapshots, stale data, narrow layouts, and
  Rain windows. Inspect renders, run targeted tests and `task check`.

## Verification Notes

Record coverage wording, independently constructed boundary fixtures, commands,
and inspected renders.

## Implementation Notes

- Required before distribution parity and final v0.2.0 release readiness.
- Do not add pagination, persistence, or increase capacities in this task.
