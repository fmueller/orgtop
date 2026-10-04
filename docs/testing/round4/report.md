# Adversarial executable testing — round 4

Date: 2026-10-04 UTC (2026-10-05 Europe/Berlin). Pinned spec:
`specs/v0.2.0.md`. Clean initial main equaled freshly fetched origin/main at
[`62e4707`](https://github.com/fmueller/orgtop/commit/62e4707d8f1e10fd8fb55f447be7de9499203c67).
**Three new defects: T-126, T-127, T-128. No fixes.** T-119–T-125 remain closed;
maintainer T-106 and other pre-existing tasks were not changed.

## Method and exact commands

Linux amd64 orb, mise-pinned Go 1.26.7, pure-Go executable, real HTTPS CONNECT,
tmux PTY, SQLite files/locks and elapsed-time application scheduling. Reused
committed round2/round3 and T-122–T-125 fixtures after reading their reports and
producing commit history (those commits have no thread trailers). Only synthetic
credentials, disposable HOME/cache, and loopback TLS were used. The proxy accepts
only api.github.com:443. No live API writes, releases, deployments, workflow reruns,
or application/model/clock injection. Request logs contain no credential values.

Run from repository root, using separate fixture sockets:

```sh
mise exec -- env CGO_ENABLED=0 go build -o /tmp/orgtop-round2 ./cmd/orgtop
mise exec -- cp /tmp/orgtop-round2 /tmp/orgtop-t125
mise exec -- python docs/testing/round2/harness.py
mise exec -- python docs/testing/round3/harness.py
mise exec -- python docs/testing/t122-entity-controls.py /tmp/orgtop-round2 round4
mise exec -- python docs/testing/t123-files-array.py
mise exec -- python docs/testing/t124-pagination.py
mise exec -- python docs/testing/t125-retry.py
mise exec -- python docs/testing/round4/harness.py
mise exec -- python docs/testing/round4/rate-headers.py
```

The first diagnostic regression launch raced the build and failed before any
binary execution. It was rerun after build completion; only the complete rerun
is retained/counts below. Round2/round3 exit zero means execution finished, so
their JSON assertions were independently checked. The new harness exits 1 for
the five recorded red assertions. Removing one unused fixture mode afterward
does not change any executed vector. Fixture servers/private tmux sockets stop
in finally blocks; old harness temporary roots are removed after inspection.

**743 assertions: 738 pass, five fail**, excluding the separately inspected
remote Windows failures. Reused suites: regression 70/70; cache 218/218; files
64/64; pagination 310/310; retry 10/10; control rendering 7/7. New matrix: 53/58;
composed headers: 6/6. The five failures are three T-126 vectors and two T-127
assertions, not five distinct defects. Retained JSON logs contain **361 GETs**,
all matching API version/media/synthetic-auth checks; T-122 independently verifies
one additional GET. PTY text was inspected directly, not inferred from unit tests.

## Exact executed matrix

| Process flow / assertion | Result |
| --- | --- |
| Four informational aliases × unknown-before, unknown-after/reset, missing repo, malformed repo | 16 correct exits 0/streams |
| Four bare boolean flags × true/false with reset | Eight usage exits 2, sentinel unchanged; populated SQLite hash also preserved |
| Twelve hostile CLI vectors from round3 | Exit 2, sentinel unchanged, zero network |
| Valid bidi branch in Stream/detail and 40x12 | Escaped, no raw bidi |
| Malformed entity reference, Rain 240x32/80x24/40x12/20x6 and detail | Safe escape, bounded one GET, responsive quit: 7/7 |
| Missing/null commit and compare files, page-2 missing files | Current repository, explicit path unknown, no complete cache row; corrected second process reacquires |
| Empty arrays and valid two-page commit | Correct complete nonmembership/membership; warm cache skips lookup |
| 299/300 compare files; unknown status; missing rename source; duplicate file | 299 complete, 300 unknown; invalid records discard whole evidence |
| Page-2 403/429; malformed/reverse/cyclic/duplicate-query Links | Incomplete; no partial membership or complete cache row |
| Eleven T-124 modes × no-cache/cold/repeat/corrected/reused | All 310 assertions pass, including exact request/page counts and SQLite admission |
| Valid plain/relation-list/semicolon-title/last-before-next/encoded-key Link | Two commit GETs, complete membership |
| Duplicate next Link | One commit GET, unknown |
| Valid `title="page, two"; rel="next"` | **T-127:** one commit GET, PATH ?1 instead of complete member |
| Valid organization two-page Link | Two listing GETs, three Events GETs, current selection |
| Organization duplicate page / duplicate per_page / invalid `%zz` query suffix | **T-126:** follows malformed URL and publishes selection; expected atomic ERROR/no Events GET |
| Organization ordinary extra query / semicolon suffix | Atomic ERROR; only initial listing request |
| Prior oversized/cross-host/conflicting/partial organization fixtures | Atomic failure; no partial or exact polls |
| Two organizations, five pages and exact repo | Fair listing order, 20-repo bound, 481 known plus unknown-more omissions |
| Six repos × 100 events and 50-scope resize to 1x1 then restore | Newest 500/600 disclosure; accounting/navigation restored |
| Cold/warm/concurrent cache clients, active/idle reset, BEGIN IMMEDIATE contention | Reuse, bounded active refusal, idle rebuild, degraded current fallback, responsive quit |
| Cache version/ownership/schema/count/ordinal/future/expired/tombstone/bootstrap/SIGKILL cases | Preserve incompatible/foreign DB, rebuild/miss/reacquire recoverable data, reuse after crash; all 218 cache-suite assertions pass |
| Shared cache: src → src2 → SRC → src/*.go → changed before SHA → original before | Member → nonmember → nonmember → member → unknown → member; exactly one initial commit lookup, none afterward |
| Source success → partial-source 503 → source recovery/enrichment 403 → enrichment recovery | Real 61s waits: retained STALE without partial publication → current unknown → complete |
| Eight distinct commits all 429 Retry-After 120 | Two initial dispatches, eight unknown; no GET at 61/119s; source retry +120.001s, eight successful lookups, badge clears |
| Source X-Poll-Interval 130 plus enrichment Retry-After 120 | No GET at 121s; source retry +130.104s from initial source request, complete recovery |
| 403 Retry-After 65 plus exhausted remaining/reset about +75s | No GET at 66s; retry 1.33ms after reset, complete recovery and no private body |
| Native Windows baseline CI | **T-128:** 24 subtests fail before process launch; not a Linux runtime claim |

## New defects and Windows diagnosis

- **T-126**, RG-010: `advancesListingPage` checks first values only and its caller
  discards query parse errors. `requests.json` records actual dispatch of each
  ambiguous/malformed URL and subsequent Events polls. This is independent of
  the fixed commit pagination path in T-124.
- **T-127**, RG-003: unconditional comma splitting in shared `linkRelation`
  mistakes a quoted title comma for a link separator. RFC 8288 sections 3/3.4
  permit this quoted parameter; Appendix B describes quote-aware parsing.
  No claim is made that GitHub currently emits titles; the valid adapter input
  must not be rejected. Retained passing variants prevent blanket rejection
  from masquerading as a fix.
- **T-128**, NFR-006: current main
  [CI run 37244638668](https://github.com/fmueller/orgtop/actions/runs/37244638668)
  and prior [run 37243414318](https://github.com/fmueller/orgtop/actions/runs/37243414318)
  repeat the same process-launch failures. Both tests build an extensionless
  `orgtop` temporary file. All 16 informational and eight boolean cases report
  `executable file not found in %PATH%`; product assertions never run. The current
  Windows Build step passes, Test fails, Startup smoke is skipped. Ubuntu amd64,
  Ubuntu arm64, macOS and cross-compile jobs pass. This is a reproducible current
  test defect, not merely a historical/transient report, and does not demonstrate
  a broken release binary. The task also requires portable fixture isolation.

Linux diagnostic:
`mise exec -- go test ./cmd/orgtop -run 'TestInformationalExecutable|TestExecutableBooleanAssignmentsPreserveCache' -count=1`
returned `ok github.com/fmueller/orgtop/cmd/orgtop 1.950s`. Windows was not executed
in this Linux orb; its runtime evidence is the current remote CI log, filtered
in `windows-failures.txt`. No CI was manually triggered or rerun.

## Limits and delivery gate

No live-host/channel acceptance, packaged install, Windows orb execution, terminal
font/color screenshot certification, mutation run, or exhaustive spec coverage is
claimed. T-106 remains the maintainer's separate release gate. New tasks contain
real spec anchors, expected/actual reproductions, acceptance and regression
requirements; no production code or existing task lifecycle was changed.

`mise exec -- task check` exited 0, including all Go tests, static checks,
distribution guards and release configuration validation. `taskrail validate`
reported `state valid`; `git diff --check` passed. Hooks were installed and
maintainer identity checked before the authorized evidence/task-only commit and
push. The adversarial red assertions and baseline Windows CI remain blockers for
their follow-up tasks, not hidden passes.
