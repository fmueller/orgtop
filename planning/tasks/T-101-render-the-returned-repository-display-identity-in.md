---
id: T-101-render-the-returned-repository-display-identity-in
title: Render the returned repository display identity in Scope rows
status: completed
priority: low
spec_ref: specs/v0.2.0.md#explicit-scope-membership
dependencies:
    - T-081-retire-v01-repository-snapshot-aggregation
updated_at: "2026-09-08T20:16:43Z"
---

# T-101-render-the-returned-repository-display-identity-in Render the returned repository display identity in Scope rows

## Description

`github.displayIdentity` (`internal/github/source.go:253`) records the FR-002
returned display identity on every `domain.RepositoryActivity`: the first
matching returned spelling, or the requested spelling for an empty page. Until
T-081 that field fed the v0.1 `domain.Snapshot` aggregation, which no view read.
T-081 retired that aggregation, so nothing downstream consumes the field today:
`domain.Retain` and `NewScopedSnapshot` take only `Events`, and a published
Scope row is labelled from `Scope.String()`, which always renders the Scope's
own retained *requested* spelling (`internal/domain/scope.go:64,109`).

The v0.1 FR-002 rule "the first matching returned spelling becomes the display
name for that successful snapshot; an empty response uses the requested
spelling" therefore reaches no view under v0.2. The rule is still implemented
and guarded at the source boundary
(`internal/github/source_test.go:304`), but not at the presentation boundary:
T-081 deleted `TestNewSnapshotUsesReturnedDisplayIdentityAndRequestedFallback`
and `TestNewSnapshotKeepsFirstReturnedDisplayIdentityPerRepository` with the
aggregation they exercised, and did not move them.

Decide whether v0.2 honours the returned spelling in Scope rows or explicitly
supersedes the v0.1 rule with the requested spelling, then make the code and
the spec agree.

Follow-up derived from T-081-retire-v01-repository-snapshot-aggregation's verification or discovery.

## Acceptance

- A repository Scope row either renders the FR-002 returned display identity
  with the requested spelling as the empty-page fallback, or the spec records
  that v0.2 deliberately labels Scope rows by the retained requested spelling.
- Whichever option is taken, `domain.RepositoryActivity.Repository` is either
  consumed for display or removed together with `github.displayIdentity` and its
  tests, so no field carries a display fact nothing reads.
- The first returned spelling wins when a repository is reported more than once,
  and repository identity comparison stays case-insensitive.
- No `ScopedSnapshot` semantics, ordering, or bounds change.

## Test Expectations

- Restore presentation-level coverage of the returned-spelling and
  requested-spelling fallback rules the deleted v0.1 tests provided, on the
  scoped snapshot or the Scope row rendering.
- Keep the source-boundary coverage at `internal/github/source_test.go:304`.

## Verification Notes

- Record the chosen option, the spec change if any, and targeted domain, TUI,
  and github test results.

## Implementation Notes

- Sequence after T-081. `Scope` retains the requested spelling by construction,
  so honouring the returned spelling means giving `Scope` or `ScopeAggregate` a
  place for it rather than re-deriving one at render time.
- 2026-09-08T20:16:39Z: verification pass
