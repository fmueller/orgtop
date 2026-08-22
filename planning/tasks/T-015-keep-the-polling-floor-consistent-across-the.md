---
id: T-015-keep-the-polling-floor-consistent-across-the
title: Keep the polling floor consistent across the source and the lifecycle
status: todo
priority: low
spec_ref: specs/v0.1.0.md#application-shell-and-refresh-lifecycle
dependencies:
    - T-007-implement-the-asynchronous-refresh-lifecycle
updated_at: "2026-08-22T11:37:45Z"
---

# T-015-keep-the-polling-floor-consistent-across-the Keep the polling floor consistent across the source and the lifecycle

## Description

FR-004 fixes the polling floor at 60 seconds. Two packages now hardcode it
independently: `defaultInterval` in the GitHub source adapter and `defaultDelay`
in the refresh lifecycle, which cannot import the adapter. Nothing keeps the two
in sync if the spec floor ever changes.

Follow-up derived from T-007-implement-the-asynchronous-refresh-lifecycle's verification or discovery.

## Acceptance

- The FR-004 polling floor has one enforced source of truth, or a test that
  fails when the adapter floor and the lifecycle floor diverge.
- The existing package boundary holds: the TUI still does not import the GitHub
  adapter.
- No scheduling behavior changes.

## Verification Notes

- Record exact commands, exit status, and the parity test.

## Implementation Notes

- go-reviewer finding on T-007 (low): `defaultDelay` in
  `internal/tui/refresh.go` duplicates `defaultInterval` in
  `internal/github/schedule.go`; the duplication is deliberate but undefended.
