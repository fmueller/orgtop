---
id: T-050-implement-the-versioned-sqlite-enrichment-store
title: Implement the versioned SQLite enrichment store
status: todo
priority: high
spec_ref: specs/v0.2.0.md#bounded-sqlite-cache
dependencies:
    - T-043-guard-the-v0-2-0-toolchain-and-dependency-baseline
updated_at: "2026-08-29T09:22:04Z"
---

# T-050-implement-the-versioned-sqlite-enrichment-store Implement the versioned SQLite enrichment store

## Description

Implement the closed private SQLite schema, platform location, permissions, migrations, keys, and transactional changed-file records.

## Acceptance

- Storage implements the closed RG-005 schema and platform contract exactly.
- Complete and incomplete records cannot be confused, and interrupted writes cannot expose partial evidence as complete.
- Credentials, headers, raw payloads, rendered screens, and speculative history are never persisted.

## Test Expectations

- Add temporary-database tests for initialization, permissions where testable, schema versions, migrations, hits, misses, atomic writes, and corruption.

## Verification Notes

- Record schema/migration fixtures and secret-content assertions.

## Implementation Notes

- Keep this a narrow enrichment cache, not a generic repository persistence layer.
- SQLite types must not enter the domain.
