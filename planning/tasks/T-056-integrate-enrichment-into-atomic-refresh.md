---
id: T-056-integrate-enrichment-into-atomic-refresh
title: Integrate enrichment into atomic refresh
status: todo
priority: high
spec_ref: specs/v0.2.0.md#changed-file-enrichment
dependencies:
    - T-052-implement-bounded-enrichment-coordination
    - T-054-implement-scoped-snapshots-and-direct-aggregation
    - T-055-implement-safe-cache-failure-and-recovery
updated_at: "2026-08-29T09:22:04Z"
---

# T-056-integrate-enrichment-into-atomic-refresh Integrate enrichment into atomic refresh

## Description

Insert normalized enrichment and membership evaluation into the asynchronous refresh lifecycle before aggregation.

## Acceptance

- Refresh atomicity, completion, timeout, cancellation, stale/degraded behavior, and later recovery implement the closed RG-003/RG-004 contracts.
- Repository-only invocations retain the v0.1 path with no unnecessary enrichment.
- A refresh publishes one coherent prepared snapshot and never labels polling as LIVE.

## Test Expectations

- Add lifecycle tests for success, shared enrichment, partial evidence, rate limits, cancellation, post-success failure, stale retention, and recovery.

## Verification Notes

- Record state transitions and evidence that keyboard/render work stays responsive.

## Implementation Notes

- Preserve non-overlapping polling and the 500-event source bound.
