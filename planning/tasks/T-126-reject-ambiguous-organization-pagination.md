---
id: T-126-reject-ambiguous-organization-pagination
title: Reject ambiguous organization pagination queries
status: completed
priority: high
spec_ref: specs/v0.2.0.md#rg-010-organization-selection-contract
dependencies: []
updated_at: "2026-10-05T00:20:57Z"
---

# T-126-reject-ambiguous-organization-pagination Reject ambiguous organization pagination queries

## Description

RG-010 requires the next organization listing URL to preserve exactly the
specified query, advance N to N+1, and reject malformed/unexpected pagination
atomically. The executable accepts ambiguous duplicate keys and malformed query
escapes, then polls and publishes the expanded repositories.

Reproduce with the build and HTTPS/PTY harness in `docs/testing/round4/report.md`.
Launch `--org acme --repo exact/keep --no-cache`. Return one valid listing record
and a next Link to the ordinary page-2 URL with one of these suffixes:

- `&page=999` (after `page=2`);
- `&per_page=1` (after `per_page=100`);
- `&ignored=%zz`.

Expected: whole expansion fails, ERROR, and no Events GET for either the exact or
expanded repositories. Actual: the malformed URL is requested, then three Events
GETs occur and a current snapshot is published. The fixture intentionally uses
the first query value; another upstream interpretation could return another page.
The actual sent URL, not fixture interpretation, proves the validation defect.

## Acceptance

- Reject query parse errors and any repeated key in an organization next URL.
- Require exactly the specified listing query and one value per key, advancing
  exactly one page; do not poll or publish a partial selection on violation.
- Preserve valid two-page expansion, ordinary extra-query rejection, exact-repo
  precedence, source recovery, and existing commit-pagination regressions.

## Test Expectations

- Add adapter vectors for the three suffixes above and duplicates of each fixed
  query key, with asymmetrical values so first-value-only validation cannot pass.
- Repeat the actual binary fixture; all malformed variants must make only the
  initial listing request, while valid expansion makes two listings/three polls.

## Verification Notes

- Baseline 62e4707d8f1e10fd8fb55f447be7de9499203c67; pinned v0.2.0 spec.
- `docs/testing/round4/results.json`, `requests.json`, `org-duplicate-page.txt`,
  `org-duplicate-size.txt`, and `org-bad-escape.txt` retain reproduction evidence.
- Valid listing, ordinary extra query, and semicolon rejection controls pass.
- No implementation or task lifecycle completion was performed.

## Implementation Notes

`internal/github/expand_pagination.go` passes `parsed.Query()` (which discards
parse errors) into `advancesListingPage`, then checks only `query.Get(key)`.
Commit pagination already uses stricter query parsing/cardinality checks.
- 2026-10-05T00:20:57Z: verification pass
- 2026-10-05T00:20:57Z: Reject malformed or repeated listing query keys atomically; pinned RG-010 verified after independent review and final gates; evidence docs/testing/t126/report.md.
