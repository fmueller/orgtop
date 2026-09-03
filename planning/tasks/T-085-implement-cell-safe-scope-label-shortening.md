---
id: T-085-implement-cell-safe-scope-label-shortening
title: Implement RG-012 cell-safe Scope label shortening
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#scope-aware-overview-and-stream
dependencies:
    - T-057-render-mixed-scopes-in-overview
updated_at: "2026-09-03T21:53:36Z"
---

# T-085-implement-cell-safe-scope-label-shortening Implement RG-012 cell-safe Scope label shortening

## Description

Implement RG-012's "Cell-Safe Shortening" as the one shortening function Overview,
Stream, Stream detail, Rain, and Interesting Now share. T-057 landed the RG-012
Scope tokens and labels but keeps rendering them through the v0.1 `shorten`
helper in `internal/tui/chrome.go`, which cuts on rune boundaries and appends a
trailing ellipsis rather than splitting the payload around one.

## Acceptance

- Label budgets are measured in rendered terminal cells; grapheme clusters stay
  indivisible and wide runes are never split.
- The token and one space are retained, and the remaining payload budget is split
  around one ellipsis, the prefix taking the larger half of the non-ellipsis cells.
- The degraded forms `<token> …`, `<token>`, the longest fitting token prefix, and
  the empty render appear at the budgets RG-012 names, and no renderer substitutes
  an unmarked payload fragment for the token.
- C0/C1 control, DEL, and the RG-012 bidi control code points in presentation
  payloads render as visible uppercase `\u{HEX}` escapes before measurement.
- Shortening never changes order, membership, column width or count, item
  admission, or any capacity decision.

## Test Expectations

- Pin A-076's normative width vectors: `P3 Acme/API:服務/界面/**` at budgets 24, 16,
  5, and 2, and the combining-mark label `acme/café/**` at budget 12.
- Cover two labels whose shortened payloads collide but whose tokens differ.

## Verification Notes

- Record the exact rendered strings and their cell widths at every pinned budget.

## Implementation Notes

- Use the repository-pinned Charm width and segmentation stack rather than byte or
  rune counts; upgrading a pinned behavior module must update the width vectors in
  the same change.
- Replace the Overview call site in `internal/tui/overview.go` and share the
  function with Stream and Rain rather than duplicating it per view.
