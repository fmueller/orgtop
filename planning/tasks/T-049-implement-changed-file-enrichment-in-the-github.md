---
id: T-049-implement-changed-file-enrichment-in-the-github
title: Implement changed-file enrichment in the GitHub adapter
status: completed
priority: high
spec_ref: specs/v0.2.0.md#changed-file-enrichment
dependencies:
    - T-043-guard-the-v0-2-0-toolchain-and-dependency-baseline
updated_at: "2026-09-01T22:13:19Z"
---

# T-049-implement-changed-file-enrichment-in-the-github Implement changed-file enrichment in the GitHub adapter

## Description

Implement the closed GitHub changed-file operations and normalize their results at the adapter boundary.

## Acceptance

- API version, endpoint eligibility, auth, pagination completeness, renames, timeouts, cancellation, rate limits, and sanitized failures implement RG-003 exactly.
- Partial or truncated file lists are explicit incomplete outcomes, never complete evidence.
- No GitHub payload or pagination type escapes the adapter.

## Test Expectations

- Add fixture tests for complete multi-page results, malformed and partial data, unsupported entities, cancellation, auth failure, rate limiting, and secret-safe errors.

## Verification Notes

- Record fixture request counts and completeness assertions.

## Implementation Notes

- Return typed normalized paths and outcomes.
- Do not perform domain membership matching in the adapter.
- 2026-09-01T22:13:15Z: verification pass
