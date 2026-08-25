---
id: T-028-report-the-release-version-from-the-binary
title: Report the release version from the binary
status: completed
priority: medium
spec_ref: specs/v0.1.0.md#cli-and-authentication-boundary
dependencies:
    - T-002-implement-cli-configuration-and-github-credential
updated_at: "2026-08-25T19:26:09Z"
---

# T-028-report-the-release-version-from-the-binary Report the release version from the binary

## Description

v0.1.0 ships a standalone binary with no way to report which build it is.
`internal/cli.ParseArgs` defines only `--repo`, no Go source declares a version
variable, and `.goreleaser.yml` sets `ldflags: [-s -w]`, which replaces
GoReleaser's default `-X main.version={{.Version}}` rather than adding to it. A
published archive therefore carries no version stamp at all, and a bug report
against a downloaded binary cannot name the build it came from.

`--help` and `-h` already work without being declared: the `flag` package
reports `ErrHelp` for an undefined `-h`. `-v` has no such treatment and must be
declared next to `--version` in the same flag set.

FR-001 and A-012 of `specs/v0.1.0.md` were amended for this task: version output
goes to stdout as a single `orgtop <version>` line, exits zero, and happens
before credential resolution, any `gh` subprocess, and the TUI. An unstamped
build reports `dev`.

## Acceptance

- `orgtop --version` and `orgtop -v` each write exactly one `orgtop <version>`
  line to stdout and exit zero.
- Neither form resolves a credential, starts `gh`, makes a network request, nor
  enters the TUI, and neither requires a `--repo` selection.
- Version output goes to stdout while usage and startup failures keep going to
  stderr, so the two are separable by a caller.
- A build with no linker stamp reports `dev`.
- Usage lists `--version` beside `--repo`, and `--help`/`-h` keep exiting zero
  with usage.
- `.goreleaser.yml` stamps the version onto the existing `ldflags`, and a
  toolchain guard fails if that assignment is dropped or renamed away from the
  variable the binary actually reads.
- The README documents `--version`/`-v` and `--help`/`-h`.
- `CHANGELOG.md` gains an `## [Unreleased]` `### Added` entry, so the flag
  reaches the published `v0.1.0` notes the release workflow extracts.
- `task check` stays green.

## Test Expectations

- Extend `internal/cli` tests: both flag spellings parse, usage names
  `--version`, and no `--repo` is required.
- Extend `cmd/orgtop/main_test.go` beside
  `TestHelpRequestExitsSuccessfullyWithoutLaunching`: a version request neither
  resolves nor launches, exits zero, and writes to the stdout seam only. The
  harness currently has one `output` writer; give the shell a separate version
  writer so the stdout/stderr split is asserted rather than assumed.
- Extend `internal/toolchain` (`yaml_test.go` already reads
  `.goreleaser.yml`): assert the `-X` target names the same symbol the CLI
  declares, so a rename cannot silently un-stamp releases.
- Extend `internal/toolchain/docs_test.go` for the README claim.
- No live credential, no network, no host clock.

## Verification Notes

- Record `task check` exit status.
- Record `go build -o /tmp/orgtop ./cmd/orgtop && /tmp/orgtop --version`,
  showing the unstamped build reports `dev`.
- Record a stamped build:
  `go build -ldflags "-X <pkg>.Version=0.1.0" -o /tmp/orgtop ./cmd/orgtop`
  followed by `/tmp/orgtop --version`.
- Record `/tmp/orgtop --version 2>/dev/null` still printing the line, and
  `/tmp/orgtop 2>/dev/null` printing nothing, to evidence the stream split.
- Record `task release:dry` and confirm the snapshot binary reports its
  snapshot version rather than `dev`.

## Implementation Notes

- Declare `var Version = "dev"` in `internal/cli` next to the flag that reads
  it, so parsing, usage, and the stamped symbol stay in one place. GoReleaser
  can `-X` an internal package.
- Mirror `flag.ErrHelp`: `ParseArgs` returns a `cli.ErrVersionRequested`
  sentinel and `cmd/orgtop` decides the exit code and the writer, matching how
  the help path is already split between the two.
- Keep `ldflags` as `-s -w` plus
  `-X github.com/fmueller/orgtop/internal/cli.Version={{.Version}}`.
  `{{.Version}}` is GoReleaser's tag without the leading `v`.
- T-024 re-verifies release readiness and is still open; this task is a
  dependency of it, so the re-verification runs against the stamped binary and
  the gate cannot pass a release that reports `dev`.
- 2026-08-25T19:26:05Z: verification pass
