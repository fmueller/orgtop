---
id: T-024-re-verify-v0-1-0-release-readiness-and-drift-after
title: Re-verify v0.1.0 release readiness and drift after the Stream legibility work
status: todo
priority: medium
spec_ref: specs/v0.1.0.md#integration-documentation-and-release-readiness
dependencies:
    - T-020-render-event-age-instead-of-a-wall-clock-time-in
    - T-021-stop-restating-the-entity-class-in-event
    - T-022-give-stream-sticky-column-headings
    - T-023-disclose-what-the-stream-list-covers-and-mark
    - T-025-inject-a-fake-clock-into-tests
    - T-026-extend-the-host-clock-guard-to-time-until-and-pin
updated_at: "2026-08-24T11:45:00Z"
---

# T-024-re-verify-v0-1-0-release-readiness-and-drift-after Re-verify v0.1.0 release readiness and drift after the Stream legibility work

## Description

T-017 re-verified release readiness against the surface as it stood after T-012
through T-016, and passed. T-020 through T-023 then change Stream's rendered
surface, the event descriptions the domain produces, the shared chrome
arithmetic, and the spec itself. The release gate has to run once more against the
final surface before a `v0.1.0` tag is pushed. This task adds no new capability.

What the preceding tasks move:

- T-020 replaces the wall-clock stamp with an age anchored to the last successful
  refresh, which changes every rendered Stream row and the `40x10` layout.
- T-021 rewords the descriptions `internal/github` produces, which changes
  normalization assertions and rendered detail across packages.
- T-022 reserves a further chrome line for Stream, which changes the content-area
  arithmetic the A-010 contract rests on.
- T-023 adds a coverage disclosure and a truncation mark, which change the
  narrowest layouts again.
- T-025 injects a fake clock into the tests, so the age assertions T-020
  introduced stop depending on the real one and the gate runs against a
  deterministic surface.
- T-026 closes the one host clock read T-025 left, the `time.Until` deadline
  measurement in `internal/github/source_test.go`, and widens the guard to cover
  it. Until it lands, the determinism guard is repository-wide in name only, so
  the gate cannot claim the surface it re-verifies is fully deterministic.

Unlike T-017, this one also has a spec to re-check. FR-005, FR-006, FR-007,
FR-010, and A-010 were amended before T-020 started, so the implemented surface
and the amended spec have to be compared rather than assumed to agree.

## Acceptance

- The deterministic wired flows in `cmd/orgtop/integration_test.go` still prove
  one and multiple repositories, view switching, async refresh, empty success,
  initial error and recovery, stale atomic failure and recovery, both completed
  views at `40x10`, Overview scroll reachability, quit, and shutdown
  cancellation, updated where T-020 through T-023 changed the rendered surface.
- Stream's post-T-020 surface is covered at `40x10` with a snapshot spanning
  minutes to weeks, proving ages, headings, disclosure, and truncation marking
  coexist within the contract.
- Scope and auth failures remain concise non-zero stderr in their post-T-012
  wording, and a sentinel token is absent from stdout, stderr, rendered output,
  errors, and test logs.
- README documents the controls as the footer advertises them and describes the
  Stream columns as they now render, including that times are ages. The
  `internal/toolchain/docs_test.go` guards match the shipped surface.
- No non-benchmark test in the repository reads the host clock, `time.Until`
  included, and `TestNoTestReadsTheHostClock` enforces it, so the gate re-verifies
  a surface whose tests cannot drift with when or how fast they run (NFR-006).
- `taskrail validate` reports valid state and `taskrail coverage` reports no
  uncovered area and no task pointing away from the active spec.
- Every amended spec clause has an implementing task that reached `completed`, and
  no amended clause is left unimplemented or contradicted by the shipped surface.
  Anything found uncovered becomes a new task rather than a silent gap.
- Existing CI runs format, vet, lint, tests, build, Taskrail, and release-config
  checks through commands matching `task check`.
- Build and release configuration still produces `orgtop`; the direct module
  requirement set is unchanged; no v0.2+ behavior or dependency lands.
- `CHANGELOG.md` still has a non-empty `## [0.1.0]` section and describes the
  Stream legibility work, so the release workflow will not refuse the tag and the
  published notes match what shipped.

## Test Expectations

- Prefer deterministic package and model integration; live GitHub smoke is
  optional and never required by CI.
- Run `task check`, `go test ./... -race`, `git diff --check`, and sanitized local
  CLI smokes for missing Scope, malformed Scope, and missing auth.
- Run `taskrail validate` and `taskrail coverage`, and read the coverage map for
  drift rather than only its exit status.

## Verification Notes

- Record exact command exit statuses, the integrated scenarios, sanitized smoke
  results, the CI-command comparison, and an explicit future-scope audit.
- Reference the T-017 verification artifact under
  `planning/artifacts/verify/T-017-re-verify-v0-1-0-release-readiness-before-tagging/`
  and note every assertion that had to change and why.
- Record the spec-to-implementation comparison for each amended clause in FR-005,
  FR-006, FR-007, FR-010, and A-010, naming the task and the test that covers it.

## Implementation Notes

- Taskrail treats `completed` as terminal, so this task exists instead of
  reopening T-017. T-017 stays completed: its work shipped.
- T-017 left two accepted low findings, both still open and both recorded as
  remaining risks rather than new work: `documentedControls` in
  `internal/toolchain/docs_test.go` is a hardcoded list that cannot detect a
  footer change in `internal/tui/chrome.go`, and `releaseVersion` in
  `internal/toolchain/release_test.go` is a manual constant. Re-confirm both are
  still accurate rather than treating them as new.
- T-019 covers the shell-side release-notes link-reference cruft and is
  independent of this gate.
