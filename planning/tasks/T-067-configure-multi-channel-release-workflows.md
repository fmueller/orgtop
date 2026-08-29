---
id: T-067-configure-multi-channel-release-workflows
title: Configure multi-channel release workflows
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#distribution-channels
dependencies:
    - T-043-guard-the-v0-2-0-toolchain-and-dependency-baseline
updated_at: "2026-08-29T09:22:04Z"
---

# T-067-configure-multi-channel-release-workflows Configure multi-channel release workflows

## Description

Implement the closed release automation for existing archives, GitHub CLI extension assets, and Homebrew formula updates from one tag.

## Acceptance

- Repository handoff, platform matrix, exact asset names, formula update, checksums, provenance, credentials, and failure behavior implement RG-011.
- All channels derive the same version and standalone executable source.
- A failed channel fails publication according to the closed contract rather than silently leaving versions divergent.

## Test Expectations

- Extend release configuration tests and dry-run fixtures to assert every platform name, checksum/provenance output, formula version, and failure path without publishing.

## Verification Notes

- Record release-check and dry-run artifact matrices.

## Implementation Notes

- Preserve archives and standalone execution.
- Keep workflow path-lane mirrors exact when changing repository workflow files.
