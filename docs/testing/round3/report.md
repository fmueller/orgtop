# Adversarial executable testing — round 3

Date: 2026-10-04 UTC. Pinned specification: `specs/v0.2.0.md`. Baseline:
[`af7255e`](https://github.com/fmueller/orgtop/commit/af7255e1cd95cef2cee2d314b8d10d5706fa0e7d),
verified clean and equal to freshly fetched `origin/main` before testing.
Verdict: **three new confirmed defects: T-123, T-124, T-125. No product fixes.**
Completed T-119/T-120/T-121/T-122 regressions pass. T-106 remains untouched.

## Process method and reproduction

Linux amd64 orb; Go 1.26.7 via mise; actual pure-Go executable in real tmux
PTYs. Reused the committed round2 HTTPS CONNECT fixture and T-122 helper, after
reading the prior reports and their producing commits. No injected model/source
seams, clock substitution, or shortened refresh floor. The loopback fixture
accepts only `api.github.com:443`, trusts a disposable certificate and supplies
only synthetic credentials through an isolated HOME/cache and `env -i`.
All requests are GET; no live API data, mutations, releases or deployments.

Run from the repository root with Python, OpenSSL and tmux available:

```sh
mise exec -- env CGO_ENABLED=0 go build -o /tmp/orgtop-round2 ./cmd/orgtop
mise exec -- python docs/testing/round2/harness.py
mise exec -- python docs/testing/t122-entity-controls.py /tmp/orgtop-round2 round3
mise exec -- python docs/testing/round3/harness.py
mise exec -- python docs/testing/round3/extra.py
```

The binary name retains round2's name solely to reuse its committed transport.
The scripts run finite servers/private tmux sockets and stop them in finally
blocks. The first, third and fourth scripts leave disposable roots named by
their `/tmp/orgtop-*-evidence/runtime.json`; remove those after inspecting them.
Never use a real cache location. Exit zero means the script completed, **not**
that its recorded assertions passed. New-defect assertions intentionally remain
red until their tasks are implemented. The T-122 helper does return nonzero for
failed assertions and removes its disposable root itself.

The retained final runs record **324 assertions: 307 pass, 17 fail**, with all
17 failures belonging to the three findings below. Main: 210/218; supplemental:
20/29; round2 regression: 70/70; T-122 helper: 7/7. Main/supplemental/regression
request logs contain 92/16/66 GETs respectively; the T-122 helper verifies one
GET. Logs retain only paths, media/API headers, monotonic time and a synthetic
credential-match boolean. Main and regression headers all match the contract.

## Exact executed matrix

| Process flow / assertion | Result |
| --- | --- |
| T-119: four info aliases × unknown-before, unknown-after/reset, missing repo and malformed repo | 16 correct zero exits/output streams |
| T-120: reset with four boolean assignments × true/false | Eight usage exits 2; sentinel unchanged |
| T-120: invalid reset against populated SQLite | Usage exit 2; identical SHA-256 |
| T-121 valid bidi branch: Stream/detail wide and Stream 40x12 | Visible escape; no raw bidi |
| T-122 malformed entity head: Rain 240x32, 80x24, 40x12, 20x6 and detail | Safe escape/width assertions, one GET, responsive quit: 7/7 |
| Complete-empty commit and compare `files: []` | Repository activity current; known no path activity, no unknown |
| Missing/null commit files and missing/null compare files | **T-123:** falsely complete-empty, no unknown |
| Commit page 1 valid + page 2 absent files | **T-123:** partial page-1 paths falsely complete |
| Compare 299 versus 300 unique valid records | 299 complete/member; 300 incomplete/unknown |
| Valid file plus unknown status; rename missing previous_filename; duplicate file | Whole result unknown; no subset admitted |
| Valid two-page commit | Complete member; three total GETs |
| Page 2 denied (403) or rate limited (429 + Retry-After 120) | Current repository, explicit path unknown; private response body absent |
| Malformed next Link without angle brackets | **T-124:** silently terminal, falsely complete |
| Cycle page 1 -> 2 -> 2 | Incomplete; bounded requests |
| Page 1 -> 3 -> 2 -> terminal | **T-124:** decreasing page followed and declared complete |
| Duplicate page query `page=2&page=999` | **T-124:** ambiguous next URL followed and declared complete |
| All 19 preceding enrichment cases | Current snapshot, no private response body, <=4 GETs each, responsive q |
| Same head shared by two pushes with different before SHAs | One lookup; two repository activities, one path member and one unknown |
| Missing/null response cold cache, then corrected valid upstream in second process | **T-123:** complete zero-path SQLite row; zero warm commit GETs; wrong non-membership retained |
| Malformed Link cold cache, then second process | **T-124:** complete one-path row; zero warm commit GETs |
| Owned schema version 99 | CACHE DEGRADED, DB SHA-256 preserved, full API fallback; idle reset succeeds |
| Foreign application_id=123 and ambiguous application_id=0 | CACHE DEGRADED, DB SHA-256 preserved, full API fallback; reset refuses and preserves DB |
| Exact owned v1 with evidence_path table dropped | Rebuild; full reacquisition; complete memberships without degradation |
| Wrong path_count or noncontiguous ordinal | Miss/reacquire, complete membership, no cache degradation |
| acquired_at/last_used_at 1,000 seconds future or exactly 30 days old before launch | Miss/reacquire, complete membership, no cache degradation |
| Reset tombstone plus orphan -wal/-shm/-journal, no main | Recovery/rebuild/reacquire; complete memberships, no degradation |
| Interrupted bootstrap file, no main | Recovery/rebuild/reacquire; complete memberships, no degradation |
| SIGKILL of a launched cache client after publication, next process | Complete current snapshot; cached evidence reused, no extra lookup |
| Cold/warm ordinary cache and two simultaneous clients (round2 rerun) | Warm skips lookup; both concurrent clients current |
| Reset during active clients | Bounded refusal (<3s), DB retained |
| Real SQLite BEGIN IMMEDIATE contention | Current membership plus CACHE DEGRADED, responsive quit |
| Idle reset then new process | Main removed, rebuilt and enrichment reacquired |
| Twelve hostile CLI cases listed below | All exit 2, sentinel unchanged, no HTTPS request |
| Ten events at ages -60, 0, 290, 310, 890, 910, 3590, 3610, 86410, 604810 seconds at response creation | All ten retained in Overview/Stream, including future and old |
| Future Stream event | Youngest `<1m` display; not negative age |
| Interesting Now on those ages | Exactly actors 0–4 inside 15m; actors 5–9 excluded |
| Rain pause, four `-` presses then six `+` presses | 15m endpoint then available endpoint, PAUSED retained; strip independently excludes old events |
| Return to Stream after window changes | All ten events remain |
| Eight distinct commit identities, all 429 Retry-After 120 | Eight unknowns, repository 8 activity, RATE LIMITED; only two initial commits dispatched |
| Same rate-limited process left untouched for 61 seconds | **T-125:** Events at +60.086s and commits at +60.087s, before retry time |
| Organization oversized, cross-host next, conflicting duplicate, partial second-selector failure | Atomic ERROR, no exact/partial repository polls |
| Empty expanded organization plus exact repo | Exact repo remains polled |
| Two organizations, five pages, one exact repo | alpha,beta,alpha,beta,alpha; 20-repo bound; 481 known omissions plus unknown-more disclosure |
| Six repositories ×100 events | Newest 500 of 600 disclosed |
| 50-scope Overview/Rain resize through 1x1 and restoration | Accounting restored, responsive navigation |
| Source success -> partial-source 503 -> source recovery with enrichment 403 -> enrichment recovery | Real 61s waits; no partial publication; retained STALE -> current PATH ?1 -> complete |

The twelve CLI inputs are the exact `cases` array in `harness.py`: path `../bad`,
qualified second colon, bare path with only org, reset+positional, reset+org,
include-forks without selector, embedded `**`, unsupported backslash escape,
no-cache boolean assignment, positional after `--`, slash in org and absolute
path. All used a pre-existing disposable cache sentinel.

## Findings and exclusions

- **T-123, RG-003:** missing required `files` is accepted as complete, persisted
  and reused. Positive explicit-empty controls rule out a disagreement about
  legitimate empty evidence. This includes commit and compare decoding and
  later-page partial admission, one focused defect rather than five tasks.
- **T-124, RG-003:** malformed/nonadvancing commit pagination can prove false
  completeness and reach cache. All files and identities in these cases are
  otherwise valid, so this is distinct from T-123.
- **T-125, RG-003:** the settled enrichment retry time reaches the badge but not
  the refresh timer. The final timed reproduction has no keyboard input. This
  is realistic valid rate-limit metadata, unlike the deliberately malformed
  transport inputs in T-123/T-124. No upstream-malformation frequency is claimed.

An initial supplemental assertion incorrectly expected Stream's future-event age
to spell `0s`; the shipped coarse display is `<1m`. It was corrected and the
whole supplemental run repeated. That mistake is not a product finding. The
first rate run included refresh keystrokes; the retained no-keystroke run rules
out that alternative cause. Red assertions were not suppressed or rewritten to
accept the observed contract violations.

## Checks, limits and task scope

Taskrail validation passed before and after every task creation, with actual
`taskrail task new` commands and verified RG-003 anchor. Only three new todo
tasks and Taskrail's generated timestamp/todo count were changed. Existing
T-071/T-106/T-118 remain untouched. The installed-skill/dev-version warning is
nonfatal; no unrelated skill refresh was performed. Maintainer identity and
`mise exec -- task hooks-install` were verified before commit.

`mise exec -- task check` exited 0: formatting, vet, lint, all Go tests,
policy/distribution fixtures, cross-build, startup smoke, licenses, Taskrail
validation/coverage and release configuration. Separate
`mise exec -- task workflow:validate` reports `state valid`, and
`mise exec -- task workflow:verify:spec` reports 12/12 areas, no drift. Python
syntax parsing and `git diff --check` pass. These green repository checks do not
override the deliberately red executable defect assertions above.

No exact subsecond recency/style threshold claim: the age vectors straddle the
boundaries by ten seconds with the real clock, and assertions inspect age text,
strip retention and controls, not ANSI color/emphasis. Rain field item-count
eligibility at an exact instant was not asserted. The reset/bootstrap tests seed
interrupted-state fixtures; SIGKILL is after publication, not a proven kill
inside a write transaction. No exhaustive filesystem race or power-loss proof,
15-minute organization re-expansion, all simultaneous Retry-After combinations,
freshness-at-exact-clock-instant, or 120-second eventual retry recovery was run.

Linux amd64 only; no real host ACL/access semantics, live GitHub account, macOS,
Windows, arm64, release artifacts, extension/Homebrew installation or maintainer
sign-off. These results **do not complete T-106 or establish release readiness**.
The binary remains unchanged with three reproducible defects pending fixes.
