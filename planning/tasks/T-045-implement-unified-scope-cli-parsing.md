---
id: T-045-implement-unified-scope-cli-parsing
title: Implement unified Scope CLI parsing
status: todo
priority: high
spec_ref: specs/v0.2.0.md#unified-repository-and-path-scopes
dependencies:
    - T-044-implement-the-unified-scope-domain-model
updated_at: "2026-08-29T09:22:04Z"
---

# T-045-implement-unified-scope-cli-parsing Implement unified Scope CLI parsing

## Description

Implement the closed CLI syntax for repository scopes, path scopes, mixed Scope sets, and cache controls while retaining exact repeated `--repo` compatibility.

## Acceptance

- Parsing, validation, usage, composition, repetition, deduplication, and diagnostics match the closed RG-001 contract.
- Cache disable/reset flags parse and compose exactly as closed while organization syntax remains owned by T-073.
- Exact/path selections at and beyond RG-009 Scope and matcher capacities receive deterministic CLI diagnostics before startup.
- Existing repository-only invocations do not require enrichment and retain v0.1 semantics.
- Parsing produces validated domain Scopes rather than transport or presentation models.

## Test Expectations

- Add deterministic CLI tests for all normative valid, invalid, capacity-boundary, cache-control, and compatibility invocation vectors.

## Verification Notes

- Record CLI test cases and sanitized command smokes.

## Implementation Notes

- Continue using the standard library flag approach while there is one command.
- Keep credential precedence and secret handling unchanged.
