---
id: T-130-honor-latest-concurrent-retry
title: Honor the latest concurrent enrichment retry deadline
status: completed
priority: high
spec_ref: specs/v0.2.0.md#rg-003-github-enrichment-contract
dependencies: []
updated_at: "2026-10-05T03:06:05Z"
---

# T-130-honor-latest-concurrent-retry Honor the latest concurrent enrichment retry deadline

## Description

RG-003 Typed Outcomes and Failure Handling requires rate limiting until the
latest instructed time, with polling no earlier than both the poll interval and
that deadline. At the round5 baseline, concurrent in-flight results retained only
the first rate-limit deadline. This is distinct from T-125's propagation of an
already-computed enrichment deadline into the TUI scheduler.

Reproduce with `docs/testing/round5/concurrent-retry.py`: real binary
`--repo acme/api --path acme/api:src --no-cache`, two valid distinct one-commit
pushes, and a barrier ensuring both enrichment GETs are in flight. Return 429
with Retry-After 65 for one, then 429 with Retry-After 95 for the other 0.4 seconds
later. The upstream becomes healthy without keyboard input or injected clocks.

Expected: no source or enrichment GET before the second response's 95-second
deadline. Historical actual: Events and both commit GETs resume approximately 65 seconds
after the first response, over 30 seconds before the latest instructed deadline.
The initial UI publishes two unknowns and an earlier rate-limit deadline; it
clears the badge and confirms both paths during the prohibited interval.

## Acceptance

- Aggregate all settled in-flight rate-limit outcomes to the maximum valid
  retry-at deadline; a later result may extend but never shorten the floor.
- Preserve immediate queued-dispatch stop after the first rate response and
  bounded terminal outcomes for queued work; no same-refresh retries.
- Publish the final latest retry constraint to application scheduling so neither
  Events nor enrichment resumes before it or a later source poll constraint.
- Preserve current repository activity, quantitative path unknowns, sanitized
  diagnostics, and automatic complete recovery after the permitted deadline.

## Test Expectations

- Deterministic coordinator concurrency >=2 tests with controlled completion
  order and asymmetric retry deadlines, including the reverse order control.
- Binary HTTP/PTY elapsed-time test with both requests already in flight;
  assert actual request timestamps, absence during the later floor, and recovery.
- Retain T-125 and round4 source/enrichment/reset constraint composition tests;
  do not weaken their timing assertions to mask an early retry.

## Verification Notes

- Baseline ba56c5dc5776664a201c7cb4eda5b206690231fb; pinned specs/v0.2.0.md.
- `docs/testing/round5/concurrent-results.json`, `concurrent-requests.json`, and
  `concurrent-responses.json` retain both 429 response times and the early GETs.
- `docs/testing/round5/concurrent-initial.txt` and `concurrent-at-70s.txt` capture
  the initial rate-limited unknowns and premature successful recovery.
- Existing T-125 regression also failed its >=120s assertion at 119.998494s
  relative to the second request. The independent 65/95s fixture establishes a
  material defect rather than relying on that millisecond-scale observation.
- Historical round5 result: confirmed by execution, not fixed at that time.
  T-106 is untouched.

## Implementation Notes

`internal/enrichment/coordinator.go:settleIdentity` now stops queued dispatch
immediately and aggregates the maximum settled in-flight deadline under its
existing mutex. Publication gives all rate-limited outcomes that final floor,
including queued identities settled before the last response. The application
continues composing it with source polling constraints. Verification and review
evidence: `docs/testing/t130/report.md`.
- 2026-10-05T03:06:05Z: verification pass
- 2026-10-05T03:06:05Z: Maximum concurrent retry floor and final limited publication implemented; all workflow-v3 review dispositions resolved. Full task check, race, differential mutation and retained real-time HTTP/PTY evidence passed. See docs/testing/t130/report.md.
