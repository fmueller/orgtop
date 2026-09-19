---
id: T-113-correct-readme-view-selection-and-detail-control
title: Correct README view selection and detail-control guidance
status: completed
priority: medium
spec_ref: specs/v0.2.0.md#integration-documentation-and-release-readiness
dependencies:
    - T-108-make-stream-arrow-key-navigation-visibly
    - T-109-clarify-overview-pr-event-counts-and-evidence
    - T-110-qualify-activity-counts-and-empty-states-by
    - T-111-keep-stream-detail-navigation-discoverable-at
    - T-112-disclose-the-fixed-interesting-now-recency-window
updated_at: "2026-09-19T19:16:16Z"
---

# T-113-correct-readme-view-selection-and-detail-control Correct README view selection and detail-control guidance

## Description

Correct the specific README contradictions from the pre-release review: the
introduction says two views, usage denies organization selection, and controls
omit Stream detail. T-070 retains the broader final documentation audit.

## Acceptance

- Describe Overview, Stream with local detail, and Rain with Interesting Now
  consistently.
- Document --org and --repo 'ORGANIZATION/*', include-forks/include-archived,
  mixed selections, and 20-repository/100-scope bounds with explicit truncation.
  Remove the contradictory claim that organization selection is unavailable.
- Document Enter/Esc, visible focus, revised event/evidence labels, snapshot
  coverage, and the strip's independent 15-minute window using completed
  predecessor behavior.
- Preserve standalone installation, authentication, POLLING, and deferred-feature
  honesty; claims agree with CLI help and the active spec.
- T-070 consumes these corrections in its final complete contract audit,
  including cache, distribution, and rate limits, rather than reimplementing them.

## Test Expectations

- Extend focused documentation guards for required claims and obsolete
  contradictions without freezing incidental wording or layout.
- Run help/version smokes, documentation guards, and `taskrail validate`.

## Verification Notes

Record checked commands and guarded claims.

## Implementation Notes

- This task does not wait for external distribution provisioning.
- Required before T-070 and final v0.2.0 release readiness; predecessor UI tasks
  settle the wording and controls this documentation describes.
- 2026-09-19T19:16:01Z: verification pass
- 2026-09-19T19:16:16Z: Completed README and documentation contract corrections for v0.2.0 views, organization selection, Overview evidence and empty states, Stream focus/detail controls, snapshot coverage, and independent Interesting Now recency. Verification pass recorded; distribution guard remains environment-blocked only because zip is unavailable in the orb.
