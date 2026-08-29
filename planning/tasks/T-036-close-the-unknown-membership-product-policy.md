---
id: T-036-close-the-unknown-membership-product-policy
title: Close the unknown-membership product policy
status: completed
priority: high
spec_ref: specs/v0.2.0.md#rg-004-unknown-membership-product-policy
dependencies:
    - T-035-close-the-github-enrichment-contract
updated_at: "2026-08-29T11:44:15Z"
---

# T-036-close-the-unknown-membership-product-policy Close the unknown-membership product policy

## Description

Choose and normatively specify one honest end-to-end policy for unknown path membership.

## Acceptance

- The spec defines unknown behavior for aggregation, Stream, Rain, Interesting Now, empty states, freshness, degraded state, atomic refresh, retries, and recovery.
- Unknown is never silently treated as member or not-member.
- Every path Scope exposes a quantitative unknown count or equivalent explicit lower-bound statement, including the all-unknown case.
- State precedence is closed for combinations of unknown, stale, rate-limited, cache-failed, and expansion-failed conditions.
- Repository scopes remain usable when path evidence is unavailable.

## Test Expectations

- Add acceptance scenarios spanning initial and all-unknown evidence, partial multi-Scope evidence, combined degraded causes, later failure, retry, and recovery.
- Assert consistent outcomes in domain, application state, and every view.

## Verification Notes

- Record the selected policy and coverage for each named product surface.
- Record `taskrail validate`.

## Implementation Notes

- Do not implement the selected policy here.
- Preserve tri-state information across boundaries rather than encoding unknown in rendering.
- 2026-08-29T11:44:11Z: verification pass
- 2026-08-29T11:44:15Z: Closed RG-004 with user-approved aggregation, per-view unknown behavior, current-PR qualification, combined state, retry, and recovery plus A-037 through A-042.
