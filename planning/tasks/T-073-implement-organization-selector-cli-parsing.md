---
id: T-073-implement-organization-selector-cli-parsing
title: Implement organization-selector CLI parsing
status: todo
priority: high
spec_ref: specs/v0.2.0.md#organization-selection
dependencies:
    - T-045-implement-unified-scope-cli-parsing
updated_at: "2026-08-29T09:55:42Z"
---

# T-073-implement-organization-selector-cli-parsing Implement organization-selector CLI parsing

## Description

Implement the RG-010 organization-selector syntax as a CLI selection input that remains distinct from the downstream Scope domain model.

## Acceptance

- Parsing, repetition, composition with exact repository/path selections, validation, deduplication handoff, usage text, and diagnostics match the closed RG-010 grammar.
- An organization-only invocation passes initial CLI validation and enters the closed first-expansion loading state; invalid, unknown, unauthorized, and empty outcomes remain distinct downstream.
- The parser produces organization selectors for the adapter boundary and never creates organization domain Scopes or aggregate rows.

## Test Expectations

- Add deterministic CLI tests for organization-only, repeated, mixed, duplicate, invalid, path-bearing, help, and cache-control composition vectors.
- Preserve every repository/path and v0.1 compatibility fixture from T-045.

## Verification Notes

- Record CLI fixture names, usage output, and sanitized command smokes.

## Implementation Notes

- Continue using the standard library flag approach.
- Do not perform network expansion during parsing.
