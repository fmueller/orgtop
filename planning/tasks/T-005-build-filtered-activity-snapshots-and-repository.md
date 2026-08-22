---
id: T-005-build-filtered-activity-snapshots-and-repository
title: Build filtered activity snapshots and repository aggregates
status: completed
priority: high
spec_ref: specs/v0.1.0.md#activity-snapshot-aggregation
dependencies:
    - T-001-implement-the-event-domain-and-repository-scope
updated_at: "2026-08-22T10:51:31Z"
---

# T-005-build-filtered-activity-snapshots-and-repository Build filtered activity snapshots and repository aggregates

## Description

Turn source-independent Events plus resolved repository display identities into the
bounded snapshot consumed by both views. Keep calculations independent of GitHub
transport, scheduling metadata, and rendering.

## Acceptance

- Filtering precedes aggregation; unrelated candidate events never contribute.
- Snapshot deduplicates, sorts, and retains only the newest 500 Events.
- Every selected repository gets a deterministic aggregate, including zero-event
  repositories, using the successful result's display identity.
- Mixed-case returned identity and requested-spelling empty fallback remain stable
  without importing GitHub response types.
- Aggregates contain total, push, and PR activity counts under FR-006 semantics.
- Rows order by total descending then resolved name case-insensitively; inputs are not
  mutated unexpectedly.

## Test Expectations

- Cover mixed categories, returned identity, empty fallback, zero rows, duplicates,
  500-event bound, ties, out-of-scope candidates, and repeatability.
- Run focused aggregation tests and `task check`.

## Verification Notes

- Record exact commands, exit status, and named aggregation cases.

## Implementation Notes

- 2026-08-22T10:51:26Z: verification pass
