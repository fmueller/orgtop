---
id: T-008-wire-an-executable-orgtop-vertical-slice
title: Wire an executable OrgTop vertical slice
status: todo
priority: high
spec_ref: specs/v0.1.0.md#integration-documentation-and-release-readiness
dependencies:
    - T-002-implement-cli-configuration-and-github-credential
    - T-007-implement-the-asynchronous-refresh-lifecycle
updated_at: "2026-08-21T22:39:23Z"
---

# T-008-wire-an-executable-orgtop-vertical-slice Wire an executable OrgTop vertical slice

## Description

Wire flags, credential resolution, GitHub source, snapshot builder, root context, and
Bubble Tea program into `cmd/orgtop` before final views land. Placeholder view content
is acceptable so constructor, cancellation, and package-direction issues are tested
early.

## Acceptance

- Binary validates Scope, resolves auth, constructs dependencies, launches Bubble
  Tea, and maps startup errors to non-zero stderr plus usage where applicable.
- Invalid/missing Scope performs no auth/network work; missing auth gives the required
  remediation without token values.
- Startup process interrupt cancels credential resolution; TUI shutdown cancels an
  in-flight source request.
- Launch seams permit deterministic tests without terminal, credentials, or GitHub.
- `go build ./cmd/orgtop` and `task check` pass without deferred behavior.

## Test Expectations

- Cover configuration ordering, auth failure, constructor flow, process-context
  cancellation, program exit, source cancellation, and sentinel-token redaction.
- Run focused launch tests, `task build`, and `task check`.

## Verification Notes

- Record exact commands/exit status and launch/cancellation/redaction tests.

## Implementation Notes

- T-007 handoff: `tui.New` takes the root context and derives the context every
  refresh runs under. The quit keystroke cancels it, but nothing else does yet,
  so wire the program's signal handling into the same context here; otherwise
  shutdown cancellation stays keystroke-only.
- T-007 handoff: the shell declares its own `tui.Source` and `tui.Result` seam
  because `internal/tui` must not import `internal/github`. Wire
  `github.Source.Refresh` through a thin adapter that maps `github.Refresh` to
  `tui.Result` and puts `(*github.RefreshError).RetryDelay` into
  `tui.Result.Delay` on the failure path.
