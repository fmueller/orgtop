---
id: T-091-kill-surviving-mutants-in-the-cache-and-tui-suites
title: Kill surviving mutants in the cache and TUI suites
status: in_progress
priority: high
spec_ref: specs/v0.2.0.md#nfr-006-verification-quality
dependencies: []
updated_at: "2026-09-05T09:32:00Z"
---

# T-091-kill-surviving-mutants-in-the-cache-and-tui-suites Kill surviving mutants in the cache and TUI suites

## Description

The weekly mutation gate now runs every gremlins mutator with
`--timeout-coefficient 20` (see `Taskfile.yml` `test:mutate:gate`). The first
full measurement on that configuration reports 83.96% efficacy and 91.37%
mutant coverage over 1657 runnable mutants, with 253 survivors concentrated in
the two packages NFR-006 names explicitly:

| Package | Killed | Lived | Timed out | Efficacy |
|---|---:|---:|---:|---:|
| internal/cache | 250 | 71 | 58 | 77.9% |
| internal/tui | 402 | 119 | 9 | 77.2% |

Every other package sits at 87% or above. The survivors cluster in
`cache/maintenance.go` (45), `tui/rain_render.go` (27), `tui/rain.go` (20),
`tui/stream.go` (12), `tui/rain_columns.go` (11), `tui/interesting.go` (10) and
`tui/rain_field.go` (9) — the bounded-cache and Rain work from T-060 through
T-063 and the cache tasks before them.

A surviving mutant means the test suite accepts a behavior change in code the
spec requires to be covered. The cache bounds and the Rain field contract are
both closed contracts (RG-005, RG-006), so a mutant that lives there is an
assertion the closed contract does not actually pin.

The 58 remaining cache timeouts are a second, related defect. Cache tests wait
out their retry bounds under mutation rather than failing fast, which is why
the gate needs a 20x timeout coefficient at all. At the default coefficient 280
of 293 cache mutants time out, and because timeouts count toward neither side
of the efficacy ratio the package reported a fabricated 100%. Bounded waits
that are injectable from tests would remove both the timeouts and most of the
gate's runtime cost.

## Acceptance

- `internal/cache` and `internal/tui` each reach at least 90% mutation
  efficacy under `task test:mutate:gate`, matching the level the other
  packages already hold.
- Surviving mutants in `cache/maintenance.go`, `tui/rain_render.go`,
  `tui/rain.go`, `tui/rain_columns.go` and `tui/rain_field.go` are killed by
  assertions on observable behavior, not by deleting or weakening the mutated
  code.
- Cache test waits are bounded by a value tests can inject, so a mutant that
  breaks locking or budget arithmetic fails fast instead of timing out.
- `internal/cache` reports no more than 5 timed-out mutants at the default
  gremlins timeout coefficient, demonstrating the suite no longer depends on
  the widened bound.
- Every new test asserts behavior the active spec requires; none assert
  implementation detail that would block a legitimate refactor.

## Verification Notes

- Record the `mutation-results.json` efficacy and per-package survivor counts
  before and after, from `task test:mutate:gate`.
- `task test:mutate:gate` before (2026-09-05, commit 804e217): 1657 runnable,
  1324 killed, 253 lived, 80 timed out, 83.96% efficacy, 91.37% mutant
  coverage. `internal/cache` 250/71/58 at 77.9%; `internal/tui` 402/119/9 at
  77.2%.
- `task test:mutate:gate` after (commit e8fa22b): 1661 runnable, 1537 killed,
  98 lived, 26 timed out, 94.01% efficacy, 91.85% mutant coverage.
  `internal/cache` 367/11/4 at 97.1%; `internal/tui` 498/24/9 at 95.4%. Every
  other package is unchanged.
- Default gremlins timeout coefficient, `internal/cache` only: 188 timed out
  before the injectable waits, 58 after `lockWaits` alone, 4 after the bounded
  cross-process readiness wait. Package wall clock 1.685s before, 0.700s after.
- Surviving mutants in the NFR-006 named files after the change:
  `cache/maintenance.go` 45 to 3, `tui/rain_render.go` 27 to 4, `tui/rain.go`
  20 to 4, `tui/rain_columns.go` 11 to 4, `tui/rain_field.go` 9 to 1. Each
  remaining survivor is an equivalent mutant: a `make()` capacity hint, or a
  boundary whose two forms agree on every reachable input.
- Evidence: `git log 804e217..e8fa22b`; commits `39e9ff5` (injectable waits),
  `fe77161` (behavior tests), `e8fa22b` (shared fixtures).

## Implementation Notes

- Full baseline measured 2026-09-05 on current main: 1657 runnable, 1324
  killed, 253 lived, 80 timed out, 83.96% efficacy, 16m35s wall clock on 32
  cores at `--workers 2`.
- `acquireGuard` in `internal/cache/lock.go` polls to a deadline; the wait
  bound its callers pass is the value tests need to be able to shrink.
- Use `task test:mutate BASE=<ref>` while iterating — the differential lane is
  the cheap one and stays on the default mutator set.
