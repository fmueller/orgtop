---
id: T-104-harden-ordered-documentation-claim-matching
title: Harden ordered documentation claim matching
status: todo
priority: low
spec_ref: specs/v0.2.0.md#nfr-006-verification-quality
dependencies: []
updated_at: "2026-09-10T08:59:54Z"
---

# T-104-harden-ordered-documentation-claim-matching Harden ordered documentation claim matching

## Description

Three documentation gates assert that a list of claims appears in a required
order by scanning each claim with `strings.Index` from position zero:
`credentialContractProblems`, `rainWindowProblems`'s preset loop, and
`helpRainWindowProblems` in `internal/toolchain/docs_test.go`. First-occurrence
matching makes the order assertion depend on where a token happens to appear
first in the whole document rather than on the sequence of the claims
themselves. It holds today only because each token's first occurrence is the one
the check means. Wording that mentions a preset, a token, or a credential step in
prose ahead of the list it belongs to would make the ordering half of the check
stop failing while still reporting success, which is the failure mode NFR-006
rules out: a verification that cannot fail is not verification.

Replace first-occurrence scanning with a scan that resumes after the previous
match, so the checks assert that the claims occur in order, and keep the
membership half of each check unchanged.

## Acceptance

- Ordered documentation claim checks match each claim at or after the end of the
  previous claim's match, so the assertion is about claim sequence rather than
  first occurrence anywhere in the document.
- A document that states the claims in the required order passes; one that
  reorders any adjacent pair fails and names the claim that is out of order.
- A document that repeats an earlier claim's text in prose before the ordered
  list still fails when the list itself is reordered.
- `credentialContractProblems`, `rainWindowProblems`, and
  `helpRainWindowProblems` share the ordered-scan helper rather than repeating
  the loop, and their existing membership diagnostics keep their current
  wording.
- The shipped `README.md`, `CONTRIBUTING.md`, and `orgtop` usage text pass
  unchanged: this is a gate correction, not a documentation change.

## Test Expectations

- Drive the change test-first against synthetic documents in
  `internal/toolchain/docs_fixture_test.go`: an in-order document passes; a
  document reordering an adjacent pair fails; a document whose prose mentions a
  later claim's text before the ordered list still fails when that list is
  reordered, which is the case first-occurrence matching passes today.
- Keep the existing falsifiability cases for dropped claims and dropped
  sections passing.
- Run `task check`.

## Verification Notes

- Record the fixture matrix, the pre-change failure of the prose-shadowing case,
  and the full-gate result.

## Implementation Notes

- The defect is in `internal/toolchain/docs_test.go` alone. Do not reword
  README, CONTRIBUTING, or the CLI usage text to work around it.
- Found while closing T-103, whose Rain window and help-text checks reuse the
  same pattern the credential check established.
