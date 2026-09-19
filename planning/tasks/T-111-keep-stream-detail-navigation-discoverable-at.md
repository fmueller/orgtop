---
id: T-111-keep-stream-detail-navigation-discoverable-at
title: Keep Stream detail navigation discoverable at narrow widths
status: todo
priority: high
spec_ref: specs/v0.2.0.md#responsive-and-degraded-experience
dependencies:
    - T-108-make-stream-arrow-key-navigation-visibly
updated_at: "2026-09-19T08:53:27Z"
---

# T-111-keep-stream-detail-navigation-discoverable-at Keep Stream detail navigation discoverable at narrow widths

## Description

At 40 columns Stream descriptions shrink to fragments while enter detail and
esc back disappear from the footer. Preserve discoverability where detail is
most useful, after visible focus is implemented by T-108.

## Acceptance

- Clarify FR-011/RG-012 footer priorities and narrow acceptance cases before
  implementation.
- At 40 columns the list advertises Enter for detail and open detail advertises
  Esc for return, alongside quit. Prefer these contextual hints over redundant
  navigation text.
- Define deterministic smaller-width fallback with mandatory quit priority
  and no overflow; preserve existing Rain footer accounting.
- Hints name supported actions and match visible focus/detail behavior.
- Update directly affected docs/help and the Unreleased changelog.

## Test Expectations

- Assert list/detail hints at 40 columns and layout boundaries; exercise
  Enter/Esc and no-color output. Guard unchanged Rain accounting and tiny-width
  quit behavior.
- Inspect narrow list/detail renders, run targeted TUI tests and `task check`.

## Verification Notes

Record exact footer outputs, navigation checks, commands, and inspected renders.

## Implementation Notes

- Required before distribution parity and final v0.2.0 release readiness.
- Introduce no extra primary view or network work.
