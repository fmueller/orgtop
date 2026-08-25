---
id: T-029-restore-changelog-notes-to-the-published-release
title: Restore CHANGELOG release notes to the published release body
status: completed
priority: high
spec_ref: specs/v0.1.0.md#integration-documentation-and-release-readiness
dependencies: []
updated_at: "2026-08-25T20:08:09Z"
---

# T-029-restore-changelog-notes-to-the-published-release Restore CHANGELOG release notes to the published release body

## Description

The `v0.1.0` release published with an empty body. Every guard the release path
carries passed: `check-changelog-version.sh` found the non-empty `## [0.1.0]`
section, `changelog-release-notes.sh` extracted 48 lines of notes, and
`goreleaser release --release-notes=...` ran against that file and reported
success. The archives, the checksums, and the version stamp are all correct; the
notes are the one thing that never arrived.

The cause is in `.goreleaser.yml`. It carried `changelog: disable: true`, on the
reasoning that CHANGELOG.md, not GoReleaser, is the source of release notes.
GoReleaser reads `--release-notes` *inside* the changelog pipe
(`internal/pipe/changelog/changelog.go`): `Pipe.Skip` returns true on
`Changelog.Disable`, so `Pipe.Run` — the only code that assigns
`ctx.ReleaseNotes` from the file — never executes, and the release publishes
with an empty body. No warning is emitted. Disabling the pipe does not preserve
the file as the sole source of notes; it discards it.

The pipe returns early once `--release-notes` is set, before any generation, so
leaving it enabled does not reintroduce commit-derived notes on the tag path.

`TestReleaseWorkflowGuardsChangelogNotes` asserted `changelog.disable == true`,
so the config that broke the release was pinned by the test written to protect
it. The gap the whole guard chain shares: every check ran against inputs, and
nothing ever read the published release back. T-011, T-017, and T-024 all passed
release readiness without that check, because it did not exist.

## Acceptance

- `.goreleaser.yml` does not disable the changelog pipe, and carries a comment
  stating why disabling it drops `--release-notes`.
- `TestReleaseWorkflowGuardsChangelogNotes` fails when the pipe is disabled, and
  requires the workflow to read the published release back.
- The release workflow verifies, after publishing, that the released body is
  non-empty and matches the extracted CHANGELOG notes, and fails the run
  otherwise.
- The published `v0.1.0` release carries the `## [0.1.0]` notes.
- `task check` passes.

## Verification Notes

- Record `go test ./internal/toolchain/ -run TestReleaseWorkflowGuardsChangelogNotes`
  failing against the old `disable: true` config and passing against the new one.
- Record `task check` exit status.
- Record `gh release view v0.1.0 --json body --jq '.body | length'` as non-zero
  and the body matching `scripts/changelog-release-notes.sh v0.1.0`.

## Implementation Notes

- `changelog: use: git` keeps the pipe enabled without an API call; the early
  return on `--release-notes` means the setting is never reached on the tag path.
- The published `v0.1.0` body is repaired with
  `gh release edit v0.1.0 --notes-file`, not by deleting and re-pushing the tag:
  the artifacts and their checksums are correct and already downloadable, and
  re-cutting the tag would change them for anyone who already fetched them.
- 2026-08-25T20:08:09Z: verification pass
- 2026-08-25T20:08:09Z: v0.1.0 release body repaired in place with gh release edit; artifacts and checksums untouched
