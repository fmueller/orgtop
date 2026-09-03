---
id: T-082-cover-contended-cache-lifecycle-reset-and-rebuild
title: Cover contended cache lifecycle reset and rebuild
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#bounded-sqlite-cache
dependencies:
    - T-055-implement-safe-cache-failure-and-recovery
updated_at: "2026-09-03T12:19:14Z"
---

# T-082-cover-contended-cache-lifecycle-reset-and-rebuild Cover contended cache lifecycle reset and rebuild

## Description

T-055 shipped `Reset` and the one structural rebuild attempt with single-process
coverage only. RG-005 bounds both on the lifecycle region of the maintenance
lock, and neither bound is currently proven under contention.

Follow-up derived from T-055-implement-safe-cache-failure-and-recovery's verification or discovery.

## Acceptance

- A reset that cannot take the lifecycle region within RG-005's two-second wait
  reports a sanitized contended cause, exits nonzero, and changes no file.
- A launch cannot rebuild a structurally corrupt database while another holder
  has the lifecycle region, and the corrupt database is preserved meanwhile.

## Test Expectations

- Hold the lifecycle region from a second process, as
  `internal/cache/boundary_test.go` already does for the admission region, and
  prove the reset wait bound rather than a hang or a race.

## Verification Notes

- Record the contended exit status, the sanitized cause, and byte-identical
  preservation of every owned file.

## Implementation Notes

- The lock mechanics are unchanged by T-055; this is test coverage, not new
  lifecycle behavior.
