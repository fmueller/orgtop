---
id: T-033-close-the-shared-visual-semantics-contract
title: Close the shared visual semantics contract
status: completed
priority: high
spec_ref: specs/v0.2.0.md#rg-008-shared-visual-semantics-contract
dependencies: []
updated_at: "2026-08-29T11:00:54Z"
---

# T-033-close-the-shared-visual-semantics-contract Close the shared visual semantics contract

## Description

Inventory shipped meanings and close RG-008 with one accessible category and recency vocabulary for all views.

## Acceptance

- The spec defines the category mapping, glyphs, ASCII fallback, recency thresholds, styles, no-color behavior, and unsupported-category rendering.
- Shipped v0.1 meanings are preserved.
- No visual encoding invents failure, success, deployment, anomaly, or priority facts.

## Test Expectations

- Add cross-view semantic vectors for every category, fallback mode, recency state, and unsupported category.

## Verification Notes

- Record the v0.1 inventory and the final shared table.
- Record `taskrail validate`.

## Implementation Notes

- Do not modify TUI code in this task.
- Own semantic calculation outside rendering; color may only reinforce non-color encoding.
- 2026-08-29T11:00:47Z: verification pass
- 2026-08-29T11:00:54Z: Closed RG-008 with user-approved category, glyph, recency, style, and fallback semantics plus A-025 through A-027.
