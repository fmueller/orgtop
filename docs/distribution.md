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
enforced by the workflow only ever opening pull requests against the source
repository and the tap, not by any repository rule. Proving that with
provisioning fixtures is tracked separately as
`T-074-provision-and-validate-distribution-repositories`.

Every ledger and withdrawal-notice change goes through a pull request the App
opens and never approves. The workflow merges it only once its checks have
settled green and a review approving it carries a login other than the App's,
and `scripts/distribution-lib.sh` takes that decision from the reviews rather
than from GitHub's `reviewDecision`: that field stays null unless a branch
protection or ruleset requires review, so reading it would stall the release on
a default branch carrying no rule. The approval requirement is therefore the
workflow's own and holds with or without branch protection.

### Required rules on the source default branch

An approval and green checks prove the pull request, not the base it lands on.
Between the last base the workflow validated and a successful merge, another
ledger event can land on `main`, and a merge afterwards would persist a
duplicate or a contradictory event that no later check can take back. Within the
pull-request-only boundary GitHub enforces that window one way, so `main` of
`fmueller/orgtop` must carry a ruleset with both:

- a `pull_request` rule requiring at least one approving review, and
- a `required_status_checks` rule with
  `strict_required_status_checks_policy: true` ("require branches to be up to
  date before merging"), which makes GitHub reject the merge once `main`
  advances past the pull request's base.

`scripts/distribution-protected-commit.sh` reads those rules through
`repos/{repo}/rules/branches/{branch}`, which the App's `metadata: read`
permission covers, and refuses the transition before opening anything when it
cannot prove both. A repository still on classic branch protection satisfies the
same requirement through `required_pull_request_reviews` and
`required_status_checks.strict`, but reading that resource needs an
`administration: read` permission the App is not granted, so the ruleset form is
the supported configuration.

The guard additionally pins the transition to the base it validated: it re-reads
`main` immediately before merging, refuses when GitHub's base commit for the
pull request is not that commit, and refuses after a merge whose squashed commit
does not sit directly on it. Each refusal is durable evidence: it is written to
the job summary with the branch, the event, the base, and the reason.

### The tap staging branch

The formula is staged on `release/orgtop-v<version>` in the tap and merged onto
the base the workflow validated. That branch is expected to exist only between
staging and the transition, and `gh pr merge --delete-branch` removes it as part
of a first publication's merge. It removes nothing when the pull request is
already merged, which is the state a retry of a completed publication restages
into, so `scripts/distribution-tap-staging-delete.sh` requests the deletion
explicitly afterwards. Because it deletes a branch in a public repository
unattended, it deletes only a branch carrying byte-for-byte the formula that tag
rendered and changing nothing else against the default branch; anything else
fails the step rather than disappearing. A branch that is already gone is the
merge having done the work, and is success.

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
