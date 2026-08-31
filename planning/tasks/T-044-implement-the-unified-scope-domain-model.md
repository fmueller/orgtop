---
id: T-044-implement-the-unified-scope-domain-model
title: Implement the unified Scope domain model
status: completed
priority: high
spec_ref: specs/v0.2.0.md#unified-repository-and-path-scopes
dependencies:
    - T-043-guard-the-v0-2-0-toolchain-and-dependency-baseline
updated_at: "2026-08-31T21:50:07Z"
---

# T-044-implement-the-unified-scope-domain-model Implement the unified Scope domain model

## Description

Replace repository-only selection concepts with the closed unified repository/path Scope model and stable identity in the domain.

## Acceptance

- Scope construction, canonical identity, repository composition, equality, deduplication, and deterministic ordering implement the closed RG-001/RG-002 contracts.
- Scope-set construction enforces closed RG-009 exact/path Scope and matcher capacities with deterministic errors before network work begins.
- Repository scopes preserve v0.1 identity and behavior.
- Domain types contain no GitHub payload, SQLite, or Bubble Tea types.

## Test Expectations

- Add table-driven domain tests for canonical identities, mixed sets, duplicates, ordering, capacity boundaries, invalid construction, and v0.1 repository compatibility.

## Verification Notes

- Record targeted domain tests and the closed contract cases covered.

## Implementation Notes

- Keep Scope source-independent and make it the shared downstream selection unit.
- Do not scaffold future source abstractions.
- 2026-08-31T21:29:57Z: verification pass
- 2026-08-31T21:50:07Z: verification pass
