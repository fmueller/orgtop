# Distribution channels

How one tag reaches three channels, and what has to be provisioned for the
release workflow to do it. The normative contract is `RG-011 Distribution
Channel Contract` in `specs/v0.2.0.md`; this file records the operational
configuration that contract assumes.

## Repositories

| Repository | Role | Contents |
| --- | --- | --- |
| `fmueller/orgtop` | Canonical source. Builds every published byte. | Six archives, six raw assets, `checksums.txt`, `provenance.intoto.jsonl`, `distribution-complete.json` |
| `fmueller/gh-orgtop` | GitHub CLI extension companion. Builds nothing. | The six raw assets, a raw-only `checksums.txt`, the unchanged source provenance lines, `distribution-complete.json` |
| `fmueller/homebrew-tap` | Homebrew tap. Builds nothing. | `Formula/orgtop.rb`, which downloads the source archives and pins their SHA-256 |

`fmueller/gh-orgtop` must be named exactly `gh-orgtop`, owned by the same
publisher, and carry the `gh-extension` topic. The release workflow verifies all
three before it stages anything.

## The distribution App

Companion repository mutation uses a GitHub App installation token, never the
workflow's own `GITHUB_TOKEN`. The workflow reads `DISTRIBUTION_APP_ID` and
`DISTRIBUTION_APP_PRIVATE_KEY` from repository secrets and mints a token scoped
to the three repositories above.

Required App permissions:

| Repository | Permissions |
| --- | --- |
| `fmueller/orgtop` | metadata: read, contents: write, pull requests: write |
| `fmueller/gh-orgtop` | metadata: read, contents: write |
| `fmueller/homebrew-tap` | metadata: read, contents: write, pull requests: write |

No administration, secrets, actions, members, issues, or organization
permission is granted. A GitHub App carries one permission set across every
installation, so `pull-requests: write` is granted App-wide and is simply unused
on `fmueller/gh-orgtop`; the per-repository restriction that RG-011 describes is
enforced by branch protection and by the workflow only ever opening pull
requests against the source repository and the tap. Proving that with
provisioning fixtures is tracked separately as
`T-074-provision-and-validate-distribution-repositories`.

The App cannot approve its own pull request and cannot push to the protected
default branch. Every ledger and withdrawal-notice change therefore waits for
the repository's required checks and an independent human approval before the
workflow merges it and continues.

## The ledger

`docs/distribution-ledger.jsonl` is the durable record of every staged,
completed, and withdrawn version. One RFC 8785 canonical object per line,
terminated by exactly one LF. It is authoritative for two decisions:

- A version already recorded `completed` is permanently non-reusable.
- A version recorded `staged` with neither `completed` nor `withdrawn` is a
  partial release, and no later version may publish until it resolves.

It is written only by the release workflow's protected pull request. Never edit
it by hand: `scripts/distribution-ledger-append.sh` compares exactly, so a
reformatted line silently changes which versions may publish.

## Withdrawal

Withdrawal is a manual `workflow_dispatch` on the release workflow with
`operation: withdraw`. It merges the withdrawal event and the durable notice
under `docs/withdrawals/` through the protected pull request first, then deletes
the tap staging branch, reverts the published formula when the tap default
branch still selects the withdrawn version, and finally deletes both releases
and their tags. The notice outlives all of it and states what cannot be
withdrawn: downloads, caches, published checksums, and issued attestations.

## Guards

`scripts/distribution-*.sh` hold every deterministic decision — the platform
matrix, the canonical records, the formula, and the pre-publication
reconciliation. They run only inside the release workflow, so
`scripts/distribution-test.sh` exercises them against fixtures and
`task test:distribution` keeps that suite in the gate.
