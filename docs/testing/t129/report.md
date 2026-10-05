# T-129 — decode escaped Link relations

## Task outcome

| Status | Outcome | User/product impact |
|---|---|---|
| Verified and completed | Decode RFC 8288 quoted pairs before relation recognition | Commit evidence and organization selection include both valid pages before cache proof or polling |

Source checkout was clean on `main`, equal to fetched `origin/main` at
`0aa5b328be10f9c4610117c2054263397786bfda`. Normative spec: `specs/v0.2.0.md`,
RG-003 and RG-010. Only T-129 was started; T-130 and release acceptance are separate.
This is a focused task regression, not a sixth global adversarial round.

## What changed

| Area | Change | Why |
|---|---|---|
| `internal/github/enrich_commit.go` | Byte-wise quoted-pair decode before relation tokens, retaining raw validation | Escaped `ne\xt` means `next`; separator and malformed/ambiguous trust handling remain conservative |
| Three GitHub test files | Escaped letters/spaces, plain, literal backslash, actual non-next, duplicate and malformed controls | Both pagination consumers exercise shared semantics |
| `harness.py` | Reuse round5 HTTPS fixture, block/delay terminal page, inspect SQLite and PTY, count/order requests | Prove no premature complete cache, correct cold/warm membership, and listings before every event GET |
| Changelog and task notes | User-visible fix; distinguish historical baseline from verified implementation | Accurate delivery and verification documentation |

URL identity/query/page/capacity guards, source ownership, API/authentication,
cache schema and lifecycle remain unchanged. No broad parser replacement or migration.

## Workflow evidence

| Step | Evidence | Result |
|---:|---|---|
| 1 Understand | Loaded workflow-v3 and autonomous-task with explicit T-129 selector; fetched and verified clean source; `taskrail status/validate`; task, round5 fixture/results, parser/callers and pinned spec inspected | Confirmed shared parser root defect; valid terminal proof required in both consumers |
| 2 Strict TDD | Focused `go test ./internal/github -run 'Test(LinkRelationDelimiters\|CommitPaginationProof\|ExpansionQuotedPagination)$' -count=1`; [red-unit.txt](red-unit.txt) | Baseline returned commit `complete/1/1` instead of `complete/2/2`, organization `1/1/false` instead of `2/2/false`; minimal decoder made tests pass |
| 2 Binary red/green | Build baseline from `git archive HEAD` in disposable `/tmp` directory; run `python docs/testing/t129/harness.py`; [red-results.json](red-results.json), [results.json](results.json) | Baseline 48/56 passing, final 56/56 passing; exact paths independently specified |
| 3 Initial checks | GitHub/enrichment/cache tests and full `mise exec -- task check`; [initial-check.txt](initial-check.txt) | Pass; an initial command mistakenly named nonexistent `internal/app`, then corrected to actual `internal/enrichment` |
| 4 Simplify | Dedicated code-simplifier Task loaded skill; removed unnecessary warm-loop variant reassignment; focused tests and 56 binary assertions pass; second Task after byte-iterator/timing changes found no further simplification | No parser/cache abstraction added |
| 5 Review | Separate read-only General, Go, Security, Python Tasks loaded code-reviewer and lane guidance; fresh candidate-validation Task | General/Go/Security: “No concrete task-relevant findings.” Python F1 validated |
| 6 Disposition | Injected 0.8-second page-two latency reproduced fixed-sleep weakness: [timing-red-results.json](timing-red-results.json), 9 failed assertions; bounded wait then 56/56 pass | F1 fixed; late task-note documentation finding also fixed |
| 7 Final checks/review | Fresh disposition-verification Task, then second fresh candidate/disposition verification after historical-note correction; full gate, mutation, rebuilt binary T-129 and T-127 checks | Both findings resolved; final reviewer: “No concrete task-relevant findings.” Two disposition cycles, no global round |
| 8 Lifecycle | `taskrail verify ... --result pass`, inspect diff, validate; `taskrail complete ...`, inspect diff, validate; [lifecycle.txt](lifecycle.txt) | Verification artifact written, task completed, state valid |

Review lanes: General for contracts/tests; Go for decoder and adapters; Security
for untrusted Link/URL boundary; Python for the new regression harness. Three
specialists, within the soft budget. Database was considered and omitted because
no SQL, client, schema, migration or persistence implementation changed; exact
SQLite admission and reuse were exercised by the harness. No framework/UI lane
was triggered: rendering code and appearance did not change. Lane guidance loaded
included ECC code/go/security/python reviewers, golang/python patterns, and
common review/testing/security rules plus security-review companion.

## Review findings

| ID | Verbatim finding and evidence | Disposition | Evidence |
|---|---|---|---|
| F1 | “Replace the fixed post-release sleep with a bounded wait for page-two processing to finish before asserting the final cache and UI.” `harness.py`, originally lines 48–53 | Fixed | 0.8-second delay: 9 failures with fixed sleep; bounded five-second wait for exact cache and final UI: 56/56 pass |
| T129-DOC | “The task’s implementation notes still describe the pre-fix behavior and contradict its current status.” Task notes, originally lines 69, 73–75 | Fixed | Marked round5 as historical; documented current implementation and verification; final fresh candidate/disposition review resolved it |

No rejected or deferred candidates. The doc-only correction needed no behavior
test; Taskrail validation and independent factual review verified it. Task status
and execution state were changed only through Taskrail commands.

## Verification

| Command/check | Scope | Result |
|---|---|---|
| `mise exec -- go test ./internal/github -count=1` | All adapter tests, including T-124/T-126/T-127 restrictions | Pass |
| `mise exec -- go test ./internal/github ./internal/enrichment ./internal/cache -count=1` | Shared evidence/cache integration | Pass |
| `mise exec -- go build -o /tmp/orgtop-round2 ./cmd/orgtop`; `python docs/testing/t129/harness.py` | Real HTTPS, PTY, SQLite cold/warm, organization ordering | 56/56 assertions pass; [requests.json](requests.json), [cold.txt](cold.txt), [warm.txt](warm.txt), [organization.txt](organization.txt) |
| `python docs/testing/t127/harness.py` | Existing title delimiters, ambiguity, atomic cache and organization ordering | 15/15 assertions pass; [t127-regression.txt](t127-regression.txt) |
| `mise exec -- task check` | Complete CI-equivalent local gate | Exit 0; [check.txt](check.txt) |
| `GOMAXPROCS=2 GOMEMLIMIT=512MiB mise exec -- task test:mutate BASE=origin/main` | Standard five-mutator differential run | 1 killed, 0 lived, 0 uncovered; 100% efficacy and coverage; [mutation.txt](mutation.txt) |
| `git diff --check`; `mise exec -- taskrail validate` | Patch/state consistency | Pass |

Two earlier mutation runs of the mutable-index decoder terminated with exit 137;
their partial results were not treated as success. A package-path experiment
skipped every mutant and was also not treated as valid evidence. The final
decoder uses a byte iterator, preserving tested behavior without mutable index
advancement, and the standard root differential command completed normally.

## Remaining risks / follow-ups

| Item | Impact | Action |
|---|---|---|
| Existing false-complete disposable cache from a pre-fix binary | This source-only fix intentionally does not migrate or invalidate old entries | An affected user can run the existing `orgtop --reset-cache` before reacquiring evidence |
| Taskrail version-label warning | Installed skills report v0.4.0 while binary labels itself 0.0.0-dev; all lifecycle validations succeeded | No skill refresh or unrelated toolchain changes in this task |
| T-130 and release acceptance | Outside this task; no deploy/release performed | Parent orchestrator owns separate follow-up |
