---
id: T-004-implement-the-bounded-github-activity-source
title: Implement the bounded GitHub activity source
status: completed
priority: high
spec_ref: specs/v0.1.0.md#github-activity-source
dependencies:
    - T-003-normalize-github-repository-event-payloads
updated_at: "2026-08-22T10:37:10Z"
---

# T-004-implement-the-bounded-github-activity-source Implement the bounded GitHub activity source

## Description

Implement the cancelable GitHub REST source for one atomic multi-repository refresh.
Use normalization and return structured success/failure scheduling metadata. Do not
implement the Bubble Tea polling loop here.

## Acceptance

- Each Scope entry requests one `GET /repos/{owner}/{repo}/events?per_page=100` page
  with specified headers and a request timeout no greater than 30 seconds.
- Success contains normalized events, one display identity per Scope entry, and poll
  delay; empty responses use requested spelling and non-empty responses use the first
  matching returned spelling.
- Failure exposes no consumable partial data and carries a sanitized error plus next
  eligible retry delay. Scheduling/error metadata does not enter Events/snapshots.
- 401, both 403 classes, 404, 429, malformed data, mismatch, empty response, and
  cancellation follow FR-003/FR-004.
- Poll/rate-limit headers produce deterministic delay metadata with safe fallback.
- No extra pages, retries, persistence, live tests, or raw payload logging are added.

## Test Expectations

- Use injected HTTP transport/fixture server for exact requests, atomicity, display
  identity, status classes, malformed/empty data, cancellation, and headers.
- Use a sentinel credential and assert it is absent from captured output/errors.
- Cover the normalization verb branches deferred from T-003: a `reopened`
  pull request and a present but unrecognized review state such as `dismissed`
  or `commented` fall back to their generic verbs.
- Run focused source tests and `task check`.

## Verification Notes

- Record exact commands, exit status, and request/atomicity/delay/redaction tests.

## Implementation Notes

- Display identity builds on T-003: `github.NormalizeEvents` keeps the returned
  repository spelling on every event and fails the whole page on a
  case-insensitive mismatch (FR-002). Selecting one display identity per Scope
  entry is this task's work: use the first normalized event's repository
  spelling, and the requested spelling when the page is empty.
- 2026-08-22T10:36:38Z: verification pass
