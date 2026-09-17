---
id: T-106-test-the-v0-2-0-release-candidate-before-tagging
title: Test the v0.2.0 release candidate before tagging
status: todo
priority: high
spec_ref: specs/v0.2.0.md#integration-documentation-and-release-readiness
dependencies:
    - T-070-document-the-closed-v0-2-0-behavior
    - T-068-verify-distribution-channel-parity
updated_at: "2026-09-17T22:16:41Z"
---

# T-106-test-the-v0-2-0-release-candidate-before-tagging Test the v0.2.0 release candidate before tagging

## Description

The maintainer's own testing pass over the closed v0.2.0 behavior, run against
a real build on real hosts before any v0.2.0 tag exists. It is deliberately not
automated: it covers what the suites cannot reach, which is a person using
OrgTop against live GitHub data in a real terminal.

A completed version is permanently non-reusable
(`scripts/distribution-ledger-append.sh`), so a defect found after the tag
publishes costs the version number rather than a retry. This task exists to
find those defects while the cost is still a commit.

## Acceptance

- The integrated binary is exercised against live GitHub data with a real
  token, in both Overview and Stream, and in the Rain and Interesting Now
  views, at the credential precedence the documentation states.
- The unified `--scope` and organization selectors are exercised with a
  repository selector, a path selector, an organization selector, and a mixed
  set, including selectors that name nothing and selectors the token cannot
  see.
- Enrichment and the SQLite cache are exercised across a cold start, a warm
  start, a cache the process cannot write, and a cache left behind by an
  earlier version.
- Degraded and responsive behavior is checked on a narrow terminal, a short
  terminal, a terminal without colour, and a terminal whose charset cannot
  render the glyphs.
- Every installation channel is installed from and launched on at least one
  host: the release archive, `gh extension install`, and `brew install`.
- Findings are recorded as tracked tasks rather than fixed silently, and each
  one is either closed or explicitly accepted before v0.2.0 is tagged.

## Test Expectations

- Manual, on real hosts, against live GitHub. The automated gate is `task
  check`; this task is what `task check` cannot see.
- Prefer the platforms the release matrix publishes; record which of the six
  were actually covered and which were not.

## Verification Notes

- Record the hosts, terminals, token precedence paths, and selectors
  exercised, the channels installed from, and every finding with its task id.
- Record explicitly which matrix targets went untested, so the release decision
  is made against a known gap rather than an assumed one.

## Implementation Notes

- Do not tag v0.2.0 in this task. Tagging is the decision this task informs.
- The distribution channels can only be installed from once a version is
  published, so the channel bullet is satisfied against the v0.0.2 rehearsal
  release or against v0.2.0 itself after tagging, whichever the maintainer
  chooses; record which.
