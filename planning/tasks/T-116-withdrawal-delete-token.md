---
id: T-116-withdrawal-delete-token
title: Give the withdrawal deletion step a token that can delete
status: completed
priority: high
spec_ref: specs/v0.2.0.md#distribution-channels
dependencies:
    - T-067-configure-multi-channel-release-workflows
updated_at: "2026-09-20T08:19:38Z"
---

# T-116-withdrawal-delete-token Give the withdrawal deletion step a token that can delete

## Description

The withdrawal path cannot delete the source release. `release.yml:764` sets
`SOURCE_TOKEN: ${{ secrets.GITHUB_TOKEN }}` and `release.yml:768` selects that
token for the source repository, but the workflow's top-level permissions are
`contents: read` and the withdrawal job deliberately does not elevate them, so
the delete is attempted with a read-only token:

    gh: Resource not accessible by integration (HTTP 403)
    .../rest/releases/releases#delete-a-release

Because the step loops over both companions under `set -e`, the source failure
also prevents the extension release from ever being reached, so a withdrawal
leaves the published source release, its git tag, and the extension draft in
place while the ledger already records the version as withdrawn.

The distribution App holds `contents: write` on all three repositories, and
the extension half of the same loop already uses it successfully. Using the
App token for both repositories keeps the default workflow token read-only,
which is the documented intent of the permissions design.

Observed in run 35497615625, withdrawing v0.0.2. The notice, the ledger event,
and the tap staging branch deletion all succeeded; the releases and the tag
were removed by hand afterwards.

## Acceptance

- A withdrawal deletes the source release, the extension release, and the git
  tag, for both a published version and a draft-only version.
- The default workflow token stays read-only; the withdrawal job does not gain
  `contents: write`.
- One repository failing does not silently skip the other: the step reports
  every repository it could not clean.
- The existing guards that refuse a withdrawal leaving a release or tag behind
  keep failing closed.

## Test Expectations

- A fixture test that fails without the fix, covering the deletion loop
  against a source repository and an extension repository.
- The deletion loop moves out of inline workflow YAML into a script the suite
  can exercise, as the companion guard did, since untested inline YAML is what
  hid both this defect and the empty-companion defect.

## Verification Notes

- Record the withdrawal run identifier and the post-withdrawal inventory of
  releases, tags, and branches in all three repositories.

## Implementation Notes

- The App token is already available to the job as `steps.app.outputs.token`.
- 2026-09-20T08:19:35Z: verification pass
