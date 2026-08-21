---
id: T-003-normalize-github-repository-event-payloads
title: Normalize GitHub repository event payloads
status: todo
priority: high
spec_ref: specs/v0.1.0.md#github-activity-source
dependencies:
    - T-001-implement-the-event-domain-and-repository-scope
updated_at: "2026-08-21T22:39:23Z"
---

# T-003-normalize-github-repository-event-payloads Normalize GitHub repository event payloads

## Description

Define GitHub adapter-owned transport payload types and normalize repository Events
API responses into domain Events. This task owns synthetic fixtures and normalization
only, not HTTP retrieval or polling.

## Acceptance

- PushEvent, PullRequestEvent including merge descriptions, PullRequestReviewEvent,
  PullRequestReviewCommentEvent, and IssueCommentEvent normalize as specified.
- Pull-request comments retain pull-request entity kind; issue-only comments and
  unsupported valid types normalize as `other`.
- Missing ID, timestamp, or repository identity and requested/returned identity
  mismatch return sanitized errors that fail a repository refresh.
- Missing optional actor/category detail yields a safe generic description.
- GitHub payload structs do not escape the adapter or become domain/TUI models.

## Test Expectations

- Add synthetic fixtures for every category, merge, unsupported type, issue-only
  comment, optional fields, malformed common fields, case variation, and mismatch.
- Assert fixtures/errors contain no credentials or private real-world payloads.
- Run focused adapter tests and `task check`.

## Verification Notes

- Record exact commands, exit status, and normalization fixture cases.

## Implementation Notes
