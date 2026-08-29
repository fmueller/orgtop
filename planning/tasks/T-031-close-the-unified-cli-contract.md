---
id: T-031-close-the-unified-cli-contract
title: Close the unified CLI contract
status: todo
priority: high
spec_ref: specs/v0.2.0.md#rg-001-unified-cli-contract
dependencies: []
updated_at: "2026-08-29T09:22:04Z"
---

# T-031-close-the-unified-cli-contract Close the unified CLI contract

## Description

Revise the normative v0.2.0 specification to close RG-001 without implementing the CLI. Define the repository, path, mixed-Scope, and cache-control invocation contract while preserving exact v0.1 `--repo` behavior and reserving organization-selector syntax for RG-010.

## Acceptance

- The spec normatively defines repository/path syntax, repetition, composition, deduplication, usage text, include/exclude availability, cache disable/reset flags, and invalid-input behavior.
- Existing exact repeated `--repo owner/repository` invocations retain their shipped meaning.
- Organization-selector syntax is explicitly delegated to RG-010 without reopening the closed repository/path grammar.
- Corresponding acceptance coverage is added and no unresolved RG-001 CLI choice remains.

## Test Expectations

- Add specification scenarios for valid repository-only, path-only, mixed, repeated, deduplicated, cache-control, and invalid invocations.
- Include explicit v0.1 compatibility cases.

## Verification Notes

- Record the revised normative headings and acceptance scenarios that close each RG-001 item.
- Record `taskrail validate` after the specification revision.

## Implementation Notes

- This is readiness work only; do not change production code.
- Keep parsing at the CLI boundary and share the resulting canonical Scope semantics with the domain contract.
