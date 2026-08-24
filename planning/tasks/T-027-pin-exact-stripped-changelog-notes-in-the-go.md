---
id: T-027-pin-exact-stripped-changelog-notes-in-the-go
title: Pin exact stripped changelog notes in the Go release guard
status: completed
priority: low
spec_ref: specs/v0.1.0.md#integration-documentation-and-release-readiness
dependencies:
    - T-019-strip-link-references-from-extracted-changelog
updated_at: "2026-08-24T18:38:29Z"
---

# T-027-pin-exact-stripped-changelog-notes-in-the-go Pin exact stripped changelog notes in the Go release guard

## Description

T-019 taught `scripts/changelog-release-notes.sh` to drop `[label]: url` link
references, and the shell suite pins the exact extracted text. The Go guard in
`internal/toolchain/release_test.go` already strips the same lines through
`isLinkReference`, but
`TestChangelogSectionDistinguishesAbsentFromSaysNothing` only asserted the
booleans `wantDocumented`/`wantEntries`. An over-broad or absent matcher on the
Go side could therefore swallow real entries, or leak the address block into
notes, without failing CI — the two guards could drift apart silently.

Raised by the T-019 code review as a LOW finding: "The shell suite is the only
place that pins 'an ordinary entry merely containing `]: ` is kept' and 'a
mid-section (non-footer) link reference is removed' as exact-content
assertions."

## Acceptance

- `TestChangelogSectionDistinguishesAbsentFromSaysNothing` pins the exact
  extracted notes, not only whether entries exist, for the cases that carry
  content.
- A case covers a link reference in the middle of a section: it is removed and
  the section's real entries survive.
- A case covers an ordinary entry that merely contains `]: `, and an entry
  containing a markdown inline link: both are kept.
- The strengthened test fails under a deliberate regression of
  `isLinkReference`/`changelogSection` and passes when restored.
- `go test ./internal/toolchain -count=1` and `task check` stay green, and the
  shell guard behaviour is unchanged.

## Test Expectations

- Extend the existing table test; add no new harness and no production code.

## Verification Notes

- Record `go test ./internal/toolchain -count=1` and `task check` exit statuses.
- Quote the deliberate-regression failure output and the restored pass.

## Implementation Notes

- Keep the Go and shell guards semantically identical; `isLinkReference`
  mirrors the awk rule `^\[.*\]: .` applied to the trimmed line.
- 2026-08-24T18:38:29Z: verification pass
