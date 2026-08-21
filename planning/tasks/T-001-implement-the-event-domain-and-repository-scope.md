---
id: T-001-implement-the-event-domain-and-repository-scope
title: Implement the event domain and repository Scope
status: todo
priority: high
spec_ref: specs/v0.1.0.md#event-domain-and-repository-scope
dependencies: []
updated_at: "2026-08-21T22:39:23Z"
---

# T-001-implement-the-event-domain-and-repository-scope Implement the event domain and repository Scope

## Description

Establish the source-independent domain seam used by every later task. Implement
repository parsing and case-insensitive Scope membership, normalized Event/category/
entity types, filtering, deterministic deduplication, and ordering. Keep GitHub
payload and Bubble Tea types out of this package.

## Acceptance

- Repository parsing implements FR-002's exact ASCII grammar, repeated-value
  deduplication, lowercase matching key, and first-spelling fallback.
- Scope filters unrelated normalized events before aggregation and exposes selected
  repositories without importing source or UI packages.
- Event contains the required ID, timestamp, repository, actor, category, entity
  kind/reference, and description data.
- Deduplication uses source event ID; ordering is timestamp descending then ID
  ascending and is deterministic.
- No path, signal, persistence, plugin, or future-source fields are added.

## Test Expectations

- Add table-driven tests for valid/invalid identifiers, case-insensitive duplicates
  and membership, filtering, duplicate IDs, and timestamp ties.
- Run focused domain tests and `task check`.

## Verification Notes

- Record exact commands, exit status, and relevant named tests during implementation.

## Implementation Notes
