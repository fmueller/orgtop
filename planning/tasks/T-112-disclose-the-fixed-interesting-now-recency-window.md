---
id: T-112-disclose-the-fixed-interesting-now-recency-window
title: Disclose the fixed Interesting Now recency window
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#interesting-now-strip
dependencies:
    - T-064-render-the-interesting-now-strip
    - T-103-extend-rain-windows-for-quiet-repositories
updated_at: "2026-09-19T18:36:06Z"
---

# T-112-disclose-the-fixed-interesting-now-recency-window Disclose the fixed Interesting Now recency window

## Description

Rain defaults to a 24-hour field but Interesting Now independently selects
events younger than 15 minutes. A populated field with an empty strip currently
lacks that explanation.

## Acceptance

- Amend RG-007's exact title, empty-state, collapse wording, and affected
  acceptance cases before implementation.
- Ordinary strip title and empty state explicitly identify the last 15 minutes.
  Explain that this is a scope-fair recent-event sample, not importance ranking.
- Define compact disclosure at supported narrow widths without losing
  shown/hidden/omitted accounting, quit priority, or tiny-width overflow hints.
- Rain window changes do not change strip eligibility. Preserve the exact
  15-minute boundary, fair selection, five-visible/twenty-stored limits,
  tick/refresh behavior, and pause semantics.
- Update related docs/help and the Unreleased changelog.

## Test Expectations

- An event aged 20 minutes remains in Rain at 24h while the strip explicitly
  reports none in the last 15m. Check immediately below and at 15m, pause,
  stale expiry, window changes, and collapsed layouts.
- Inspect wide/narrow/empty renders; run focused strip/Rain tests and
  `task check`.

## Verification Notes

Record boundary fixtures, commands, and inspected copy.

## Implementation Notes

- Required before distribution parity and final v0.2.0 release readiness.
- This changes disclosure, not event selection or Rain lifetime.
- 2026-09-19T18:35:53Z: verification pass
- 2026-09-19T18:36:06Z: Fixed Interesting Now disclosure is implemented, rendered/manual-tested, independently reviewed with all findings resolved, verified pass, and covered by the full repository gate.
