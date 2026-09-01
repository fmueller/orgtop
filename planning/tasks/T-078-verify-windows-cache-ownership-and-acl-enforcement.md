---
id: T-078-verify-windows-cache-ownership-and-acl-enforcement
title: Verify Windows cache ownership and ACL enforcement
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#bounded-sqlite-cache
dependencies:
    - T-050-implement-the-versioned-sqlite-enrichment-store
updated_at: "2026-09-01T22:46:37Z"
---

# T-078-verify-windows-cache-ownership-and-acl-enforcement Verify Windows cache ownership and ACL enforcement

## Description

RG-005 requires that on Windows OrgTop uses handle-based type/owner checks, rejects
reparse points, and requires the cache directory and mutable files to be owned by
the current user under inherited ACLs that do not grant another user write access.
T-050 shipped the handle-based type, reparse-point, and hard-link checks in
`internal/cache/paths_windows.go`, but not the owner SID or ACL verification: the
`golang.org/x/sys/windows` security helpers are not an approved direct requirement
(`internal/toolchain/dependencies_test.go` pins the direct requirement set), so the
owner and ACL checks were deliberately left open rather than approximated.

Close the remaining Windows half of the location, ownership, and permission
contract, and decide explicitly whether the `advapi32` security APIs are reached
through `syscall.NewLazyDLL` — as the Windows lock-region binding already is in
`internal/cache/lock_windows.go` — or by admitting a new approved direct
requirement to the v0.2.0 dependency surface.

## Acceptance

- The Windows cache directory and every mutable cache file are proven to be owned
  by the current user before use, and a directory or file owned by another account
  is cache-unavailable rather than repaired.
- Inherited ACLs granting another user write access to the cache directory or its
  mutable files are rejected under the same degraded policy as a POSIX ownership
  failure, without failing an otherwise valid interactive launch.
- The chosen mechanism keeps `CGO_ENABLED=0` cross-compilation of all six release
  GOOS/GOARCH pairs working, and either keeps the direct requirement set in
  `internal/toolchain/dependencies_test.go` unchanged or updates that gate and its
  rationale in the same change.

## Test Expectations

- Add Windows-guarded tests for an owned cache directory, a directory owned by
  another account, and a mutable file whose inherited ACL grants another user
  write access, skipping cleanly on non-Windows hosts.
- Keep the existing POSIX ownership and mode tests in `internal/cache` passing
  unchanged.

## Verification Notes

- Record the Windows ownership/ACL decision, the resulting degraded cause, and the
  cross-compile evidence for all six release platforms.

## Implementation Notes

- Keep the checks inside `internal/cache/paths_windows.go`; the shared
  `inspectOwnedPaths`/`repairOwnedPaths` seam already isolates them.
- Every path mutation still happens only under exclusive lifecycle access.
