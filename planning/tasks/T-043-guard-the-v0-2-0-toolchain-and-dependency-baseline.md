---
id: T-043-guard-the-v0-2-0-toolchain-and-dependency-baseline
title: Guard the v0.2.0 toolchain and dependency baseline
status: completed
priority: high
spec_ref: specs/v0.2.0.md#integration-documentation-and-release-readiness
dependencies:
    - T-042-consolidate-v0-2-0-implementation-readiness
updated_at: "2026-08-31T20:49:42Z"
---

# T-043-guard-the-v0-2-0-toolchain-and-dependency-baseline Guard the v0.2.0 toolchain and dependency baseline

## Description

Establish the earliest implementation guard for the closed v0.2.0 contracts: verify the pinned Go/TUI toolchain, select only currently required dependencies, and keep local and CI quality lanes synchronized.

## Acceptance

- Toolchain and dependency changes implement only the closed normative contracts, including the selected SQLite approach.
- `task check` remains an exact mirror of the CI checks job and planning/CI path lanes remain exact mirrors.
- No future-source, generic persistence, animation, baseline, or plugin dependency is introduced.

## Test Expectations

- Run toolchain guard tests, dependency license checks, build, and the local CI-equivalent gate.
- Add repository guard coverage for any new pinned dependency or workflow invariant.

## Verification Notes

- Record selected versions, license results, and why each added dependency is required by a closed contract.
- Record `task check`.
- Acceptance reads "the CI checks job" as every CI job that gates every change,
  which is `checks` plus `build-test`. `task check` has run `build`, `test`, and
  `run:smoke` since v0.1.0, and those steps live in `build-test`; mirroring only
  the job literally named `checks` would reject a composition that was deliberate
  from the start. `cross-compile` stays out of the mirror on purpose.

## Implementation Notes

- Keep this task narrowly focused on enabling later work safely.
- Prefer the standard library and existing libraries; add Bubbles only for a closed concrete component and do not add Harmonica.
- 2026-08-31T20:49:37Z: verification pass
