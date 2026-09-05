---
id: T-095-preserve-viewport-state-through-zero-row-resize
title: Preserve viewport state through zero-row resize
status: completed
priority: low
spec_ref: specs/v0.2.0.md#responsive-and-degraded-experience
dependencies:
    - T-065-implement-deterministic-responsive-overflow
updated_at: "2026-09-05T15:04:19Z"
---

# T-095-preserve-viewport-state-through-zero-row-resize Preserve viewport state through zero-row resize

## Description

Preserve valid Overview offsets and Stream focus/offset state while a reported
terminal height leaves zero body rows, so restoring a usable height returns the
operator to the same content instead of jumping to the top.

Follow-up derived from T-065-implement-deterministic-responsive-overflow's verification or discovery.

## Acceptance

- Shrinking a scrolled Overview or Stream to a chrome-only height preserves its
  valid numeric navigation state.
- Restoring the prior dimensions restores the prior visible range.
- Empty or refreshed-shorter content still clamps retained state safely.

## Verification Notes

- Record failing-first targeted tests and the full CI-equivalent check.

## Implementation Notes

- 2026-09-05T15:04:19Z: verification pass
- 2026-09-05T15:04:19Z: Preserved valid Overview and Stream offsets at zero body-row capacity and verified restoration with focused and full checks
