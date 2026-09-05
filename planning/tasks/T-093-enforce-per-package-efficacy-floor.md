---
id: T-093-enforce-per-package-efficacy-floor
title: Enforce the mutation efficacy floor per package
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#nfr-006-verification-quality
dependencies:
    - T-092-floor-mutation-efficacy-repo-wide
updated_at: "2026-09-05T12:18:21Z"
---

# T-093-enforce-per-package-efficacy-floor Enforce the mutation efficacy floor per package

## Description

T-092 raised every package above the 90% mutation efficacy floor NFR-006
expects and raised the weekly gate's `--threshold-efficacy` from 70 to 90. That
threshold is enforced against the **repository total**, not against each
package: gremlins offers no per-package threshold.

So the floor is currently held by measurement, not by the gate. At the moment
the repository sits at 95.11% overall while its thinnest package,
`internal/github`, sits at 91.8%. A single package could regress to roughly
60% before the aggregate crossed 90 and the gate reddened, which is exactly the
blind spot the widened gate was meant to remove.

The run already emits everything needed. `task test:mutate:gate` writes a
`mutation-results.json` whose `files[].mutations[].status` values group by the
file's directory into per-package killed, lived, and timed-out counts. The
grouping T-091 and T-092 used to report their tables is a dozen lines.

| Package | Efficacy at 4689cb4 |
|---|---:|
| internal/auth | 100.0% |
| internal/cache | 97.1% |
| internal/cli | 99.0% |
| internal/domain | 92.6% |
| internal/enrichment | 100.0% |
| internal/github | 91.8% |
| internal/tui | 95.4% |

A related measurement hazard belongs with this work. gremlins walks the module
directory tree, so a nested git worktree under `.claude/worktrees/` is scanned
as extra source and reports a fabricated mutant coverage — 11.34% against the
true 91.85% when it happened during T-091. The gate's `--exclude-files` should
close that off rather than relying on the operator remembering to clean up.

## Acceptance

- The weekly gate fails when any single package falls below 90% mutation
  efficacy, not only when the repository total does.
- The per-package verdict names every package that is under the floor, with its
  killed, lived, and timed-out counts, so the failure is actionable without a
  second run.
- A package with no runnable mutants is not reported as a failure.
- Timed-out mutants stay outside the ratio, exactly as gremlins reports them, so
  the enforced figure is the one the run publishes.
- The check runs from the `mutation-results.json` the existing gate already
  writes; the gate is not run twice.
- The gate excludes nested worktree checkouts from its analysis, so a
  `.claude/worktrees/` copy cannot inflate or deflate the reported figures.
- `internal/toolchain` guards the new floor the way
  `TestMutationTiersSplitTheMutatorSet` guards the existing threshold: the
  guard reads the enforced value, not merely the presence of a flag.

## Verification Notes

- Prove the check fails by lowering one package's measured efficacy in a
  fixture `mutation-results.json`, and passes on the real one.
- Record the per-package table the check prints, before and after.
- TODO: record verification evidence paths.

## Implementation Notes

- The weekly gate is `test:mutate:gate` in `Taskfile.yml`; it already passes
  `-o` in the workflow lane and thresholds the aggregate at 90.
- `.github/workflows` holds the weekly mutation lane; the per-package verdict
  belongs where the aggregate threshold already reds the run.
- Follow `scripts/check-changelog.sh` and `scripts/check-commit-msg.sh` for the
  repository's convention on a shell guard plus its own test script, and
  `internal/toolchain/ci_test.go` for the Taskfile and workflow guards.
- Keep the report grouping by directory, matching how T-091 and T-092 reported
  their tables.
