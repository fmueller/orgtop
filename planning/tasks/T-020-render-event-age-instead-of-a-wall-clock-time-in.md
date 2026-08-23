---
id: T-020-render-event-age-instead-of-a-wall-clock-time-in
title: Render event age instead of a wall-clock time in Stream
status: completed
priority: high
spec_ref: specs/v0.1.0.md#stream-view
dependencies: []
updated_at: "2026-08-23T20:40:30Z"
---

# T-020-render-event-age-instead-of-a-wall-clock-time-in Render event age instead of a wall-clock time in Stream

## Description

Stream stamps each row with `event.OccurredAt.Local().Format("15:04:05")`
(`internal/tui/stream.go`, `layoutStreamRows`). The snapshot is bounded to 500
events drawn from one `per_page=100` page per repository, so a quiet repository's
page reaches back weeks or months. A time of day cannot place those events, and
it hides the reverse-chronological order the domain guarantees.

Observed with a synthetic snapshot aged 2m / 25m / 3h / 26h / 4d / 23d:

```
14:38:00  acme/backend   push          alice · pushed 3 commits to main
14:15:00  acme/frontend  pull request  bob · opened pull request #42
11:40:00  acme/backend   review        carol · approved pull request #41
12:40:00  acme/infra     comment       dave · commented on pull request #17
14:40:00  acme/infra     other         erin · watch activity
14:40:00  acme/frontend  push          alice · pushed 1 commit to release
```

The column reads `14:38 → 14:15 → 11:40 → 12:40 → 14:40 → 14:40`: down, then up
again. The ordering is correct per FR-005 but reads as broken, and the last two
rows claim the same instant as the header's `updated 14:40:00` while being four
days and twenty-three days old.

Replacing the clock with an age fixes the legibility defect and costs less width,
which matters at the `40x10` contract: `3w` is two columns against eleven for a
date and time.

Ages anchor to the last successful refresh, not `time.Now()`. The polling floor
is 60 seconds, so a wall-clock age would drift between redraws and disagree with
the `updated` field in the same header. Anchoring to last success also keeps
`STALE` honest: ages freeze with the snapshot they describe instead of aging past
data that is no longer being refreshed.

## Acceptance

- Stream renders each event's age at the last successful refresh in place of the
  wall-clock time, in one unit at whole-unit resolution, coarsening as it grows:
  under a minute, then minutes, hours, days, weeks, years.
- The age column is right-aligned and fixed-width, so the ordering reads straight
  down the column and rows stay aligned.
- Ages are computed against the last-success instant the header reports, never
  against the current clock. A `STALE` snapshot keeps the ages it had when it was
  current.
- An event timestamped after the last-success instant renders as the youngest age
  rather than a negative or empty one.
- Bucket boundaries are exact and tested at both sides: a whole unit rounds down,
  so 119 seconds is one minute, not two.
- The rendered ages of a snapshot are non-decreasing from the first row to the
  last, which is the visible form of the FR-005 ordering guarantee.
- No wall-clock time of day appears in a Stream row at any width.
- The narrowest layout still fits `40x10` and A-010 continues to hold.
- Overview and the shared header are unchanged; the header keeps reporting the
  last-success clock time, which the ages are relative to.

## Test Expectations

- Table-driven unit tests over the bucket boundaries in `internal/tui`, including
  the future-timestamp clamp and the zero-value last-success case.
- A `internal/tui` test asserting rendered ages are non-decreasing down a snapshot
  whose events span minutes to weeks.
- A wired flow in `cmd/orgtop/integration_test.go` proving a multi-day snapshot
  renders ages rather than clock times.
- A `STALE` test proving the ages do not advance while the snapshot is stale.

## Verification Notes

- Record `task check`, `go test ./... -race`, and `git diff --check` exit statuses.
- Capture a before and after render of the same synthetic snapshot at `100x12` and
  `40x10`, showing the ordering now reads monotonically.
- Regression proof: reverting to the clock format must fail the ordering test.

## Implementation Notes

- Discovered in manual testing after T-017 verified release readiness. This blocks
  the `v0.1.0` tag: it undercuts the Release Intent that a developer can learn
  something useful about near-current activity.
- The `clock` field of `streamLayout` and the `clockLayout` constant in
  `internal/tui/chrome.go` are the current seam. `clockLayout` is also used by the
  header's `updated` field, which keeps its wall-clock spelling.
- `State.LastSuccess` already carries the anchor instant. `Model.now` exists for
  recording successes and is not the anchor for rendering.
- Keep this task to the age spelling. Column headings are T-022; the description
  rework is T-021; coverage disclosure and truncation marking are T-023.
- 2026-08-23T20:40:26Z: verification pass
