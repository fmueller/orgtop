---
id: T-048-implement-organization-selection-snapshots
title: Implement organization selection snapshots
status: todo
priority: high
spec_ref: specs/v0.2.0.md#organization-selection
dependencies:
    - T-047-implement-bounded-organization-expansion
updated_at: "2026-08-29T09:22:04Z"
---

# T-048-implement-organization-selection-snapshots Implement organization selection snapshots

## Description

Integrate bounded re-expansion and immutable per-refresh selection snapshots into application state.

## Acceptance

- Re-expansion timing, request budget, atomic snapshot use, truncation disclosure, provenance, failure degradation, and recovery implement the closed RG-010 policy.
- Failed re-expansion retains the last successful selection rather than narrowing or emptying it.
- Expanded and exact repository Scopes are behaviorally identical downstream.

## Test Expectations

- Add deterministic lifecycle tests for additions/removals during refresh, failed re-expansion, recovery, deduplication, truncation, and empty organizations.

## Verification Notes

- Record lifecycle transitions and bounded request evidence.

## Implementation Notes

- Keep expansion separate from event polling and bind both operations to context cancellation.
