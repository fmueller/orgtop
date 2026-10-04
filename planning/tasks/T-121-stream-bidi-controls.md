---
id: T-121-stream-bidi-controls
title: Escape bidi controls in Stream event summaries
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#rg-012-mixed-scope-presentation-contract
dependencies: []
updated_at: "2026-10-04T21:05:10Z"
---

# T-121-stream-bidi-controls Escape bidi controls in Stream event summaries

## Description

An actual binary/HTTPS/PTY flow leaves U+202E (right-to-left override) raw in
Stream's event description, violating RG-012's visible uppercase escapes for
bidi presentation controls. The same event's detail correctly renders
`release-\u{202E}abc`. This is not limited to an impossible GitHub login: the
reproduction uses a valid Git branch name (`git check-ref-format` exits 0).

Reproduction:

1. Build the unchanged executable and serve `hostile-event.json` from
   `docs/testing/round1/` as the repository events response through a bounded
   local HTTPS CONNECT proxy. Use only a disposable synthetic credential.
2. Start `orgtop --repo acme/api --path acme/api:src --no-cache` in a 120x30
   terminal. Return `commit.json` for the fixture head's commit request.
3. Press `2`. Capture the terminal row: its description contains a literal
   U+202E before `abc`, not the visible ASCII escape `\u{202E}`.
4. Press Enter on that push. Detail escapes the same value correctly.

In a bidi-aware rendering of the actual terminal capture the branch appears
as `release-cba`, demonstrating the misleading presentation; the byte-level
capture, not the image, proves the control remains raw.

## Acceptance

- Stream's source-derived summary fields cannot emit raw RG-012 bidi controls;
  serialize them as visible uppercase `\u{HEX}` before width measurement and
  shortening, consistently with detail and Scope labels.
- Preserve ordinary Unicode/grapheme presentation, source identity, membership,
  event ordering, and existing constrained-width behavior.
- Check other summary surfaces consuming the same prepared source text for this
  exact leak; apply the existing sanitization ownership rather than inventing a
  second escaping scheme.

## Test Expectations

- Add a real adapter-to-renderer fixture containing U+202E in a valid branch
  name. Assert absence of raw controls and presence of the visible escape in
  wide Stream output, with cell-safe behavior at constrained widths.
- Include the existing detail behavior as a consistency assertion and cover
  the RG-012 bidi set, not just ASCII escape-sequence stripping.

## Verification Notes

- Confirmed at d1dabc381adeec81b8b08da24fea1d4ebabbbf0d on Linux amd64.
- `docs/testing/round1/hostile-stream.txt` and `hostile-detail.txt` retain actual
  tmux captures; `bidi-check.json` records raw-control and escaped-text checks.
- Completed T-085 covers Scope-label sanitization and completed T-086 covers
  detail; neither fixes Stream summaries. No duplicate open task was found.

## Implementation Notes

- Inspect `rowDetail` in `internal/tui/stream.go` and the existing detail/label
  sanitization helper. Do not change immutable source identifiers to fix display.
- No fix was implemented in this round.
- 2026-10-04T21:05:10Z: verification pass
- 2026-10-04T21:05:10Z: Verified after workflow-v3 review and final task check; existing TUI sanitizer only, source identity unchanged. Evidence docs/testing/t121-stream-bidi.md.
