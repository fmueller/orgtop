---
id: T-117-retry-tap-staging-branch
title: Delete the tap staging branch a retry recreates
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#distribution-channels
dependencies:
    - T-074-provision-and-validate-distribution-repositories
updated_at: "2026-09-20T10:18:36Z"
---

# T-117-retry-tap-staging-branch Delete the tap staging branch a retry recreates

## Description

A retry of an already completed publication restages the tap formula, which
recreates `release/orgtop-v<version>` in `fmueller/homebrew-tap`. The merge step
that follows finds its pull request already merged, so `gh pr merge
--delete-branch` reports `! Pull request ... was already merged` and deletes
nothing. The step exits 0 and the retry succeeds, but the staging branch is left
behind in the tap.

No published byte is affected and the formula on the default branch is correct,
so this is residue rather than a partial release. It matters because the tap is
a public repository a production retry would leave a dangling release branch in,
and because RG-011 expects the staging branch to exist only between staging and
the transition.

Observed in the v0.0.5 rehearsal, run 35501755880 attempt 2, where
`release/orgtop-v0.0.5` survived the successful retry and was only removed by
the later withdrawal.

## Acceptance

- A retry of a completed publication leaves no `release/orgtop-v<version>`
  branch in `fmueller/homebrew-tap`; the tap carries only its default branch
  once the run succeeds.
- The deletion is idempotent and fails closed on nothing it did not create: a
  first publication still deletes the branch through its own merge, and a branch
  that carries anything other than the exact published formula is not deleted
  silently.
- The retry still reconciles the formula against the default branch rather than
  assuming it, so an out-of-band tap change is still caught.

## Test Expectations

- A fixture covering a retry whose tap pull request is already merged, asserting
  the branch is gone and no formula bytes changed.
- A fixture covering a first publication, asserting the existing merge-and-delete
  path is unchanged.

## Verification Notes

- Record the run id, the tap branch listing before and after, and the formula
  digest on the default branch across both.

## Implementation Notes

- The merge step is `Merge the formula onto the verified tap base` in
  `.github/workflows/release.yml`.
- `gh pr merge --delete-branch` is not a cleanup path for an already merged pull
  request; the branch deletion has to be requested explicitly for that case.
- 2026-09-20T10:18:32Z: verification pass
