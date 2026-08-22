---
id: T-002-implement-cli-configuration-and-github-credential
title: Implement CLI configuration and GitHub credential resolution
status: todo
priority: high
spec_ref: specs/v0.1.0.md#cli-and-authentication-boundary
dependencies:
    - T-001-implement-the-event-domain-and-repository-scope
updated_at: "2026-08-21T22:39:23Z"
---

# T-002-implement-cli-configuration-and-github-credential Implement CLI configuration and GitHub credential resolution

## Description

Implement a testable launch-configuration boundary without starting the product TUI
yet. Parse repeated exact `--repo` values into domain Scope and resolve existing
github.com credentials through a context-bound command runner. Leave final binary
wiring to T-008.

## Acceptance

- Standard Go flags accept repeated `--repo`; missing, malformed, or glob-like
  values return a configuration error suitable for non-zero usage output before
  auth or network work.
- Credential precedence is trimmed `GH_TOKEN`, trimmed `GITHUB_TOKEN`, then
  `gh auth token --hostname github.com` with a 10-second context bound.
- Environment credentials prevent `gh` invocation; failed/empty credentials produce
  the required remediation.
- Errors and test diagnostics never contain token values.
- No Cobra, custom OAuth, saved configuration, or GitHub CLI extension is added.

## Test Expectations

- Inject environment lookup and command execution to cover precedence, whitespace,
  command arguments, timeout/cancellation, missing auth, invalid Scope, and secret
  redaction with a sentinel token.
- Run focused package tests and `task check`.

## Verification Notes

- Record exact commands, exit status, and named parser/auth/redaction tests.

## Implementation Notes

- Carried over from T-001: `domain.NewScope` rejects an empty value list with the
  sentinel `domain.ErrEmptyScope` ("no repository selected"). That sentinel is a
  domain invariant, not FR-001 user-facing copy. The CLI boundary owns the FR-001
  message: launching without `--repo` must exit non-zero and print usage plus an
  actionable "at least one --repo is required" remediation. Match with
  `errors.Is(err, domain.ErrEmptyScope)` and translate; never surface the raw
  sentinel text to the user.
- Invalid `--repo` values wrap `domain.ErrInvalidRepository` through
  `domain.NewScope`; translate them the same way and keep the rejected value in the
  message.
