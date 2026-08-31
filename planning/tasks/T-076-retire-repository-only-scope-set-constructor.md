---
id: T-076-retire-repository-only-scope-set-constructor
title: Retire the repository-only Scope set constructor
status: todo
priority: low
spec_ref: specs/v0.2.0.md#unified-repository-and-path-scopes
dependencies:
    - T-045-implement-unified-scope-cli-parsing
updated_at: "2026-08-31T22:16:03Z"
---

# T-076-retire-repository-only-scope-set-constructor Retire the repository-only Scope set constructor

## Description

`domain.NewRepositoryScopeSet` was the v0.1 selection entry point. Since T-045
the CLI expands every selection through `expandSelections` and
`domain.NewScopeSet`, so no production code calls the repository-only
constructor; only test helpers in `cmd/orgtop`, `internal/github`, `internal/tui`,
and `internal/domain` still use it. Decide whether it stays as a supported domain
constructor or is retired in favour of the unified construction path.

Follow-up derived from T-045-implement-unified-scope-cli-parsing's verification or discovery.

## Acceptance

- The repository-only constructor is either removed together with its test
  helpers and tests, or retained with a documented current responsibility rather
  than as an unused API.
- Repository-only selections keep their FR-001 v0.1 identity, ordering, and
  first-spelling behavior whichever option is taken.
- No production caller is added to preserve the constructor.

## Test Expectations

- Keep the v0.1 repository-only compatibility coverage that
  `TestNewRepositoryScopeSetKeepsV01Behavior` provides, moved onto the unified
  construction path if the constructor is removed.

## Verification Notes

- Record the call-site survey, the chosen option, and targeted domain, CLI, and
  TUI test results.

## Implementation Notes

- Sequence this after T-047 and T-073: organization expansion may want a
  repository-list constructor, which would settle the decision the other way.
- Do not change `ScopeSet` semantics, capacities, or identity while doing this.
