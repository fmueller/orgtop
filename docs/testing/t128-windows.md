# T-128: portable executable CLI regressions

## Scope and baseline

Task: `T-128-run-cli-regressions-on-windows`, pinned to
`specs/v0.2.0.md#nfr-006-verification-quality`. The clean local `main` and
fetched `origin/main` both started at
`1c521d0d0dd24b394494831b9161f7a92a03acf6`.

[Round-four Windows failures](round4/windows-failures.txt) and
[its report](round4/report.md) identify 16 informational and eight boolean
assignment cases failing before launch. The fetched baseline's own
[Windows job](https://github.com/fmueller/orgtop/actions/runs/37249592302/job/111574525657)
confirmed the same 24 `executable file not found in %PATH%` errors:
Build passed, Test failed, Startup smoke was skipped. This is fixture evidence,
not a claim that the distributed executable is broken.

## Changes and invariants

Only CLI test fixtures changed. Native executable names use `.exe` on Windows;
Linux and macOS retain extensionless names. A compiled native `gh` probe replaces
the POSIX shell script. A positive control invokes it by `gh` through the isolated
PATH, checks its marker and failure exit, then removes the marker before the real
assertions. Both suites clear inherited `GH_TOKEN` and `GITHUB_TOKEN`, isolate
HOME, USERPROFILE, LOCALAPPDATA and XDG_CACHE_HOME, and seed the sentinel under
the actual `os.UserCacheDir` (including macOS's Library/Caches convention).
Inherited system environment remains available for Windows process startup.

All original 16 informational and eight boolean cases remain, with unchanged
help/version streams and exit requirements, usage exit 2 and byte-for-byte cache
preservation. The exact help prefix uses the invoked executable's basename,
matching `main`'s existing contract (`orgtop.exe` on Windows), rather than assuming
an extensionless name. The boolean cases additionally check the credential marker. No
Windows skips, CI configuration changes, production behavior changes, or weakened
assertions were introduced. No user-visible behavior changed, so no changelog
entry was needed.

## Workflow-v3 evidence

| Step | Executed evidence | Result |
|---|---|---|
| 1 Understand | Fetch/clean baseline verification; task/status/validate; active spec; both suites; round-four logs; baseline native job | Ownership and acceptance confirmed; only T-128 started |
| 2 Strict TDD | `mise exec -- go test ./cmd/orgtop -run 'TestExecutable(Path\|Isolation)$' -count=1` with old fixture behavior | Red: Windows path lacked `.exe`; inherited USERPROFILE/LOCALAPPDATA and both credentials survived |
| 2 Green | Same portable unit checks after the minimal helper changes; focused command below | Green: explicit Windows/Linux/macOS name cases and poisoned-environment isolation checks; all original 24 executable cases pass on Linux |
| 3 Initial checks | Focused verbose tests; `mise exec -- task check`; `git diff --check` | All passed |
| 4 Simplify | Dedicated Task loaded personal `code-simplifier`; inspected the three test files; reran focused tests | No safe simplification needed; no edits |
| 5 Independent review | Separate parallel read-only Tasks loaded personal `code-reviewer` for General, Go and Security; routed ECC guidance and Go/Security companion references | Each concluded: “No concrete task-relevant findings.” |
| 5 Candidate validation | Fresh read-only candidate-validation Task inspected actual changes and the three verbatim conclusions | 0 validated, 0 rejected, 0 deduplicated; “No concrete task-relevant findings.” |
| 6 Disposition | No concrete candidates or validated findings | No invented disposition rows; no fixes or deferrals |
| 7 Recheck/dispositions | Full `mise exec -- task check` exited 0; fresh read-only disposition-verification Task loaded personal `code-reviewer`, inspected changes and reran focused tests | “No concrete task-relevant findings.”; native Windows remained the explicit pending acceptance item |
| 8 Runtime delivery | Reviewed code-only pushes; passing native Windows test/smoke; final local gate; Taskrail verify then complete | Native acceptance passed before lifecycle finalization; see delivery sequence below |

General covers acceptance, regression preservation and fixture ordering. Go covers
`os/exec`, `t.Setenv` cleanup, executable naming and OS cache semantics. Security
covers inherited credential/cache isolation. Database review was omitted because
no schema, persistence implementation or cache API changed; framework/UI lanes
were omitted because no rendering or framework behavior changed. No review lane
budget extension was needed. The native CI follow-up used a second simplifier
Task (no edits), fresh General and Go review Tasks, and fresh candidate-validation
(0 validated/rejected/deduplicated). Each review again concluded:
“No concrete task-relevant findings.” Security was not repeated for the one-line
basename expectation because no isolation or credential code changed. The second
full `mise exec -- task check` also exited 0; a fresh disposition-verification
Task reran focused tests and concluded: “No concrete task-relevant findings.”
No review findings were deferred. These two review/check cycles account for the
native runtime feedback; no unresolved review finding was carried forward.

Focused Linux command (also executed by the simplifier and reviewers):

```sh
mise exec -- go test ./cmd/orgtop \
  -run 'TestExecutable(Path|Isolation)$|TestInformationalExecutable|TestExecutableBooleanAssignmentsPreserveCache' \
  -count=1 -v
```

It passed all 24 original subtests plus the portable fixture checks. This is not
Windows runtime evidence. The Security reviewer could not independently rerun Go
with its bare PATH; the implementing checkout and other reviewers used `mise exec`
successfully. The Taskrail binary emits an installed-skill version warning, but
`mise exec -- taskrail validate` passes; no unrelated skill refresh was performed.

## Delivery sequence and native runtime gate

1. Install hooks and check maintainer identity (Felix Müller); perform all local
   review/check gates with Taskrail still `in_progress` locally.
2. Push only the three reviewed Go test files in
   [11918cc](https://github.com/fmueller/orgtop/commit/11918cc5fe0db518dd67202d465be269514db185).
   This necessary provisional delivery allows normal push-triggered runtime CI.
3. Observe
   [automatic CI run 37250873505](https://github.com/fmueller/orgtop/actions/runs/37250873505)
   with `gh run watch --exit-status` and `gh run view`. No manual rerun,
   workflow dispatch, tag, release or deployment is authorized or performed.
   Checks, cross-compile, Linux amd64/arm64 and macOS test/smoke passed.
   [Windows job](https://github.com/fmueller/orgtop/actions/runs/37250873505/job/111578237018)
   successfully launched the binaries and probe but failed eight help cases:
   actual `Usage: orgtop.exe ...` did not match hardcoded `Usage: orgtop `.
   `gh run watch --exit-status` exited 1. Startup smoke was skipped.
4. Treat that native failure as the follow-up TDD red evidence, verify `main`
   passes `filepath.Base(os.Args[0])` to usage, and adjust only the exact expected
   prefix to the invoked binary's basename. Focused Linux tests returned green.
   Repeat simplification, independent review, candidate/disposition verification
   and full local checks before another code-only push. No Taskrail verify or
   complete operation ran while native acceptance was failing.
5. Push the reviewed one-line correction in
   [603e6c7](https://github.com/fmueller/orgtop/commit/603e6c707a51d8eea58ef6f87ec7845e2e3f4a1c).
   Observe the resulting
   [automatic CI run 37251540577](https://github.com/fmueller/orgtop/actions/runs/37251540577)
   with `gh run watch --exit-status` (exit 0). Its event is `push` and its head SHA
   is exactly the reviewed correction above. All six jobs passed, including
   [checks](https://github.com/fmueller/orgtop/actions/runs/37251540577/job/111580014803)
   and the four native platform test/smoke lanes.
6. Inspect the completed
   [Windows job](https://github.com/fmueller/orgtop/actions/runs/37251540577/job/111580217836)
   and its log: Build, Test and Startup smoke all passed. Test ran
   `task test` → `go test ./...` and reported
   `ok github.com/fmueller/orgtop/cmd/orgtop 10.962s`; the unchanged unconditional
   24-case matrices and fixture checks are part of that package. Startup smoke
   ran `task run:smoke` → `go run ./cmd/orgtop --help` and printed
   `Usage: orgtop.exe ...`. This is native runtime evidence, not a cross-build.
7. Run final `mise exec -- task check` after native CI: exit 0, including the Go
   suite, formatting/vet/lint, license, policy/distribution guards, build,
   startup smoke, Taskrail validation and release configuration validation.
8. At 2026-10-05T01:35:12Z, run `mise exec -- taskrail verify
   T-128-run-cli-regressions-on-windows --result pass` with the native CI links,
   review and test details, then `mise exec -- taskrail complete
   T-128-run-cli-regressions-on-windows --note ...`. Inspect each state diff and
   validate after each writer (`state valid`). T-128 is completed; active spec
   remains v0.2.0, no current task, 125 completed, three todo, zero blocked.
   The tool-generated local verification report is
   `planning/artifacts/verify/T-128-run-cli-regressions-on-windows/20261005T013512Z/report.json`;
   it stays gitignored under the repository's artifact policy. This audit and the
   tool-generated task/state transitions form the final evidence-only commit;
   no Go code changes after the successful native run.
9. Push the lifecycle/evidence commit
   [4a08a0d](https://github.com/fmueller/orgtop/commit/4a08a0dad2012d002cfab88ed1fa29d812d8160c).
   Its automatic
   [Planning checks](https://github.com/fmueller/orgtop/actions/runs/37252091564)
   passed Taskrail validation and spec coverage (watch exit 0). The full native
   CI correctly did not repeat for a docs/planning-only push. Local `task check`
   builds the host executable; cross-compilation passed in the native-evidence
   CI's separate cross-compile job, not inside that local command.

## Limits

This Linux orb cannot execute Windows binaries. Native Windows results must come
from the ordinary CI matrix, not cross-compilation. No packaged installation,
distribution publication, live GitHub credential or release-readiness claim is
made. Other tracked tasks remain untouched.
