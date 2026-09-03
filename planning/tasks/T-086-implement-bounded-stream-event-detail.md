---
id: T-086-implement-bounded-stream-event-detail
title: Implement bounded Stream event detail
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#scope-aware-overview-and-stream
dependencies:
    - T-058-render-scope-context-in-stream
updated_at: "2026-09-03T22:25:07Z"
---

# T-086-implement-bounded-stream-event-detail Implement bounded Stream event detail

## Description

RG-012's "Stream Overlap and Event Detail" bundles two behaviors. T-058 landed the
first: the bounded per-row overlap presentation in `internal/tui/scope_context.go`.
The second, `enter`-triggered bounded Stream event detail, is unimplemented and
owned by no task; `internal/tui/stream.go` handles no `enter` keystroke today.

Detail must read only the prepared snapshot: source event ID, timestamp and
prepared age, repository, category, optional actor, entity kind/reference,
description, every member Scope with its current-PR qualification, and every
unknown Scope with its prepared reason. Not-member Scopes are omitted.

Follow-up derived from T-058-render-scope-context-in-stream's verification or discovery.

## Acceptance

- `enter` on the focused event opens bounded detail from the same prepared
  snapshot, and `esc` returns to the unchanged focused event and Stream viewport.
- Detail performs no API, cache, normalization, matching, aggregation, or ranking
  work, and never fetches additional facts.
- Detail greedily grapheme-wraps every logical line, shortens no non-Scope fact and
  no full detail Scope label, and bounds presentation by vertical scrolling.
- While detail is open, a refresh follows the stable source event ID: retained
  means its prepared facts replace the detail atomically and Stream focus moves to
  its new index; removed means detail closes and Stream clamps its prior focus.
- `q` remains available and the `40x10` minimum keeps view, transport/freshness,
  one detail line, and the quit hint.

## Test Expectations

- Cover open/close, wrapping, unknown reasons, current-PR qualification,
  refresh-while-open for a retained and for a removed event, and `40x10`.

## Verification Notes

- Record the rendered detail lines for a multi-Scope event and the two
  refresh-while-open transitions.

## Implementation Notes

- Stream's focused-event index and the `lines A-B of N` range ladder belong to
  T-065-implement-deterministic-responsive-overflow; sequence accordingly.
- Reuse the prepared Scope tokens and `scopeContext` groups rather than
  re-deriving membership in the detail renderer.
