---
id: T-053-implement-explicit-scope-membership-outcomes
title: Implement explicit Scope membership outcomes
status: completed
priority: high
spec_ref: specs/v0.2.0.md#explicit-scope-membership
dependencies:
    - T-046-implement-canonical-path-matching
    - T-052-implement-bounded-enrichment-coordination
updated_at: "2026-09-02T22:15:41Z"
---

# T-053-implement-explicit-scope-membership-outcomes Implement explicit Scope membership outcomes

## Description

Implement domain evaluation of repository and path Scopes with explicit member, not-member, and unknown outcomes.

## Acceptance

- Evaluation implements the closed matcher, evidence-completeness, and RG-004 unknown policy contracts.
- Complete evidence can produce member or not-member; missing, incomplete, expired, unsupported, denied, rate-limited, or failed evidence remains unknown as specified.
- One event may independently belong to multiple Scopes.

## Test Expectations

- Add table tests for all evidence outcomes, repository fallback, overlap, renames, and unknown recovery vectors.

## Verification Notes

- Record the full outcome matrix and targeted domain test results.

## Implementation Notes

- Use an explicit typed outcome rather than a boolean.
- Keep policy and matching out of render functions.
- 2026-09-02T22:15:38Z: verification pass
