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

## Test Expectations

- Add lifecycle tests for each failure mode, continued interaction, source fallback where specified, reset/disable behavior, and recovery.

## Verification Notes

- Record visible states, sanitized errors, and equivalent post-recovery membership.

## Implementation Notes

- Prepare failure state outside rendering and avoid broad persistence abstractions.
