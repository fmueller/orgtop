---
id: T-040-close-the-organization-selection-contract
title: Close the organization selection contract
status: todo
priority: high
spec_ref: specs/v0.2.0.md#rg-010-organization-selection-contract
dependencies:
    - T-031-close-the-unified-cli-contract
    - T-034-close-the-capacity-and-verification-budget
updated_at: "2026-08-29T09:22:04Z"
---

# T-040-close-the-organization-selection-contract Close the organization selection contract

## Description

Close RG-010 with an exact bounded organization selector and re-expansion contract.

## Acceptance

- The spec defines syntax, composition, eligibility, user controls, capacity, deterministic ordering/truncation, disclosure, provenance, re-expansion, request budget, failures, path composition, and invalid/empty outcomes.
- Expansion produces ordinary canonical repository Scopes at the CLI/adapter boundary.
- Capacity, request budget, and interval align with RG-009.
- Every eligibility rule names the organization-listing response fact that proves it and adds no unbudgeted per-repository activity query.
- Organization-only startup, initial expansion failure, first successful empty expansion, and failed re-expansion have distinct closed states.

## Test Expectations

- Add acceptance vectors for organization-only startup, pagination, deduplication, multiple selectors exceeding global capacity, truncation, empty organizations, initial/auth failures, failed re-expansion, and fixed refresh snapshots.

## Verification Notes

- Record how every RG-010 choice and RG-009 interaction is closed.
- Record `taskrail validate`.

## Implementation Notes

- Do not implement organization calls here.
- Keep organization selectors and GitHub listing types out of downstream domain and TUI models.
