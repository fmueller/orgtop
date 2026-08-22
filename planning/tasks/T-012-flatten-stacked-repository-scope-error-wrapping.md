---
id: T-012-flatten-stacked-repository-scope-error-wrapping
title: Flatten stacked repository scope error wrapping
status: completed
priority: low
spec_ref: specs/v0.1.0.md#cli-and-authentication-boundary
dependencies:
    - T-002-implement-cli-configuration-and-github-credential
updated_at: "2026-08-22T14:38:21Z"
---

# T-012-flatten-stacked-repository-scope-error-wrapping Flatten stacked repository scope error wrapping

## Description

Cosmetic follow-up from the T-002 go-reviewer pass (low severity, non-blocking).
An invalid `--repo` value currently renders two stacked wraps:

```
orgtop: --repo: repository scope: invalid repository identifier "acme/*": repository contains an unsupported character "*"
```

The `repository scope: ` segment comes from `domain.NewScope`
(`internal/domain/scope.go:31`) and stacks with the CLI's own `--repo: ` prefix
(`internal/cli/config.go:54`). The message is correct and satisfies FR-001/FR-002
- it keeps the rejected value and names the flag - but it is redundant to read.

Do this only alongside T-008, when the binary's stderr formatting is finalized, so
the wording is settled once against real output rather than twice.

## Acceptance

- An invalid `--repo` value produces one prefix chain, not two overlapping ones,
  while still naming the `--repo` flag and keeping the rejected value verbatim.
- `errors.Is(err, domain.ErrInvalidRepository)` still holds at the CLI boundary.
- `cli.ErrMissingRepository` translation is unchanged and `domain.ErrEmptyScope`
  sentinel text still never reaches the user.
- Domain error text stays usable for non-CLI callers; do not weaken the reason
  detail (which component failed and why) to shorten the message.
- No public API, flag surface, or exit-status change.

## Test Expectations

- Update the invalid-value assertions in `internal/cli/config_test.go` to pin the
  intended message shape rather than only substring presence.
- Keep `internal/domain/scope_test.go` green; adjust it only if the domain wrap
  itself changes.
- Run focused `internal/cli` and `internal/domain` tests plus `task check`.

## Verification Notes

- Record exact commands, exit status, and the before/after rendered message.

## Implementation Notes

- Two viable shapes: drop the `repository scope: ` wrap in `domain.NewScope` and
  let each caller supply its own context, or have the CLI translate
  `domain.ErrInvalidRepository` into its own message instead of wrapping the
  domain text. Prefer whichever keeps the domain error useful standalone.
- Purely cosmetic. Do not bundle behavior changes into it.
- 2026-08-22T14:38:16Z: verification pass
