# T-120 boolean assignment verification

Date: 2026-10-04. Pinned contract: `specs/v0.2.0.md#rg-001-unified-cli-contract`.
Only T-120 was selected, explicitly by user approval; `taskrail next` was not
used. T-106 and T-118 remain unchanged. No release or tag was requested.

## Base and intent

`git fetch --quiet origin main` confirmed a clean local main and origin/main at
[`d9ffb18`](https://github.com/fmueller/orgtop/commit/d9ffb18cd7655882af6090afc07996d0d674ec36),
including T-119. Taskrail validation passed before starting T-120. The task and
`docs/testing/round1/{report.md,cli.json,invalid-reset.json}` were inspected:
the reported `--reset-cache --no-cache=false` invocation removed a populated
disposable database with exit 0 instead of usage exit 2.

The parser now identifies the first boolean assignment after informational
precedence, parses only its preceding input, and validates preceding selection
values before rejecting it. It consumes value-flag arguments and respects
positional input, unknown flags, and the flag terminator. The change neither
opens cache state nor resolves credentials. Valid bare controls and value-flag
assignments retain their behavior.

## Workflow evidence

| Step | Executed evidence | Outcome |
| --- | --- | --- |
| 1 Understand | Read T-120, RG-001, round-1 evidence, parser, shell, tests and cache API | Confirmed parser ownership and side-effect boundary |
| 2 Strict TDD | `mise exec -- go test ./internal/cli ./cmd/orgtop -run 'TestBoolean\|TestExecutableBoolean\|TestRejectedConfiguration' -count=1` before production change | Failed: accepted boolean assignments; executable reset removed seeded DB |
| 2 Diagnostic TDD | `mise exec -- go test ./internal/cli -run TestBooleanGrammar -count=1` | Failed: earlier malformed repo/path was incorrectly outranked; prefix parsing/validation fixed it |
| 2 Green | `mise exec -- go test ./internal/cli ./cmd/orgtop -count=1` | Passed |
| 3 Initial checks | `mise exec -- task check` | Caught forbidden host-clock read in new test; replaced with fixed Date and Store.WithClock |
| 3 Recheck | Focused CLI, executable and toolchain tests | Passed; next full check exposed missing system zip |
| 4 Simplify | Dedicated Task loaded personal code-simplifier; inspected actual diff, ran focused tests and diff check | No edits: no safe complexity reduction found |
| 5 Independent lanes | Separate parallel read-only Tasks loaded personal code-reviewer: General, Go, Security | Each: “No concrete task-relevant findings.” |
| 5 Candidate validation | Fresh read-only Task loaded code-reviewer in candidate-validation mode | Empty candidate set validated; no rejected IDs |
| 6 Disposition | No validated findings | No fixes or deferrals required |
| 7 Disposition verification | Fresh read-only Task loaded code-reviewer in disposition-verification mode; inspected updated diff including zip setup prerequisite and ran full task check | “No concrete task-relevant findings.” Full gate passed |
| 7 Mutation | `mise exec -- task test:mutate BASE=d9ffb18cd7655882af6090afc07996d0d674ec36` | 8 killed, 0 lived, 0 uncovered, 2 timed out; efficacy and coverage 100% |
| 7 Post-mutation focused check | `mise exec -- go test ./internal/cli ./cmd/orgtop -run 'TestBoolean\|TestInformational\|TestExecutableBoolean\|TestRejectedConfiguration' -v -count=1` | Passed, including all eight executable reset assignment cases |
| 7 Final gate | `mise exec -- task check` after mutation and audit documentation | Passed, including distribution fixtures and release config |
| 8 Lifecycle | `mise exec -- taskrail verify T-120-reject-boolean-assignments --result pass`, then `mise exec -- taskrail complete T-120-reject-boolean-assignments` | Verification pass at 20:41:50Z; completed at 20:42:01Z; diff inspected and state validated after each write |

General loaded the ECC general reviewer and common rules. Go loaded the Go
reviewer and golang-patterns companion. Security loaded the security reviewer,
security-review companion and common security rules. General covers conventions
and test semantics; Go covers parsing and language contracts; Security covers
input rejection, credential nonexecution and cache data-loss risk. Database was
omitted because no schema, query, transaction or persistence implementation was
changed; framework and other domain lanes do not apply. One review cycle, zero
concrete findings. The candidate validator could not independently rerun tools
using its plain PATH; the subsequent verifier used mise and passed the full gate.

## Verification scope and limitations

- Parser tables cover eight boolean names/aliases, both dash forms, true/false,
  repeated flags, reset combinations, value consumption, scan ordering,
  terminators, valid bare flags, and T-119 informational precedence.
- The shell's fake credential resolver and launch seam are never invoked for
  rejected true/false launch input. The real executable runs with disposable
  HOME/XDG_CACHE_HOME, a populated valid closed SQLite store, and checks exact
  unchanged database bytes plus usage exit 2 on all eight reset combinations.
  This is real process verification, not live GitHub or maintainer acceptance.
- Full `mise exec -- task check` passed after installing zip. The existing orb
  setup now includes zip; `bash -n .agents/setup` passed, and two setup runs
  completed successfully in 4s and 0s. README and Unreleased changelog updated.
- Mutation timeouts are the two increment/decrement loop mutants at config.go
  lines 168 and 192; they are timeouts, not killed verdicts. No mutant lived.
- Taskrail emits a non-fatal installed-skill version warning (v0.4.0 versus
  0.0.0-dev). State validation passes; no skill refresh or unrelated task change
  was performed. Final lifecycle and delivery evidence is recorded by Taskrail
  verification and the task commit. No task-specific follow-up is needed.

Taskrail's ignored local verification artifacts are under
`planning/artifacts/verify/T-120-reject-boolean-assignments/20261004T204150Z/`.
This tracked report preserves the workflow evidence independently of those
disposable files. After completion, state is idle with 117 completed tasks,
zero active or blocked tasks, and four todo tasks; the active spec is unchanged.
