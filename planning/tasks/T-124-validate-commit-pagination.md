---
id: T-124-validate-commit-pagination
title: Reject malformed and nonadvancing commit pagination
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#rg-003-github-enrichment-contract
dependencies: []
updated_at: "2026-10-04T22:04:52Z"
---

# T-124-validate-commit-pagination Reject malformed and nonadvancing commit pagination

## Description

RG-003 requires malformed or inconsistent commit pagination to produce incomplete
evidence, and a next URL must advance to an unseen page. The adapter currently
ignores a malformed next Link as if no next page existed and also follows a
decreasing page number when that URL has not yet been visited. It then presents
the accumulated subset as complete and can cache it.

Reproduce at the round-3 baseline using `docs/testing/round3/harness.py` and the
build/HTTPS/PTY commands in its report. Launch
`--repo acme/api --path acme/api:src --no-cache` with one valid size-1 PushEvent.
Serve valid matching commit SHA, sole parent, and one distinct modified src path
per page. The following independent variants fail:

- `link-malformed`: page 1 has
  `Link: https://api.github.com/repos/acme/api/commits/HEAD?per_page=100&page=2; rel="next"`
  (HEAD means the event's exact 40-hex head; required angle brackets are absent).
  Expected incomplete/1 unknown, not terminal completeness. Actual: only page 1
  requested, confirmed path activity, no PATH ? badge.
- `link-reverse`: well-formed Links lead page 1 -> 3 -> 2 -> no next, with unique
  paths and consistent identity. Actual: all three requested and the result
  declared complete despite the nonadvancing transition 3 -> 2.
- `link-duplicate-query`: next has `per_page=100&page=2&page=999`. Actual: the
  ambiguous query is followed and the result declared complete.

`extra.py` repeats malformed-Link with the cache enabled: SQLite contains a
complete one-path record, and a second process reuses it with zero commit GETs.
These are synthetic malformed responses, not claims of normal GitHub pagination.

## Acceptance

- Distinguish no next relation from an invalid offered next relation. Malformed
  next metadata cannot prove terminal completeness or admit a partial path set.
- Validate the complete page query, including exactly one expected page and
  per_page value; require each next page to advance beyond the current page and
  retain unseen-URL, host, repository, SHA, and resource-bound checks.
- Any invalid page chain yields incomplete, path unknown and no persistent
  complete record. Repository membership remains current and evaluable.
- Valid final pages without next remain complete; valid increasing chains,
  complete-empty results and existing bounds remain unchanged.

## Test Expectations

- Add adapter regressions for the three executed vectors, malformed relation
  syntax, repeated/cyclic URLs and ordinary valid two-page completion.
- Repeat the built CLI/HTTPS/PTY cases and cold/warm-cache evidence; malformed
  pagination must not survive as reusable complete evidence.

## Verification Notes

- Confirmed at af7255e1cd95cef2cee2d314b8d10d5706fa0e7d against pinned v0.2.0.
- `docs/testing/round3/results.json`, `requests.json`, `extra-results.json`,
  `extra-requests.json`, `link-malformed.txt` and `link-reverse.txt` retain the
  assertions, actual dispatched URLs, persistence and PTY output.
- Positive control `paged-good` and negative control `link-cycle` pass. This
  defect is not missing files (T-123): every response here has a valid array.
- No existing open task covers this adapter validation defect.

## Implementation Notes

- `internal/github/enrich_commit.go`: `linkRelation` silently skips bad angle
  brackets; `expectedPageQuery` uses only first query values and `page > 1`;
  `commitEvidence` tracks visited URLs, not monotonic page progression.
- No product fix or lifecycle completion was performed in round 3.
