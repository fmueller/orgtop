---
id: T-119-cli-informational-precedence
title: Honor help and version precedence before CLI parse failures
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#rg-001-unified-cli-contract
dependencies: []
updated_at: "2026-10-04T20:13:53Z"
---

# T-119-cli-informational-precedence Honor help and version precedence before CLI parse failures

## Description

Round-1 executable testing of d1dabc381adeec81b8b08da24fea1d4ebabbbf0d
found that help/version precedence depends on argument order and parse success.
RG-001 requires informational requests to win even when another supplied argument
is invalid, before credential, network, cache, or terminal work.

Reproduce with a normal build (`mise exec -- go build -o /tmp/orgtop ./cmd/orgtop`):

- `/tmp/orgtop --bad --help`: actual exit 2, unknown-flag diagnostic; expected
  help and exit 0. Reversing the arguments already succeeds.
- `/tmp/orgtop --version --reset-cache --bad`: actual exit 2; expected only
  the version on stdout, exit 0, and no reset.
- `/tmp/orgtop --version --repo`: actual exit 2, missing-value diagnostic;
  expected version and exit 0.

## Acceptance

- Both informational spellings honor RG-001 precedence independently of unknown
  flags, malformed selections, missing values, and administrative flags before
  or after them, without running authentication, cache, network, or TUI work.
- A help/version-looking string consumed as a value is not accidentally treated
  as a free-standing informational request; preserve the documented flag grammar.
- Requests without an informational action retain left-to-right diagnostics.

## Test Expectations

- Add asymmetric parser tests for errors before and after each informational
  spelling, not just otherwise-valid requests.
- Add executable tests asserting exit status and stdout/stderr, with observable
  fake credential/cache hooks or a disposable cache to prove no side effects.

## Verification Notes

- Confirmed against the built executable on Linux amd64, 2026-10-04.
- See `docs/testing/round1/report.md` and `docs/testing/round1/cli.json` for
  actual argument arrays, expected/actual exits, and captured stdout/stderr.
- Existing T-045/T-073 parsing work is completed; no open equivalent task exists.

## Implementation Notes

- `internal/cli/config.go` returns `flags.Parse` failures before its version
  check. Standard `flag` help handling also stops only when it reaches help.
- No fix was implemented during the testing round.
- 2026-10-04T20:13:53Z: verification pass
- 2026-10-04T20:13:53Z: Implemented and independently reviewed RG-001 precedence; focused, executable, mutation and full task check pass.
