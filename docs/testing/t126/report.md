# T-126 — strict organization pagination

## Task outcome

| Status | Outcome | Product impact |
| --- | --- | --- |
| Complete | Reject query parse errors and repeated keys before following a listing next URL | Invalid expansion is ERROR, with no exact or expanded repository Events polling/publication |

Source was clean main equal to freshly fetched origin/main at
[`599837c`](https://github.com/fmueller/orgtop/commit/599837c8177068db065716cc133b00292bfde904).
Pinned normative spec: `specs/v0.2.0.md`, RG-010. Only T-126 was started;
T-127, T-128 and release acceptance T-106 remain separate.

## What changed

| Area | Change | Reason |
| --- | --- | --- |
| `internal/github/expand_pagination.go` | Parse RawQuery with error handling; require one value per expected key | Exact five-key listing query and N+1 contract, including malformed discarded fields |
| `internal/github/expand_test.go` | Asymmetric duplicates for page, per_page, type, sort and direction; invalid escape and semicolon | Detect first-value-only validation and discarded parse errors |
| `docs/testing/t126-pagination.py` | Reuse round2 real HTTPS CONNECT/PTY fixture, nine scenarios | Observe executable atomic failure and valid listing/poll sequence |
| `CHANGELOG.md` | User-visible organization pagination fix | Explain the changed failure behavior |
| Taskrail state/artifact | Supported start, verify pass, complete | Tracked lifecycle, no hand-edited status |

## Workflow evidence

| Step | Evidence | Result |
| --- | --- | --- |
| 1 Understand | Read task, pinned listing/query/atomicity contract, round4 report and actual malformed GET/request evidence; adapter and selection callers | Confirmed Query() drops errors and Get() only checks first values |
| 2 Strict TDD | `mise exec -- go test ./internal/github -run TestExpansionRejectsInvalidPagination -count=1`; `red-unit.txt` | Six baseline failures: each duplicate key and bad escape dispatched page two and produced unexpected 501 rather than ErrInvalidPagination |
| 2 Binary red | Build baseline and `mise exec -- python docs/testing/t126-pagination.py`; `red-results.json`, `red-requests.json` | 12/28 failed assertions; six bad inputs followed invalid URLs and polled/published selections; valid/extra-query/semicolon controls passed |
| 2 Green | Same binary harness after minimal parse/cardinality fix; `results.json`, `requests.json`, `green-unit.txt` | 28/28 binary assertions; invalid variants only initial listing/no Events; valid two listings/three ordered Events polls |
| 3 Initial checks | Simplifier reran `mise exec -- go test ./internal/github ./internal/tui ./internal/cli -count=1` and binary harness | Pass. Initial parent command mistakenly named nonexistent `internal/app`; corrected to actual tui/cli ownership, not hidden as a product failure |
| 4 Simplify | Dedicated Task loaded `code-simplifier`, inspected diff and contract | No edits warranted; existing minimal flow preserved; focused tests/binary passed |
| 5 Independent review | Parallel dedicated General, Go, Security Tasks loaded `code-reviewer` and indexed ECC guidance | Each: “No concrete task-relevant findings.” |
| 5 Candidate validation | Fresh read-only Task loaded `code-reviewer` candidate-validation mode; all three conclusions supplied verbatim | Zero supplied, validated or rejected candidate IDs; independently reran all Go tests/vet and binary fixture through mise |
| 6 Disposition | No validated findings | No fixes or deferrals needed; no invented findings |
| 7 Recheck | Full gate, differential mutation, real regression fixtures below | All pass |
| 7 Disposition verification | Fresh read-only Task loaded `code-reviewer` disposition-verification mode, inspected actual diff and recorded gate/binary evidence | “No concrete task-relevant findings.” Empty dispositions verified; one review cycle, no unresolved issues |
| 8 Finalize | `taskrail verify T-126-reject-ambiguous-organization-pagination --result pass --summary ...`, then `taskrail complete ... --note ...`; validate and diff inspection after each writer | Pass; completed only after independent review, disposition verification and final checks |

Review selection: General always required, Go for Go changes, Security for URL
input/trust-boundary checks. Two specialist lanes, within the default budget.
Database was omitted because persistence/schema behavior did not change; cache
regressions were nevertheless executed. No web/UI framework, networking config,
ML/RAG or healthcare behavior changed. General loaded ECC code-reviewer; Go
loaded ECC go-reviewer and golang-patterns; Security loaded ECC security-reviewer
and security-review. Go/Security initially attempted bare Go (unavailable outside
mise) and honestly reported that limitation; General/candidate validation and
parent/simplifier independently used the pinned mise tools successfully.

## Verification

All commands ran from repository root via `mise exec --`.

| Command/check | Scope | Result |
| --- | --- | --- |
| `env CGO_ENABLED=0 go build -o /tmp/orgtop-round2 ./cmd/orgtop` | Real standalone binary | Pass before each parent red/green run |
| `python docs/testing/t126-pagination.py` | Nine HTTPS/PTY cases, synthetic credentials, disposable TLS/HOME, API headers, quit | 28/28; inspected ERROR text (`duplicate-page.txt`) and current three-repo text (`valid.txt`) directly |
| `python docs/testing/round2/harness.py` | Atomic malformed/partial organizations, fair five-page bound, exact precedence, retained stale source failure, automatic source and enrichment recovery | 70/70 (`regression-results.json`); real elapsed-time refreshes, no clock/model injection |
| `python docs/testing/t124-pagination.py` | Valid and malformed commit pagination, cache admission and corrected/reused process flows | 310/310 (`commit-results.json`) |
| `go test ./...`, `go vet ./...` | All Go packages | Pass in candidate validation and full gate |
| `task check` | CI-equivalent local gate | Exit 0; `check.txt` |
| `task test:mutate BASE=599837c8177068db065716cc133b00292bfde904` | Default differential mutators | 4 killed, 0 lived, 0 uncovered, 100% efficacy and coverage; `mutation.txt` |
| `taskrail validate`, `git diff --check` | Planning integrity and patch hygiene | Pass after lifecycle writes |
| `task hooks-install`, `bash scripts/check-author.sh` | Commit hooks/maintainer author | Pass; maintainer identity configured; no attribution trailers |

`red-results.json`/`red-requests.json` preserve actual baseline dispatch, not
merely the fixture's first-value interpretation. Green logs demonstrate no
malformed next URL is dispatched. Servers and private tmux sockets stopped in
finally blocks and disposable roots were removed. Evidence contains no real
credentials or live API writes. No UI appearance change was made.

## Review findings and follow-ups

No concrete findings from General, Go, Security, candidate validation or final
disposition verification. Their common conclusion was quoted above. No T-126
blocker or new follow-up task remains.

Taskrail emits an installed-skill/version warning (v0.4.0 skills versus
0.0.0-dev binary); all validation/lifecycle commands succeeded. Refreshing that
tooling is outside T-126. Existing T-127 quoted-comma Link and T-128 Windows
fixture issues remain independently tracked, not fixed or claimed passing here.
No tag, deployment, release workflow or live-channel validation was performed.
