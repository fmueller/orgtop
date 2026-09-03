---
id: T-081-retire-v01-repository-snapshot-aggregation
title: Retire the v0.1 repository snapshot aggregation
status: todo
priority: low
spec_ref: specs/v0.2.0.md#explicit-scope-membership
dependencies:
    - T-057-render-mixed-scopes-in-overview
    - T-058-render-scope-context-in-stream
    - T-054-implement-scoped-snapshots-and-direct-aggregation
updated_at: "2026-09-03T11:52:19Z"
---

# T-081-retire-v01-repository-snapshot-aggregation Retire the v0.1 repository snapshot aggregation

## Description

T-054 added `domain.ScopedSnapshot` (`internal/domain/scoped_snapshot.go`) with
per-Scope aggregation, alongside the v0.1 `domain.Snapshot`
(`internal/domain/snapshot.go`) with its `Aggregate` rows and unexported
`aggregate` helper. Both bound, deduplicate, and sort the same event set, and
both count push and pull-request activity through the shared
`isPullRequestActivity`. Once T-056 publishes the scoped snapshot and T-057 and
T-058 render prepared Scope rows, `domain.Snapshot` has no production caller.
Decide whether it is removed or retained with a documented responsibility.

Follow-up derived from T-054-implement-scoped-snapshots-and-direct-aggregation's verification or discovery.

## Acceptance

- The v0.1 snapshot type is either removed together with `Aggregate`,
  `RepositoryActivity`, and their tests, or retained with a documented current
  responsibility rather than as an unused duplicate aggregation path.
- Repository Scope counts keep the FR-006 v0.1 direct category semantics, the
  500-event bound, deduplication, and reverse-chronological order whichever
  option is taken.
- No second aggregation path is left that a view could consume by accident.

## Test Expectations

- Keep the v0.1 repository aggregation coverage `internal/domain/snapshot_test.go`
  provides, moved onto the scoped snapshot if the type is removed.

## Verification Notes

- Record the call-site survey, the chosen option, and targeted domain and TUI
  test results.

## Implementation Notes

- Sequence this after T-056, T-057, and T-058; before then the TUI still consumes
  `domain.Snapshot`.
- Do not change `ScopedSnapshot` semantics, ordering, or bounds while doing this.
