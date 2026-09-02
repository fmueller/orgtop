---
id: T-055-implement-safe-cache-failure-and-recovery
title: Implement safe cache failure and recovery
status: todo
priority: high
spec_ref: specs/v0.2.0.md#bounded-sqlite-cache
dependencies:
    - T-051-implement-cache-freshness-and-deterministic
    - T-054-implement-scoped-snapshots-and-direct-aggregation
updated_at: "2026-08-29T09:22:04Z"
---

# T-055-implement-safe-cache-failure-and-recovery Implement safe cache failure and recovery

## Description

Integrate cache unavailable, unwritable, corrupt, and migration-incompatible outcomes into application state according to the closed policy.

## Acceptance

- Disable/reset behavior, actionable errors, degraded operation, unknown evidence, stale/current qualification, and recovery implement RG-004/RG-005.
- Cache failure never panics, leaks secrets, or turns incomplete evidence into complete membership.
- Removing the disposable cache changes request volume only, not eventual matching semantics.
- `--reset-cache` executes RG-005's exact procedure: lifecycle-exclusive validation, the
  atomic rename to `enrichment-v1.resetting`, sidecar-first deletion, the ownership guard
  that preserves a foreign or ambiguous database, and the one structural rebuild attempt
  per process. T-045 parses the flag and T-051 left the store side unimplemented.
- Write admission proves the pending replacement's own page growth against the 128 MiB
  projected post-checkpoint ceiling, not only the pre-write state as T-051 shipped.

## Test Expectations

- Add lifecycle tests for each failure mode, continued interaction, source fallback where specified, reset/disable behavior, and recovery.
- Prove a maximum-size replacement stays within the projected retained ceiling, and that a
  reset leaves no fresh main database beside a stale write-ahead log.

## Verification Notes

- Record visible states, sanitized errors, and equivalent post-recovery membership.

## Implementation Notes

- Prepare failure state outside rendering and avoid broad persistence abstractions.
