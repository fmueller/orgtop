---
id: T-070-document-the-closed-v0-2-0-behavior
title: Document the closed v0.2.0 behavior
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#integration-documentation-and-release-readiness
dependencies:
    - T-030-decouple-readme-structure-from-doc-tests
    - T-069-integrate-the-closed-v0-2-0-binary-flow
    - T-068-verify-distribution-channel-parity
updated_at: "2026-08-29T09:22:04Z"
---

# T-070-document-the-closed-v0-2-0-behavior Document the closed v0.2.0 behavior

## Description

Update README and help text for the implemented closed v0.2.0 contracts after the documentation guard is decoupled from v0.1 prose structure.

## Acceptance

- Documentation matches the closed Scope syntax, mixed examples, matcher semantics, unknown policy, cache location/bounds/cleanup/disable/reset, controls, Rain overlap/pause, Interesting Now, organization selection, and distribution channels.
- Additional GitHub request/rate-limit/degraded behavior is explained.
- v0.1 auth, secret safety, POLLING/not-LIVE, standalone installation, and non-goal honesty are preserved.

## Test Expectations

- Extend documentation/help tests against the implemented closed contracts and verify deferred claims remain absent.
- Run CLI help and version smokes.

## Verification Notes

- Record guarded documentation sections, help output checks, and standalone/channel installation wording.

## Implementation Notes

- This task intentionally depends on existing `T-030-decouple-readme-structure-from-doc-tests`.
- Do not depend on an external design document or promise unresolved behavior.
