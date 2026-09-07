#!/usr/bin/env bash
# Render the durable withdrawal notice for one version.
#
# Usage:
#   distribution-notice.sh --version V --state incomplete|completed
#                          --reason TEXT --staged-sha SHA256 [--replacement V]
#
# The notice is printed to stdout and is committed to `docs/withdrawals/v<V>.md`
# on the canonical default branch by the same protected pull request that
# appends the withdrawal event. It outlives the deleted releases and tags, which
# is why the ledger's notice_url points at the default branch rather than at a
# release.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/distribution-lib.sh
. "$script_dir/distribution-lib.sh"

usage="$0 --version <version> --state incomplete|completed --reason <text> --staged-sha <sha256> [--replacement <version>]"
version="" state="" reason="" staged_sha="" replacement=""

while [ $# -gt 0 ]; do
  case "$1" in
  --version) version="${2-}" && shift 2 ;;
  --state) state="${2-}" && shift 2 ;;
  --reason) reason="${2-}" && shift 2 ;;
  --staged-sha) staged_sha="${2-}" && shift 2 ;;
  --replacement) replacement="${2-}" && shift 2 ;;
  *) usage_error "$usage" ;;
  esac
done

require_version "$version"
require_sha256 "the staged digest" "$staged_sha"
case "$state" in
incomplete | completed) ;;
*) die "publication state must be 'incomplete' or 'completed', got '$state'" ;;
esac
require_reason "$reason"
[ -z "$replacement" ] || require_version "$replacement"

# Affected channels follow from how far publication got. An incomplete
# publication may never have reached the tap, so the notice names the channels
# the version could have reached rather than claiming each one published.
if [ "$state" = completed ]; then
  channels="the source release archives, the GitHub CLI extension release, and the Homebrew tap formula"
  reuse="Version $version is permanently reserved. A fix ships under a later semantic version."
else
  channels="the source release archives and, where staging reached them, the GitHub CLI extension release and the Homebrew tap staging branch"
  reuse="Version $version may only be staged again at the identical source commit with all twelve original artifact digests. Different bytes require a new semantic version."
fi

cat <<NOTICE
# Withdrawal notice: v$version

- Version: $version
- Publication state at withdrawal: $state
- Staged ledger event digest: \`$staged_sha\`
- Affected channels: $channels
$(if [ -n "$replacement" ]; then printf -- '- Replacement version: %s\n' "$replacement"; fi)
## Reason

$reason

## What was withdrawn

The v$version releases and tags were deleted from
\`$source_repository\` and \`$extension_repository\`, and the
\`$formula_path\` formula in \`$tap_repository\` no longer selects this
version.

## What cannot be withdrawn

Executables already downloaded, package and build caches holding them, published
checksums, and issued build attestations for v$version cannot be revoked. A host
that installed v$version before this notice keeps running it until it upgrades.

## Reuse

$reuse
NOTICE
