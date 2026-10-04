# T-123 — required changed-file arrays

Date: 2026-10-04 UTC. Pinned specification: `specs/v0.2.0.md`, RG-003.
Clean source matched freshly fetched `origin/main` at
[`93a4b45`](https://github.com/fmueller/orgtop/commit/93a4b455d4df35b5656efb4e0280668fd3249f53).
Only T-123 selected; T-124/T-125 remain separately owned and unchanged.

## Task outcome

| Status | Outcome | Product impact |
| --- | --- | --- |
| Verified/completed | Missing/null/wrong-type files are incomplete | Current repository activity, quantitative path unknowns, no newly cached false evidence |

## Changes

| Area | Change | Reason |
| --- | --- | --- |
| GitHub `pathSet.addRecords` | Reject nil decoded files before collection | Shared commit/compare transport completeness boundary |
| Adapter regressions | Seven shapes × first commit page/later page/compare | Reject absent/null/object/string/number/boolean; accept explicit empty array |
| `t123-files-array.py` | Actual executable HTTPS CONNECT/private tmux PTY/SQLite cold/warm regression | Unknown presentation, persistence exclusion and corrected-response reacquisition |
| Changelog | User-visible unknown/cache correction | Document observable behavior |

No identity, sole-parent applicability, pagination, bounds, schema, cache admission,
or rendering changes. Typed JSON decoding already rejects wrong field types;
absent/null decode to nil while explicit `[]` decodes to a non-nil empty slice.
Existing callers return an incomplete outcome without any previously collected
paths after rejection. Existing cache admission stores only complete outcomes.

## Workflow evidence

| Step | Evidence | Result |
| --- | --- | --- |
| 1 Understand | Loaded workflow-v3 and autonomous-task explicit T-123; status/validate; task, RG-003/RG-005, round3 report/scripts/results; fixture-producing commits inspected | Eligible task, pinned spec, observed adapter cause |
| 2 Strict TDD | `mise exec -- go test ./internal/github -run TestEnrichmentRequiresFilesArray -count=1` before production edit | Six absent/null cases fail: complete empty or partial instead of incomplete; [red unit output](red-unit.txt) |
| 2 Strict TDD | Same tests after minimal nil guard; `mise exec -- go test ./internal/github -count=1` | All 21 cases and adapter suite pass |
| 2 Binary red/green | Rebuilt executable; same finite cold/warm harness before/after fix | Baseline 64 assertions, 20 fail; final 64/64 pass |
| 3 Initial gate | `mise exec -- task check` | Exit 0 |
| 4 Simplify | Dedicated Task loaded personal code-simplifier, inspected actual diff/new files | No edits recommended; adapter tests and diff check pass |
| 5 Independent review | Separate parallel read-only General, Security, Go, Database Tasks loaded personal code-reviewer and index mappings | All: “No concrete task-relevant findings.” |
| 5 Lane coverage | General ECC code-reviewer; Security ECC security-reviewer + security-review; Go ECC go-reviewer + golang-patterns; Database ECC database-reviewer + postgres-patterns/database-migrations | General plus three specialists: input trust, Go JSON semantics, persistent evidence integrity |
| 5 Candidate validation | Fresh read-only code-reviewer candidate-validation Task checked all lane reports/code/evidence | No candidates to reject/deduplicate; no validated findings |
| 6 Disposition | No concrete findings | No fixes or deferrals |
| 7 Final verification | Fresh code-reviewer disposition-verification Task plus final task check and differential mutation | No unresolved/new task-relevant issues; one cycle |
| 8 Lifecycle | Taskrail verify pass, then complete; inspect diff and validate after each transition | State valid, T-123 done |

Other language/framework/domain lanes omitted: no corresponding changed behavior
or file types. Network Configuration omitted: production networking is unchanged;
the synthetic fixture only reuses committed transport. Database lane was retained
despite no schema edit because persistence exclusion is an explicit criterion.
Security review caught an assertion-count reporting error: the actual count is
64, not the initially stated 72. Candidate validation independently confirmed 64.

## Reproduction and checks

```sh
mise exec -- env CGO_ENABLED=0 go build -o /tmp/orgtop-round2 ./cmd/orgtop
mise exec -- python docs/testing/t123-files-array.py
mise exec -- go test ./internal/github -count=1
mise exec -- task check
mise exec -- task test:mutate BASE=93a4b455d4df35b5656efb4e0280668fd3249f53
```

| Check | Decisive result |
| --- | --- |
| Baseline binary | [Red output](red-binary.txt): 20 failures, including persisted commit/compare complete-zero records and complete-partial later-page record; warm lookup count 0 |
| Corrected binary | [Results](results.json): 64/64 pass; [GET log](requests.json) with synthetic credential-match booleans only |
| Cold malformed | [Missing capture](missing-cold.txt), [later-page capture](paged-missing-cold.txt): current repository, `PATH ?1`, `1 unknown`; SQLite evidence query returns no rows |
| Warm corrected | [Missing warm capture](missing-warm.txt): path membership recovered; exactly one new immutable lookup per malformed case |
| Controls | Explicit-empty commit/compare persist zero-path complete records and skip warm enrichment; valid paging persists two paths and skips warm enrichment |
| Full gate | Initial and final `task check` exit 0: formatting, vet, lint, tests, policy/distribution fixtures, cross-build, startup, licenses, Taskrail, release config |
| Differential mutation | Killed 1, lived 0, uncovered 0, timed out 0; efficacy/coverage 100%; skipped 1798 outside diff |

Harness setup reuses the recorded round3 response vectors and round2 loopback-only
HTTPS CONNECT fixture, disposable certificate, synthetic token, isolated HOME/cache,
and actual pure-Go binary in private tmux PTYs. It deletes disposable cache and
certificate roots and stops server/socket in `finally`. No live API requests,
release, deployment, or production data. Exit nonzero if any assertion fails.
TUI layout/styling is unchanged; executed PTY text checks cover changed semantics.

## Limits / follow-ups

Linux amd64 synthetic fixture only, not release readiness. No retrospective purge
of false evidence cached by older binaries: existing records remain subject to the
unchanged freshness/reset contract. T-124/T-125 remain open; this task neither
changes malformed pagination handling nor refresh Retry-After scheduling. The
installed-skill/dev-version Taskrail warning is nonfatal and left unchanged.
