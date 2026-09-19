---
id: T-105-complete-the-protected-ledger-step-when-its-event
title: Complete the protected ledger step when its event already landed
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#distribution-channels
dependencies:
    - T-067-configure-multi-channel-release-workflows
updated_at: "2026-09-19T17:37:41Z"
---

# T-105-complete-the-protected-ledger-step-when-its-event Complete the protected ledger step when its event already landed

## Description

`scripts/distribution-protected-commit.sh` tests whether the default branch
already carries the exact ledger event once, at line 64, before it enters the
approval poll. Inside the poll it reads only `pull_request_readiness`. An event
that lands on the default branch by any route other than the script's own
`gh pr merge` therefore never ends the wait: the loop spends its full bound and
dies with "the release stays partial until it merges", while the event it was
waiting for is already committed.

Observed during the T-074 v0.0.1 rehearsal. The maintainer merged the staged
ledger pull request directly instead of approving it, so it merged with zero
reviews. `pull_request_readiness` correctly refuses to call that ready, because
it reads approvals from the reviews; but the step had no way to notice the
merge had happened, and the release would have failed with its durable staged
record already on the default branch.

## Acceptance

- The approval poll re-checks whether the default branch carries the exact
  event, and completes the step when it does, whatever landed it.
- An event landed out of band is accepted only when it is byte-identical to the
  event the step would have appended; a different event for the same version
  and kind still fails closed as contradictory same-version state.
- The independent-approval requirement is unchanged for the merge the script
  performs itself: it still refuses its own login and still requires green
  checks.
- A test fails without the re-check, against a fixture whose pull request is
  merged and unapproved while the event sits on the default branch.

## Test Expectations

- Extend `scripts/distribution-test.sh` with a fixture where the event is on the
  default branch and the pull request carries no approving review, and assert
  the step completes rather than polling to its bound.

## Verification Notes

- Record the fixture name and the poll outcome for both the landed and the
  still-open case.

## Implementation Notes

- Do not weaken `pull_request_readiness`. The gap is the missing re-check, not
  the approval rule.
- The rehearsal could not carry this fix: v0.0.1's staged ledger event pins
  `source_commit` to 7be99b1, so changing the workflow would have made the
  retried tag produce a different staged event and fail closed as contradictory.
- 2026-09-19T09:36:04Z: Dependency corrected from T-074 to completed T-067; this fix must land before T-074's final rehearsal rather than wait on it.
- 2026-09-19T17:34:56Z: verification pass
- 2026-09-19T17:37:41Z: Implemented exact default-branch ledger polling and merge-race reconciliation with strict canonical-ledger validation. Fixture suite covers landed, pending, failed, missing, delayed, duplicate, contradictory, malformed, blank, truncated, merge-race, and timeout cases. Full task check passed with a temporary fixture-only zip shim because zip is unavailable in the orb; no external release action was run.
