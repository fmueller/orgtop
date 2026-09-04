---
id: T-061-implement-deterministic-bounded-rain-state
title: Implement deterministic bounded Rain state
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#ambient-rain-view
dependencies:
    - T-054-implement-scoped-snapshots-and-direct-aggregation
    - T-060-implement-shared-discrete-recency-semantics
updated_at: "2026-09-04T14:48:21Z"
---

# T-061-implement-deterministic-bounded-rain-state Implement deterministic bounded Rain state

## Description

Implement Rain as a pure bounded state transition driven by scoped events, explicit ticks, refreshes, and dimensions.

## Acceptance

- Cadence, placement, movement, collisions, lifetime, item/column limits, delayed ticks, refresh replacement, one-item-per-matching-Scope overlap, and column allocation implement closed RG-006/RG-009 values.
- `p` pause freezes movement and expiry, queues arrivals within existing bounds, and resumes admission deterministically without stopping refreshes.
- Identical inputs yield identical state and expired items are removed.
- State density represents direct displayed events only.

## Test Expectations

- Add pure tests for every normative transition vector, overlap, pause, bounded paused arrival, resume, bound, tie-break, delayed tick, refresh, and resize case without sleeps.

## Verification Notes

- Record deterministic state snapshots and maximum allocation evidence.

## Implementation Notes

- Keep Rain state independent of Lip Gloss rendering and wall-clock reads.
- Use Bubble Tea ticks only; no physics library.
- 2026-09-04T14:48:10Z: verification pass
