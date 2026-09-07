---
id: T-096-migrate-the-windows-cache-lock-to-golang-org-x-sys
title: Migrate the Windows cache lock to golang.org/x/sys/windows
status: todo
priority: low
spec_ref: specs/v0.2.0.md#bounded-sqlite-cache
dependencies:
    - T-078-verify-windows-cache-ownership-and-acl-enforcement
updated_at: "2026-09-07T10:52:28Z"
---

# T-096-migrate-the-windows-cache-lock-to-golang-org-x-sys Migrate the Windows cache lock to golang.org/x/sys/windows

## Description

`internal/cache/lock_windows.go` resolves `LockFileEx` and `UnlockFileEx` through
`syscall.NewLazyDLL` because `golang.org/x/sys` was not an approved direct
requirement at the time. T-078 admitted it for the cache owner and ACL checks,
and `golang.org/x/sys/windows` wraps both calls, so the hand-rolled kernel32
binding and its `unsafe.Pointer` overlapped handling can be replaced by the
maintained wrappers.

## Acceptance

- The Windows maintenance lock uses `golang.org/x/sys/windows` rather than a
  lazily resolved kernel32 handle, and no `unsafe` remains in
  `internal/cache/lock_windows.go`.
- Lock acquisition, contention, the wait bound, the retry interval, and the
  `ErrContended` error semantics are unchanged.
- The stale rationale comment at the top of `internal/cache/lock_windows.go` is
  replaced by the reason the file ends up in its final shape.

## Test Expectations

- The existing `internal/cache` lock and contention tests pass unchanged.

## Verification Notes

- Record the Windows CI leg result and the six-target cross-compile evidence.

## Implementation Notes
