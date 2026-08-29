---
id: T-068-verify-distribution-channel-parity
title: Verify distribution channel parity
status: todo
priority: medium
spec_ref: specs/v0.2.0.md#distribution-channels
dependencies:
    - T-069-integrate-the-closed-v0-2-0-binary-flow
    - T-074-provision-and-validate-distribution-repositories
updated_at: "2026-08-29T09:22:04Z"
---

# T-068-verify-distribution-channel-parity Verify distribution channel parity

## Description

Build and verify the closed channel artifact matrix and installation contracts without performing a live release.

## Acceptance

- Archive, GitHub CLI extension, and Homebrew artifacts redistribute matching per-target executable digests, install/report one version, preserve arguments, and use unchanged credential precedence as required by RG-011.
- Raw extension executable names, Windows suffixes, checksums, provenance, companion metadata, and formula contents match the closed matrix.
- A machine without GitHub CLI or Homebrew retains a complete archive installation path.

## Test Expectations

- Run snapshot/dry release of the integrated binary, cross-build, cross-channel digest checks, artifact-name checks, local install smokes, argument-forwarding fixtures, and partial-channel failure/retry simulation.

## Verification Notes

- Record the complete artifact/checksum matrix and command exit statuses.

## Implementation Notes

- Do not publish tags or external repositories during task verification.
