---
id: T-019-strip-link-references-from-extracted-changelog
title: Strip link references from extracted changelog release notes
status: completed
priority: low
spec_ref: specs/v0.1.0.md#integration-documentation-and-release-readiness
dependencies:
    - T-017-re-verify-v0-1-0-release-readiness-before-tagging
updated_at: "2026-08-24T18:08:05Z"
---

# T-019-strip-link-references-from-extracted-changelog Strip link references from extracted changelog release notes

## Description

`scripts/changelog-release-notes.sh` captures every line between a version
heading and the next `## ` heading. A Keep a Changelog file collects its
`[label]: url` link references at the foot of the file, below the last version
heading, so the newest release's extracted notes carry those reference lines
verbatim.

For `v0.1.0` that means the published GitHub Release body would end with:

```
[Unreleased]: https://github.com/fmueller/orgtop/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/fmueller/orgtop/releases/tag/v0.1.0
```

Two raw markdown reference definitions that render as nothing and read as
cruft. The effect is cosmetic: the section is genuinely non-empty either way,
so no guard passes or fails wrongly, and no tag is refused or published that
should not be.

T-017 already made the Go-side guard stricter. `changelogSection` in
`internal/toolchain/release_test.go` skips link references, so an entryless
section whose only remaining lines are the link block now fails ordinary CI
instead of reading as written. The shell script the release workflow actually
runs was deliberately left alone, because changing what it extracts changes
published release-note content and was outside T-017's re-verification scope.

## Acceptance

- `scripts/changelog-release-notes.sh` omits markdown link-reference definition
  lines (`[label]: url`) from the notes it prints, for the last version section
  as well as any earlier one.
- Trailing blank lines left behind by the removal are trimmed, so the notes
  neither start nor end with whitespace.
- A section whose only non-heading content is the link block is reported as
  empty, so `scripts/check-changelog-version.sh` refuses that tag rather than
  publishing a body of addresses. This aligns the shell guard with the Go guard
  T-017 added.
- A link reference that appears in the middle of a section is removed the same
  way, and ordinary entries that merely contain `]: ` are kept.
- `scripts/check-changelog-test.sh` covers the cases above and `task
  test:changelog` stays green.
- `TestChangelogSectionDistinguishesAbsentFromSaysNothing` and
  `TestChangelogDocumentsTheReleaseVersion` in
  `internal/toolchain/release_test.go` still pass unchanged, and the two
  implementations agree on every case the shell tests exercise.

## Test Expectations

- Extend `scripts/check-changelog-test.sh` rather than adding a new harness.
- Keep the assertions on the shell scripts; no Go production code is involved.

## Verification Notes

- Record `task test:changelog`, `task check`, and `go test ./internal/toolchain
  -count=1` exit statuses.
- Show the extracted `v0.1.0` notes before and after, proving the link block is
  gone and the `### Added`/`### Fixed` content is untouched.

## Implementation Notes

- Discovered during T-017 re-verification. Evidence:
  `scripts/changelog-release-notes.sh v0.1.0 CHANGELOG.md` prints both link
  reference lines at the end of its output.
- `isLinkReference` in `internal/toolchain/release_test.go` is the shape the Go
  side already recognizes; matching it keeps the two guards consistent.
- Purely a release-notes presentation change. Do not change which tags are
  accepted beyond the entryless-section case named in Acceptance.
- 2026-08-24T18:08:01Z: verification pass
