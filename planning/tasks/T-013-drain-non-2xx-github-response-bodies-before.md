---
id: T-013-drain-non-2xx-github-response-bodies-before
title: Drain non-2xx GitHub response bodies before closing
status: todo
priority: low
spec_ref: specs/v0.1.0.md#github-activity-source
dependencies:
    - T-004-implement-the-bounded-github-activity-source
updated_at: "2026-08-22T10:36:51Z"
---

# T-013-drain-non-2xx-github-response-bodies-before Drain non-2xx GitHub response bodies before closing

## Description

`Source.fetch` classifies a non-2xx status before reading the body, so error
responses are closed without being drained. `net/http` can then only reuse the
keep-alive connection when the body has been consumed, so each failed repository
refresh may cost a new TCP/TLS handshake. Behavior is correct today; this is a
connection-reuse nit raised during the T-004 review.

## Acceptance

- Non-2xx repository responses drain a bounded amount of the body before closing
  it, so an idle keep-alive connection can be reused.
- Error classification, reported causes, and retry delays are unchanged.
- A malicious or oversized error body cannot force an unbounded read.

## Verification Notes

- Record `task check` output and the focused `internal/github` source tests.

## Implementation Notes

- The affected call site is `Source.fetch` in `internal/github/source.go`.
- The v0.1.0 poll floor is 60 seconds per repository, so the practical impact is
  small; keep the change minimal and do not add retries or pooling configuration.
