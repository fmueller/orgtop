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

## Test Expectations

- Add acceptance vectors for pagination, deduplication, truncation, empty organizations, auth failures, failed re-expansion, and fixed refresh snapshots.

## Verification Notes

- Record how every RG-010 choice and RG-009 interaction is closed.
- Record `taskrail validate`.

## Implementation Notes

- Do not implement organization calls here.
- Keep organization selectors and GitHub listing types out of downstream domain and TUI models.
