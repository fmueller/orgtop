#!/usr/bin/env bash
# Upload one release draft's assets with RG-011 create-if-absent semantics.
#
# Usage:
#   distribution-upload-assets.sh --tag T --repo OWNER/NAME --dir DIR
#                                  --channel source|extension
#
# DIR contains the build output for one channel. Expected assets already
# attached to the draft are downloaded and compared byte for byte. Missing
# assets are uploaded without `--clobber`; no remote asset is deleted or
# overwritten. The comparison phase completes before any upload, so a
# digest-divergent existing asset fails closed without adding other missing
# assets. A transient upload failure may still leave a partial draft, which a
# later invocation reconciles from the exact bytes already present.
#
# Requires GH_TOKEN with contents write on the named repository.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/distribution-lib.sh
. "$script_dir/distribution-lib.sh"

usage="$0 --tag <tag> --repo <owner/name> --dir <dir> --channel source|extension"
tag="" repo="" dir="" channel=""

while [ $# -gt 0 ]; do
  case "$1" in
  --tag) tag="${2-}" && shift 2 ;;
  --repo) repo="${2-}" && shift 2 ;;
  --dir) dir="${2-}" && shift 2 ;;
  --channel) channel="${2-}" && shift 2 ;;
  *) usage_error "$usage" ;;
  esac
done

require_tag "$tag"
[ -n "$repo" ] && [ -d "$dir" ] || usage_error "$usage"
case "$channel" in
source | extension) ;;
*) die "unknown channel '$channel'" ;;
esac

version="${tag#v}"
artifact_output=""
if ! artifact_output="$(artifact_names "$version" "$channel")"; then
  die "could not determine the expected $channel assets"
fi
artifacts=()
mapfile -t artifacts <<<"$artifact_output"
expected=("${artifacts[@]}" "$checksums_asset" "$provenance_asset")

for name in "${expected[@]}"; do
  [ -f "$dir/$name" ] || die "the $channel build output is missing '$name'"
done

# A release draft carrying an unexpected name is already contradictory. Refuse
# before the upload phase rather than leaving a larger partial state behind.
# Capture the command's status explicitly: process substitution would otherwise
# let `mapfile` succeed after a failed GitHub API request and make the draft look
# empty or partial.
attached_output=""
if ! attached_output="$(gh release view "$tag" --repo "$repo" --json assets --jq '.assets[].name')"; then
  die "could not read $repo asset inventory"
fi
attached=()
if [ -n "$attached_output" ]; then
  mapfile -t attached <<<"$attached_output"
fi

contains_name() {
  local needle="$1"
  shift
  local candidate
  for candidate in "$@"; do
    [ "$candidate" = "$needle" ] && return 0
  done
  return 1
}

for name in "${attached[@]}"; do
  [ -n "$name" ] || continue
  contains_name "$name" "${expected[@]}" ||
    die "$repo release carries unexpected asset '$name'"
done

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
missing=()

# Preflight every existing asset first. In particular, do not upload the first
# missing name and discover a divergent existing name later in the same run.
for name in "${expected[@]}"; do
  local_file="$dir/$name"
  if contains_name "$name" "${attached[@]}"; then
    expected_sha="$(digest_of_file "$local_file")"
    published_file="$work/$name"
    if ! gh release download "$tag" --repo "$repo" --pattern "$name" --output "$published_file"; then
      die "could not download existing $repo asset '$name' for comparison"
    fi
    actual_sha="$(digest_of_file "$published_file")"
    if [ "$actual_sha" != "$expected_sha" ]; then
      die "$repo asset '$name' digest mismatch: expected $expected_sha, actual $actual_sha; refusing to overwrite"
    fi
    echo "guard: reusing the exact $repo asset $name"
  else
    missing+=("$name")
  fi
done

# No --clobber: GitHub rejects a name collision rather than replacing the
# asset. A concurrent writer therefore fails closed instead of mutating bytes.
for name in "${missing[@]}"; do
  if ! gh release upload "$tag" "$dir/$name" --repo "$repo"; then
    die "could not add missing $repo asset '$name' without overwriting an existing asset"
  fi
  echo "guard: added $repo asset $name"
done

echo "guard: reconciled the $channel release draft for $version"
