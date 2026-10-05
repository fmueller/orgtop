---
id: T-129-decode-escaped-link-relations
title: Decode escaped Link relations before proving pagination complete
status: completed
priority: high
spec_ref: specs/v0.2.0.md#rg-003-github-enrichment-contract
dependencies: []
updated_at: "2026-10-05T02:32:54Z"
---

# T-129-decode-escaped-link-relations Decode escaped Link relations before proving pagination complete

## Description

RG-003 requires terminal pagination proof before complete path evidence. The
shared Link relation parser treats valid `rel="ne\xt"` as absence of next rather
than decoding the quoted pair to `next`. RFC 8288 section 3 requires interpreting
parameter values after unquoting; Appendix B.4 consumes a backslash and appends
the following byte. This is distinct from T-127's quoted-title delimiter fix.

Reproduce with `docs/testing/round5/link-probes.py` against the baseline binary.
A valid one-commit event has before SHA `1111111111111111111111111111111111111111`
and head SHA `2222222222222222222222222222222222222222`. Page one reports the
exact SHA/sole parent, `src/first.go`, and a same-endpoint page-two Link with
`rel="ne\xt"`. Page two contains `other/last.go`. Launch
`--repo acme/api --path acme/api:other` with a disposable cache.

Expected: two commit GETs, confirmed membership in `other`, and a complete
two-path cache row only after page two. Actual: one commit GET, no unknown badge,
`No activity in retained snapshot`, and SQLite `complete=1,path_count=1` with
only `src/first.go`. A second process with corrected plain `rel="next"` makes
zero enrichment GETs and reuses the false negative.

The same shared parser under RG-010 silently ends organization expansion after
page one: `--org acme --repo exact/keep --no-cache` polls only exact/keep and
acme/r001, omitting acme/r002 rather than fetching both listings first.

## Acceptance

- Decode quoted-pair relation values before identifying relation tokens;
  `rel="ne\xt"` must identify the same next link as `rel="next"`.
- Fetch and validate all offered commit pages; never admit a first-page subset
  as complete or derive confirmed nonmembership from that subset.
- Apply the same relation semantics to organization listing; publish/poll the
  full valid selection only after listing completion.
- Preserve URL identity/query/capacity restrictions and conservative outcomes for
  malformed or ambiguous pagination. Keep quoted title/URI delimiter handling.

## Test Expectations

- Unit/adaptor coverage for escaped letters and escaped spaces in relation lists,
  plain next, actual non-next (`rel="la\st"`), and malformed controls.
- Binary HTTPS/PTY/SQLite cold and corrected-warm runs must show both paths,
  correct terminal-path membership, and no premature complete cache row.
- Binary organization request log must show both listing GETs preceding every
  exact or expanded Events GET. Existing T-124/T-126/T-127 regressions stay green.

## Verification Notes

- Baseline ba56c5dc5776664a201c7cb4eda5b206690231fb; pinned specs/v0.2.0.md.
- `docs/testing/round5/link-results.json` and `link-requests.json`: six failed
  assertions, one shared-root defect; ordinary/escaped-list/non-next and eight
  invalid URL/header controls per consumer passed.
- `docs/testing/round5/commit-escaped-next.txt`,
  `commit-escaped-next-corrected-warm.txt`, and `org-escaped-next.txt` are PTY
  captures; SQLite query results and zero warm lookup count are in results JSON.
- Syntax source: https://www.rfc-editor.org/rfc/rfc8288.html#section-3 and
  https://www.rfc-editor.org/rfc/rfc8288.html#appendix-B.4.
- Historical round5 result: confirmed by execution, not fixed at that time;
  this evidence did not constitute release acceptance.
- T-129 verification against source main/origin/main
  `0aa5b328be10f9c4610117c2054263397786bfda`: focused source tests reproduced
  premature completeness and partial organization selection before the fix.
  `docs/testing/t129/results.json` records 56 passing binary assertions, including
  an empty cache while page two is blocked, both paths after completion, zero
  warm enrichment GETs, and listing completion before every repository poll.
  `docs/testing/t129/check.txt` and `mutation.txt` record the full local gate
  and differential mutation checks. This is task verification, not a new
  global adversarial round or release acceptance.

## Implementation Notes

The baseline `internal/github/enrich_commit.go:linkRelation` split raw quoted
text into relation tokens without decoding quoted pairs. The fix decodes each
quoted pair to its following byte before token recognition, retaining the raw
value for conservative validation. `splitLink` continues to own delimiter
scanning. URL, query, page, capacity, and cache contracts remain unchanged;
there is no parser replacement or cache migration.
- 2026-10-05T02:32:54Z: verification pass
- 2026-10-05T02:32:54Z: Decoded quoted-pair relations with unchanged trust/pagination/cache contracts; reviewed and verified with full gate and focused binary proof.
