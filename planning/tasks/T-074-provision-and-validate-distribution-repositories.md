---
id: T-074-provision-and-validate-distribution-repositories
title: Provision and validate distribution repositories
status: todo
priority: high
spec_ref: specs/v0.2.0.md#distribution-channels
dependencies:
    - T-067-configure-multi-channel-release-workflows
updated_at: "2026-08-29T09:55:50Z"
---

# T-074-provision-and-validate-distribution-repositories Provision and validate distribution repositories

## Description

Provision and validate the external GitHub CLI extension companion and Homebrew tap required by the closed RG-011 release contract.

## Acceptance

- The named companion repository exists with the required `gh-` name, `gh-extension` topic, release permissions, and documented ownership; the named tap/formula repository also exists with documented ownership.
- Cross-repository workflow permissions and required secret names are configured and verified without recording secret values.
- A sandbox or draft-release rehearsal proves raw extension assets, formula handoff, byte-identical per-target digests, staged visibility, idempotent retry, reconciliation, and cleanup/withdrawal behavior.
- Provisioning evidence is durable and reviewable; missing external authorization remains an explicit release blocker rather than being approximated by local dry runs.

## Test Expectations

- Use non-production tags or draft/sandbox releases and remove them according to the closed cleanup policy.
- Verify repository metadata, topics, permissions, handoff endpoints, artifact digests, and retry behavior without exposing credentials.

## Verification Notes

- Record repository URLs, non-secret configuration evidence, rehearsal identifiers, digest comparisons, and cleanup results.

## Implementation Notes

- Do not publish a production v0.2.0 tag in this task.
- Keep release archives as a complete standalone path independent of GitHub CLI and Homebrew installation.
