---
id: T-118-extension-usage-name
title: Name the extension channel in its own usage text
status: completed
priority: low
spec_ref: specs/v0.2.0.md#distribution-channels
dependencies:
    - T-068-verify-distribution-channel-parity
updated_at: "2026-10-20T19:48:48Z"
---

# T-118-extension-usage-name Name the extension channel in its own usage text

## Description

The extension channel redistributes the same executable under the name
`gh-orgtop`, and the usage and flag text names the program it was invoked as.
A user who runs `gh orgtop --help` therefore reads `Usage: gh-orgtop --repo
OWNER/REPOSITORY ...`, which is neither the command they typed nor the command
the README documents.

Nothing about parity is wrong: the arguments are forwarded unchanged, the
version line reads `orgtop <version>` on both channels, and the two executables
are byte-identical. Observed while verifying channel parity in
T-068-verify-distribution-channel-parity, where the usage text was the only
difference between a direct run and a run through `gh orgtop`.

## Acceptance

- Usage and flag text presented through the extension names the command a user
  can type, rather than the raw executable name.
- A direct run is unchanged, and both channels stay one executable: the fix is
  in how the program renders its own name, not a second build or a wrapper.

## Test Expectations

- A test pinning the rendered usage for each invocation name, including the
  Windows `.exe` form.

## Verification Notes

- Record the usage text of a direct run and of a run through the extension
  executable under its published name.

## Implementation Notes

- The usage text is rendered in `internal/cli`.
- `gh` execs the extension executable directly, so `argv[0]` is the only signal
  available; the mapping from the published `gh-orgtop-<os>-<arch>` names back
  to `gh orgtop` belongs with the rendering.
- 2026-10-20T19:48:47Z: verification pass
- 2026-10-20T19:48:48Z: Reviewed and verified after strict red-green TDD; same executable renders gh orgtop for extension usage. Final task check and fresh disposition verification pass; focused evidence in docs/testing/v0.2.0-readiness.md.
