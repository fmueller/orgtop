---
id: T-066-implement-honest-degraded-state-presentation
title: Implement honest degraded-state presentation
status: todo
priority: high
spec_ref: specs/v0.2.0.md#responsive-and-degraded-experience
dependencies:
    - T-065-implement-deterministic-responsive-overflow
    - T-055-implement-safe-cache-failure-and-recovery
    - T-048-implement-organization-selection-snapshots
updated_at: "2026-08-29T09:22:04Z"
---

# T-066-implement-honest-degraded-state-presentation Implement honest degraded-state presentation

## Description

Present source, enrichment, membership, cache, organization-expansion, rate-limit, and stale conditions consistently across shared chrome and all views.

## Acceptance

- Distinct user consequences receive the distinct states and recovery transitions defined by the closed RG-004/RG-005/RG-010 contracts.
- Unknown or stale data is never presented as complete, empty by default, LIVE, or current contrary to the policy.
- Path-Scope output quantifies unknown coverage or states its lower-bound meaning, including all-unknown and combined degraded conditions.
- Polling, navigation, resize, and quit remain responsive during degraded work.

## Test Expectations

- Add state-matrix TUI tests for first-load errors, stale-after-success, each degraded cause, combinations allowed by the contract, rate limits, and recovery.

## Verification Notes

- Record visible copy/state for each cause and transition.

## Implementation Notes

- Prepare state outside renderers and preserve token secrecy in errors and fixtures.
