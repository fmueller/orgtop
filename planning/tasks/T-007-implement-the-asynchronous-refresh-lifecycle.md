---
id: T-007-implement-the-asynchronous-refresh-lifecycle
title: Implement the asynchronous refresh lifecycle
status: todo
priority: high
spec_ref: specs/v0.1.0.md#application-shell-and-refresh-lifecycle
dependencies:
    - T-006-build-the-bubble-tea-application-shell
    - T-004-implement-the-bounded-github-activity-source
updated_at: "2026-08-21T22:39:23Z"
---

# T-007-implement-the-asynchronous-refresh-lifecycle Implement the asynchronous refresh lifecycle

## Description

Connect the injected source and snapshot builder to the Bubble Tea model through
asynchronous commands and deterministic timer seams. Own single-flight scheduling,
atomic publication, freshness transitions, and source cancellation while leaving
view-specific rendering to T-009 and T-010.

## Acceptance

- Init renders LOADING immediately and starts source I/O only through a command;
  keyboard, resize, and quit remain responsive while work is pending.
- At most one refresh is in flight; each timer starts after completion using success
  or structured failure delay metadata.
- First failure produces ERROR; later failure preserves the snapshot under STALE;
  complete/empty success publishes atomically, records freshness, and clears errors.
- Failed multi-repository results never publish partial candidates; success interval
  recalculation and rate-limited failure delay are honored.
- Shutdown cancels in-flight source work; scheduling/error metadata stays outside
  snapshots and views.

## Test Expectations

- Use fake pending/success/ordinary-failure/rate-limited-failure sources and
  deterministic clocks/timers for responsiveness, no overlap, delay, states,
  atomicity, recovery, empty success, and cancellation.
- Run focused lifecycle tests and `task check`.

## Verification Notes

- Record exact commands, exit status, and transition/scheduling/cancellation tests.

## Implementation Notes

- T-006 handoff: the shell header drops scope context near `40` columns whenever
  `State.Cause` is set, because the candidate ladder in `internal/tui/chrome.go`
  prefers the cause over the repository count. Revisit that ordering once this
  task produces real sanitized cause strings, and confirm the `40x10`
  `STALE`-with-cause layout still reads well.
