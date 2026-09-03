---
id: T-083-wire-the-enrichment-cache-into-the-launch
title: Wire the enrichment cache into the launch
status: todo
priority: high
spec_ref: specs/v0.2.0.md#changed-file-enrichment
dependencies:
    - T-056-integrate-enrichment-into-atomic-refresh
updated_at: "2026-09-03T12:57:17Z"
---

# T-083-wire-the-enrichment-cache-into-the-launch Wire the enrichment cache into the launch

## Description

T-056 wired `enrichment.Coordinator` into the refresh lifecycle with a nil
`Cache` (`cmd/orgtop/enrichment.go` `newEnrichmentAdapter`), so every refresh
reacquires changed-file evidence from GitHub and `--no-cache` selects the only
behavior that exists. `internal/cache` already provides `Open`, the RG-005
lifecycle procedures, and the freshness/maintenance surface the coordinator
consumes.

Follow-up derived from T-056-integrate-enrichment-into-atomic-refresh's verification or discovery.

## Acceptance

- A launch without `--no-cache` opens the cache at the RG-005 default location,
  binds it to the coordinator, and closes it when the program ends.
- A cache that cannot be opened is degradation, not a failed launch: the launch
  continues without reuse and the sanitized cause reaches the prepared
  `CACHE DEGRADED` state rather than the process exit path.
- `--no-cache` performs no cache operation at all, and the reused evidence a
  refresh serves from the cache is indistinguishable in membership from evidence
  it acquired.

## Test Expectations

- Cover the opened, disabled, and unopenable cache launches, and that a
  degraded cache leaves otherwise complete current evidence valid.

## Verification Notes

- Record the launch wiring, the cache-failure path, and targeted cmd and cache
  test results.

## Implementation Notes

- Keep the store lifecycle in the launch sequence; `internal/tui` must not learn
  about `internal/cache`.
