#!/usr/bin/env bash
# Attach the completion manifest to one release with create-or-compare semantics.
#
# Usage:
#   distribution-manifest-asset.sh --tag T --repo OWNER/NAME --manifest FILE
#
# Both channels carry the byte-identical manifest. An asset already attached is
# never overwritten: it is downloaded and compared, so a retry that finds one
# manifest present adds only the missing one. A divergent manifest fails closed
# rather than replacing published bytes.
#
# Requires GH_TOKEN with contents write on the named repository.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/distribution-lib.sh
. "$script_dir/distribution-lib.sh"

usage="$0 --tag <tag> --repo <owner/name> --manifest <file>"
tag="" repo="" manifest=""

while [ $# -gt 0 ]; do
  case "$1" in
  --tag) tag="${2-}" && shift 2 ;;
  --repo) repo="${2-}" && shift 2 ;;
  --manifest) manifest="${2-}" && shift 2 ;;
  *) usage_error "$usage" ;;
  esac
done

require_tag "$tag"
[ -n "$repo" ] && [ -f "$manifest" ] || usage_error "$usage"

asset=distribution-complete.json
expected="$(digest_of_file "$manifest")"

if gh release view "$tag" --repo "$repo" --json assets --jq '.assets[].name' | grep -qxF "$asset"; then
  published="$(mktemp)"
  trap 'rm -f "$published"' EXIT
  gh release download "$tag" --repo "$repo" --pattern "$asset" --output "$published" --clobber
  actual="$(digest_of_file "$published")"
  [ "$actual" = "$expected" ] || die "$repo already carries a different $asset ($actual, not $expected)"
  echo "guard: $repo already carries the exact $asset"
  exit 0
fi

staged="$(dirname "$manifest")/$asset"
[ "$staged" = "$manifest" ] || cp "$manifest" "$staged"
gh release upload "$tag" "$staged" --repo "$repo"
echo "guard: added $asset to $repo"
