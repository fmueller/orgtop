---
id: T-046-implement-canonical-path-matching
title: Implement canonical path matching
status: completed
priority: high
spec_ref: specs/v0.2.0.md#unified-repository-and-path-scopes
dependencies:
    - T-044-implement-the-unified-scope-domain-model
updated_at: "2026-09-01T21:23:44Z"
---

# T-046-implement-canonical-path-matching Implement canonical path matching

## Description

Implement the exact closed path grammar, normalization, canonicalization, and matching semantics as a domain responsibility.

## Acceptance

- Matching behavior implements every closed RG-002 rule, including separators, roots, directories, case, dotfiles, overlap, invalid patterns, and rename paths.
- Matcher canonicalization agrees with Scope identity and cache-key contracts.
- No shell-dependent or renderer-specific matching behavior exists.

## Test Expectations

- Translate all normative matcher vectors into deterministic table tests, including malformed and overlapping patterns.

## Verification Notes

- Record exact closed vectors and targeted test results.

## Implementation Notes

- Keep matching in the domain and independent of GitHub and SQLite representations.
- 2026-09-01T21:23:41Z: verification pass
