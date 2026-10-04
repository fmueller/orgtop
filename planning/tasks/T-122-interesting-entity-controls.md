---
id: T-122-interesting-entity-controls
title: Escape controls in Interesting Now entity references
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#rg-012-mixed-scope-presentation-contract
dependencies: []
updated_at: "2026-10-04T21:21:46Z"
---

# T-122-interesting-entity-controls Escape controls in Interesting Now entity references

## Description

Interesting Now renders the source-derived entity reference without RG-012's
visible control escaping. A hostile malformed PushEvent `payload.head` containing
`bad` + U+202E + `HEAD` survives normalization and appears literally in the Rain
strip as `commit bad<raw U+202E>HEAD`. Event detail correctly shows
`commit · bad\u{202E}HEAD` for the same event.

Reproduction on the unchanged built executable:

1. Build `CGO_ENABLED=0 go build -o /tmp/orgtop-round2 ./cmd/orgtop`.
2. Serve `docs/testing/round2/hostile-entity.json` as a successful JSON array for
   `GET /repos/acme/api/events?per_page=100` through the bounded loopback HTTPS
   fixture transport described in `docs/testing/round2/report.md`. Refresh its
   timestamp to within the last 15 minutes. Use only a synthetic token.
3. In a 240x32 PTY, run `orgtop --repo acme/api --no-cache`; press `3`.
4. Capture the Interesting Now row and inspect its code points: U+202E is raw,
   and its visible uppercase escape is absent. Press `2`, then Enter: detail
   contains the visible escape and no raw U+202E.

Expected: no source-derived entity reference becomes terminal control or bidi
output; escape it before cell measurement just as detail does. Actual: the rich
Interesting Now form emits the raw control. This is a malformed-input rendering
contract failure, not a claim that GitHub generates non-hexadecimal commit SHAs
or that a valid branch controls a commit SHA.

## Acceptance

- Interesting Now entity references render RG-012's C0/C1, DEL and bidi controls
  as visible uppercase `\u{HEX}` before measuring or choosing a width register.
- Reuse the existing presentation escaping ownership; preserve immutable source
  identifiers, event membership, ordering, strip selection and normal Unicode.
- Keep wide and constrained forms within the cell budget and consistent with
  existing event-detail escaping. Ordinary hexadecimal commit references remain
  unchanged.

## Test Expectations

- Add a source-adapter-to-renderer regression using a hostile `payload.head`, not
  only `payload.ref` or actor. Assert the raw-control absence and visible escape
  at a width that retains the entity field; include constrained widths.
- Cover the RG-012 bidi set and representative other controls; preserve positive
  width graphemes and source identity. Compare the same event's existing detail.
- Reproduce the fixed behavior using the actual built binary/HTTPS/PTY flow.

## Verification Notes

- Confirmed repeatedly on Linux amd64 at
  b42b880952a906bff73478fe219677af5d0cb873, pinned to specs/v0.2.0.md.
- `docs/testing/round2/entity-rain.txt`, `entity-detail.txt`, and
  `entity-check.json` retain actual capture and code-point evidence.
- T-121 is completed and its valid-branch/actor regression passes. It corrected
  Stream descriptions and the Interesting Now actor, not this entity field.
  Its earlier rejected branch-to-entity hypothesis remains correctly rejected:
  this reproduction instead supplies the hostile value in `payload.head`.
- Open T-071, T-106 and T-118 do not cover this defect. No duplicate task found.

## Implementation Notes

- `internal/github/normalize.go` classifies push `payload.head` as `EntityRef`;
  `internal/tui/interesting.go` retains it. In `interesting_render.go`, `form`
  escapes the actor but appends `entityText()` unescaped before `lipgloss.Width`.
- No fix was implemented in adversarial testing round 2. This task remains todo.
