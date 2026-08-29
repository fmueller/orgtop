---
id: T-037-close-the-sqlite-cache-contract
title: Close the SQLite cache contract
status: todo
priority: high
spec_ref: specs/v0.2.0.md#rg-005-sqlite-cache-contract
dependencies:
    - T-035-close-the-github-enrichment-contract
    - T-036-close-the-unknown-membership-product-policy
    - T-034-close-the-capacity-and-verification-budget
updated_at: "2026-08-29T09:22:04Z"
---

# T-037-close-the-sqlite-cache-contract Close the SQLite cache contract

## Description

Close RG-005 with a complete, measurable contract for the disposable local SQLite enrichment cache.

## Acceptance

- The spec defines location, permissions, schema, migrations, keys, completeness, positive and negative caching, freshness, transactions, corruption, reset/disable behavior, bounds, cleanup, and failures.
- Cache state stores no credentials, headers, raw payloads, rendered screens, or speculative history.
- Equivalent reacquired evidence produces identical membership after cache deletion.

## Test Expectations

- Add normative vectors for hit, miss, expiry, interrupted write, migration, corruption, unavailable storage, cleanup ordering, and every bound.

## Verification Notes

- Record measurable limits and the user-visible failure policy.
- Record `taskrail validate`.

## Implementation Notes

- This is specification work only.
- Keep SQLite behind the smallest enrichment-focused boundary and out of domain models.
