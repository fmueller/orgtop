# T-121 Stream bidi-control verification

Date: 2026-10-04. Pinned specification: `specs/v0.2.0.md`, RG-012.
Baseline: [0e582ef](https://github.com/fmueller/orgtop/commit/0e582ef225ad352f6e5732a1a5ef5161e4b5cfa3),
verified clean and equal to freshly fetched `origin/main` (includes T-119/T-120).
Only T-121 was explicitly started; T-106 and T-118 were not changed.

## Workflow evidence

| Step | Evidence | Result |
| --- | --- | --- |
| 1 Understand | Read T-121, RG-012, round1 hostile fixture, captures, bidi-check and report; trace normalization, Stream, detail, Interesting Now | TUI owns escaping; source identity remains unchanged |
| 2 Strict TDD | `mise exec -- go test ./cmd/orgtop -run TestHostileSummaryFlowEscapesBidiBeforeLayout -count=1` | Red: raw bidi and missing visible escapes in Stream and Rain; green after two existing-helper calls |
| 3 Initial checks | `mise exec -- go test ./cmd/orgtop ./internal/tui -count=1`; `mise exec -- task check` | Both passed; actual binary/HTTPS/PTY verification below |
| 4 Simplify | Dedicated Task loaded `code-simplifier`, inspected scoped diff, ran focused tests and diff check | No edits; no unnecessary complexity found |
| 5 Review | Separate parallel read-only General, Security and Go Tasks loaded `code-reviewer` and routed ECC guidance; Security loaded security-review and Go loaded golang-patterns | Each returned “No concrete task-relevant findings.” |
| 5 Candidate validation | Fresh read-only Task loaded candidate-validation mode | One new entity-reference candidate, rejected on direct source/capture evidence below |
| 6 Disposition | Candidate rejected as not reproducible for its claimed branch path; no task-relevant fixes/deferred findings | No scope expansion |
| 7 Recheck | Two independent sanitizer-removal experiments, restored focused green, final `mise exec -- task check`; fresh disposition-verification Task | Each removal failed the regression (exit 1); restored implementation and full gate passed; rejection confirmed, no unresolved findings |
| 8 Finalize | Taskrail verify/complete performed after review and final gate; maintainer identity and hooks before commit | Lifecycle/delivery recorded in Taskrail and git |

General covers correctness, acceptance, tests and ownership; Security covers the
source-to-terminal input boundary; Go covers language and test conventions.
Database, network-configuration and other framework lanes were omitted: no
persistence, transport, concurrency or web-framework code changed. No lane budget
extension was needed. The General reviewer could not invoke plain `taskrail`;
the owning thread used `mise exec -- taskrail` successfully throughout.

## Regression and mutation checks

The integration test reads the recorded hostile JSON through the real HTTP source
adapter and renderer. It independently enumerates all 12 RG-012 bidi controls,
asserts visible uppercase escapes in actor/branch and existing detail, preserves
combining text, wide text and an emoji ZWJ grapheme, checks Stream widths 1, 8, 40,
80, 120 and full output at 240 cells, and restores the identical wide view after
resize. Interesting Now checks the identical prepared actor text. No source or
domain values are rewritten.

`mise exec -- task test:mutate BASE=0e582ef225ad352f6e5732a1a5ef5161e4b5cfa3`
exited 0 but generated **zero applicable mutants** (1798 skipped); this is not a
claim of mutation coverage. To cover the relevant call-removal risk directly,
each of the Stream and Interesting Now sanitizer calls was removed separately.
Each removal made the regression fail with exit 1; both were restored and the
regression passed. Reviewer checks also passed focused Go vet and the GitHub
adapter tests. `git diff --check` passed.

## Candidate disposition, verbatim and verified

Candidate-validation supplied no ID and reported:

> Interesting Now can still render a source-provided bidi control raw in its entity reference.

Its claimed failure sequence was:

> With the hostile branch reference used by the test fixture, Rain’s Interesting Now entry includes that reference through entityText(). Its raw U+202E can reorder terminal output, and width measurement happens before any sanitization.

Rejected: `internal/github/normalize.go` classifies pushes with `payload.Head`
as `EntityRef`, not the branch. The fixture head is a clean hexadecimal SHA.
The branch enters `Description`, which Interesting Now stores but does not
render. The executable capture shows the clean commit SHA and escaped actor,
with no raw bidi. An unrelated hypothetical hostile entity reference is not
this branch fixture or an identical prepared summary-text consumer.

The fresh disposition reviewer concluded:

> Verdict: REJECTED — rejection confirmed.

> No concrete task-relevant findings.

## Actual rendered verification

Reuse round1's executable method: build the pure-Go binary, serve the hostile
events and commit fixtures via a bounded loopback HTTPS CONNECT proxy accepting
only `api.github.com:443`, with a disposable certificate, synthetic credential,
HOME and no cache. Only fixture time was refreshed for Interesting Now eligibility
and actor was changed to `tester` + U+202E + `actor` to exercise its shared consumer.
No live account, token, release, or deployment was used.

The first process raced proxy readiness and reported connection refused; it was
discarded. After a successful readiness request, a new process fetched events
and commit successfully and was exercised through Stream, detail, Escape, narrow
resize, and Rain. `git check-ref-format` accepted the hostile branch (exit 0).

| Actual PTY state | Assertions and inspected result |
| --- | --- |
| Stream 120x30 | Actor and branch show `\u{202E}`; R1/P2 membership; no raw bidi |
| Detail 120x30 | Same Actor/Description escapes and both memberships; no raw bidi |
| Stream 40x12 | Retains age/repository/push/R1/P2, optional detail `teste…`; no raw bidi |
| Interesting Now 240x30 | Escaped shared actor, clean head SHA, repository/sponsor and overlap; no raw bidi |

Actual `tmux capture-pane` text was byte-checked for absence of the full RG-012
bidi set, and for the visible escape where the width allowed it. Captures were
rasterized using ImageMagick/DejaVu Sans Mono as in round1 and inspected using
`view_media`; these are renderings of real captures, not generated UI concepts or
claims about terminal colors. Initial 16-bit PNG inspections reported blank for
three images; conversion to 8-bit RGB and fresh inspection confirmed all states.

The owning Amp thread retains directly reviewable `t121-stream-120.png`,
`t121-detail-120.png`, `t121-stream-40.png`, `t121-interesting-240.png` and their
actual text captures under `.amp/in/artifacts/`. These orb review artifacts are
not release assets or tracked source files.

No task-local blockers remain. This targeted Linux orb run is not T-106's
maintainer live-host/channel sign-off, and does not establish real-user terminal,
macOS or Windows visual acceptance. No tag or deployment was requested or run.
