---
id: T-084-keep-repository-membership-through-canceled
title: Keep repository membership through canceled evidence
status: todo
priority: low
spec_ref: specs/v0.2.0.md#explicit-scope-membership
dependencies: []
updated_at: "2026-09-03T12:57:17Z"
---

# T-084-keep-repository-membership-through-canceled Keep repository membership through canceled evidence

## Description

`ScopeSet.Evaluate` (`internal/domain/membership.go`) returns `(nil, false)` for
an `OutcomeCanceled` evidence result before any per-Scope evaluation runs, so a
canceled attempt drops the repository Scope membership of the event too, even
though `Scope.Evaluate` decides a repository Scope from repository identity
without consulting evidence at all. `specs/v0.2.0.md` states that
enrichment-only unknown outcomes compose without invalidating independently
confirmed repository membership.

The path is currently unreachable through the lifecycle: `applyRefresh`
discards the whole attempt when the shutdown context is done, and
`OutcomeCanceled` originates only from that same cancellation. Decide whether
the whole-event short circuit is the intended reading of RG-004 or a defect,
and record the decision.

Discovered during the T-056 review.

## Acceptance

- The interaction between `OutcomeCanceled` and repository Scope membership is
  either corrected or documented as the intended closed behavior.
- Canceled partial work is still never synthesized into unknown membership.

## Test Expectations

- Cover a mixed selection whose shared event carries canceled evidence.

## Verification Notes

- Record the spec reading, the chosen option, and targeted domain test results.

## Implementation Notes

- Do not change the RG-004 unknown reason groups or the aggregation semantics
  while doing this.
