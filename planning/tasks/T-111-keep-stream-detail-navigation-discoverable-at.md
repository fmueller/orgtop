---
id: T-111-keep-stream-detail-navigation-discoverable-at
title: Keep Stream detail navigation discoverable at narrow widths
status: completed
priority: high
spec_ref: specs/v0.2.0.md#responsive-and-degraded-experience
dependencies:
    - T-108-make-stream-arrow-key-navigation-visibly
updated_at: "2026-09-19T16:50:32Z"
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
- 2026-09-19T16:48:46Z: verification pass
- The normative v0.2.0 amendment clarifies FR-011/RG-012's one-line footer
  priority: Stream keeps `enter detail` or `esc back` with `q quit` before
  redundant navigation/scroll hints, while Rain keeps RG-007 accounting.
- `contextualFooterLadder` inserts only the Stream list/detail contextual rung;
  the generic, Overview, and Rain ladders remain unchanged.
- Strict TDD was red against the pre-change footer at `40x10`, `30x10`, `21x10`,
  and fit boundaries, then green after the minimal change. Regression coverage
  includes `40x10`, `30x10`, `21x10`, `20x10`, `19x10`, `17x10`, `16x10`, and
  `6x10` in ASCII/UTF-8 no-color renders.
- Manual report and inspected render were recorded in the workflow evidence.
- `mise exec -- task check` passed, including tests, lint, vet, distribution
  guards, mutation-floor checks, build, startup smoke, licenses, Taskrail
  validation/coverage, and GoReleaser validation. Independent General and
  Go/TUI/domain reviews, candidate validation, and disposition verification
  returned no concrete findings.
- 2026-09-19T16:50:32Z: Implemented the FR-011/RG-012 footer priority amendment and Stream contextual hints; verified with strict TDD, full task check, manual renders, and independent review.
