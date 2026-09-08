---
id: T-100-scope-context-outranks-header-cause
title: Give Scope context priority over the header cause text
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#scope-aware-overview-and-stream
dependencies:
    - T-087-report-mixed-scope-context-in-the-shared-header
updated_at: "2026-09-08T09:43:26Z"
---

# T-100-scope-context-outranks-header-cause Give Scope context priority over the header cause text

## Description

`headerCandidates` in `internal/tui/chrome.go` appends `required, cause` before
`required, context`, so a tightening header drops the whole Scope-context
segment while it keeps the concise sanitized cause text. RG-012's closed header
priority order lists Scope context ahead of the stale last-success and the
concise sanitized error, so the cause should give way first.

The condition is reachable whenever `state.Cause` is populated beside a Scope
selection at a narrow width, for example a `FreshnessError` state at `40x10`.
The ordering predates T-087; T-087 expanded the Scope-context segment into
RG-012's rung ladder at the same two positions without reordering them.

Observed by the go-reviewer pass over T-087 at `internal/tui/chrome.go:149-150`.

Follow-up derived from T-087-report-mixed-scope-context-in-the-shared-header's verification or discovery.

## Acceptance

- A tightening header exhausts the Scope-context rung ladder before it drops the
  cause text, matching RG-012's closed segment priority order.
- The stale path keeps reporting the last success beside its cause (FR-008).
- The RG-010 selection detail stays the lowest-priority segment.

## Verification Notes

- Record the rendered header at the widths where a populated cause and a Scope
  selection compete, per freshness state.

## Implementation Notes

- The header priority order is shared with
  T-065-implement-deterministic-responsive-overflow; keep the change to the
  candidate ladder ordering itself.
- 2026-09-08T09:43:18Z: verification pass
