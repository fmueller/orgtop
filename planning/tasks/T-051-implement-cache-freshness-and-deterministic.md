---
id: T-051-implement-cache-freshness-and-deterministic
title: Implement cache freshness and deterministic cleanup
status: todo
priority: high
spec_ref: specs/v0.2.0.md#bounded-sqlite-cache
dependencies:
    - T-050-implement-the-versioned-sqlite-enrichment-store
updated_at: "2026-08-29T09:22:04Z"
---

# T-051-implement-cache-freshness-and-deterministic Implement cache freshness and deterministic cleanup

## Description

Implement the closed cache validity, revalidation, retention, and bounded cleanup policy.

## Acceptance

- Positive/negative caching, freshness, expiry, total physical byte/row/age limits, checkpoint/vacuum behavior, cleanup triggers, ordering, and work budgets implement RG-005/RG-009 exactly.
- Expired, invalid, or incomplete entries never satisfy complete evidence.
- Cleanup deterministically returns the store to documented bounds without an unbounded foreground pause.

## Test Expectations

- Use controlled clocks and temporary stores to test every logical/physical boundary, WAL/SHM growth, lock outcome, tie-break, trigger, cleanup batch, and equivalent-evidence result.

## Verification Notes

- Record before/after bounds and deterministic cleanup ordering.

## Implementation Notes

- Run cache work outside TUI input and rendering paths.
