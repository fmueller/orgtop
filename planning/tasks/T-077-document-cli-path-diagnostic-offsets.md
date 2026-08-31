---
id: T-077-document-cli-path-diagnostic-offsets
title: Document unified CLI path diagnostic offsets
status: todo
priority: low
spec_ref: specs/v0.2.0.md#unified-repository-and-path-scopes
dependencies:
    - T-045-implement-unified-scope-cli-parsing
updated_at: "2026-08-31T22:16:03Z"
---

# T-077-document-cli-path-diagnostic-offsets Document unified CLI path diagnostic offsets

## Description

RG-002 closes pattern diagnostics as zero-based UTF-8 byte offsets and states
that repository-prefix validation precedes pattern tokenization for a qualified
`--path` value, but it does not say which text a qualified value's offsets are
counted from. T-045 implemented them component-relative: the repository prefix and
the pattern each start at byte zero, and the diagnostic quotes the component it
names. Confirm that reading against the closed contract and document it, or
correct the implementation to whole-value offsets.

Follow-up derived from T-045-implement-unified-scope-cli-parsing's verification or discovery.

## Acceptance

- The offset origin for bare and qualified `--path` values is stated in the
  user-facing documentation and matches what the binary reports.
- Every RG-002 offset example (empty input, leading slash, repeated slash,
  trailing slash) still resolves to the documented byte.
- If the implementation changes, the closed diagnostic ordering and the
  single-cause-with-usage behavior are preserved.

## Test Expectations

- Extend the CLI diagnostic vectors in `internal/cli/scope_test.go` with the
  qualified-value offset cases the documentation states.

## Verification Notes

- Record the documented wording, the CLI vectors covering it, and help smokes.

## Implementation Notes

- Coordinate with T-070, which owns the closed v0.2.0 documentation pass; this
  task only settles and documents the offset origin.
- Implementation lives in `internal/cli/pattern.go`; do not move pattern text
  diagnostics into the domain.
