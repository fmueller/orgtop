#!/usr/bin/env bash
# Assert that one tag has exactly one draft-visible release.
#
# Usage:
#   distribution-release-guard.sh --repo OWNER/NAME --tag TAG
#
# GitHub's REST releases list can omit drafts for the workflow token. The gh
# release inventory includes drafts, so it is the source used for this
# pre-publication same-version guard. A bounded inventory fails closed when it
# reaches its limit rather than allowing an unseen duplicate through.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/distribution-lib.sh
. "$script_dir/distribution-lib.sh"

usage="$0 --repo <owner/name> --tag <tag>"
repo=""
tag=""

while [ $# -gt 0 ]; do
  case "$1" in
  --repo) repo="${2-}" && shift 2 ;;
  --tag) tag="${2-}" && shift 2 ;;
  *) usage_error "$usage" ;;
  esac
done

[ -n "$repo" ] && [ -n "$tag" ] || usage_error "$usage"
require_tag "$tag"

release_inventory="$(gh release list --repo "$repo" --limit 100 --json tagName)"
release_count="$(jq 'length' <<<"$release_inventory")"
if [ "$release_count" -ge 100 ]; then
  die "the draft-visible release inventory reached its 100-release limit; refusing to publish without a complete inventory"
fi

count="$(jq --arg tag "$tag" '[.[] | select(.tagName == $tag)] | length' <<<"$release_inventory")"
if [ "$count" -ne 1 ]; then
  die "$repo carries $count releases for $tag, want exactly 1; more than one release for one tag is non-reconcilable same-version state, and none means the draft was never created"
fi

echo "guard: $repo carries exactly one release for $tag"
