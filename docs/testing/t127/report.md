# T-127 quoted Link pagination verification

## Task outcome

| Status | Outcome | Product impact |
| --- | --- | --- |
| Complete, verified | Shared Link splitting preserves quoted/escaped attributes and URI brackets | Valid commit and organization pagination succeeds; only terminal commit evidence is cached |

Scope: exactly `T-127-parse-quoted-link-parameters`. Clean `main` and fetched
`origin/main` both started at `f2b23973d1a81d197c28b8958463fcd38d10bc9b`.
Pinned normative spec: `specs/v0.2.0.md`, RG-003 and affected RG-010 invariants.
T-128 was not changed. No release, deployment, or unrelated task was performed.

## What changed

| Area | Change | Contract served |
| --- | --- | --- |
| `internal/github/enrich_commit.go` | Small shared delimiter scanner for both entry and parameter splitting; quoted pairs consume the next byte; controls and unclosed constructs fail conservatively | Legal unrelated quoted attributes cannot hide `next`; existing relation and URL validation retained |
| `internal/github/enrich_pagination_test.go`, `expand_test.go`, `link_test.go` | Quotes, commas, semicolons, escapes, order, relation lists, URI brackets, multiple entries/fields, ambiguous and malformed controls | Complete commit path collection and valid organization flow without weaker rejection |
| `docs/testing/t127/harness.py` | Reuses round4 HTTPS/PTY reproduction, gates page two, inspects actual SQLite state, then uses a warm second-page path scope | No partial cache admission; terminal paths reusable without enrichment HTTP; organization expansion before polling |
| `CHANGELOG.md` | Unreleased fix entry | Public behavior documentation |
| Taskrail state and verification artifacts | Supported start → verify pass → complete commands | Reviewed work only; no hand-edited status |

## Workflow evidence

All project commands ran through `mise exec --`. Hooks were installed with
`task hooks-install`; configured identity was `Felix Müller <felix@fmueller.io>`
and `scripts/check-author.sh` passed. The shallow repository was unshallowed
before differential mutation history was used. RFC 8288 section 3 and Appendix B
were read at https://www.rfc-editor.org/rfc/rfc8288.html. This is a delimiter
repair, not a generic RFC parser: existing conservative relation acceptance,
including rejection of unquoted offered `next`, is intentionally preserved.

| Step | Evidence | Result |
| --- | --- | --- |
| 1 Understand | `git fetch --quiet origin`; identical requested main refs; `taskrail status`, `validate`; task, spec, round4 harness and parser/callers read; `taskrail start T-127-parse-quoted-link-parameters`, diff inspected, validate | Ownership is the shared GitHub adapter relation parser; no cache/schema/domain or URL-validation change needed |
| 2 Strict TDD | `go test ./internal/github -run 'Test(LinkRelationDelimiters\|CommitPaginationProof)' -count=1` failed before implementation: quoted comma `incomplete/0/1`, wanted `complete/2/2`; escaped quote also failed; scanner added, same test passed | Red → green. Later baseline-split deliberate regression also proved organization tests fail; retained [red.txt](red.txt) |
| 3 Initial checks | `go test ./internal/github -count=1` passed; initial `task check` caught staticcheck QF1003 in quote handling; changed to tagged switch, `task lint` reported 0 issues; binary harness 15/15 | Fail identified and corrected, not suppressed |
| 4 Simplify | Dedicated Task loaded personal `code-simplifier`; inspected actual diff, task/spec, harness; no edits. Rejected table-field rewrite as unnecessary positional-table boilerplate; focused tests/lint/diff check passed | No unnecessary abstraction or architecture change |
| 5 Independent review | Four separate parallel read-only Tasks loaded `code-reviewer`: General, Go, Security, Python. Fresh candidate-validation Task validated sole T127-PY-1; no rejected or duplicate candidates | General/Go/Security: “No concrete task-relevant findings.” Python test-order gap validated |
| 6 Disposition | T127-PY-1 fixed by exact first-two listing page paths and exact remaining repository polls. Runtime trace deliberately reordered to page1/poll/page2/poll: [order-red.txt](order-red.txt) fails; actual trace passes. A supplemental actual binary production mutant polls before page two: [order-production-red.txt](order-production-red.txt) fails; production restored and final binary passes. Differential survivor prompted next-URI delimiter test: production bracket-close predicate deliberately negated, [mutation-red.txt](mutation-red.txt) fails; restored test passes | All findings fixed; none deferred. Also removed the impossible second scanner-error branch: entry splitting already proves balanced constructs and valid bytes |
| 7 Recheck and disposition verification | Final `task check` exit 0; rebuilt binary harness 15/15; differential mutation 9 killed/0 lived/0 timed out. Fresh read-only `code-reviewer` disposition-verification Task inspected final code, ignored second-scan validity flag, tests and evidence, reran focused tests/diff check | T127-PY-1 **RESOLVED**; “New task-relevant findings: None.” One review/fix/recheck cycle; no unresolved findings |
| 8 Finalize | `taskrail verify T-127-parse-quoted-link-parameters --result pass --summary ... --details ...`; diff inspected/validate; `taskrail complete T-127-parse-quoted-link-parameters --note ...`; diff inspected/validate/status | Verified 2026-10-05T00:52:53Z, completed 00:53:04Z; state valid, idle, 124 done/0 active/0 blocked/4 todo |

Lane selection: General mandatory; Go for production/tests; Security for
server-provided API pagination input; Python for the new binary test harness.
Three specialist lanes satisfy the soft budget. Database omitted because there
is no database implementation/schema change (cache assertions only); framework,
network-configuration, and other languages/domains omitted as unaffected.
General loaded ECC `code-reviewer.md`; Go loaded `go-reviewer.md` and
`golang-patterns/REFERENCE.md`; Security loaded `security-reviewer.md`,
`security-review/REFERENCE.md`, its cloud-infrastructure resource and common
security rules; Python loaded `python-reviewer.md` and
`python-patterns/REFERENCE.md`. Candidate validation loaded Python guidance;
disposition verification loaded General, Go, Python, and Security guidance.
Reviewers inspected the actual diff and surrounding contracts; not all reran
the binary harness or full gate, so those results were supplied as retained
executed evidence rather than claimed as independent reruns.

## Review findings

Sole validated candidate, verbatim:

> FINDING T127-PY-1 — tests
>
> Severity: low
>
> Evidence: docs/testing/t127/harness.py:81-84 checks that two organization-listing requests and both expected repository-poll requests occurred, but does not compare their positions in sent.
>
> Finding: The organization assertion does not prove that both listing pages were fetched before polling began.
>
> Failure/impact: A regression that starts polling after page one but still fetches page two could satisfy this assertion, so the harness would not prove the specified ordering. The retained run log is correctly ordered, but that ordering is not enforced by the check.
>
> Recommended direction: Assert that the second listing request precedes the first events request, and that the listing requests correspond to both expected pages.

| ID | Disposition | Evidence |
| --- | --- | --- |
| T127-PY-1 | Fixed; independently verified RESOLVED | Exact page identities and request order now asserted; deliberately wrong trace fails; real final trace passes |

## Verification

| Command/check | Result | Retained evidence |
| --- | --- | --- |
| `mise exec -- go test ./internal/github -count=1` | Pass (also independently rerun by reviewers) | [focused.txt](focused.txt) retains coverage-profile run; no live API |
| `mise exec -- task check` | Exit 0; format, vet, lint, all Go tests, hook/message/author guards, changelog/distribution/mutation-floor fixtures, build/startup, licenses, Taskrail/spec coverage and release-config checks | [check.txt](check.txt); release check only, no publication |
| `mise exec -- task test:mutate BASE=f2b23973d1a81d197c28b8958463fcd38d10bc9b` | Exit 0; killed 9, lived 0, not covered 3, timed out 0; efficacy 100%, mutator coverage 75% | [mutation.txt](mutation.txt) |
| `mise exec -- go build -o /tmp/orgtop-round2 ./cmd/orgtop` then `mise exec -- python3 docs/testing/t127/harness.py` | Exit 0; 15/15 assertions | [results.json](results.json), [requests.json](requests.json) |
| Cold binary HTTPS/PTY | Two exact-commit GETs; zero evidence/paths while terminal response blocked; afterward `complete=1`, `path_count=2`, exact paths `other/end.go`, `src/main.go`; confirmed `src` activity | [cold.txt](cold.txt), results/requests |
| Warm binary HTTPS/PTY | Confirmed `other` membership from page-two path; zero enrichment lookups | [warm-terminal-path.txt](warm-terminal-path.txt), results/requests |
| Ambiguous next binary control | One commit GET; `PATH ?1`; no evidence/paths cached | [ambiguous.txt](ambiguous.txt) |
| Organization quoted title | Exact listing pages 1 then 2 before either repository poll; both selected repositories current | [organization.txt](organization.txt), requests/results |
| `git diff --check`, `mise exec -- taskrail validate` | Passed after every lifecycle write; final state valid | [lifecycle.txt](lifecycle.txt), Taskrail artifact below |

Taskrail verification artifact:
`planning/artifacts/verify/T-127-parse-quoted-link-parameters/20261005T005253Z/report.json`.
An unchanged copy is retained as [verification.json](verification.json), since
`planning/artifacts/` is git-ignored. The lifecycle command details are retained
there; `lifecycle.txt` contains verify/complete output. The binary harness cleans its disposable cache,
certificates and PTYs. The terminal captures verify semantics, not a visual
appearance change; no UI styling was modified.

The supplemental production-order mutant was run after lifecycle finalization
as an extra test-sensitivity check: a temporary early events GET inside
`nextListingPage` made the binary organization assertion fail. It was removed
in full (`expand_pagination.go` has no final diff), then the correct binary was
rebuilt and all 15 assertions passed again. The previously reviewed and gated
final production source is unchanged by this supplemental experiment.

## Remaining risks / follow-ups

| Item | Impact | Action |
| --- | --- | --- |
| Gremlins reports three switch predicates not covered | Do not equate 100% efficacy with 100% mutation coverage. Diagnostic output identifies `<`, quote, and separator switch predicates; ordinary Go coverage marks all scanner branch bodies covered | Coverage-tool limitation retained; no surviving exercised mutant, no task blocker |
| Taskrail skill/version warning | Installed skills say v0.4.0 while repository PATH binary identifies as 0.0.0-dev; all state commands and validation succeeded | Do not refresh tools/skills as unrelated work in this task |
| T-128 and release-candidate work | Separate open tasks remain; no claim of v0.2.0 release readiness | Parent workflow selects subsequent work separately; no new follow-up task needed for T-127 |
