---
id: T-120-reject-boolean-assignments
title: Reject boolean flag assignments before cache or launch work
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#rg-001-unified-cli-contract
dependencies: []
updated_at: "2026-10-04T20:42:01Z"
---

# T-120-reject-boolean-assignments Reject boolean flag assignments before cache or launch work

## Description

Round-1 executable testing found that Go's ordinary boolean assignment syntax
is accepted despite RG-001 requiring boolean flags in bare form only. False
assignments also erase the presence of a flag when standalone cache-reset
compatibility is checked.

Reproduce against a normal build in a disposable HOME/XDG_CACHE_HOME:

- `orgtop --reset-cache=true` returns 0 and performs reset; expected usage exit 2.
- `orgtop --reset-cache --no-cache=false` returns 0 and performs reset; expected
  usage exit 2 before any cache work.
- `orgtop --no-cache=true --repo acme/api` advances into launch/authentication
  rather than rejecting the grammar. In the non-terminal process probe it
  returned 1 from TUI startup, rather than expected usage exit 2.

This is separate from T-119: no informational action is present here.

## Acceptance

- Reject assignment forms of bare-only booleans with usage exit 2 before
  credential, network, cache, or TUI work, subject to RG-001 informational
  precedence. Cover both true and false rather than only truthy assignments.
- Invalid administrative combinations cannot delete or open cache state.
- Keep valid bare controls and `--flag=VALUE` on value flags working unchanged.

## Test Expectations

- Table-test all bare-only boolean controls, repeated forms, and true/false
  assignments, including inclusion controls beside reset.
- At executable level, seed disposable valid cache state, invoke the rejected
  reset combinations, and assert unchanged database bytes and nonzero usage exit.
- Assert that rejected launch input never invokes a fake credential resolver.

## Verification Notes

- Confirmed on Linux amd64 at d1dabc381adeec81b8b08da24fea1d4ebabbbf0d.
- `docs/testing/round1/cli.json` captures process output and exit statuses;
  `docs/testing/round1/invalid-reset.json` records the populated-cache deletion
  reproduction. See the adjacent report for the disposable test environment.
- No existing open task covers this grammar/administrative side-effect defect.

## Implementation Notes

- `internal/cli/config.go` registers plain `flags.Bool` controls and checks
  parsed boolean values rather than rejecting assignment syntax.
- No production code was changed during testing.
- 2026-10-04T20:41:50Z: verification pass
- 2026-10-04T20:42:01Z: RG-001 bare-only boolean grammar enforced before side effects with strict TDD, executable cache-byte preservation and resolver nonexecution tests; simplifier, General/Go/Security reviews, candidate/disposition verification and final task check passed. Audit in docs/testing/t120-boolean-assignments.md.
