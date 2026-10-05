---
id: T-127-parse-quoted-link-parameters
title: Preserve quoted Link parameters when parsing pagination
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#rg-003-github-enrichment-contract
dependencies: []
updated_at: "2026-10-05T00:53:04Z"
---

# T-127-parse-quoted-link-parameters Preserve quoted Link parameters when parsing pagination

## Description

RG-003 requires complete valid commit pagination to establish path evidence.
A legal quoted Link parameter containing a comma is split as though the comma
separated links, preventing the actual next page from being followed.

Reproduce with `docs/testing/round4/harness.py`: launch
`--repo acme/api --path acme/api:src --no-cache`, serve one valid one-commit push,
and answer its commit request with a valid exact-SHA/single-parent/files response
and `Link: <https://api.github.com/repos/acme/api/commits/HEAD?per_page=100&page=2>;
title="page, two"; rel="next"` (replace HEAD with the fixture's 40-digit SHA).
The terminal second page is valid and contains a distinct path.

Expected: two commit GETs and confirmed path membership. Actual: one commit GET,
`PATH ?1`, and `1 unknown`, despite the valid pagination response. Identical
normal, relation-list, semicolon-title, and last-before-next controls succeed.
RFC 8288 sections 3 and 3.4 permit quoted-string link parameters; its Appendix B
explicitly parses quoted strings before interpreting comma separators.

## Acceptance

- Parse Link entry/parameter separators without splitting inside quoted values
  or URI brackets; a quoted comma in an unrelated title must not hide next.
- Follow the valid next URL and preserve complete membership and cache admission.
- Preserve conservative incomplete outcomes for truly malformed/ambiguous next
  links and all URL identity, query, increasing-page, and capacity restrictions.
- Since organization expansion shares relation parsing, valid quoted attributes
  must not turn its valid listing into an ERROR either.

## Test Expectations

- Cover quoted comma/semicolon attributes, escaped quotes, attribute ordering,
  multiple entries, relation lists, and malformed controls at adapter level.
- Repeat the binary two-page case and cold/warm cache acquisition/reuse; confirm
  all paths are stored only after the terminal page and no lookup on warm reuse.

## Verification Notes

- Baseline 62e4707d8f1e10fd8fb55f447be7de9499203c67; pinned v0.2.0 spec.
- `docs/testing/round4/results.json`, `requests.json`, and `link-title-comma.txt`
  retain the failed assertions, actual GETs, and real PTY rendering.
- Authoritative syntax: https://www.rfc-editor.org/rfc/rfc8288.html#section-3
- No implementation or task lifecycle completion was performed.

## Implementation Notes

`internal/github/enrich_commit.go` implements `linkRelation` with unconditional
`strings.Split(value, ",")` and `strings.Split(entry, ";")`. Review the shared
parser rather than weakening its malformed-target validation.
- 2026-10-05T00:52:53Z: verification pass
- 2026-10-05T00:53:04Z: Implemented shared quoted/escaped/bracket-aware Link splitting without weakening URL or relation restrictions. Reviewed and disposition verified; final full gate, differential mutation and binary HTTPS/PTYS cold/warm/terminal cache checks passed. Auditable report and reproduction: docs/testing/t127/.
