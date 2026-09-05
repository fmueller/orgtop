---
id: T-063-select-bounded-interesting-now-entries
title: Select bounded Interesting Now entries
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#interesting-now-strip
dependencies:
    - T-054-implement-scoped-snapshots-and-direct-aggregation
    - T-060-implement-shared-discrete-recency-semantics
updated_at: "2026-09-05T07:49:26Z"
---

# T-063-select-bounded-interesting-now-entries Select bounded Interesting Now entries

## Description

Implement deterministic Interesting Now eligibility, ordering, retention, and bounds from prepared direct event facts.

## Acceptance

- Selection implements all closed RG-007/RG-009 rules for ties, duplicate source events, multi-Scope fairness/global selection, unknowns, refresh, recency, stored and visible limits.
- Entries contain only normalized in-Scope directly observable events.
- No baseline, anomaly, productivity, severity, inferred importance, or opaque score is calculated.

## Test Expectations

- Add pure table tests for over-limit inputs, ordering ties, duplicate source events, multi-Scope membership, fairness/starvation, unknowns, refresh replacement, expiry, and empty results.

## Verification Notes

- Record explainable inputs for each selected result and all bound assertions.

## Implementation Notes

- Keep selection outside Rain rendering and separate from the full Stream.
- 2026-09-05T07:49:22Z: verification pass
