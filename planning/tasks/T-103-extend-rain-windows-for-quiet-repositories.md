---
id: T-103-extend-rain-windows-for-quiet-repositories
title: Extend Rain windows for quiet repositories
status: todo
priority: high
spec_ref: specs/v0.2.0.md#ambient-rain-view
dependencies:
    - T-090-move-rain-page-range-and-hidden-counts-into-the
updated_at: "2026-09-09T21:32:58Z"
---

# T-103-extend-rain-windows-for-quiet-repositories Extend Rain windows for quiet repositories

## Description

Extend the bounded Rain field with useful windows for quiet and open-source
repositories. Keep the existing short windows, add longer finite windows and an
honest current-snapshot `available` choice, and make 24 hours the default.

## Acceptance

- Rain exposes the ordered presets `15m`, `30m`, `60m`, `6h`, `24h`, `7d`, and
  `available`; it starts at `24h`, and `-`/`+` stop at the endpoints without
  wrapping.
- Every finite window removes an item at its exact age boundary. `available`
  admits confirmed memberships from the bounded current or retained stale snapshot
  regardless of age and removes an item when a later successful snapshot no longer
  returns it.
- The visible label is exactly `window available`; help and documentation explain
  that this means the fetched newest-100-per-repository snapshot, not complete
  repository history.
- Shared visual recency becomes `new`, `recent`, `aging`, and `old`. Crossing 60
  minutes changes an item's visual state to `old` without removing it from `6h`,
  `24h`, `7d`, or `available`.
- Full-color and no-color Rain remain understandable. No-color recency accounting
  includes old items in its full and compact forms.
- Window changes, pause/resume, stale retention, refresh reconciliation, field
  placement, responsive layout, and per-column/global capacities remain
  deterministic and bounded.
- User-facing Rain documentation and the Unreleased changelog describe the new
  default, presets, controls, and `available` limitation.

## Test Expectations

- Drive the change test-first with asymmetric fixtures at every finite boundary,
  including just before and exactly at `60m`, `6h`, `24h`, and `7d`.
- Cover the complete `-`/`+` sequence and endpoints, 24-hour startup, immediate
  restoration and removal when changing windows, and state retention through
  view changes and resize.
- Cover an event older than seven days under `available`, then omit it from a
  later successful snapshot and assert that it disappears without implying
  historical persistence.
- Cover paused and stale `available` behavior, capacity pressure, rich-color and
  no-color rendering, and constrained full/compact context forms.
- Run focused TUI and binary integration tests, differential mutation testing for
  the changed Rain/recency logic, and the full `task check` gate.

## Verification Notes

- Record the tested preset/boundary matrix, rendered default and `available`
  labels, snapshot-removal scenario, mutation result, and full-gate result.

## Implementation Notes

- Keep window eligibility in Rain state and visual recency in the existing shared
  recency owner; rendering only formats prepared state.
- Reuse the current normalized snapshot. Do not add source pagination, event-history
  persistence, a new cache responsibility, or any unbounded state.
- Preserve the fixed 15-minute `Interesting Now` window independently of the Rain
  field selection.
