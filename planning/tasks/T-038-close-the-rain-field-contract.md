---
id: T-038-close-the-rain-field-contract
title: Close the Rain field contract
status: completed
priority: high
spec_ref: specs/v0.2.0.md#rg-006-rain-field-contract
dependencies:
    - T-036-close-the-unknown-membership-product-policy
    - T-033-close-the-shared-visual-semantics-contract
    - T-034-close-the-capacity-and-verification-budget
updated_at: "2026-08-29T12:44:53Z"
---

# T-038-close-the-rain-field-contract Close the Rain field contract

## Description

Close RG-006 with deterministic, bounded Rain field state transitions and responsive column behavior.

## Acceptance

- The spec defines cadence, placement, movement, collisions, lifetime, bounds, arrival, delayed ticks, refresh replacement, mixed-Scope columns, and narrow-terminal behavior.
- One event matching multiple visible Scopes creates one bounded item per matching column, with deterministic column/global capacity accounting.
- `p` pauses movement and expiry while bounded arrivals queue; resume admission and motion remain deterministic without stopping polling.
- Identical explicit inputs produce identical state without wall-clock dependence.
- Motion communicates arrival and recency only.

## Test Expectations

- Add explicit event, tick, pause, bounded-arrival, resume, refresh, overlap, and resize vectors covering expiry, delayed ticks, capacity, and narrow columns.

## Verification Notes

- Record every numeric bound and deterministic tie-break.
- Record `taskrail validate`.

## Implementation Notes

- Do not add animation code in this task.
- Keep Rain calculations outside rendering and do not introduce physics or Harmonica.
- 2026-08-29T12:44:49Z: verification pass
- 2026-08-29T12:44:53Z: Closed RG-006 with user-approved repeating motion, configurable Rain window, deterministic placement/collisions, fair capacity, pause queue, refresh reconciliation, paging, and edge-state vectors A-049 through A-054.
