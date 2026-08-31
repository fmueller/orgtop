---
id: T-075-make-the-changelog-release-guards-portable-to-bsd
title: Make the changelog release guards portable to BSD sed
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#integration-documentation-and-release-readiness
dependencies:
    - T-043-guard-the-v0-2-0-toolchain-and-dependency-baseline
updated_at: "2026-08-31T21:04:24Z"
---

# T-075-make-the-changelog-release-guards-portable-to-bsd Make the changelog release guards portable to BSD sed

## Description

`task test:changelog` fails on macOS because `scripts/changelog-release-notes.sh`
uses a GNU-sed-only construct. BSD sed rejects it with
`sed: 2: "/^[[:space:]]*$/{$d;N;ba}": unexpected EOF (pending }'s)`, so the guard
reports a well-formed `## [0.1.0]` section as empty. CI runs the checks job on
ubuntu-latest, so the failure is invisible there and only reds `task check` for a
macOS contributor.

Discovered during T-043 verification: the failure reproduces on a clean checkout
at a36bbc5 with no local changes, so it predates that task.

## Acceptance

- `task test:changelog` and `task check` pass on both BSD sed (macOS) and GNU sed
  (Linux) with no behavior change to the extracted release notes.
- A documented-but-empty CHANGELOG section is still rejected on both platforms.

## Test Expectations

- Extend `scripts/check-changelog-test.sh` cases so the empty-section and
  populated-section outcomes are asserted rather than inferred from exit status.

## Verification Notes

- Record `task test:changelog` output on macOS and the Linux CI checks job.

## Implementation Notes

- Prefer a portable construct over branching on the sed implementation.
- Keep the guard's failure semantics identical; only the extraction changes.
- 2026-08-31T21:04:24Z: verification pass
