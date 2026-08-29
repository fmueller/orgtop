---
id: T-058-render-scope-context-in-stream
title: Render Scope context in Stream
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#scope-aware-overview-and-stream
dependencies:
    - T-056-integrate-enrichment-into-atomic-refresh
updated_at: "2026-08-29T09:22:04Z"
---

# T-058-render-scope-context-in-stream Render Scope context in Stream

## Description

Extend Stream with deterministic, bounded Scope context while preserving reverse-chronological investigation.

## Acceptance

- Scope labels, RG-012 overlap presentation, quantitative unknown/incomplete coverage, scrolling/position indicators, ordering, stale/degraded state, and compact rendering implement the closed contracts.
- A multi-Scope event is neither accidentally duplicated nor stripped of membership context.
- Existing Stream order and v0.1 state retention remain intact.

## Test Expectations

- Add tests for overlap, long labels, bounded context, scrolling, ties, unknown states, resize, wide runes, and `40x10`.

## Verification Notes

- Record deterministic rows and viewport/overflow evidence.

## Implementation Notes

- Render prepared membership context only; do not perform matching in Stream.
