---
id: T-123-require-enrichment-files-array
title: Reject absent changed-file arrays before proving enrichment complete
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#rg-003-github-enrichment-contract
dependencies: []
updated_at: "2026-10-04T22:04:47Z"
---

# T-123-require-enrichment-files-array Reject absent changed-file arrays before proving enrichment complete

## Description

RG-003 requires missing required fields to make the whole enrichment outcome
incomplete. Commit and compare responses currently deserialize absent or null
`files` into a nil slice and accept it as a complete empty set. A later commit
page missing `files` instead blesses the earlier page's partial paths. Both
cases falsely decide membership and can persist reusable complete evidence.

Reproduce against the unmodified executable at the round-3 baseline:

1. Run the build and HTTPS/PTY harness commands in
   `docs/testing/round3/report.md`; `harness.py` cases `missing`, `null`,
   `compare-missing`, `compare-null`, and `paged-missing` are the primary vectors.
2. The event has valid distinct 40-hex before/head SHAs and size 1 (commit) or 2
   (compare). Supply matching identity/parent metadata but omit `files` or set
   it to JSON null. Launch `--repo acme/api --path acme/api:src --no-cache`.
3. Expected: repository activity remains current; the path has 1 unknown and
   `PATH ?1`. Actual: no unknown badge, and the path shows `No activity in
   retained snapshot`. Missing files on page 2 produces confirmed path activity.
4. `extra.py` repeats missing/null with caching enabled, queries SQLite, then
   launches a second process while the fixture supplies valid changed files.
   Actual: a complete commit record with path_count=0 persists, the second
   process makes zero commit requests, and the false non-membership remains.

These are deliberately malformed transport responses, not claims that GitHub
normally omits changed-file arrays. Explicit `files: []` is the passing control
and must remain valid complete-empty evidence.

## Acceptance

- Require a present JSON array for `files` on every commit page and comparison;
  absent/null/wrong-type data is incomplete, never complete-empty or partial.
- Discard all accumulated page paths after any invalid page. Keep repository
  membership valid and render quantitative path unknowns without a stale badge.
- Do not persist the invalid result. A later valid response must be reacquired
  and determine membership; explicit empty arrays still prove non-membership.
- Keep payload shape validation in the GitHub adapter. Preserve valid pagination,
  identity checks, per-event parent applicability and all existing bounds.

## Test Expectations

- Add adapter regressions for absent/null files on commit page 1, later commit
  pages, and comparisons, plus explicit-empty positive controls.
- Repeat actual binary/HTTPS/PTY cold and warm-cache cases, proving both unknown
  presentation and absence of newly persisted complete evidence on malformed data.

## Verification Notes

- Baseline: af7255e1cd95cef2cee2d314b8d10d5706fa0e7d; Linux amd64, pure-Go build,
  pinned `specs/v0.2.0.md`, synthetic token and loopback-only HTTPS fixture.
- `docs/testing/round3/results.json` and `extra-results.json` contain the failing
  assertions, actual SQLite rows and request-count evidence; `missing.txt`,
  `paged-missing.txt`, and `missing-warm.txt` retain actual PTY captures.
- This is separate from completed T-119 through T-122 (CLI and escaping).
  Existing open T-071/T-106/T-118 do not own malformed enrichment completeness.

## Implementation Notes

- `internal/github/enrich_payload.go` uses slices that conflate absent/null and
  empty arrays. `commitEvidence`, `compareEvidence`, and `pathSet.addRecords`
  admit nil files. Do not fix this in cache or presentation code.
- No product fix or lifecycle completion was performed in round 3.
