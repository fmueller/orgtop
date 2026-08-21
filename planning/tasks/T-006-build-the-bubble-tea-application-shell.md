---
id: T-006-build-the-bubble-tea-application-shell
title: Build the Bubble Tea application shell
status: todo
priority: high
spec_ref: specs/v0.1.0.md#application-shell-and-refresh-lifecycle
dependencies:
    - T-005-build-filtered-activity-snapshots-and-repository
updated_at: "2026-08-21T22:39:23Z"
---

# T-006-build-the-bubble-tea-application-shell Build the Bubble Tea application shell

## Description

Create the Bubble Tea v2 root model structure and shared semantic Lip Gloss styles
around snapshot state. Own mode, dimensions, shared chrome, navigation, per-view
state slots, rendering delegation, and quit messages. Establish separate Overview
and Stream extension seams; source commands and refresh transitions belong to T-007.

## Acceptance

- Root model has explicit Overview/Stream modes and separate state/delegation seams.
- `1`, `2`, and `tab` switch mode while preserving per-view state; `q` and `ctrl+c`
  produce quit behavior.
- Resize/shared chrome satisfies the `40x10` retained-context contract without panic.
- Header separates POLLING from LOADING/ERROR/STALE; no LIVE label appears.
- Root rendering performs no normalization, filtering, aggregation, source I/O, or
  timer work.

## Test Expectations

- Drive navigation, quit, and resize messages to cover mode/state preservation,
  rendering delegation, shared context, and `40x10` behavior.
- Run focused shell tests and `task check`.

## Verification Notes

- Record exact commands, exit status, and shell/navigation/narrow tests.

## Implementation Notes
