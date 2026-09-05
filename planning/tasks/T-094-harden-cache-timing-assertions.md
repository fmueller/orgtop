---
id: T-094-harden-cache-timing-assertions
title: Harden the cache concurrency proof against slow-host timing
status: todo
priority: low
spec_ref: specs/v0.2.0.md#nfr-006-verification-quality
dependencies:
    - T-091-kill-surviving-mutants-in-the-cache-and-tui-suites
updated_at: "2026-09-05T12:36:42Z"
---

# T-094-harden-cache-timing-assertions Harden the cache concurrency proof against slow-host timing

## Description

T-091 introduced two wall-clock assertions into `internal/cache`, and both
proved fragile on the Windows runner. The first one broke CI: in run
33965099356 `TestConcurrentProcessesCannotBothMutate` failed with

    store_concurrency_test.go:81: the holder process never signalled readiness

because `holderReadyWait` was 500 ms and the runner needed longer merely to
spawn a second test binary and take the region. It had passed on the previous
push, so it was a borderline flake rather than a hard break. Commit 7ad3627
widened the bound to 5 s, raised the injected waits from 40 ms/200 ms to
100 ms/500 ms, and rescoped `TestContendedWorkReturnsPromptlyRatherThanWaitingOut`
against the 2 s production reset bound.

That restored a green build but left the design unchanged: the package still
holds its correctness in constants chosen against one host's speed. The
measured spread is the problem. Windows CI runs `internal/cache` in roughly
30-38 s where a Linux developer host runs it in 0.70 s -- about 40 to 50 times
slower. Any margin narrower than an order of magnitude will fail there
eventually, and both margins T-091 wrote were factors of 2.5.

The readiness wait is already the healthier of the two, because what makes it
fail fast under mutation is the exit channel, not the deadline: a holder whose
setup is broken exits and is caught at once, so the 5 s deadline only governs a
holder that is alive but slow. `TestContendedWorkReturnsPromptlyRatherThanWaitingOut`
has no such structural signal. It still asserts that a contended acquisition
returns inside the 2 s production reset bound while the injected busy bound is
100 ms, and the same 100 ms doubles as the SQLite `busy_timeout`, so the two
values cannot be separated without touching the driver setup.

The work is to remove the remaining dependence on host speed, or to state
plainly which assertion genuinely needs a clock and give it a margin sized to
the measured 50:1 spread rather than to local timings.

## Acceptance

- No assertion in `internal/cache` fails on a host running the package 50 times
  slower than a Linux developer host.
- Any assertion that still needs a clock documents the measured host spread it
  is sized against, and carries at least an order of magnitude of margin.
- The cross-process proof keeps its current mutation-killing strength: a holder
  whose setup is broken is still detected through its exit rather than by
  waiting out a deadline, and `TestHolderReadinessGivesUpAsSoonAsTheHolderExits`
  still fails when that exit case is removed.
- `internal/cache` mutation efficacy does not fall below the 97.09% measured at
  7ad3627, and its timed-out mutant count does not rise above 4.
- The injected busy bound and the SQLite `busy_timeout` are either deliberately
  coupled with that coupling documented, or separated so a timing assertion can
  move without changing driver behaviour.

## Verification Notes

- Reproduce the slow host rather than trusting the local clock: run the package
  under load, or with an injected delay, at roughly 50 times the local wall
  time, and record the figures.
- Re-run `go test -count=5 ./internal/cache` and `go test -race -count=2
  ./internal/cache`.
- Re-run scoped gremlins on `./internal/cache` and compare against the 7ad3627
  baseline of 367 killed, 11 lived, 4 timed out, 97.09% efficacy.
- Confirm on the Windows leg of CI, not only locally; the failure this task
  exists for was invisible on Linux.
- TODO: record verification evidence paths.

## Implementation Notes

- The assertions live in `internal/cache/lock_wait_test.go`
  (`testWaits`, `TestContendedWorkReturnsPromptlyRatherThanWaitingOut`,
  `TestHolderReadinessGivesUpAsSoonAsTheHolderExits`) and
  `internal/cache/store_concurrency_test.go` (`holderReadyWait`,
  `awaitHolderReady`, `TestConcurrentProcessesCannotBothMutate`).
- The injected bound reaches SQLite through the `busy_timeout` DSN parameter in
  `internal/cache/store.go`; `lockWaits` and `defaultWaits` are in
  `internal/cache/lock.go`.
- Prefer a structural signal over a wider deadline wherever one exists. The
  exit-channel pattern in `awaitHolderReady` is the model: it turned a blind
  10 s poll into a failure detected in 5 ms.
- Cost of the injected busy bound was measured across 40/100/150 ms at
  0.973 s/0.887 s/1.005 s, i.e. noise, so raising it further is cheap if that is
  the answer.
- Historical figures: run 33965099356 is the Windows failure, run 33966098445
  is the green run after 7ad3627 with `internal/cache` at 29.882 s.
