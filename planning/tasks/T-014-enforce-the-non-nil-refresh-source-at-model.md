---
id: T-014-enforce-the-non-nil-refresh-source-at-model
title: Enforce the non-nil refresh source at model construction
status: completed
priority: low
spec_ref: specs/v0.1.0.md#application-shell-and-refresh-lifecycle
dependencies:
    - T-007-implement-the-asynchronous-refresh-lifecycle
updated_at: "2026-08-22T15:12:20Z"
---

# T-014-enforce-the-non-nil-refresh-source-at-model Enforce the non-nil refresh source at model construction

## Description

`tui.New` documents that its `Source` must not be nil but does not enforce it. A
nil source panics later inside the Bubble Tea command goroutine that runs the
refresh instead of failing fast where the caller can see it.

Follow-up derived from T-007-implement-the-asynchronous-refresh-lifecycle's verification or discovery.

## Acceptance

- Constructing the root model without a source fails fast at the construction
  seam rather than inside an asynchronous refresh command.
- The chosen failure mode is documented on the constructor and covered by a test.
- No public API beyond the constructor contract changes.

## Verification Notes

- Record exact commands, exit status, and the constructor guard test.

## Implementation Notes

- go-reviewer finding on T-007 (low): `New`'s "the source must not be nil"
  precondition at `internal/tui/model.go` is unenforced and untested, and the
  panic surfaces at `internal/tui/refresh.go` inside the refresh command.
- 2026-08-22T15:12:15Z: verification pass
