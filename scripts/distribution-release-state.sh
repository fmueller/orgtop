#!/usr/bin/env bash
# Resolve what release, if any, one tag already carries in a channel.
#
# Usage:
#   distribution-release-state.sh --repo OWNER/NAME --tag TAG
#
# Writes one `state=absent|draft|published` line, in GITHUB_OUTPUT form.
#
# GoReleaser resolves an existing release for the tag through
# `use_existing_draft`, and a draft is the only thing that lookup matches. A
# retry of a tag whose release is already published therefore creates a second
# release beside it, which is the non-reconcilable same-version state the
# duplicate-release guard refuses — correctly, but only after the build, which
# leaves the retry unable to progress. Resolving the state before the build lets
# the release pipe be skipped for a published release while the create path and
# draft reuse stay exactly as they were.
#
# The inventory is the same draft-visible, bounded read the duplicate-release
# guard uses: GitHub's REST releases list can omit drafts for the workflow
# token, which would report a staged draft as absent, and a truncated inventory
# fails closed rather than resolving against an incomplete picture.
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

release_inventory="$(gh release list --repo "$repo" --limit 100 --json tagName,isDraft)"
release_count="$(jq 'length' <<<"$release_inventory")"
if [ "$release_count" -ge 100 ]; then
  die "the draft-visible release inventory reached its 100-release limit; refusing to resolve the release for $tag without a complete inventory"
fi

matches="$(jq -c --arg tag "$tag" '[.[] | select(.tagName == $tag)]' <<<"$release_inventory")"
count="$(jq 'length' <<<"$matches")"
case "$count" in
0) state=absent ;;
1) state="$(jq -r '.[0] | if .isDraft then "draft" else "published" end' <<<"$matches")" ;;
*) die "$repo carries $count releases for $tag, want at most 1; more than one release for one tag is non-reconcilable same-version state" ;;
esac

echo "state=$state"
