---
id: T-035-close-the-github-enrichment-contract
title: Close the GitHub enrichment contract
status: todo
priority: high
spec_ref: specs/v0.2.0.md#rg-003-github-enrichment-contract
dependencies:
    - T-032-close-the-path-matcher-contract
    - T-034-close-the-capacity-and-verification-budget
updated_at: "2026-08-29T09:22:04Z"
---

# T-035-close-the-github-enrichment-contract Close the GitHub enrichment contract

## Description

Close RG-003 by specifying the exact bounded GitHub changed-file enrichment flow and its normalized adapter outcome.

## Acceptance

- The spec selects API operations and versions and defines eligibility, auth, pagination completeness, request bounds, concurrency, timeouts, rate limits, renames, unsupported entities, errors, and atomicity impact.
- Incomplete evidence is never classified as complete.
- The contract defines API hard-cap, entity mutation between pages, force-push, deletion, and repeated observation semantics so current entity state is not silently assigned to historical events.
- Transport and pagination types remain adapter-owned.

## Test Expectations

- Add fixture-oriented acceptance vectors for pagination, page mutation, API hard caps, force-push/deletion, malformed/incomplete responses, cancellation, rate limiting, authorization failures, unsupported entities, and sanitized errors.

## Verification Notes

- Record the completeness proof and bounded-work clauses.
- Record `taskrail validate`.

## Implementation Notes

- This task changes normative specification only.
- Define normalized outcomes suitable for the domain without leaking GitHub payload types.
