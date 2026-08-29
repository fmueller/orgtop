---
id: T-064-render-the-interesting-now-strip
title: Render the Interesting Now strip
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#interesting-now-strip
dependencies:
    - T-063-select-bounded-interesting-now-entries
    - T-062-render-and-navigate-the-rain-view
updated_at: "2026-08-29T09:22:04Z"
---

# T-064-render-the-interesting-now-strip Render the Interesting Now strip

## Description

Render selected Interesting Now entries as a readable bounded companion to the Rain field.

## Acceptance

- Event, Scope/entity, recency context, no-color encoding, collapse, empty state, and height behavior implement the closed contract.
- The strip never grows with uptime or event volume and never appears to be anomaly intelligence or a second Stream.
- Collapsing under constrained height remains understandable.

## Test Expectations

- Add render tests for maximum entries, long content, no color, empty/collapsed states, mixed Scopes, and narrow heights.

## Verification Notes

- Record bounded line counts and representative renders.

## Implementation Notes

- Render prepared entries without reranking or recency calculation.
