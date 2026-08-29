---
id: T-041-close-the-distribution-channel-contract
title: Close the distribution channel contract
status: todo
priority: high
spec_ref: specs/v0.2.0.md#rg-011-distribution-channel-contract
dependencies: []
updated_at: "2026-08-29T09:22:04Z"
---

# T-041-close-the-distribution-channel-contract Close the distribution channel contract

## Description

Close RG-011 with a normative, operational contract for GitHub CLI extension and Homebrew distribution alongside release archives.

## Acceptance

- The spec names repository ownership, release production, exact platform assets, formula location/update, checksums/provenance, publication staging/order, idempotent retry, reconciliation, channel failure, and withdrawal behavior.
- Every channel redistributes the byte-identical standalone executable for each platform target without changing credentials or requiring its installer at runtime.
- GitHub CLI naming/topic constraints and trust limitations are explicitly handled.

## Test Expectations

- Add channel-parity acceptance vectors for asset names, cross-channel executable digests, versions, checksums, argument forwarding, credentials, absent tools, staged visibility, partial publication failure, retry, reconciliation, and withdrawal.

## Verification Notes

- Record the exact matrix and cross-repository responsibilities.
- Record `taskrail validate`.

## Implementation Notes

- This is specification work only and creates no release infrastructure.
- Preserve the existing archive path as a complete standalone installation route.
