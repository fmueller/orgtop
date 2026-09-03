---
id: T-030-decouple-readme-structure-from-doc-tests
title: Decouple README structure from the toolchain documentation tests
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#fr-012-documentation
dependencies:
    - T-042-consolidate-v0-2-0-implementation-readiness
updated_at: "2026-09-03T21:34:57Z"
---

# T-030-decouple-readme-structure-from-doc-tests Decouple README structure from the toolchain documentation tests

## Description

`internal/toolchain/docs_test.go` guards FR-011 of `specs/v0.1.0.md` by asserting
literal substrings and a literal heading against `README.md`. The intent is
sound: documentation that stops describing the shipped surface, or starts
promising an unshipped one, should fail the build. The implementation binds that
intent to one file's prose and layout, so an edit that makes the documentation
more accurate can still fail, and a v0.2.0 edit that documents shipped v0.2.0
behavior will fail by construction.

Three couplings carry the problem:

- **File.** Every check reads `README.md` only. Moving contributor material into
  `CONTRIBUTING.md` dropped `task check` out of the searched file and failed
  `TestReadmeDocumentsPollingControlsAndChecks`, although the command was better
  documented after the move than before it. The command was restored to the
  README to keep the gate green; that is the workaround this task replaces.
- **Layout.** `streamColumnsSection` locates the column checks by the exact
  string `### stream columns` and ends them at the next `\n### `. Renaming the
  section, promoting it to `##`, or nesting a `####` under it silently changes
  or empties the searched window; the empty case fails, and the widened case
  passes checks it should not.
- **Substring.** `deferredClaims` is a lowercased substring scan over the whole
  file, so it matches inside unrelated words. `rain` matches `constraint`,
  `restraint`, and `training`; `search` matches `research`. Ordinary prose about
  a rate-limit constraint fails the honesty gate for a capability the sentence
  does not mention.

The v0.2.0 collision is the forcing reason. `deferredClaims` is a v0.1.0 deny
list, and v0.2.0 ships several of the entries on it: FR-008 makes Rain a primary
view, FR-009 adds `Interesting Now`, and FR-002 adds path scopes. FR-012 of
`specs/v0.2.0.md` requires the README to document Rain semantics, path matching,
unknown-membership behavior, and cache bounds. Every one of those sentences trips
a test whose deny list still encodes the previous release. The list has to become
a function of the active spec rather than a constant, or FR-012 cannot be
satisfied with the gate green.

The guarantees themselves stay. This task changes what the tests bind to, not
what they promise.

## Acceptance

- Documentation checks resolve over a declared documentation set rather than a
  single hardcoded path. User-facing claims (invocation, credential precedence,
  polling semantics, controls, Stream columns, version and help flags) stay
  scoped to `README.md`; contributor-facing claims (the local gate command)
  are satisfied by any file in the set, so `CONTRIBUTING.md` can hold them.
- The Stream column checks locate their section by a stable identifier that does
  not depend on the heading's exact wording or depth, and the section window
  ends at the next heading of the same or shallower depth rather than at the
  next `###`.
- A section-scoped check fails when its section is missing, rather than
  silently searching an empty or over-wide window. `streamColumnsSection`
  already fatals on a missing heading; the equivalent must hold for whatever
  replaces it.
- Deferred-capability checks match on word boundaries, not raw substrings.
  `constraint`, `restraint`, `training`, and `research` in ordinary prose do not
  fail the gate, while `Rain`, `filtering`, `search`, and `clustering` as claims
  still do.
- The deferred-capability list is derived from, or explicitly keyed to, the
  active spec version rather than being a v0.1.0 constant, so documenting a
  capability that the active spec ships is not a failure and documenting one it
  defers still is.
- The tests keep failing for the reasons they exist: a README that stops naming
  a control, a Stream column, a credential step, `--version`/`-v`,
  `--help`/`-h`, the polling semantics, or the local gate command still fails.
- `task check` stays green, and `README.md` and `CONTRIBUTING.md` are each free
  to hold the material that belongs to their audience.

## Test Expectations

- Extend `internal/toolchain/docs_test.go` with the negative cases the current
  suite cannot express: feed the checks fixture documents rather than only the
  repository's own files, and assert that a document omitting each guarded claim
  fails while a restructured document carrying the same claims passes.
- Cover the substring regressions directly: a fixture containing `constraint`
  and `research` and no deferred claim passes; a fixture claiming `Rain` under a
  spec that defers it fails.
- Cover the section-window regressions: a renamed column heading, a `##` column
  heading, and a `####` subsection under it each behave as specified rather than
  passing vacuously.
- No live credential, no network, no host clock, consistent with the existing
  `internal/toolchain` suite.

## Verification Notes

- Record `task check` exit status.
- Record `go test ./internal/toolchain/...` with the new fixture cases named.
- Record that reverting the README's `task check` mention leaves the gate green
  once `CONTRIBUTING.md` documents it, evidencing the file coupling is gone.
- Record a scratch README carrying a Rain section passing under a v0.2.0 active
  spec and failing under v0.1.0, evidencing the deny list follows the spec.

## Implementation Notes

- `specs/v0.2.0.md` is a Draft and states it is not implementation-ready; its
  readiness gates must be closed by normative spec revision before this task is
  started. It is filed against FR-012 because that is the requirement the
  current tests will block, not because the spec is ready.
- Scope is `internal/toolchain/docs_test.go` and the documentation files it
  reads. This is a test-quality change under NFR-006 and needs no production
  code.
- Prefer deriving guarded strings from the binary where the binary already
  declares them, as `TestReadmeDocumentsTheDocumentedUsage` derives the
  invocation from `cli.ParseArgs`. The control keys and Stream column headings
  live in `internal/tui` and are currently repeated by hand with a comment
  explaining why; revisit whether they can be exported for the test instead of
  duplicated, and keep the comment if they cannot.
- Keep the checks in `internal/toolchain`. That package holds no production code
  and exists to guard the repository's own configuration, which is where a
  documentation gate belongs.
- The task exists because a v0.1.0-era gate outlived its release. Whatever
  replaces the deny list should be reviewed when the active spec changes, and
  the replacement should make that review obvious rather than implicit.
- 2026-09-03T21:34:53Z: verification pass
