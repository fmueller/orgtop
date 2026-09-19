---
id: T-074-provision-and-validate-distribution-repositories
title: Provision and validate distribution repositories
status: blocked
priority: high
spec_ref: specs/v0.2.0.md#distribution-channels
dependencies:
    - T-067-configure-multi-channel-release-workflows
    - T-105-complete-the-protected-ledger-step-when-its-event
    - T-107-upload-release-assets-create-if-absent-so-a-retry
updated_at: "2026-09-19T09:36:04Z"
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
- 2026-09-07T11:47:13Z: External authorization missing. fmueller/gh-orgtop now exists with the gh-extension topic and fmueller/homebrew-tap exists, but the distribution GitHub App is not created and gh secret list -R fmueller/orgtop returns no secrets, so DISTRIBUTION_APP_ID and DISTRIBUTION_APP_PRIVATE_KEY are unconfigured (release.yml:112-113, :527-528). App creation is a browser-only flow no agent can perform, and the draft-release rehearsal needs its installation token. RG-011 requires this to stay an explicit release blocker rather than be approximated by local dry runs. Setup steps recorded at ~/Downloads/orgtop_gh_setup_T074.md.
- 2026-09-18T00:15Z: Provisioning complete and verified. Distribution App id
  4982629 (`orgtop-distribution`), installation 162588282, permissions exactly
  `contents: write`, `metadata: read`, `pull_requests: write` and no others, no
  webhook events, `repository_selection: selected` over exactly
  `fmueller/orgtop`, `fmueller/gh-orgtop`, `fmueller/homebrew-tap` (confirmed
  through `/installation/repositories`, total_count 3). `DISTRIBUTION_APP_ID`
  and `DISTRIBUTION_APP_PRIVATE_KEY` are set on `fmueller/orgtop`; no secret
  value was read or recorded.
- 2026-09-18T00:15Z: v0.0.1 rehearsal, runs 35274418650, 35275507747,
  35276965201 (publish) and 35279448446, 35279637706, 35279773653, 35280447912,
  35280683027 (withdraw). Proven: companion guard replay, App token minting,
  source and extension drafts created and reconciled, six raw executables
  byte-identical across independent builds, archive-to-raw equality, formula
  staged on `release/orgtop-v0.0.1` with the tap default branch untouched,
  staged visibility (no asset anonymously downloadable at any point), durable
  staged ledger event through protected pull request #6, reconciliation failing
  closed on a divergent retry, and the complete withdrawal: notice merged
  through #7, tap staging branch deleted, both drafts and the tag deleted,
  notice outliving them at `docs/withdrawals/v0.0.1.md`, ledger recording
  `staged` then `withdrawn` with `publication_state: incomplete` and
  `tap_commit: null`.
- 2026-09-18T00:15Z: Five release-path defects found and fixed, each with a
  test that fails without its fix: provenance assembled from the undecoded
  Sigstore bundle (653e777); tap staging assuming an existing `Formula`
  directory, `commit -a` unable to stage the first untracked formula, and
  `git status --porcelain` collapsing the new directory (7be99b1); archives not
  reproducible across a retry because no `mod_timestamp` was pinned (8f3c137);
  withdrawal decoding an absent tap formula (f3d44d7); withdrawal deleting a
  draft release by tag, which GitHub's get-release-by-tag endpoint cannot see
  (125da34). Every one was invisible to `task check` and the fixture suite, and
  the reproducibility defect would have made the first v0.2.0 retry
  unrecoverable.
- 2026-09-18T00:15Z: Publication itself is not yet rehearsed. v0.0.1 could not
  be completed because its staged event pins `source_commit` 7be99b1, which
  predates the reproducibility fix, so the rehearsal continues on v0.0.2 at a
  commit carrying all five fixes. Still unproven: the publication transitions,
  the completion manifests, the completed ledger event, final reconciliation,
  and a retry that reconciles successfully rather than failing closed.
- 2026-09-18T00:15Z: T-105 records the one defect this task could not carry:
  the protected-commit poll never re-checks whether its event already landed,
  so an out-of-band merge strands the step until its bound.
- 2026-09-18T01:00Z: v0.0.2 retry rehearsal, runs 35281659191 and 35282570875,
  two attempts each. Proven: the tap staging fixes hold (`create mode 100644
  Formula/orgtop.rb`, 50 insertions, on a tap carrying no Formula directory),
  `use_existing_draft` makes a retry reuse its draft rather than create a
  second release for the tag (draft 391100794 reused on attempt 2), and archive
  reproducibility now survives a re-stamped working tree. Two further defects
  found and fixed: archives still diverged with `mod_timestamp` alone, because
  an archive records metadata for every member and a checkout restamps
  LICENSE and README.md (1d8e658); and a retry created a second draft for the
  same tag, because a draft is invisible to get-release-by-tag (1d8e658, with a
  guard refusing a tag that carries anything but exactly one release).
- 2026-09-18T01:00Z: The last open acceptance item is a retry that resumes and
  reconciles rather than one that fails closed. It is blocked on T-107:
  GoReleaser's publish step cannot upload create-if-absent, so a retry against
  a draft that already holds its assets fails with `already_exists` on each,
  and the only alternative it offers overwrites published bytes, which RG-011
  forbids. A first publication is unaffected. v0.0.2 was never recorded in the
  ledger and is still a free version number; every draft, tag, staging branch,
  and staged pull request from both rehearsals is removed.
- 2026-09-19T09:36:04Z: T-074 final rehearsal is blocked until T-107's retry-safe asset upload and T-105's protected-ledger poll fix are complete; T-074 remains incomplete
- 2026-09-19T09:36:04Z: T-105 and T-107 are implementation prerequisites of the final T-074 rehearsal; both follow the completed T-067 release workflow configuration.
