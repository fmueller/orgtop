---
id: T-114-protected-ledger-merge-atomic
title: Make protected ledger merges atomic across concurrent base changes
status: todo
priority: high
spec_ref: specs/v0.2.0.md#distribution-channels
dependencies:
    - T-074-provision-and-validate-distribution-repositories
updated_at: "2026-09-19T23:14:48Z"
---

# T-114-protected-ledger-merge-atomic Make protected ledger merges atomic across concurrent base changes

## Description

Close the concurrent-base-change race in the protected distribution-ledger
transition without weakening the independent-approval or pull-request-only
boundary. T-074 proves current-head pinning and post-merge reconciliation, but
not atomic protection against an exact or contradictory ledger event landing
between the final base read and a successful pull-request merge.

Follow-up derived from T-074-provision-and-validate-distribution-repositories's verification or discovery.

## Acceptance

- A fixture reproduces a successful merge after an exact or contradictory
  default-branch ledger event lands concurrently.
- The chosen server-side merge/CAS/protection design prevents a duplicate or
  contradictory ledger event from persisting, while retaining the independent
  approval and pull-request-only workflow boundary.
- The release workflow fails closed with durable evidence when the concurrent
  transition cannot be proven safe; no production v0.2.0 publication is part
  of this task.

## Verification Notes

- Applies to v0.2.0 and must be resolved before production release readiness
  is signed off. Discovered by the independent Security/release-integrity
  review of T-074; see the T-074 review disposition and
  `scripts/distribution-protected-commit.sh` merge path.

## Implementation Notes

- T-074 intentionally defers this follow-up because changing repository
  rulesets or bypassing the protected pull-request boundary was not authorized
  by that rehearsal.
