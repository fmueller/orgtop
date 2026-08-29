---
id: T-032-close-the-path-matcher-contract
title: Close the path matcher contract
status: todo
priority: high
spec_ref: specs/v0.2.0.md#rg-002-path-matcher-contract
dependencies: []
updated_at: "2026-08-29T09:22:04Z"
---

# T-032-close-the-path-matcher-contract Close the path matcher contract

## Description

Revise the normative specification to close RG-002 with an exact, testable path matcher and canonical Scope identity contract.

## Acceptance

- The spec defines grammar, escaping, normalization, separators, root and directory semantics, case behavior, dotfiles, invalid patterns, overlap, and rename treatment.
- Canonical Scope identity is fully specified for CLI, domain, cache, tests, and presentation use.
- The contract does not rely on implicit shell glob behavior.

## Test Expectations

- Add normative examples and edge cases for every matcher dimension, including old/new rename paths and invalid syntax.
- Add identity and overlap acceptance vectors.

## Verification Notes

- Map every RG-002 clause to normative text or an acceptance vector.
- Record `taskrail validate`.

## Implementation Notes

- Do not implement the matcher in this task.
- Keep matching source-independent and avoid importing GitHub, SQLite, or Bubble Tea types into the contract.
