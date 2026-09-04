---
id: T-088-match-deferred-claim-spelling-variants
title: Match deferred-claim spelling variants in the documentation gate
status: todo
priority: low
spec_ref: specs/v0.2.0.md#fr-012-documentation
dependencies:
    - T-030-decouple-readme-structure-from-doc-tests
updated_at: "2026-09-04T00:00:00Z"
---

# T-088-match-deferred-claim-spelling-variants Match deferred-claim spelling variants in the documentation gate

## Description

`deferredClaimsBySpec` in `internal/toolchain/docs_test.go` matches claims on
word boundaries, so a deferred capability spelled with a hyphen or written
solid escapes the honesty gate: `\bdrilldown\b` does not match `drill-down` or
`drill down`, and neither `real time` nor `real-time` matches `realtime`. The
gate would pass documentation that promises a capability the active spec
defers, which is the failure FR-012 exists to prevent.

The gate becomes live the moment someone edits the documentation — notably
T-070, which rewrites README.md under FR-012.

Follow-up derived from T-030-decouple-readme-structure-from-doc-tests's
verification or discovery.

## Acceptance

- The deferred-capability detection catches `drill-down`, `drill down`, and
  `realtime` in addition to the spellings already listed, under both keyed
  spec versions.
- The existing word-boundary prose guarantees still hold: `constraint`,
  `restraint`, `training`, and `research` do not trip the honesty gate.
- The repository's own files keep passing: no current README.md or
  CONTRIBUTING.md prose trips the added spellings.
- `task check` stays green.

## Verification Notes

- Record the RED evidence for the added spellings and the GREEN run of
  `go test ./internal/toolchain/...` naming the new cases.
- Record `task check` exit status.

## Implementation Notes

- Scope stays `internal/toolchain/docs_test.go` and
  `internal/toolchain/docs_fixture_test.go`; no production code, no changelog
  entry (not user-visible).
- Prefer listing the spelling variants explicitly in `deferredClaimsBySpec`
  over normalizing hyphens and spaces in documents and claims: an explicit
  vocabulary fails loudly when a new spelling matters, while silent
  normalization also rewrites the matched text the failure messages quote.
