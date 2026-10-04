# T-124 — validated commit pagination

Date: 2026-10-04 UTC. Pinned specification: `specs/v0.2.0.md`, RG-003.
Clean main matched freshly fetched `origin/main` at
[`5f2a46f`](https://github.com/fmueller/orgtop/commit/5f2a46f59fab614eea7e14a0a810652736b912cf).
Only T-124 selected; T-125 and release acceptance remain separately owned.

## Task outcome

| Status | Outcome | Product impact |
| --- | --- | --- |
| Verified/completed | Invalid offered next links and nonadvancing chains are incomplete | Current repository activity, quantitative path unknowns, no new persistent partial proof |

## Changes

| Area | Change | Why |
| --- | --- | --- |
| `internal/github/enrich_commit.go` | Track current page; parse the complete query; require singleton page/per-page and strict advancement; distinguish invalid offered relation from absent relation | RG-003 completeness proof |
| `internal/github/enrich_pagination_test.go` | Asymmetric pages/paths/query values, malformed relation syntax, repeated/aliased URLs, valid terminal and increasing controls | Reject plausible false-completeness implementations |
| `docs/testing/t124-pagination.py` | Reuse round3 vectors and round2 HTTPS/PTY transport; no-cache and cold/repeated/corrected/reused SQLite flows | Execute real binary behavior and persistence contract |
| Changelog | Document unknown rather than partial path membership | User-visible correction |

URL scheme/host/userinfo/fragment, canonical repository, exact SHA, sole-parent
identity, visited-URL, 10-page/1,000-path and request checks remain. Increasing
chains may skip a page number; final pages without next, complete-empty and
T-123 required-array behavior remain intact. No schema, cache-admission, domain,
rendering, API, or Retry-After scheduling changes. Invalid outcomes discard all
paths; existing cache admission accepts only complete immutable evidence.

## Workflow evidence

| Step | Evidence | Result |
| --- | --- | --- |
| 1 Understand | Loaded workflow-v3 and autonomous-task explicit T-124; fetched baseline; task/status/validate; pinned spec and round3 malformed/reverse/duplicate/cache report and scripts; inspected producing commit | Confirmed adapter ownership and false-completeness path |
| 2 Strict TDD | `mise exec -- go test ./internal/github -run TestCommitPagination -count=1` before production edit | Malformed relation, duplicate query, parse errors and reverse progression failed; [baseline red unit output](red-unit.txt) also rerun in detached baseline checkout |
| 2 Binary red | Baseline pure-Go executable through finite HTTPS/PTY/cache script | 168 checks, 30 failures; [output](red-binary.txt), including false complete persistence and cache reuse |
| 2 Green | Minimal adapter fix, same tests and targeted GitHub/enrichment/cache suite | [Green unit output](green-unit.txt) |
| 3 Initial checks | `mise exec -- go test ./internal/github ./internal/enrichment ./internal/cache`; initial `task check` | Passed |
| 4 Simplify | Dedicated Task loaded personal code-simplifier | Proposed removing visited-URL map; rejected because user explicitly required retaining it. Restored guard, reread diff and reran adapter suite |
| 5 Independent review | Separate read-only General, Security, Go and Database Tasks loaded code-reviewer and lane mappings | General/Go/Database candidates below; Security: “No concrete task-relevant findings.” |
| 5 Lane coverage | General ECC code-reviewer; Security ECC security-reviewer + security-review/common-security; Go ECC go-reviewer + golang-patterns; Database ECC database-reviewer + postgres-patterns/database-migrations | General plus three specialists: URL/input trust, Go parsing, explicit persistence-exclusion acceptance |
| 5 Candidate validation | Fresh read-only code-reviewer Tasks checked original candidates and second-cycle residual/Database candidate | Four validated, none rejected, none deduplicated |
| 6 Disposition | Strict red/green for every finding; extra malformed syntax regressions strengthened | All fixed, none deferred; [first review red](review-red.txt), [second-cycle red](cycle2-red.txt), [Database red](database-red.txt) |
| 7 Mutation strengthening | First differential run killed 9/lived 1 at `<=`; added same-page/different-query-order URL test; deliberately changed `<=` to `<`, then restored | [Deliberate regression](mutation-red.txt): complete/3 paths/3 requests instead of incomplete/0/2; restored test passes |
| 7 Final checks | Full gate, real binary matrix, differential mutation after final fixes | All pass; details below |
| 7 Disposition verification | Fresh code-reviewer after cycle one found relation-list residual; final fresh verifier after cycle two checked all four dispositions | “All four candidates are RESOLVED; none is validly deferred or unresolved.” “New task-relevant findings: None.” Two cycles total |
| 8 Lifecycle | Taskrail verify pass then complete; inspect diff and validate after each write | Finalized only after independent review and final checks |

Other language/framework/domain lanes omitted: no corresponding affected code or
behavior. Network Configuration omitted: transport configuration is unchanged;
the harness reuses existing synthetic loopback HTTPS transport. Database added
because persistence exclusion is explicit acceptance despite no SQL/schema edit.
The default three-specialist budget was not exceeded. No new threads created.

## Review findings

| ID | Verbatim finding and evidence | Disposition | Evidence |
| --- | --- | --- | --- |
| General F1 | “Reject malformed `rel` quoting instead of following the offered pagination URL and treating its result as complete.” Original `enrich_commit.go:124,128-130` stripped all quotes and accepted `rel="next""` | Fixed | Exactly two surrounding double quotes; extra-quote regression red complete/2/2 then green incomplete/0/1 |
| Go G-1 | “A malformed `Link` header that still signals `rel="next"` but omits the semicolon, such as `<target> rel="next"`, is treated as if no next relation was offered.” Original `enrich_commit.go:116-120` searched only semicolon parameters | Fixed | Inspect malformed suffix after closing bracket; test red complete/1/1 then green incomplete/0/1 |
| Residual R1 | “linkRelation can still treat malformed metadata that signals `next` as no next relation.” Original cycle-one `enrich_commit.go:118,142`, `<target> rel="prev next"` | Fixed | Same suffix/token detection handles relation lists; valid separated list is a positive control; malformed list red complete/1/1 then green incomplete/0/1 |
| Database F001 | “A malformed `rel` parameter can be treated as no next link, allowing a partial commit-file page to be returned as complete.” Original `enrich_commit.go:124-126`, `<target> rel "next"` missing equals | Fixed | Detect malformed relation key fields; red complete/1/1 then green incomplete/0/1; binary confirms no cache record |

Candidate validation confirmed each against actual code and pinned acceptance.
Trailing relation junk and single-quoted relation tests also failed before their
minimal fix and now pass. No finding is deferred or silently omitted.

## Verification

```sh
mise exec -- env CGO_ENABLED=0 go build -o /tmp/orgtop-round2 ./cmd/orgtop
mise exec -- python docs/testing/t124-pagination.py
mise exec -- go test ./internal/github ./internal/enrichment ./internal/cache
mise exec -- task check
mise exec -- task test:mutate BASE=5f2a46f59fab614eea7e14a0a810652736b912cf
```

| Check | Decisive result |
| --- | --- |
| Final binary | [Results](results.json): **310/310 pass**, [process output](green-binary.txt), [GET log](requests.json) with synthetic credential-match booleans only |
| Malformed/cyclic/backward/ambiguous/relation metadata | No-cache and cold/repeated-cache processes keep repository current and path unknown; no complete evidence rows; exact bounded commit GET counts |
| Corrected/reused evidence | Same immutable identity reacquired after invalid cold/repeat responses; corrected membership; subsequent process dispatches zero commit GETs |
| Positive controls | Valid two-page persists two paths; complete-empty persists zero; both reuse without enrichment requests |
| T-123 control | Valid first page plus missing later files stays unknown and uncached; correction reacquired |
| Representative captures | [Malformed cold](link-malformed-cold.txt), [repeat](link-malformed-repeat.txt), [corrected](link-malformed-corrected.txt), [reverse](link-reverse-cold.txt), [missing equals](link-no-equals-cold.txt) |
| Full local gate | Final `mise exec -- task check` exit 0; [log](check.txt): formatting, vet, lint, tests, policy/distribution fixtures, cross-build, startup, licenses, Taskrail and release configuration |
| Differential mutation | [Log](mutation.txt): **10 killed, 0 lived, 0 uncovered, 0 timed out; efficacy/coverage 100%**, 1796 skipped outside differential scope |
| Independent final check | Fresh disposition verifier independently reran GitHub/enrichment/cache tests and diff check: pass |

Harness uses only synthetic event/file responses, disposable certificate/token,
isolated HOME/cache and private tmux PTYs; it stops the finite server/socket and
deletes disposable roots in `finally`. No live API requests, production writes,
release or deployment. TUI appearance is unchanged; actual PTY text checks cover
the changed membership semantics. Hooks installed; maintainer identity confirmed
as Felix Müller / felix@fmueller.io before commit.

## Remaining risks / follow-ups

Linux amd64 synthetic fixtures only, not release readiness. Existing false
complete records acquired by older binaries are not retrospectively purged:
unchanged TTL/reset policy applies. T-125 remains separately owned and untouched;
no scheduling fix or follow-up task created. Taskrail emits a nonfatal installed-
skills/dev-version warning; validation passes and no unrelated skill refresh was
performed. IO Delegate doctor was not provider-ready; reads remained bounded
native inspection. No task-local blocker or unresolved review finding remains.
