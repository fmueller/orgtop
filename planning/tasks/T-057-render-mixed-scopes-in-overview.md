---
id: T-057-render-mixed-scopes-in-overview
title: Render mixed Scopes in Overview
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#scope-aware-overview-and-stream
dependencies:
    - T-056-integrate-enrichment-into-atomic-refresh
updated_at: "2026-08-29T09:22:04Z"
---

# T-057-render-mixed-scopes-in-overview Render mixed Scopes in Overview

## Description

Extend Overview to present repository and path Scopes as one prepared activity model.

## Acceptance

- Labels, ordering, direct counts, empty states, unknown/incomplete coverage, truncation disclosure, and state transitions implement the closed contracts.
- Path scopes are distinguishable but never represented as synthetic GitHub repositories.
- Rows beyond the body budget are reachable or explicitly accounted for.

## Test Expectations

- Add render/model tests for mixed and overlapping Scopes, empty and unknown states, loading/error/stale/recovery, overflow, wide labels, and `40x10`.

## Verification Notes

- Record golden/plain render assertions at representative dimensions.

## Implementation Notes

- Consume prepared aggregates; do not match or aggregate in rendering.
- Preserve Overview as the initial canonical view.
