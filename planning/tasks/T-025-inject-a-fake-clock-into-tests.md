---
id: T-025-inject-a-fake-clock-into-tests
title: Inject a fake clock into tests instead of depending on the real one
status: todo
priority: medium
spec_ref: specs/v0.1.0.md#application-shell-and-refresh-lifecycle
dependencies: []
updated_at: "2026-08-23T20:43:52Z"
---

# T-025-inject-a-fake-clock-into-tests Inject a fake clock into tests instead of depending on the real one

## Description

NFR-006 requires deterministic tests, but three test sites still read the real
clock instead of a pinned one.

The gap is a missing seam. `internal/tui` has one, but it is unexported:
`Model.now` (`internal/tui/model.go:41`) defaults to `time.Now` and is replaced
in-package by `lifecycle` (`internal/tui/refresh_test.go:63`). `tui.New` exposes
no equivalent, so a test outside the package cannot pin the instant the shell
stamps `State.LastSuccess` with (`internal/tui/refresh.go:74`). `github.Source`
already shows the shape this should take: an exported `Now func() time.Time`
field whose nil value falls back to `time.Now` (`internal/github/source.go:70`,
`internal/github/source.go:248`).

The three sites, in descending order of how much real time they depend on:

- `cmd/orgtop/integration_test.go:187` (`agedEventsPage`) builds fixture
  `created_at` values as offsets from `time.Now()`, because the wired shell's
  last-success instant cannot be injected. The assertions in
  `TestMultiDaySnapshotRendersAgesRatherThanClockTimes` then compare rendered
  ages (`2h`, `3d`, `2w`) against buckets whose edges the elapsed test runtime
  could in principle cross. The chosen ages sit mid-bucket, with the nearest
  edge 22h away, so this does not fail in practice; it is a design dependency on
  the real clock, not a live flake.
- `internal/auth/credential_test.go:120` and `:125` bracket a call with
  `time.Now()` and assert on the measured elapsed real time. This is the only
  site whose outcome varies with machine speed and scheduler noise.
- `internal/tui/refresh_test.go` passes `time.Now()` as the pinned instant at
  eleven call sites (lines 111, 132, 201, 280, 317, 333, 349, 369, 394, 435,
  451). The value is frozen once it reaches `Model.now`, so these are already
  deterministic within a run; they are inconsistent rather than fragile, and
  they hide which instants a test actually depends on.

Anchoring on `#application-shell-and-refresh-lifecycle` because the missing
seam is the shell's: the fix is an exported clock on `tui.New`, and the
integration and refresh test sites follow from it.

## Acceptance

- `tui.New` accepts an injectable clock, defaulting to `time.Now` when the
  caller supplies none, following the nil-means-`time.Now` shape
  `github.Source.Now` already uses. The unexported `Model.now` field keeps
  working for in-package tests.
- `cmd/orgtop/integration_test.go` pins that clock and builds every fixture
  timestamp as an offset from the pinned instant rather than from `time.Now()`.
  `TestMultiDaySnapshotRendersAgesRatherThanClockTimes` keeps its literal age
  expectations (`2h`, `3d`, `2w`) and no longer depends on how long the test
  takes to run.
- `internal/auth/credential_test.go` no longer asserts on measured elapsed real
  time. It asserts the observable outcome (that the deadline was honored, that
  the call returned rather than blocked) instead of how many milliseconds it
  took.
- `internal/tui/refresh_test.go` passes a fixed instant rather than
  `time.Now()`, so each test states the instant it depends on.
- No non-benchmark test in the repository calls `time.Now()`, `time.Since`, or
  otherwise reads the host clock. A boundary test enforces this, in the shape of
  the existing AST-walking guard in `internal/tui/boundary_test.go`.
- Production defaults are unchanged: a binary built from this change reads the
  real clock exactly as before, and no exported behavior other than the new
  optional clock changes.

## Test Expectations

- A boundary test that parses every `_test.go` file in the repository and fails
  on a call to `time.Now` or `time.Since`, modelled on `assertNoShellImports`
  in `internal/tui/boundary_test.go`.
- A `cmd/orgtop` test proving the pinned clock reaches the rendered ages: with
  the clock frozen, a rendered age is identical across two renders separated by
  real elapsed time.
- A test proving the nil/absent clock still defaults to `time.Now`, so the
  binary keeps its production behavior.

## Verification Notes

- Record `task check`, `go test ./... -race`, and `git diff --check` exit
  statuses.
- Regression proof: reverting the integration fixture to `time.Now()` must fail
  the new boundary test.
- Regression proof: removing the default in the injectable clock must fail the
  nil-clock test.

## Implementation Notes

- Discovered while reviewing T-020, which introduced
  `cmd/orgtop/integration_test.go:187`. T-020 deliberately did not add the seam:
  its rules forbade changing a public API, and `tui.New` is one.
- Keep the clock optional. A required parameter would churn every `tui.New`
  caller and the binary's own wiring for no gain; the `github.Source.Now`
  precedent in this repository is an optional field with a nil fallback.
- `internal/github` already has the seam and needs no change. Its tests set
  `Source.Now` directly.
- This is not a tag blocker for `v0.1.0`: no rendered output changes and no
  test currently fails. It is an NFR-006 determinism debt.
