---
id: T-092-floor-mutation-efficacy-repo-wide
title: Raise every package to a 90 percent mutation efficacy floor
status: completed
priority: high
spec_ref: specs/v0.2.0.md#nfr-006-verification-quality
dependencies:
    - T-091-kill-surviving-mutants-in-the-cache-and-tui-suites
updated_at: "2026-09-05T12:06:04Z"
---

# T-092-floor-mutation-efficacy-repo-wide Raise every package to a 90 percent mutation efficacy floor

## Description

T-091 lifted `internal/cache` to 97.1% and `internal/tui` to 95.4% mutation
efficacy, and the weekly gate now reports 94.01% overall. Two packages still
sit at or below the level NFR-006 expects of the rest:

| Package | Killed | Lived | Timed out | Efficacy |
|---|---:|---:|---:|---:|
| internal/enrichment | 60 | 9 | 1 | 87.0% |
| internal/cli | 90 | 10 | 11 | 90.0% |

`internal/enrichment` is below the bar. `internal/cli` is exactly on it with no
margin: a single new uncovered branch drops it under.

The `internal/enrichment` survivors are concentrated in `coordinator.go`: six
`INVERT_LOOPCTRL` mutants where a `continue` that skips one work item is
accepted as a `break` that abandons every later one, plus the `Now` clock
default, the cached-record error classification, and the ledger's degraded
disclosure in `bounds.go`. Bounded enrichment coordination is a closed
contract, so a skip that silently becomes an abandonment is exactly the kind of
change the suite has to refuse.

The `internal/cli` survivors are the Scope expansion arithmetic in `scope.go`
— the projected Scope count and the filtered/patterned product — and the
segment bookkeeping in `pattern.go`.

`internal/cli/pattern.go` also carries 10 of the repository's 26 timed-out
mutants. Every one of them mutates a loop index advance (`i++`, `i += width`)
into a form that never advances, so the pattern scanner spins instead of
returning. A hang cannot be caught by an assertion, so these are not efficacy
survivors, but they are the same reporting debt the cache carried before T-091:
a mutant that returns no verdict counts toward neither side of the ratio.

## Acceptance

- Every package the weekly gate measures reports at least 90% mutation
  efficacy under `task test:mutate:gate`.
- `internal/enrichment` and `internal/cli` each hold a margin above the floor
  rather than landing exactly on it.
- Surviving mutants are killed by assertions on observable behavior, not by
  deleting or weakening the mutated code.
- Every new test asserts behavior the active spec requires; none assert
  implementation detail that would block a legitimate refactor.
- Any survivor left alive is documented as an equivalent mutant with the
  reason it produces identical observable behavior.
- The `internal/cli/pattern.go` timeouts are accounted for: either killed, or
  recorded with the reason a non-advancing loop index cannot be caught by an
  assertion.

## Verification Notes

- Record the `mutation-results.json` per-package efficacy before and after,
  from `task test:mutate:gate`.
- `task test:mutate:gate` before (commit 2e7dcb2): 1661 runnable, 1537 killed,
  98 lived, 26 timed out, 94.01% efficacy, 91.85% mutant coverage.
- `task test:mutate:gate` after (commit 4689cb4): 1661 runnable, 1556 killed,
  80 lived, 25 timed out, 95.11% efficacy, 91.86% mutant coverage.
- Per-package efficacy after, every package above the floor:

  | Package | Killed | Lived | Timed out | Efficacy |
  |---|---:|---:|---:|---:|
  | internal/auth | 4 | 0 | 0 | 100.0% |
  | internal/cache | 367 | 11 | 4 | 97.1% |
  | internal/cli | 100 | 1 | 10 | 99.0% |
  | internal/domain | 239 | 19 | 1 | 92.6% |
  | internal/enrichment | 69 | 0 | 1 | 100.0% |
  | internal/github | 279 | 25 | 0 | 91.8% |
  | internal/tui | 498 | 24 | 9 | 95.4% |

- `internal/enrichment` 87.0% to 100.0% (0 survivors); `internal/cli` 90.0% to
  99.0%. The one remaining CLI survivor is `scope.go:236`'s `make()` capacity
  hint, whose mutated form grows the same slice through `append`.
- The gate now thresholds efficacy at 90 rather than 70, guarded by
  `TestMutationTiersSplitTheMutatorSet`. gremlins thresholds the repository
  total rather than each package, so the aggregate is what the gate enforces
  and the per-package figures are read from the run's own report.
- The 10 timed-out mutants in `internal/cli/pattern.go` (lines 74, 108, 115,
  152, 161, 164, 167, 171) each turn a loop index advance into a form that
  never advances, so the scanner spins instead of returning. A hang cannot be
  caught by an assertion, and neither scan loop carries a structural progress
  guard. They stay recorded rather than killed; adding a guard would be a
  production change this task does not make. `internal/enrichment/bounds.go:60`
  times out for the same class of reason: `Concurrency <= 0` mutated to `< 0`
  builds a zero-capacity dispatch channel and deadlocks.
- Evidence: `git log 2e7dcb2..HEAD`; commits `a1168b0` (behavior tests),
  `4689cb4` (shared argument builders).

## Implementation Notes

- Baseline measured 2026-09-05 at commit 2e7dcb2: 1661 runnable, 1537 killed,
  98 lived, 26 timed out, 94.01% efficacy, 91.85% mutant coverage.
- Remove any `.claude/worktrees/` checkouts before running the gate. gremlins
  walks the module directory tree, so a nested worktree is scanned as extra
  source and reports a fabricated mutant coverage (11.34% against 91.85%).
- Use `task test:mutate BASE=<ref>` while iterating, or scope a run to one
  package with `gremlins unleash ./internal/<pkg>`.
- 2026-09-05T12:05:59Z: verification pass
