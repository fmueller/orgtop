---
id: T-128-run-cli-regressions-on-windows
title: Make executable CLI regression tests runnable on Windows
status: todo
priority: high
spec_ref: specs/v0.2.0.md#nfr-006-verification-quality
dependencies: []
updated_at: "2026-10-04T23:48:54Z"
---

# T-128-run-cli-regressions-on-windows Make executable CLI regression tests runnable on Windows

## Description

The current main CI fails before executing the CLI assertions in
`TestInformationalExecutable` and `TestExecutableBooleanAssignmentsPreserveCache`
on Windows. Both tests build a temporary binary named `orgtop` without `.exe`
and pass that extensionless absolute path to `os/exec`. All 16 informational and
eight boolean-assignment cases report executable not found in %PATH%.

This is a current verification defect under NFR-006, not evidence that the
distributed executable fails. Baseline CI run 37244638668 at
62e4707d8f1e10fd8fb55f447be7de9499203c67 and prior run 37243414318 both show the
same two tests failing at process launch. Build and cross-compile jobs pass;
Windows Startup smoke is skipped after Test fails. Linux/macOS/ARM tests pass.

## Acceptance

- Build and invoke the temporary CLI using a platform-correct executable path
  so both regression suites execute all 24 assertions on Windows as on Linux.
- Preserve help/version stream and exit assertions and boolean-assignment usage
  exit 2 plus unchanged-cache assertions. Do not skip or weaken tests on Windows.
- Keep fixture isolation and credential/cache side-effect checks effective on
  Windows; account for its executable and cache-directory conventions.
- Obtain passing native Windows CI for these tests and the normal test/smoke
  lane. Cross-compilation alone is not runtime verification.

## Test Expectations

- Run the two named tests locally on Linux and natively on Windows, then the
  standard repository gate. Verify the existing invalid-argument matrix remains.

## Verification Notes

- Current remote evidence: https://github.com/fmueller/orgtop/actions/runs/37244638668/job/111560151785
- Prior remote evidence: https://github.com/fmueller/orgtop/actions/runs/37243414318
- `docs/testing/round4/windows-failures.txt` retains a filtered, non-secret
  current failure excerpt; `report.md` records the exact Linux diagnostic.
- The Linux orb did not execute Windows binaries. No workflow was triggered or
  rerun; remote evidence is from already-running baseline CI, not a simulated run.
- No distribution bug, product fix, or lifecycle completion is claimed.

## Implementation Notes

Start at `cmd/orgtop/informational_test.go:16` and `cmd/orgtop/main_test.go:157`.
The informational test also creates a POSIX `gh` shell script and sets
XDG_CACHE_HOME; preserve intended test coverage when making the fixture portable.
