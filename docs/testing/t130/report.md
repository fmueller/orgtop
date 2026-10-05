# T-130 — latest concurrent enrichment retry

## Task outcome

| Status | Outcome | Product impact |
| --- | --- | --- |
| Completed and verified | Maximum settled in-flight retry deadline is retained and published | Current repository activity and quantitative path unknowns remain until permitted automatic recovery |

Pinned spec: `specs/v0.2.0.md`, RG-003. Clean source `main` equaled freshly
fetched `origin/main` at
[`d7d0299`](https://github.com/fmueller/orgtop/commit/d7d02991f1b315d332ab8a0c0bdcbe1724c25a27).
Only T-130 was selected with `autonomous-task` and `workflow-v3`. T-106 and
other tasks are untouched. This is the final round5 fix, not a sixth global
adversarial round. No release, deployment, production API or database writes.

## What changed

| Area | Change | Reason |
| --- | --- | --- |
| Coordinator | Set the stop immediately; retain maximum retry under existing mutex | Later responses may extend, never shorten, the floor |
| Final publication | Give all rate-limited items the final refresh floor | Queued diagnostic timing agrees with scheduling |
| Ledger/TUI comments | Describe the latest constraint | Match actual field semantics |
| Controlled concurrency test | Two in flight, one queued; both asymmetric completion orders | Prove extension, non-shortening, bounded terminal outcomes, no queued dispatch |
| Binary regressions | Read actual outgoing Retry-After at header emission | Independent response-based timing without weakening T-125 |
| Changelog/task notes | Unreleased fix and historical/current distinction | Document user-visible behavior and evidence |

## Workflow evidence

| Step | Evidence | Result |
| ---: | --- | --- |
| 1 Understand | Fetched/verified clean requested refs; `mise exec -- taskrail status/validate`; read task, active spec, round5 barrier65/95 request/response/results, T125 and round4 fixtures, coordinator and scheduling callers; unshallowed before fixture-producing history | Confirmed first-result-only aggregation in actual failing path; no API/schema/auth/cache change |
| 2 TDD | `mise exec -- go test ./internal/enrichment -run TestConcurrentRetryKeepsLatestDeadline -v` | Red: floor `00:01:05`, wanted `00:01:35.4`; reverse control passed. Minimal max-under-lock change made both green |
| 3 Initial checks | Focused rate/cancel/application scheduling tests; `go test -race ./internal/enrichment ./internal/tui ./cmd/orgtop`; real executable probes | Passed; initial build-raced probes received no responses and are excluded, not treated as product failures |
| 4 Simplify | Dedicated Task explicitly loaded `code-simplifier`; inspected task-local code/tests/fixtures; ran enrichment/TUI/cmd tests | No safe simplification; no edits; all three packages passed |
| 5 Review | Separate parallel General, Go, Python Tasks loaded `code-reviewer`; fresh candidate-validation Task inspected every candidate | Three validated, none rejected or duplicated |
| 6 Disposition | Every validated finding below fixed; strengthened per-item deadline assertion failed before publication fix | Red outcomes 0/2 retained 65s in extension; reverse outcome 1 retained 65s. Final-floor publication made both green |
| 7 Recheck | Final race checks, 100 concurrent-test repetitions, mutation, full `task check`, four final binary runs; fresh disposition-verification Task loaded `code-reviewer` | All three findings RESOLVED; no additional concrete code finding. Missing-report gap closed by this report |
| 8 Finalize | Lifecycle and delivery recorded below after gates | No task verification/completion before independent review |

Lane selection: General always; Go covers the mutex, dispatch state, cancellation,
and publication invariants; Python covers HTTP/PTY timing fixtures. Go loaded
`go-reviewer.md` and `golang-patterns/REFERENCE.md`; Python loaded
`python-reviewer.md` and `python-patterns/REFERENCE.md`; General loaded
`ecc/agents/code-reviewer.md` (no companions listed). Candidate validation and
disposition verification loaded Go/Python companion guidance. Security omitted:
no changed credential, trust, transport, URL, or input handling. Database omitted:
no changed persistence/schema/SQL/transaction behavior; coordinator concurrency
is covered by Go. No framework lane applies. Budget: two specialist lanes.
One review/fix/recheck cycle; nothing deferred.

## Review findings

| ID | Verbatim finding and evidence | Disposition | Fix evidence |
| --- | --- | --- | --- |
| T130-1 | “Queued work settled after the first rate limit keeps that first retry deadline even if a later in-flight result extends the refresh’s final retry floor.” Coordinator gate/settlement; test previously checked only kinds | Fixed | Final publication uses the maximum for every limited item; strengthened test red then green |
| T130-GO-1 | “Update the ledger and prepared-state comments to say these fields hold the latest instructed enrichment retry deadline; their current descriptions contradict the changed behavior.” `bounds.go:94–95`, `state.go:85–87` | Fixed | Both comments say latest |
| T130-PY-1 | “The new independent deadline checks are based on a timestamp taken before the response headers are emitted, with the retry interval duplicated in fixture logic rather than derived from the emitted Retry-After header.” Original hooks at send_response | Fixed | Hooks parse actual buffered header immediately before end_headers emits it; explicit lower-bound qualification; no duplicated interval |

Fresh disposition reviewer: “T130-1 — RESOLVED”, “T130-GO-1 — RESOLVED”,
“T130-PY-1 — RESOLVED, with the measurement limitation explicit.” Its only
additional evidence gap was the then-absent `report.md`, now present with final
logs and timing evidence. A fresh scoped disposition-verification Task loaded
`code-reviewer` and confirmed “RESOLVED — prior evidence gap” and “No concrete
contradictory task-local claim found.” No unresolved code findings.

## Verification

All commands ran from the repository root through the mise-pinned toolchain.
Build finished before the final concurrent executions; the same pure-Go binary
served every final fixture. HTTPS CONNECT permits only api.github.com:443;
credentials are synthetic, HOME/cache disposable, fixtures stop in finally.

| Command/check | Scope | Result |
| --- | --- | --- |
| `mise exec -- env CGO_ENABLED=0 go build -o /tmp/orgtop-round2 ./cmd/orgtop`; `cp /tmp/orgtop-round2 /tmp/orgtop-t125` | Real executable | Exit 0 |
| `mise exec -- go test -race ./internal/enrichment ./internal/tui ./cmd/orgtop` | Coordinator, application scheduler, binary wiring, cancellation/bounds | All packages pass |
| `mise exec -- go test ./internal/enrichment -run TestConcurrentRetryKeepsLatestDeadline -count=100` | Both controlled orders repeated | Pass |
| `mise exec -- python docs/testing/t130-retry.py` | Barrier65/95 no-input recovery | 8/8 pass; [requests](concurrent/requests.json), [responses](concurrent/responses.json), [results](concurrent/results.json) |
| `mise exec -- python docs/testing/t125-retry.py` | Original no-input120s plus independent response floor | 11/11 pass; [requests](t125/requests.json), [responses](t125/responses.json), [results](t125/results.json) |
| `mise exec -- python docs/testing/round4/harness.py` | Existing130s source/120s enrichment composition and bounded fixture matrix | 58/58 pass; [requests](source/requests.json), [results](source/results.json) |
| `mise exec -- python docs/testing/round4/rate-headers.py` | Existing403 Retry-After65/reset75 composition | 6/6 pass; [requests](reset/requests.json), [results](reset/results.json) |
| Header instrumentation stub with outgoing Retry-After97 | Independently vary interval from all fixture constants | Both hooks read97; timestamps precede emission |
| `mise exec -- task test:mutate BASE=d7d02991f1b315d332ab8a0c0bdcbe1724c25a27` | Differential default five mutators | Killed1, lived0, uncovered0, timed out0, skipped1821; efficacy100% |
| `mise exec -- task check` | Full CI-equivalent local gate | Passed through release config validation, including formatting/lint/vet/tests/cross-build/smoke/license/planning/distribution/changelog guards |
| `git diff --check`; `mise exec -- taskrail validate` | Diff and tracked state | Pass; state valid |

Existing `TestEnrichmentRetrySchedulesApplicationTimerAndRecovers` passes all
four asymmetric controls: enrichment120/source75, source180/enrichment95,
elapsed retry, and60s fallback. Publication remains CURRENT with quantitative
unknowns and repository activity; input remains responsive without dispatch;
ordinary scheduling and complete membership return on recovery.

### Exact real-time evidence

No clock injection or keypress during any wait. Each independent floor is
`max(response_emission_monotonic + actual_outgoing_Retry-After)`. The timestamp
is immediately before the buffered header write: a conservative server-side
lower bound, not exact client receipt time. A post-write timestamp could be
later than client receipt and would incorrectly claim that lower-bound property.
No epsilon or tolerance is subtracted from any timing assertion.

| Case | Observed constraint | First Events GET | Recovery |
| --- | --- | --- | --- |
| T130 | Responses at1397.777786400(+65) and1398.177703727(+95); latest floor1493.177703727 | 1493.187649289, floor+0.009945562s | Commit GETs floor+0.050919837s/+0.075112116s; no GET at70s or floor−1s; current2 activity/path?2 becomes2 confirmed, rate badge clears |
| T125 | Actual outgoing120s responses; latest response-derived floor in retained JSON | 120.046647111s after latest response decision; actual emitted-header floor+0.046570323s | No GET at61s/119s; exactly1 Events plus8 commit GETs; unknowns and rate badge clear |
| Round4 source | Source130s outlasts enrichment120s | 130.051434579s after initial enrichment request | No GET at121s; source/path recovery complete |
| Round4 reset | Reset monotonic1472.515756607 beats relative65s | 1472.516557771, reset+0.000801164s | No GET at66s; exactly2 permitted GETs, complete path recovery |

T130 retained [initial](concurrent/initial.txt), [waiting](concurrent/at-70s.txt),
and [recovered](concurrent/recovered.txt) PTY states verify currentness, quantitative
unknowns, automatic membership recovery, cleared rate badge, and absence of the
private fixture body. No rendering appearance code changed.

### Historical T125 119.998494s observation

Round5 logs show requests at678.757051195 and678.763139411, then Events at
798.761676277. Thus the existing assertion failed at119.998494427s after the
second request, while Events was120.004625082s after the first. The old
coordinator used the first settled response, not the latest. However, archived
T125 evidence has no response timestamps: neither response receipt nor its
derived absolute deadline can be reconstructed from these request records.
This is compatible with the confirmed first-result defect, but does not prove
the exact historical millisecond-scale response-deadline violation. It is not
classified as a separate milliseconds bug or used to weaken an assertion.
The rerun preserves the original assertion and additionally verifies the actual
outgoing header-derived lower bound independently; both pass. The round5
65/95s response/request records independently establish the material defect.

## Remaining risks / follow-ups

| Item | Impact | Action |
| --- | --- | --- |
| Historical exact client response time unavailable | Cannot causally reconstruct119.998494s against the actual response deadline | Retain uncertainty; fresh instrumented results supersede request-only timing for this fix |
| Taskrail version warning | Installed skills say0.4.0, binary reports0.0.0-dev; commands/state validation succeed | No unrelated skill/toolchain mutation in this task |
| T-106 maintainer release acceptance | This fix does not authorize release | Leave task untouched; no sixth global round |

## Lifecycle and delivery

Final `mise exec -- task check` returned `TASK_CHECK_EXIT=0`. At
2026-10-05T03:06:05Z, `taskrail verify T-130-honor-latest-concurrent-retry
--result pass --summary ... --details ...` created its verification report,
then `taskrail complete T-130-honor-latest-concurrent-retry --note ...` completed
the task. Each transition's diff was inspected and `taskrail validate` returned
`state valid`. Status is idle: 127 completed, zero active/blocked, three todo;
the active spec remains v0.2.0 and T-106 remains unchanged. Generated lifecycle
artifacts under `planning/artifacts/verify/.../20261005T030605Z/` remain ignored
per repository policy; this report and the task notes retain durable evidence.

Hooks were installed with `mise exec -- task hooks-install`; maintainer git
identity is Felix Müller / felix@fmueller.io and the author guard passed.
User authorization covers the completed-task commit and push to main only.
The containing commit and freshly fetched remote equality are confirmed in the
delivery response after the push; no deployment or release is authorized.
