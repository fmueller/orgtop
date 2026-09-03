---
id: T-087-report-mixed-scope-context-in-the-shared-header
title: Report mixed Scope context in the shared header
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#scope-aware-overview-and-stream
dependencies:
    - T-058-render-scope-context-in-stream
updated_at: "2026-09-03T22:25:12Z"
---

# T-087-report-mixed-scope-context-in-the-shared-header Report mixed Scope context in the shared header

## Description

`scopeCount` in `internal/tui/chrome.go` summarizes the selection as
`<N> repositories` using `ScopeSet.Len()`, which counts Scopes rather than
repositories. A selection of one repository plus two of its path Scopes renders
`3 repositories` in the shared header, which is factually wrong once mixed Scopes
exist. RG-012 spells this segment `<R> repos · <S> scopes`, shortening to
`<S> scopes` and then `<S>S`.

Observed while rendering Stream Scope context at `40x10` during T-058.

Follow-up derived from T-058-render-scope-context-in-stream's verification or discovery.

## Acceptance

- The header Scope context reports distinct repositories and Scopes separately and
  never labels a Scope count as a repository count.
- The segment follows RG-012's `<R> repos · <S> scopes`, `<S> scopes`, `<S>S`
  shortening ladder in the closed header priority order.
- The full repository list keeps its existing request-order spelling.

## Test Expectations

- Cover a repository-only selection, a mixed repository/path selection, and every
  rung of the shortening ladder at the widths that select it.

## Verification Notes

- Record the rendered header segment per selection shape and per width rung.

## Implementation Notes

- The full header segment ladder is closed under RG-012 and shared with
  T-065-implement-deterministic-responsive-overflow; keep this change to the
  Scope-context segment itself.
