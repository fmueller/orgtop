---
id: T-089-resolve-terminal-capability-and-charset-at-launch
title: Resolve terminal capability and charset at launch
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#shared-glyph-and-recency-semantics
dependencies:
    - T-059-implement-shared-event-category-semantics
    - T-060-implement-shared-discrete-recency-semantics
updated_at: "2026-09-04T11:23:15Z"
---

# T-089-resolve-terminal-capability-and-charset-at-launch Resolve terminal capability and charset at launch

## Description

Resolve RG-008's rendering capabilities from the real process environment at
launch and inject them into the model as prepared state, so the shared category
and recency vocabularies render at a terminal's actual repertoire instead of
always falling back.

## Acceptance

- The color capability resolves through the pinned Charm color-profile stack,
  and `TERM=dumb` or a non-empty `NO_COLOR` forces `no-color`.
- The glyph repertoire resolves from the effective locale and `TERM` through the
  shared charset resolution, and `NO_COLOR` changes color only.
- Both capabilities are resolved once outside rendering, carried as prepared
  state, and remain injectable so tests need no live terminal.
- No view or renderer inspects the environment, the host clock, or the terminal
  for itself.

## Test Expectations

- Add tests that prove the launch wiring, not only the pure resolution helpers:
  an injected environment reaches the rendered category glyph and the recency
  styles, covering UTF-8 and non-UTF-8 locales, `TERM=dumb`, `NO_COLOR`, and the
  truecolor/ansi/no-color profiles.

## Verification Notes

- Record the resolved capability for each environment fixture and the rendered
  output it produced.

## Implementation Notes

- `resolveCharset` and the shared category vocabulary already exist in
  `internal/tui/category.go` with full unit coverage, but nothing calls them
  with the process environment: every production render currently takes the
  ASCII fallback regardless of the terminal (found while completing T-059).
- T-060 owns the color capability enum and the recency style table; this task
  only resolves and injects them.
- `github.com/charmbracelet/colorprofile` is presently an indirect dependency;
  promoting it to direct is expected here and must keep the T-043 toolchain and
  license guards green.
