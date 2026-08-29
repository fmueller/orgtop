---
id: T-047-implement-bounded-organization-expansion
title: Implement bounded organization expansion
status: todo
priority: high
spec_ref: specs/v0.2.0.md#organization-selection
dependencies:
    - T-073-implement-organization-selector-cli-parsing
updated_at: "2026-08-29T09:22:04Z"
---

# T-047-implement-bounded-organization-expansion Implement bounded organization expansion

## Description

Implement the closed organization listing operation and expand selectors into deterministic repository Scopes at the adapter boundary.

## Acceptance

- Parsed-selector handoff, source-proven eligibility, pagination, capacity, ordering, truncation, deduplication, cancellation, errors, and empty results match the closed RG-010/RG-009 contracts without unbudgeted activity probes.
- GitHub request, response, and pagination types remain inside the adapter.
- Downstream code receives only canonical repository Scopes plus prepared disclosure/provenance state.

## Test Expectations

- Use fixture adapter tests for pagination, eligibility, over-capacity truncation, duplicates, cancellation, authorization, unknown organizations, and empty success.

## Verification Notes

- Record request counts, retained ordering, and bound assertions.

## Implementation Notes

- An organization selector is not a domain Scope.
- Do not add organization aggregate rows.
