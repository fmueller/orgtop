---
id: T-060-implement-shared-discrete-recency-semantics
title: Implement shared discrete recency semantics
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#shared-glyph-and-recency-semantics
dependencies:
    - T-059-implement-shared-event-category-semantics
updated_at: "2026-09-04T11:37:49Z"
---

# T-060-implement-shared-discrete-recency-semantics Implement shared discrete recency semantics

## Description

Implement the closed discrete recency model and shared semantic style mapping using explicit time inputs.

## Acceptance

- Threshold boundaries, new/recent/aging/expired progression, expiry, styles, and no-color behavior implement RG-008 exactly.
- Recency changes emphasis only and never category, importance, quality, severity, or anomaly meaning.
- Views consume one prepared recency vocabulary.

## Test Expectations

- Add controlled-clock boundary tests and cross-view assertions for every state and expiry transition.

## Verification Notes

- Record threshold-edge results and shared-use evidence.

## Implementation Notes

- Inject explicit time/age inputs; do not read wall time during rendering.
- 2026-09-04T11:37:46Z: verification pass
