---
id: T-069-integrate-the-closed-v0-2-0-binary-flow
title: Integrate the closed v0.2.0 binary flow
status: todo
priority: high
spec_ref: specs/v0.2.0.md#integration-documentation-and-release-readiness
dependencies:
    - T-066-implement-honest-degraded-state-presentation
    - T-068-verify-distribution-channel-parity
updated_at: "2026-08-29T09:22:04Z"
---

# T-069-integrate-the-closed-v0-2-0-binary-flow Integrate the closed v0.2.0 binary flow

## Description

Wire the completed Scope, organization, enrichment, cache, membership, and three-view components through the standalone executable.

## Acceptance

- End-to-end startup, auth, selection, refresh, enrichment, cache, aggregation, navigation, degraded state, cancellation, and exit implement all closed normative contracts.
- Exact repository-only invocations preserve v0.1 behavior and avoid path enrichment.
- Architecture follows adapter to normalization to enrichment to membership to aggregation to application state to rendering.

## Test Expectations

- Add deterministic binary/model integration fixtures for mixed Scopes, organizations, overlap, cache reuse/failure, unknowns, Rain, narrow terminals, rate limits, stale recovery, and shutdown.

## Verification Notes

- Record integrated scenarios, request counts, state transitions, and sanitized outputs.

## Implementation Notes

- Keep source payloads in adapters, domain calculations outside TUI, and SQLite behind enrichment.
- Add no deferred source or analytics architecture.
