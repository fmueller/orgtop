---
id: T-115-retry-reconciles-published-release
title: Reconcile a retry against an already published source release
status: todo
priority: high
spec_ref: specs/v0.2.0.md#distribution-channels
dependencies:
    - T-074-provision-and-validate-distribution-repositories
updated_at: "2026-09-20T07:39:09Z"
---

# T-115-retry-reconciles-published-release Reconcile a retry against an already published source release

## Description

Close the retry defect that the v0.0.2 rehearsal exposed: a retry of a tag
whose source release is already published creates a second release for that
tag instead of reconciling the published one.

`.goreleaser.yml` sets `draft: true` with `use_existing_draft: true`. That
lookup resolves an existing *draft* for the tag, which is what fixed the
earlier duplicate-draft defect (1d8e658). A *published* release is not a
draft, so the lookup finds nothing and GoReleaser creates a fresh draft
alongside the published release. `scripts/distribution-release-guard.sh`
then fails closed with "carries 2 releases for v0.0.2", which is correct
behavior but leaves the retry unable to make progress.

This is the state a retry lands in whenever the source release transition
succeeded and a later channel failed, which is precisely the partial release
RG-011 expects a retry to reconcile.

Observed in run 35496598314 (rerun), tag v0.0.2, published release 392347493
with 14 assets alongside spurious empty draft 392350389, which was deleted
during cleanup.

Follow-up derived from T-074-provision-and-validate-distribution-repositories's verification or discovery.

## Acceptance

- A retry of a tag whose source release is already published reconciles that
  release instead of creating a second release for the tag.
- Reconciliation stays compare-exactly: an asset already present is never
  overwritten, and a digest that diverges from the staged ledger event fails
  closed.
- A first publication and a retry against an existing draft both keep their
  current behavior, including `use_existing_draft` draft reuse.
- The duplicate-release guard keeps refusing genuine two-release state and is
  not weakened to accommodate the retry path.

## Test Expectations

- A fixture test that fails without the fix, covering a retry against a
  published release for the tag.
- Existing draft-reuse and duplicate-release guard tests keep passing.

## Verification Notes

- Record the run identifiers, the release ids resolved for the tag, and the
  digest comparison against the staged ledger event.

## Implementation Notes

- GoReleaser resolves the release in one `goreleaser release` invocation that
  also builds; separating build from release resolution is the likely shape of
  the fix, and it must not change first-publication behavior.
