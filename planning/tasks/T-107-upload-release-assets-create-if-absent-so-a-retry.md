---
id: T-107-upload-release-assets-create-if-absent-so-a-retry
title: Upload release assets create-if-absent so a retry reconciles
status: todo
priority: high
spec_ref: specs/v0.2.0.md#distribution-channels
dependencies:
    - T-067-configure-multi-channel-release-workflows
updated_at: "2026-09-17T22:50:21Z"
---

# T-107-upload-release-assets-create-if-absent-so-a-retry Upload release assets create-if-absent so a retry reconciles

## Description

RG-011 requires every operation to be create-if-absent or compare-exactly:
expected assets are reused, missing pieces are created, and assets are never
overwritten. GoReleaser's publish step cannot express that. It offers
`mode: keep-existing`, which keeps the existing release but still attempts to
upload every asset, and `replace_existing_artifacts`, which deletes and
re-uploads them. The first fails a retry, the second overwrites published
bytes.

Observed on the v0.0.2 retry rehearsal, run 35282570875 attempt 2. With
`use_existing_draft` in place the retry correctly reused draft 391100794
rather than creating a second release for the tag, then failed uploading each
of the fourteen assets with `422 Validation Failed [{Resource:ReleaseAsset
Field:name Code:already_exists}]` and aborted the release.

A first publication is unaffected: nothing exists to collide with. Only the
retry RG-011 promises is unreachable, and it fails loudly before any
publication transition rather than silently, so the current state is safe but
incomplete.

## Acceptance

- A retry of a tag whose draft already holds some or all of its assets
  completes: assets already present with the expected digest are reused
  untouched, absent assets are uploaded, and no asset is ever deleted or
  overwritten.
- An asset present under an expected name with an unexpected digest fails
  closed, and names the asset and both digests.
- A first publication still produces the complete twelve-artifact matrix,
  `checksums.txt`, and the provenance bundle, with the source and extension
  drafts unchanged in content from what they carry today.
- The upload path is exercised by fixtures rather than only by a live tag, so a
  regression is caught by `task check`.

## Test Expectations

- Extend `scripts/distribution-test.sh` with fixtures covering an empty draft,
  a partially populated draft, a fully populated draft, and a draft holding a
  digest-divergent asset under an expected name.
- Keep the existing reconciliation tests passing unchanged; this task changes
  how assets arrive, not what the drafts must end up holding.

## Verification Notes

- Record the fixture names and, if a live tag is used, the run ids for the
  first attempt and the retry, plus the asset sets each attempt found and
  uploaded.

## Implementation Notes

- `release.skip_upload` on the GoReleaser config leaves the build and the draft
  to GoReleaser while the assets are uploaded by a guard that can compare
  before it writes; that is the likely shape, and it keeps one build producing
  every published byte.
- Do not reach for `replace_existing_artifacts`. Overwriting is the one thing
  RG-011 names as forbidden here, and reproducible bytes do not make it
  permitted.
- This blocks the last open item of T-074: a retry that resumes and reconciles
  rather than one that fails closed.
- 2026-09-19T09:36:04Z: Dependency corrected from T-074 to completed T-067; this fix must land before T-074's final rehearsal rather than wait on it.
