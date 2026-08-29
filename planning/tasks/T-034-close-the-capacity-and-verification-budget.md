---
id: T-034-close-the-capacity-and-verification-budget
title: Close the capacity and verification budget
status: todo
priority: high
spec_ref: specs/v0.2.0.md#rg-009-capacity-and-verification-budget
dependencies:
    - T-031-close-the-unified-cli-contract
    - T-032-close-the-path-matcher-contract
updated_at: "2026-08-29T09:22:04Z"
---

# T-034-close-the-capacity-and-verification-budget Close the capacity and verification budget

## Description

Close RG-009 by setting coherent tested resource limits and deterministic verification vectors for v0.2.0.

## Acceptance

- The spec sets limits for Scopes, matchers, changed files, enrichment concurrency and queueing, SQLite work, Rain items, and strip items.
- Limits align with responsiveness, API discipline, and organization expansion.
- Deterministic fixtures demonstrate behavior at and beyond each bound.

## Test Expectations

- Specify fixture sizes, expected bounded outcomes, cancellation checks, and TUI responsiveness vectors without live services or sleeps.

## Verification Notes

- Record a table mapping each resource to its limit and acceptance vector.
- Record `taskrail validate`.

## Implementation Notes

- This task defines budgets; it does not optimize or implement them.
- Avoid introducing generalized scalability architecture beyond the release requirements.
