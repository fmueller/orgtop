---
id: T-093-enforce-per-package-efficacy-floor
title: Enforce the mutation efficacy floor per package
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#nfr-006-verification-quality
dependencies:
    - T-092-floor-mutation-efficacy-repo-wide
updated_at: "2026-09-08T07:30:35Z"
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
- Real gremlins report: a gate-flagged run scoped to `internal/auth`
  (`--output-statuses lt --workers 2 --timeout-coefficient 20`, every other
  package excluded) published 4 killed, 0 lived, 1 not covered, 100.00%
  efficacy. `task test:mutate:floor REPORT=...` read that same JSON and
  reported `| internal/auth | 100.00% | 4 | 0 | 0 |`, agreeing with the figure
  gremlins published and confirming NOT COVERED stays outside the ratio.
- Per-package table on a report carrying the T-092 figures, all above the floor
  (accepted, exit 0):

  | Package | Efficacy | Killed | Lived | Timed out |
  |---|---:|---:|---:|---:|
  | internal/auth | 100.00% | 120 | 0 | 2 |
  | internal/cache | 97.10% | 268 | 8 | 2 |
  | internal/cli | 99.00% | 297 | 3 | 2 |
  | internal/domain | 92.59% | 250 | 20 | 2 |
  | internal/enrichment | 100.00% | 140 | 0 | 2 |
  | internal/github | 91.80% | 168 | 15 | 2 |
  | internal/tui | 95.42% | 313 | 15 | 2 |

- The same report with `internal/github` lowered to 6 killed / 4 lived is
  rejected (exit 1) while every other package and the repository total stay
  above the floor:
  `check-mutation-floor: internal/github is at 60.00% efficacy: killed 6,
  lived 4, timed out 2, below the 90% floor`
- `scripts/check-mutation-floor-test.sh` covers at-and-above-floor, a single
  thin package under the floor, timeouts outside the ratio, a package with no
  runnable mutants (`n/a`, not a failure), a raised floor, a package just under
  the floor whose printed figure must not round up to it, a module-root package,
  and missing, malformed, and empty reports.
- `TestMutationFloorIsEnforcedPerPackage` reads the enforced `--floor` value:
  lowering it to 70 and dropping the `.claude/` exclusion fails the guard, and
  restoring both passes it.
- The floor is written once, in the `test:mutate:floor` target: mutation.yml's
  summary and issue-body steps call the target instead of repeating `--floor 90`,
  and the guard fails if a `--floor` literal reappears in the workflow. Restoring
  the literal fails `TestMutationFloorIsEnforcedPerPackage` with `mutation.yml
  must reach the floor through the Taskfile target, not carry its own --floor
  value`; removing it passes.
- `task check` equivalents run clean: `gofmt -l .`, `task vet`, `task lint`
  (0 issues), `task test` (all packages ok), `task test:mutation-floor`,
  `shellcheck scripts/check-mutation-floor*.sh`.

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
- 2026-09-08T07:30:28Z: verification pass
