---
id: T-052-implement-bounded-enrichment-coordination
title: Implement bounded enrichment coordination
status: todo
priority: high
spec_ref: specs/v0.2.0.md#changed-file-enrichment
dependencies:
    - T-049-implement-changed-file-enrichment-in-the-github
    - T-051-implement-cache-freshness-and-deterministic
    - T-044-implement-the-unified-scope-domain-model
updated_at: "2026-08-29T09:22:04Z"
---

# T-052-implement-bounded-enrichment-coordination Implement bounded enrichment coordination

## Description

Coordinate cache reuse and GitHub enrichment as bounded, cancelable, coalesced application work.

## Acceptance

- Concurrency, queueing, request bounds, timeouts, cache reuse, duplicate coalescing, rate-limit delays, and retries match the closed RG-003/RG-005/RG-009 contracts.
- Identical entities shared by events or Scopes do not trigger one request per Scope.
- Work never runs synchronously in rendering or keyboard handling.
- Coordination drives the store's freshness and maintenance surface that T-051 shipped
  but left without a caller: one batched hit touch per refresh, a maintenance check, and
  at most one bounded cleanup batch, all outside TUI input and rendering paths.

## Test Expectations

- Add deterministic fake-adapter/cache tests for coalescing, bounds, cancellation, cache hits/misses, rate limits, and no-tight-retry behavior.
- Prove the refresh performs at most one touch transaction and one cleanup batch, and that
  a skipped or contended cache operation degrades the refresh without failing it.

## Verification Notes

- Record peak concurrency, queue limits, request counts, and cancellation results.

## Implementation Notes

- Keep orchestration in application/source services, not the domain or TUI renderer.
- `internal/cache` exposes `Lookup`, `Save`, `Touch`, `MaintenanceDue`, and `Maintain`; the
  cache has no caller yet, so this task closes RG-009's requirement that cleanup runs as
  bounded background work rather than inside a renderer or key handler.
