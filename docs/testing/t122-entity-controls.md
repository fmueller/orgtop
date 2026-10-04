# T-122 entity presentation regression

Pinned contract: `specs/v0.2.0.md#rg-012-mixed-scope-presentation-contract`.
Source checkout was clean on `main`, equal to freshly fetched `origin/main` at
[`7084304`](https://github.com/fmueller/orgtop/commit/70843043539d57dd0d6cfa35addebca751508f8b).
Only T-122 was started; no other task was claimed.

## Outcome and change

Interesting Now now passes its entity text through the existing presentation
sanitizer before the width ladder measures it. Source normalization, immutable
IDs, membership, prepared selection/order and ordinary Unicode are unchanged.
The actual route is malformed `PushEvent.payload.head` → normalized `EntityRef`
→ Interesting Now; the branch-to-entity hypothesis remains rejected. Detail
already escaped the same entity correctly.

## Workflow-v3 evidence

| Step | Evidence | Result |
| --- | --- | --- |
| 1 Understand | Loaded workflow-v3 and autonomous-task with explicit T-122 selector; inspected task, pinned RG-012, normalization, strip/detail, round2 fixture/report/harness | Confirmed actual head route and one-line presentation boundary |
| 2 Strict TDD | `mise exec -- go test ./internal/tui -run '^TestInterestingEntityControlsFromSource$' -count=1` | Red: raw controls emitted and escaped-fit boundary violated; green after sanitizer call |
| 3 Initial checks | Focused test; `mise exec -- go test ./internal/tui ./internal/github ./internal/domain`; initial `mise exec -- task check`; real binary harness | Pass; baseline executable failed wide/80-column entity checks, fixed executable 7/7 |
| 4 Simplify | Dedicated Task loaded code-simplifier | Replaced Go quoting/manual JSON conversion with json.Marshal; focused/package tests and diff check passed |
| 5 Review | Separate parallel General, Security, Go Tasks loaded code-reviewer and routed reviewer guidance; fresh candidate-validation Task | Each returned “No concrete task-relevant findings.” Candidate IDs: none |
| 6 Disposition | No validated findings, no deferrals or fixes needed | No disposition rows invented |
| 7 Recheck | Full task check; differential mutation and explicit sanitizer-deletion probe; fresh disposition-verification Task | Deletion detected, restored focused test green; no unresolved findings |
| 8 Finalize | Taskrail verify pass then complete, validate after each writer | Finalized only after review and verification |

Lane routing: General used ECC code-reviewer (no companion); Security used ECC
security-reviewer plus security-review/reference and its cloud resource; Go used
ECC go-reviewer plus golang-patterns (also consulted TDD guidance). These three
cover correctness, the source-to-terminal trust boundary and Go implementation.
Python is a finite reproduction script reusing the established harness, not a
production runtime change; no Python specialist was added. No framework,
database, persistence, concurrency or other domain lane was triggered. The
specialist count stayed below the soft budget. Candidate validation and fresh
disposition verification were separate read-only Tasks.

## Test coverage and mutation evidence

The source-adapter-to-renderer table exercises all RG-012 bidi controls, NUL,
ESC/ANSI, newline, tab, DEL, representative C1 controls, positive-width CJK,
combining, ZWJ and variation-selector graphemes, and an ordinary hexadecimal
head. It checks the independently spelled full row at 240 columns and the exact
escaped fit boundary, one cell below it, then 40, 20 and 8 columns. The same
normalized event's detail must contain the independently specified escape.
Prepared state, source identity and ordering are checked after rendering.

`mise exec -- task test:mutate BASE=70843043539d57dd0d6cfa35addebca751508f8b`
completed: 0 killed, 0 lived, 0 not covered, 1798 skipped. Default mutators have
no applicable mutation for this added sanitizer call; this is not evidence of a
mutation score. A focused manual deletion probe temporarily removed the call:
the regression exited 1 with raw-control and fit-boundary failures. Restoring it
returned the focused test to green.

## Real binary reproduction and visual inspection

Build before editing production code, then build the fixed binary:

```sh
mise exec -- env CGO_ENABLED=0 go build -o /tmp/orgtop-t122-before ./cmd/orgtop
# Apply the production fix before building the second executable.
mise exec -- env CGO_ENABLED=0 go build -o /tmp/orgtop-t122-after ./cmd/orgtop
mise exec -- python docs/testing/t122-entity-controls.py /tmp/orgtop-t122-before before
mise exec -- python docs/testing/t122-entity-controls.py /tmp/orgtop-t122-after after
```

The first command must be run on the baseline, not the fixed checkout. The
finite script reuses round2's actual HTTPS CONNECT transport and tmux PTYs,
refreshes the retained hostile fixture timestamp, and makes `payload.ref`
ordinary so only the hostile head is under test. Synthetic credential, isolated
HOME/cache, disposable trusted certificate, loopback-only server, and a private
tmux socket keep this independent of real credentials and GitHub. It verifies
one actual `/repos/acme/api/events?per_page=100` GET and then stops the server,
tmux session and removes its disposable root. No application injection seam or
poll-floor shortcut is used.

| Executed assertion | Baseline | Fixed |
| --- | --- | --- |
| 240x32 entity escaped | Fail: raw U+202E | Pass: `commit bad\u{202E}HEAD` |
| 80x24 safe | Fail: raw U+202E | Pass |
| 40x12 and 20x6 safe/bounded | Pass | Pass |
| Detail entity parity | Pass | Pass |
| Bounded HTTPS request / responsive quit | Pass | Pass |

Raw captures, request log and results are generated under
`/tmp/orgtop-t122-{before,after}-evidence`. Review images rasterized the real
after-run captures with ImageMagick/DejaVu Sans Mono, following round2's method.
Wide Rain, 40x12 Rain and detail images were inspected with view_media: escaped
entity readable in wide/detail, constrained row retains category/repository/
scope/age without overlap. These are capture-text renders, not terminal font or
color capability proof; executed code-point checks establish control safety.

Final repository gate: `mise exec -- task check` (format, vet, lint, all Go tests,
policy/changelog/distribution tests, cross-build, startup smoke, licenses,
Taskrail validation/coverage, release config). Taskrail reports an existing
installed-skill version warning (v0.4.0 versus 0.0.0-dev); validation succeeds.
No skill refresh, release/tag/deployment or unrelated task transition was made.
