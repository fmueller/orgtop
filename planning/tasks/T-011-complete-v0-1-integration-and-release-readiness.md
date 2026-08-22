---
id: T-011-complete-v0-1-integration-and-release-readiness
title: Complete v0.1 integration and release readiness
status: todo
priority: medium
spec_ref: specs/v0.1.0.md#integration-documentation-and-release-readiness
dependencies:
    - T-008-wire-an-executable-orgtop-vertical-slice
    - T-009-implement-the-overview-activity-view
    - T-010-implement-the-scrollable-stream-activity-view
updated_at: "2026-08-21T22:39:23Z"
---

# T-011-complete-v0-1-integration-and-release-readiness Complete v0.1 integration and release readiness

## Description

Exercise completed binary/model boundaries, update user setup documentation, and
perform final v0.1 polish. Validate Phase 0 CI/release tooling still mirrors local
checks; change it only for a real mismatch. Do not add deferred features.

## Acceptance

- Deterministic flows prove one/multiple repositories, view switching, async refresh,
  empty success, initial error/recovery, stale atomic failure/recovery, both completed
  views at `40x10`, quit, and shutdown cancellation without live credentials.
- Scope/auth failures remain concise non-zero stderr; a sentinel token is absent from
  stdout, stderr, rendered output, errors, and test logs.
- README documents usage, auth precedence/`gh auth login`, POLLING limitations,
  controls, and development commands without claiming deferred features.
- Existing CI runs format, vet, lint, tests, build, Taskrail, and release-config checks
  through commands matching `task check`.
- Build/release configuration produces `orgtop`; no v0.2+ behavior/dependency lands.

## Test Expectations

- Prefer deterministic package/model integration; live GitHub smoke is optional and
  never required by CI.
- Run `task check`, `git diff --check`, and sanitized local CLI smokes for missing
  Scope and missing auth.

## Verification Notes

- Record exact command exit statuses, integrated scenarios, sanitized smoke results,
  CI-command comparison, and explicit future-scope audit.

## Implementation Notes

- T-010 handoff: `layoutStreamRows` in `internal/tui/stream.go` formats event times
  as `event.OccurredAt.Local()`, matching the header's local `updated HH:MM:SS`
  clock, but no test distinguishes that from a regression that dropped `.Local()`
  and rendered the source's UTC instants. `streamBase` in
  `internal/tui/stream_test.go` is already a `time.Local` instant, so the
  conversion is a no-op there. Cover it with a timezone-parameterized render
  during integration polish.
- T-010 handoff: `TestStreamMeasuresWideRunesByTheirRenderedWidth` in
  `internal/tui/stream_test.go` guards Stream rows against byte-wise measurement
  and truncation of wide runes. `internal/tui/overview_test.go` has no equivalent,
  so `padRight`, `widestWidth`, and `truncate` stay unguarded for wide runes on the
  Overview path. Extend the coverage to Overview rows and to the shared header
  candidates.
