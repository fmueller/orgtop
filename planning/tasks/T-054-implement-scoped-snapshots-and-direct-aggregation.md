---
id: T-054-implement-scoped-snapshots-and-direct-aggregation
title: Implement scoped snapshots and direct aggregation
status: todo
priority: high
spec_ref: specs/v0.2.0.md#explicit-scope-membership
dependencies:
    - T-053-implement-explicit-scope-membership-outcomes
    - T-048-implement-organization-selection-snapshots
updated_at: "2026-08-29T09:22:04Z"
---

# T-054-implement-scoped-snapshots-and-direct-aggregation Implement scoped snapshots and direct aggregation

## Description

Build immutable application snapshots containing normalized events, per-Scope outcomes, and direct deterministic aggregates.

## Acceptance

- Snapshot inclusion, quantitative unknown coverage, overlap, event categories, RG-012 mixed-Scope ordering, and aggregation implement the closed normative contracts.
- Repository counts retain v0.1 direct semantics; path counts use only outcomes allowed by the closed policy.
- Overlapping Scope sums are never represented as a deduplicated organization total.

## Test Expectations

- Add deterministic snapshot tests for mixed Scopes, overlap, unknowns, empty scopes, category counts, ordering ties, and the 500-event bound.

## Verification Notes

- Record aggregate fixtures and expected ordered outputs.

## Implementation Notes

- Calculate before rendering and keep view models read-only.
