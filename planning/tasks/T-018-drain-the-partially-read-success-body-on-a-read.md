---
id: T-018-drain-the-partially-read-success-body-on-a-read
title: Drain the partially read success body on a read failure
status: todo
priority: low
spec_ref: specs/v0.1.0.md#github-activity-source
dependencies:
    - T-013-drain-non-2xx-github-response-bodies-before
updated_at: "2026-08-22T14:58:27Z"
---

# T-018-drain-the-partially-read-success-body-on-a-read Drain the partially read success body on a read failure

## Description

`Source.fetch` drains a bounded prefix of a non-2xx body before closing it
(T-013), but the 2xx path is asymmetric: when `io.ReadAll(response.Body)` fails
partway, for example on a mid-stream reset or an expired request deadline, the
remaining body is closed undrained, so `net/http` cannot reuse that keep-alive
connection. Behavior is correct today; this is the residual connection-reuse gap
the T-013 review recorded as low severity and out of that task's scope.

## Acceptance

- A failed `io.ReadAll` on a 2xx repository response drains a bounded amount of
  the remaining body before closing it.
- The drain bound is shared with the non-2xx path rather than duplicated.
- Reported causes and retry delays for read failures are unchanged.
- A malicious or oversized body cannot force an unbounded read.

## Verification Notes

- Record `task check` output and the focused `internal/github` source tests.

## Implementation Notes

- The affected call site is the `io.ReadAll` error branch of `Source.fetch` in
  `internal/github/source.go`; reuse `errorBodyDrainLimit`.
- Evidence for the gap: the T-013 go-reviewer finding, recorded in
  `planning/artifacts/verify/T-013-drain-non-2xx-github-response-bodies-before/`.
- Keep the change minimal: no retries and no pooling configuration.
