---
id: T-017-re-verify-v0-1-0-release-readiness-before-tagging
title: Re-verify v0.1.0 release readiness before tagging
status: todo
priority: medium
spec_ref: specs/v0.1.0.md#integration-documentation-and-release-readiness
dependencies:
    - T-012-flatten-stacked-repository-scope-error-wrapping
    - T-013-drain-non-2xx-github-response-bodies-before
    - T-014-enforce-the-non-nil-refresh-source-at-model
    - T-015-keep-the-polling-floor-consistent-across-the
    - T-016-share-the-stream-scrolling-mechanism-with-the
updated_at: "2026-08-22T14:12:58Z"
---

# T-017-re-verify-v0-1-0-release-readiness-before-tagging Re-verify v0.1.0 release readiness before tagging

## Description

T-011 completed v0.1 integration, documentation, and release readiness against the
surface as it stood at commit `cbcb4ad`. T-012 through T-016 change parts of that
surface after the fact, so the release gate has to run once more against the final
one before a `v0.1.0` tag is pushed. This task adds no new capability.

What the follow-ups move:

- T-016 gives Overview the shared clamped scrolling and puts the scroll keys in the
  shared footer, which changes `40x10` rendering, the footer candidates, and the
  documented controls.
- T-012 changes the user-facing `--repo` rejection text.
- T-013, T-014, and T-015 change the source, the model constructor, and the polling
  floor without changing observable behavior, so they need confirmation rather than
  re-specification.

## Acceptance

- The deterministic wired flows in `cmd/orgtop/integration_test.go` still prove
  one/multiple repositories, view switching, async refresh, empty success, initial
  error/recovery, stale atomic failure/recovery, both completed views at `40x10`,
  quit, and shutdown cancellation, updated where T-016 changed the rendered surface.
- Overview scrolling reachability from T-016 is covered by at least one wired flow at
  `40x10` with more selected repositories than the body budget.
- Scope/auth failures remain concise non-zero stderr in their post-T-012 wording, and
  a sentinel token is absent from stdout, stderr, rendered output, errors, and test
  logs.
- README documents usage, auth precedence/`gh auth login`, POLLING limitations, the
  controls as the footer advertises them after T-016, and development commands
  without claiming deferred features. The `internal/toolchain/docs_test.go` guards
  match the shipped controls.
- Existing CI runs format, vet, lint, tests, build, Taskrail, and release-config
  checks through commands matching `task check`.
- Build/release configuration still produces `orgtop`; the direct module requirement
  set is unchanged; no v0.2+ behavior or dependency lands.
- `CHANGELOG.md` has a non-empty `## [0.1.0]` section, so the release workflow will
  not refuse the tag.

## Test Expectations

- Prefer deterministic package/model integration; live GitHub smoke is optional and
  never required by CI.
- Run `task check`, `go test ./... -race`, `git diff --check`, and sanitized local
  CLI smokes for missing Scope, malformed Scope, and missing auth.

## Verification Notes

- Record exact command exit statuses, the integrated scenarios, sanitized smoke
  results, the CI-command comparison, and an explicit future-scope audit.
- Reference the T-011 verification artifact under
  `planning/artifacts/verify/T-011-complete-v0-1-integration-and-release-readiness/`
  and note every assertion that had to change and why.

## Implementation Notes

- Taskrail treats `completed` as terminal, so this task exists instead of reopening
  T-011. T-011 stays completed: its work shipped in `cbcb4ad`.
- T-011 handoff: `TestFooterAdvertisesOnlyImplementedControls` in
  `internal/tui/model_test.go` and the controls table in `README.md`, guarded by
  `TestReadmeDocumentsPollingControlsAndChecks` in `internal/toolchain/docs_test.go`,
  both fail as soon as T-016 adds scroll keys to the footer. Update them with T-016
  rather than leaving them for this task if that is the natural place.
- T-011 handoff: `TestStartupFailsConciselyWhenNoLocalCredentialExists` in
  `cmd/orgtop/integration_test.go` asserts the exact single-line remediation, so
  T-012's rewording lands there too.
- T-011 left one accepted low finding: that test stubs the `gh` process, so its
  "no exec internals leaked" assertions cannot fail. The guarantee is covered by
  `internal/auth/credential_test.go`. Drop the redundant assertions or leave them
  documented; do not treat it as new work.
