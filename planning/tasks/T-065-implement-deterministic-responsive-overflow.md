---
id: T-065-implement-deterministic-responsive-overflow
title: Implement deterministic responsive overflow
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#responsive-and-degraded-experience
dependencies:
    - T-057-render-mixed-scopes-in-overview
    - T-058-render-scope-context-in-stream
    - T-062-render-and-navigate-the-rain-view
    - T-064-render-the-interesting-now-strip
updated_at: "2026-08-29T09:22:04Z"
---

# T-065-implement-deterministic-responsive-overflow Implement deterministic responsive overflow

## Description

Implement the closed RG-012 resize, overflow, paging/scrolling, label shortening, and constrained-layout policies across all three views.

## Acceptance

- Hidden rows, columns, and strip entries are reachable or explicitly accounted for according to the closed contract.
- At `40x10`, every view retains view identity, transport/freshness, primary content/state, and quit hint.
- Smaller positive dimensions remain bounded, deterministic, interactive, and panic-free without starving a Scope.

## Test Expectations

- Add dimension matrices for mixed Scope counts, overflow positions, wide runes, tiny positive sizes, repeated resize, and Rain column fairness.

## Verification Notes

- Record representative renders and overflow/fairness assertions.

## Implementation Notes

- Hide secondary detail before primary data and keep calculations bounded per update/render.
