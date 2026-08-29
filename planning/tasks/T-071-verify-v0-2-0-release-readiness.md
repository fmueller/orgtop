---
id: T-071-verify-v0-2-0-release-readiness
title: Verify v0.2.0 release readiness
status: todo
priority: high
spec_ref: specs/v0.2.0.md#integration-documentation-and-release-readiness
dependencies:
    - T-070-document-the-closed-v0-2-0-behavior
    - T-068-verify-distribution-channel-parity
updated_at: "2026-08-29T09:22:04Z"
---

# T-071-verify-v0-2-0-release-readiness Verify v0.2.0 release readiness

## Description

Perform final deterministic verification of the integrated, documented, distributable v0.2.0 release against the closed specification.

## Acceptance

- Every Potential Features area and normative acceptance scenario has objective passing evidence.
- v0.1 compatibility, bounds, cancellation, determinism, architecture, secret safety, no-LIVE wording, and explicit non-goals are preserved.
- Documentation, external repository provisioning evidence, and post-integration distribution parity are complete before this task verifies release readiness.
- Repository quality gates, Taskrail validation, cross-builds, startup smoke, release checks, and changelog requirements pass.

## Test Expectations

- Run `task check`, Taskrail validation/coverage, cross-compile smoke, startup smoke, release checks/dry run, deterministic end-to-end fixtures, and focused bound/rate-limit/cancellation suites.
- Use no live GitHub credentials or publication side effects.

## Verification Notes

- Record exact command exit statuses and a scenario-to-evidence matrix, including docs and distribution artifacts.
- Record residual manual checks, if any, without marking unmet normative acceptance complete.

## Implementation Notes

- This is final verification, not a second readiness consolidation task.
- Fix only concrete release blockers and keep any correction within its owning architecture boundary.
