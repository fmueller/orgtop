---
id: T-039-close-the-interesting-now-selection-contract
title: Close the Interesting Now selection contract
status: todo
priority: high
spec_ref: specs/v0.2.0.md#rg-007-interesting-now-selection-contract
dependencies:
    - T-036-close-the-unknown-membership-product-policy
    - T-038-close-the-rain-field-contract
    - T-033-close-the-shared-visual-semantics-contract
    - T-034-close-the-capacity-and-verification-budget
updated_at: "2026-08-29T09:22:04Z"
---

# T-039-close-the-interesting-now-selection-contract Close the Interesting Now selection contract

## Description

Close RG-007 with a deterministic selection contract based only on directly observable current event facts.

## Acceptance

- The spec defines eligibility, ordering, tie-breaks, visible and stored bounds, recency window, duplicate handling, refresh, unknown membership, and narrow-height collapse.
- Duplicate handling distinguishes duplicate source events from legitimate multi-Scope membership and defines deterministic cross-Scope fairness or an explicit global-selection rule.
- The contract introduces no anomaly, baseline, productivity, severity, derived signal, or opaque ranking.
- Every selected entry is explainable from normalized facts.

## Test Expectations

- Add vectors for over-limit input, ties, duplicate source events, multi-Scope membership, Scope fairness, refresh replacement, unknown membership, expiry, and constrained height.

## Verification Notes

- Record the direct facts used by each selection rule and all limits.
- Record `taskrail validate`.

## Implementation Notes

- This is normative specification work only.
- Keep selection in prepared application/domain state, not in a renderer.
