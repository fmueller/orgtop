---
id: T-071-verify-v0-2-0-release-readiness
title: Verify v0.2.0 release readiness
status: completed
priority: high
spec_ref: specs/v0.2.0.md#integration-documentation-and-release-readiness
dependencies:
    - T-070-document-the-closed-v0-2-0-behavior
    - T-068-verify-distribution-channel-parity
    - T-082-cover-contended-cache-lifecycle-reset-and-rebuild
    - T-093-enforce-per-package-efficacy-floor
    - T-094-harden-cache-timing-assertions
    - T-103-extend-rain-windows-for-quiet-repositories
    - T-105-complete-the-protected-ledger-step-when-its-event
    - T-107-upload-release-assets-create-if-absent-so-a-retry
    - T-106-test-the-v0-2-0-release-candidate-before-tagging
updated_at: "2026-10-20T19:36:10Z"
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
- 2026-10-20T19:35:56Z: verification pass
- 2026-10-20T19:36:10Z: Completed deterministic v0.2.0 verification after final checks, dedicated simplification, parallel General/Security review, fresh candidate validation and disposition verification (no findings). Durable 85-scenario/12-area matrix, exact command exits, six-target snapshot digests, historical external provisioning and explicit T-106 waivers: docs/testing/v0.2.0-readiness.md. Fixed concrete pre-tag changelog guard by organizing existing 0.2.0 notes; no product behavior change. T-118 help naming remains open and non-blocking; no live credentials, tag, release, deployment or v0.3 work.
