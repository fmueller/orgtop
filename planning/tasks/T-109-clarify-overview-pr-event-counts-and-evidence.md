---
id: T-109-clarify-overview-pr-event-counts-and-evidence
title: Clarify Overview PR event counts and evidence labels
status: completed
priority: high
spec_ref: specs/v0.2.0.md#scope-aware-overview-and-stream
dependencies:
    - T-054-implement-scoped-snapshots-and-direct-aggregation
updated_at: "2026-09-19T13:44:56Z"
---

# T-109-clarify-overview-pr-event-counts-and-evidence Clarify Overview PR event counts and evidence labels

## Description

Overview labels PR-related events as pull requests even though reviews and PR
comments contribute. Current PR denotes membership evidence, not the open-PR
backlog. Remove these misleading interpretations without changing aggregation.

## Acceptance

- Align FR-007, RG-004/RG-012, and affected acceptance wording with explicit
  event-count semantics before changing presentation.
- Overview uses PR events and an unambiguous compact spelling rather than
  implying distinct, open, or waiting pull-request counts.
- Explain current-PR qualification as membership established from an open PR's
  current files, retaining its subset relationship and the unknown distinction.
- Preserve counts, ordering, overlap behavior, and source/enrichment work.
- Update directly affected help/docs and the Unreleased changelog.

## Test Expectations

- Multiple reviews/comments and a PR event on the same PR count as separate
  eligible events, not unique PRs. Include an unrelated issue comment and
  qualified path membership.
- Assert rich/compact labels; inspect wide, narrow, and no-color renders.
  Run targeted domain/TUI tests and `task check`.

## Verification Notes

Record independently derived counts, commands, and inspected output.

## Implementation Notes

- Required before distribution parity and final v0.2.0 release readiness.
- Do not turn this wording correction into a PR-backlog feature.
- 2026-09-19T13:44:41Z: verification pass
- 2026-09-19T13:44:56Z: Overview PR event/evidence labels clarified; aggregation and cross-view invariants preserved; verification pass recorded.
