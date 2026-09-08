---
id: T-097-cut-shared-body-text-on-grapheme-clusters
title: Cut shared body text on grapheme clusters
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#scope-aware-overview-and-stream
dependencies:
    - T-085-implement-cell-safe-scope-label-shortening
updated_at: "2026-09-08T08:17:07Z"
---

# T-097-cut-shared-body-text-on-grapheme-clusters Cut shared body text on grapheme clusters

## Description

T-085 landed RG-012's cell-safe shortening for Scope labels, which cuts on whole
grapheme clusters through the pinned Charm stack. The v0.1 `shorten` helper in
`internal/tui/chrome.go:356` and the `truncate` it builds on
(`internal/tui/chrome.go:368`) still cut rune by rune, so they can split a
combining sequence. They shorten the Stream and Rain bodies and the Interesting
Now strip lines (`internal/tui/stream.go:55`, `internal/tui/rain_render.go:144`,
`internal/tui/model.go:416`), so the package now carries two truncation
semantics for non-label content.

## Acceptance

- The shared body cut is grapheme-cluster aware, so no rendered body line splits a
  combining sequence or a wide rune.
- The mark is still paid for out of the same limit and a limit too tight for it
  still yields nothing rather than an unmarked cut.
- Stream, Rain, and Interesting Now rendered output is unchanged for content that
  carries no multi-code-point cluster.

## Verification Notes

- Record the rendered strings and cell widths of a combining and a wide-rune body
  line at the limits the existing chrome tests pin.

## Implementation Notes

- 2026-09-08T08:17:03Z: verification pass
