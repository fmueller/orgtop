---
id: T-034-close-the-capacity-and-verification-budget
title: Close the capacity and verification budget
status: completed
priority: high
spec_ref: specs/v0.2.0.md#rg-009-capacity-and-verification-budget
dependencies:
    - T-031-close-the-unified-cli-contract
    - T-032-close-the-path-matcher-contract
updated_at: "2026-08-29T11:11:45Z"
---

# T-034-close-the-capacity-and-verification-budget Close the capacity and verification budget

## Description

Close RG-009 by setting coherent tested resource limits and deterministic verification vectors for v0.2.0.

## Acceptance

- The spec sets limits for Scopes, matchers, changed files, enrichment concurrency and queueing, SQLite work, Rain items, and strip items.
- One combined work ledger bounds organization expansion, polling, pagination, enrichment, and SQLite work per cycle.
- The contract fixes the global 500-event truncation stage, prohibits enrichment of already discarded events, gives exact selections deterministic capacity precedence, and discloses every omitted class.
- Limits align with responsiveness, API discipline, organization expansion, Rain overlap representations, and paused-arrival queueing.
- Deterministic fixtures demonstrate behavior at and beyond each bound.

## Test Expectations

- Specify fixture sizes, combined request counts, truncation stages, expected bounded outcomes, cancellation checks, and TUI responsiveness vectors without live services or sleeps.

## Verification Notes

- Record a table mapping each resource to its limit and acceptance vector.
- Record `taskrail validate`.

## Implementation Notes

- This task defines budgets; it does not optimize or implement them.
- Avoid introducing generalized scalability architecture beyond the release requirements.
- 2026-08-29T11:11:34Z: verification pass
- 2026-08-29T11:11:45Z: Closed RG-009 with user-approved capacities, refresh ledger, UI state bounds, disclosures, and deterministic verification vectors A-028 through A-030.
