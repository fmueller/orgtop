---
id: T-042-consolidate-v0-2-0-implementation-readiness
title: Consolidate v0.2.0 implementation readiness
status: todo
priority: high
spec_ref: specs/v0.2.0.md#open-decisions-and-readiness-gates
dependencies:
    - T-031-close-the-unified-cli-contract
    - T-032-close-the-path-matcher-contract
    - T-035-close-the-github-enrichment-contract
    - T-036-close-the-unknown-membership-product-policy
    - T-037-close-the-sqlite-cache-contract
    - T-038-close-the-rain-field-contract
    - T-039-close-the-interesting-now-selection-contract
    - T-033-close-the-shared-visual-semantics-contract
    - T-034-close-the-capacity-and-verification-budget
    - T-040-close-the-organization-selection-contract
    - T-041-close-the-distribution-channel-contract
updated_at: "2026-08-29T09:22:04Z"
---

# T-042-consolidate-v0-2-0-implementation-readiness Consolidate v0.2.0 implementation readiness

## Description

Review the eleven completed gate revisions together and convert v0.2.0 from a contradictory Draft into one coherent implementation-ready normative specification.

## Acceptance

- RG-001 through RG-011 are all closed in normative text with corresponding acceptance coverage.
- Cross-gate values, terminology, limits, state transitions, and dependencies agree without unresolved placeholders.
- Draft/not-ready language is removed or updated only when the complete specification supports that status change.
- No implementation task is started before this consolidation completes.

## Test Expectations

- Cross-check CLI identity with matcher/cache keys, unknown policy with every view, all capacity budgets, and distribution/documentation contracts.
- Run structural and coverage validation for the revised spec.

## Verification Notes

- Record a gate-by-gate closure matrix and any cross-gate corrections.
- Record `taskrail validate` and advisory spec coverage results.

## Implementation Notes

- This is the only readiness consolidation task.
- Do not change production code; implementation tasks consume the resulting closed contracts.
