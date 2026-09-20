#!/usr/bin/env bash
# Assert that the companion repositories can actually receive a release.
#
# Usage:
#   distribution-companion-guard.sh --extension OWNER/NAME --tap OWNER/NAME
#
# Ownership, the gh- prefix, and the gh-extension topic are what GitHub CLI
# discovery needs. They are not publisher proof, and the documentation says so;
# they are checked because staging into the wrong repository is not recoverable
# by retry.
#
# Emptiness is checked for the same reason, one step earlier than it announces
# itself. GitHub accepts a draft release in a repository that carries no commit
# but refuses to publish one: undrafting resolves a target commitish, and an
# empty repository has none. Unchecked, the failure arrives at the extension
# publish transition with HTTP 422 "Repository is empty" -- after the source
# release is already public, leaving an explicitly partial release that no
# retry can reconcile until the repository is provisioned. Here it is a
# pre-build guard, and nothing has been published when it fails.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/distribution-lib.sh
. "$script_dir/distribution-lib.sh"

usage="$0 --extension <owner/name> --tap <owner/name>"
extension=""
tap=""

while [ $# -gt 0 ]; do
  case "$1" in
  --extension) extension="${2-}" && shift 2 ;;
  --tap) tap="${2-}" && shift 2 ;;
  *) usage_error "$usage" ;;
  esac
done

[ -n "$extension" ] && [ -n "$tap" ] || usage_error "$usage"

# require_commit <owner/name> fails unless the repository carries at least one
# commit. GitHub answers the commit listing of an empty repository with HTTP
# 409 rather than an empty array, so the request failing is itself the signal.
require_commit() {
  local repo="$1"
  if ! gh api "repos/$repo/commits?per_page=1" >/dev/null 2>&1; then
    die "$repo carries no commit; GitHub cannot publish a release in an empty repository, and an unpublishable companion must not be discovered after the source release is public"
  fi
}

extension_json="$(gh api "repos/$extension")"
extension_owner="$(jq -r '.owner.login' <<<"$extension_json")"
extension_name="$(jq -r '.name' <<<"$extension_json")"
extension_topics="$(jq -r '(.topics // []) | index("gh-extension") != null' <<<"$extension_json")"

[ "$extension_owner" = fmueller ] ||
  die "$extension must be owned by fmueller; got: $extension_owner"
[ "$extension_name" = gh-orgtop ] ||
  die "$extension must be named gh-orgtop for GitHub CLI discovery; got: $extension_name"
[ "$extension_topics" = true ] ||
  die "$extension must carry the gh-extension topic for GitHub CLI discovery"
require_commit "$extension"

tap_json="$(gh api "repos/$tap")"
tap_owner="$(jq -r '.owner.login' <<<"$tap_json")"
tap_name="$(jq -r '.name' <<<"$tap_json")"

[ "$tap_owner" = fmueller ] ||
  die "$tap must be owned by fmueller; got: $tap_owner"
[ "$tap_name" = homebrew-tap ] ||
  die "$tap must be named homebrew-tap; got: $tap_name"
require_commit "$tap"

echo "guard: $extension and $tap are provisioned and can receive a release"
