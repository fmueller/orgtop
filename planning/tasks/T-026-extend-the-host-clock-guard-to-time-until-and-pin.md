---
id: T-026-extend-the-host-clock-guard-to-time-until-and-pin
title: Extend the host clock guard to time.Until and pin the github request deadline test
status: todo
priority: low
spec_ref: specs/v0.1.0.md#github-activity-source
dependencies:
    - T-025-inject-a-fake-clock-into-tests
updated_at: "2026-08-24T11:28:16Z"
---

# T-026-extend-the-host-clock-guard-to-time-until-and-pin Extend the host clock guard to time.Until and pin the github request deadline test

## Description

T-025 added the repository-wide host clock guard in
`internal/toolchain/determinism_test.go`, but scoped it to `time.Now` and
`time.Since`, the two readers its own acceptance named.

`time.Until(d)` is `d.Sub(time.Now())`: it reads the host clock exactly as much.
One site still calls it. `internal/github/source_test.go:93` converts the
recorded request deadline into a remaining duration, and
`internal/github/source_test.go:232` then asserts that duration is positive and
no greater than the 30s request timeout (`internal/github/source.go:24`,
`internal/github/source.go:139`). The 30s budget makes the assertion robust in
practice, but it is a design dependency on the real clock, which NFR-006 rules
out, and the guard cannot grow to cover `time.Until` while it stands.

`internal/auth/credential_test.go` already shows the shape the replacement
takes: record the absolute deadline rather than a remaining duration, and pin
the budget by bracketing it with caller deadlines on either side, so the
assertion rests on the min() semantics of `context.WithTimeout` rather than on
elapsed real time.

## Acceptance

- `internal/github/source_test.go` records the absolute request deadline instead
  of a duration derived from the host clock, and still pins the 30s request
  timeout: a caller deadline inside the budget survives untouched, one beyond it
  is shortened.
- `hostClockReaders` in `internal/toolchain/determinism_test.go` lists `Until`
  alongside `Now` and `Since`, and the comment recording why `Until` was
  deferred is removed.
- No non-benchmark test in the repository calls `time.Now`, `time.Since`, or
  `time.Until`.

## Test Expectations

- The existing `TestNoTestReadsTheHostClock` covers the new reader once it is
  listed; extend `TestHostClockReadsInSeesThroughEveryImportForm` only if the
  detection logic itself changes.
- The reshaped github deadline assertion must fail when the request timeout is
  widened, shortened, or removed.

## Verification Notes

- Record `task check`, `go test ./... -race`, and `git diff --check` exit
  statuses.
- Regression proof: widening `requestTimeout` in `internal/github/source.go`
  must fail the reshaped deadline test.
- Regression proof: restoring `time.Until` to `internal/github/source_test.go`
  must fail `TestNoTestReadsTheHostClock`.

## Implementation Notes

- Found by the T-025 review pass, which flagged the `time.Until` site as a
  disclosed gap in an otherwise repository-wide guard.
- Purely test-side: no production behavior changes, and the 30s request timeout
  itself stays exactly as it is.
- T-024, the v0.1.0 release gate, depends on this task: its determinism
  acceptance cannot be met while a test still measures a deadline against the
  host clock.
