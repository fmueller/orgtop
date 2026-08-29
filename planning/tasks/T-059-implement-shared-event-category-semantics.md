---
id: T-059-implement-shared-event-category-semantics
title: Implement shared event category semantics
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#shared-glyph-and-recency-semantics
dependencies:
    - T-043-guard-the-v0-2-0-toolchain-and-dependency-baseline
updated_at: "2026-08-29T09:22:04Z"
---

# T-059-implement-shared-event-category-semantics Implement shared event category semantics

## Description

Centralize the closed category vocabulary, glyph table, ASCII fallback, and semantic styles for Overview, Stream, Rain, and shared chrome.

## Acceptance

- `push`, `pull_request`, `review`, `comment`, `other`, and unsupported rendering implement RG-008 while preserving shipped meanings.
- Category always has a stable non-color encoding.
- No category or style invents failure, success, deployment, priority, or anomaly facts.

## Test Expectations

- Add cross-view tests for every category, fallback, no-color/reduced-color mode, and unsupported input.

## Verification Notes

- Record the shared mapping and proof that each view uses it.

## Implementation Notes

- Own semantic mappings outside rendering functions; views may apply prepared tokens/styles only.
