# T-125 — enrichment retry scheduling

## Task outcome

| Status | Outcome | Product impact |
| --- | --- | --- |
| Completed and verified | The application waits for source delay, poll floor and settled enrichment retry | Current repository activity and path unknowns remain visible until permitted recovery |

Pinned specification: `specs/v0.2.0.md`, RG-003. Initial clean `main` and freshly
fetched `origin/main` both matched
[`92f47ed`](https://github.com/fmueller/orgtop/commit/92f47ed5cc2724a4637088bbc493bd0940a839e0).
Only T-125 was started. No releases, deployments, live API calls or unrelated task
transitions were performed.

## What changed

| Area | Change | Contract |
| --- | --- | --- |
| `internal/tui/refresh.go` | Calculate delay once at publication; include remaining settled enrichment retry | Both scheduling constraints and 60-second floor; expansion-only rules unchanged |
| `internal/tui/enrichment_test.go` | Application timer regression, asymmetric ordering, elapsed retry, floor and recovery | Current/unknown publication, responsive input, ordinary recovery scheduling |
| `docs/testing/t125-retry.py` | Real executable HTTPS/PTY 125-second no-input regression | No source/enrichment before instructed retry; successful automatic recovery |
| `CHANGELOG.md` | Unreleased fix | User-visible scheduling change |

## Workflow evidence

| Step | Executed evidence | Result |
| --- | --- | --- |
| 1 Understand | Loaded `workflow-v3` and explicit `autonomous-task T-125-honor-enrichment-retry-time`; status/validate; task, round3 report/extra, active spec, refresh/enrichment/organization ownership inspected | Header parser and badge already correct; application timer omitted enrichment constraint |
| 2 Strict TDD | `mise exec -- go test ./internal/tui -run TestEnrichmentRetrySchedulesApplicationTimerAndRecovers -count=1` | Before production edit: `application timer delays [1m15s], want [2m0s]`; after edit full TUI suite passed |
| 3 Initial checks | `mise exec -- go test ./internal/tui -count=1`; `mise exec -- task check`; actual baseline/fixed process runs | Passed checks; baseline reproduced automatic violation |
| 4 Simplify | Dedicated Task loaded personal `code-simplifier`; inspected actual diff | No Go simplification needed; parsed fixture Events URL before counting, removed unused variable; TUI/syntax/diff checks passed |
| 5 Independent review | Separate parallel General, Go and Python Tasks loaded personal `code-reviewer`; Go `go-reviewer` + `golang-patterns`; Python `python-reviewer` + `python-patterns`; General common review/testing guidance | All three: “No concrete task-relevant findings.” |
| 5 Candidate validation | Fresh Task loaded `code-reviewer` candidate-validation mode | 0 validated, 0 rejected, 0 deduplicated; no findings |
| 6 Disposition | Empty validated finding set | Nothing fixed or deferred; no invented candidates |
| 7 Recheck/verification | Final `task check`, differential mutation, finalized 125-second binary run; fresh `code-reviewer` disposition-verification Task inspected code and evidence | One review cycle; “Unresolved or newly introduced task-relevant issues: none.” |
| 8 Lifecycle | `taskrail verify ... --result pass` at 2026-10-04T23:38:59Z; `taskrail complete ...` at 23:39:09Z; diff inspected and validate after each writer | Completed; state valid, idle, 122 done / 3 todo, no active or blocked tasks |

Lane choice: General required, Go for application timer/state, Python for the
acceptance fixture. Security omitted: no production trust/auth/API boundary changed,
and fixture uses only isolated synthetic credentials. Database omitted: no cache or
persistence behavior changed. Concurrency specialist omitted: no goroutine, lock or
dispatch design changed; Go review included cancellation/organization context.
No framework specialist applies to the scheduling-only Bubble Tea change. Budget
was two specialists, within the default three. General's bare `go` and `taskrail`
attempts lacked PATH; main and other workers used `mise exec --` and passed.

## Executed process evidence

Reused round2's committed transport method after inspecting its producing commit
[`7084304`](https://github.com/fmueller/orgtop/commit/70843043539d57dd0d6cfa35addebca751508f8b)
and round3 reproduction. Pure-Go actual executable, Linux amd64, disposable trusted
certificate for `api.github.com`, loopback CONNECT proxy, isolated `env -i` HOME and
cache, synthetic token, real tmux PTY. No substituted clock, shortened floor or
refresh keypresses. The finite fixture stops its server/tmux and removes its root.

Baseline: built the clean pinned revision in a disposable detached worktree to
`/tmp/orgtop-round2`, then executed `mise exec -- python docs/testing/round3/extra.py`.
That script exits zero even with failed assertions; the asserted 120-second hold
**failed**. [Baseline request log](baseline-requests.json): relative to first Events,
the second Events arrived at +60.087010s, commits at +60.087778s and +60.087992s.

Final commands:

```sh
mise exec -- env CGO_ENABLED=0 go build -o /tmp/orgtop-t125 ./cmd/orgtop
mise exec -- python docs/testing/t125-retry.py
```

Finalized fixture exit 0, **10/10 assertions passed**. An earlier fixed-binary run
passed timing/recovery but failed its raw-URL Events-count assertion; the query-aware
count was corrected by the simplifier and the entire 125-second run repeated.
All times below are monotonic, relative to the latest initial rate-limit response:

| Observation | Elapsed seconds | Result |
| --- | ---: | --- |
| 61-second boundary | 61.000121321 | Still exactly three requests: one Events and two initial commits |
| Before instructed retry | 119.000505045 | Same three requests; no source or enrichment dispatched |
| Automatic Events retry | 120.003194425 | Not early |
| First recovery commit | 120.045126861 | Not early |
| Last of eight recovery commits | 120.176655982 | All eight succeed |
| Snapshot captured after waiting 125 seconds | ≥125 | Both rows show eight activity; unknowns and rate badge cleared |
| First/only keypress, quit after recovery | >125 | Responsive exit 0 |

Retained evidence: [assertions](results.json), [requests](requests.json),
[initial PTY](initial.txt), [61-second PTY](at-61s.txt), [recovered PTY](recovered.txt).
Initial and waiting state both show repository `8 activity`, path `8 unknown`,
`PATH ?8`, `RATE LIMITED`. Neither private response body nor raw credentials appears.

## Verification and remaining limitations

| Command | Result |
| --- | --- |
| `mise exec -- go test ./internal/tui -count=1` | Pass, including cancellation and organization scheduling coverage |
| `mise exec -- task check` (initial and final) | Exit 0; formatting, vet, lint (0 issues), all Go tests, script guards, build, startup smoke, licenses, Taskrail validation/coverage, release config |
| `mise exec -- task test:mutate BASE=92f47ed5cc2724a4637088bbc493bd0940a839e0` | Exit 0; Killed 0, Lived 0, Not covered 0, Timed out 0, Not viable 0, Skipped 1806 |
| Finalized `t125-retry.py` | Exit 0; 10/10 pass |
| Python syntax parse and `git diff --check` | Pass |
| Taskrail validation after start, verify and complete | `state valid` |

Default differential mutators produced no eligible mutants for this max-argument
change; **no mutation efficacy is claimed**. The actual red-before-fix test proves
that omitting the enrichment bound is detected. No full mutation gate was added.
Existing Taskrail installed-skill version warnings and assembly license inspection
warnings were nonfatal; no unrelated tooling changes were made. No blocking issue
or task follow-up remains. Maintainer release-readiness work remains outside scope.
