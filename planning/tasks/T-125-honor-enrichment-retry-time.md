---
id: T-125-honor-enrichment-retry-time
title: Honor enrichment retry time when scheduling the next refresh
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#rg-003-github-enrichment-contract
dependencies: []
updated_at: "2026-10-04T23:39:09Z"
---

# T-125-honor-enrichment-retry-time Honor enrichment retry time when scheduling the next refresh

## Description

RG-003 says an enrichment rate limit stops dispatch and polling resumes no
earlier than both the existing poll interval and GitHub's retry time. The actual
application publishes the retry badge correctly but schedules the next source
poll and enrichment at the ordinary 60-second floor, ignoring a 120-second
enrichment Retry-After. This occurs automatically without keyboard input.

Reproduce using the build and isolated HTTPS/PTY commands in
`docs/testing/round3/report.md`, then run its `extra.py`:

1. Launch `--repo acme/api --path acme/api:src --no-cache`. Serve a successful
   events page containing eight valid one-commit pushes with distinct head SHAs.
2. Answer every commit GET with HTTP 429, `Retry-After: 120`, and a small JSON
   message. Keep the events endpoint successful. Do not press refresh.
3. The first publication correctly shows repository `8 activity`, path
   `8 unknown`, `PATH ?8`, and `RATE LIMITED` with a retry time 120 seconds away.
   Queued enrichment stops after two actual commit requests in the recorded run.
4. Wait 61 seconds in the real running process. Expected: no source or enrichment
   request before the instructed retry. Actual: a second Events GET arrives at
   +60.086 seconds and two more commit GETs at +60.087 seconds; the retry badge
   moves another 60 seconds forward. The request log records monotonic times.

This is a valid HTTP rate-limit combination, not malformed source data. A first
diagnostic run with refresh keystrokes also failed; the retained final run uses
no input during the timed interval and reproduces the same automatic violation.

## Acceptance

- Carry the settled enrichment retry time into application refresh scheduling.
  A successful source poll with rate-limited enrichment must not schedule either
  source polling or new enrichment before both source and enrichment constraints.
- Preserve current repository activity and quantitative path unknowns during the
  wait; keep the TUI responsive. Rate-limited evidence is not persistent cache data.
- At or after the permitted retry, a successful response must recover membership
  and clear only the recovered rate-limit condition. Ordinary nonlimited refreshes
  must retain their current scheduling behavior.
- Keep cancellation, organization retry rules and the source poll floor intact.

## Test Expectations

- Add an application-level timer regression for source success plus enrichment
  retry greater than the poll floor, not merely a badge or coordinator test.
- Cover the opposite ordering (source delay later than enrichment retry), elapsed
  retry times and a subsequent successful recovery.
- Repeat a real binary/HTTPS/PTY timed run with Retry-After 120 and no refresh
  keystrokes; assert no Events or commit request at the 60-second boundary.

## Verification Notes

- Baseline: af7255e1cd95cef2cee2d314b8d10d5706fa0e7d, Linux amd64, pure-Go binary,
  pinned `specs/v0.2.0.md`; only a synthetic credential and loopback fixture used.
- `docs/testing/round3/extra-requests.json`, `extra-results.json`, `rate-stop.txt`
  and `rate-after-floor.txt` retain exact request timing, failed assertion and
  before/after real PTY output. The private fixture body is not rendered.
- No existing open task covers this application scheduling defect. It is separate
  from malformed response validation in T-123/T-124 and completed CLI/UI fixes.

## Implementation Notes

- `internal/tui/refresh.go` publishes `evidence.retryAt` as EnrichmentRetryAt but
  `Model.delay` uses only source result delay and defaultDelay for polled attempts.
  Trace scheduling ownership rather than changing the GitHub header parser, which
  already produces the observed correct badge time.
- No product fix or lifecycle completion was performed in round 3.
- 2026-10-04T23:38:59Z: verification pass
- 2026-10-04T23:39:09Z: Settled enrichment retry gates application polling; reviewed TDD and 125s no-input binary recovery evidence pass.
