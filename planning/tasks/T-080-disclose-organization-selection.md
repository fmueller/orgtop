---
id: T-080-disclose-organization-selection
title: Disclose organization selection and bind expansion cancellation
status: completed
priority: high
spec_ref: specs/v0.2.0.md#organization-selection
dependencies:
    - T-048-implement-organization-selection-snapshots
updated_at: "2026-09-03T11:06:44Z"
---

# T-080-disclose-organization-selection Disclose organization selection and bind expansion cancellation

## Description

Render the RG-010 selection summary and the SELECTION STALE marker the T-048
lifecycle prepares, stop dispatching a poll once the refresh context is
cancelled, and cover the organization launch end to end without live
credentials.

Follow-up derived from T-048-implement-organization-selection-snapshots's review.

## Acceptance

- The shared chrome shows `selection: R repos · S scopes · X exact · G expanded`
  for an expanded selection, appends the eligible-omission and remaining-page
  disclosures RG-010 defines, and never presents a bounded result as the
  complete organization.
- The SELECTION STALE marker and its sanitized cause appear beside, never
  instead of, the primary source state. An invocation without an organization
  selector keeps its shipped chrome.
- A refresh whose context is cancelled during expansion dispatches no
  repository poll.
- An organization-only launch is covered end to end against a stubbed GitHub
  API: expansion, publication, and clean shutdown, with no credential value in
  any output.

## Test Expectations

- Add chrome tests for the summary, its omission and remaining-page suffixes,
  the stale marker and cause, and the unchanged exact-selection header.
- Add a lifecycle test for cancellation during expansion, and an end-to-end
  binary test for an `--org` launch.

## Verification Notes

- Record rendered chrome copy per state and the end-to-end request sequence.

## Implementation Notes

- Prepare disclosure state outside the renderers; the chrome only formats what
  the selection snapshot already carries. Leave the cross-cutting badge
  priority and `+N status` overflow to T-066.
- 2026-09-03T11:06:44Z: verification pass
