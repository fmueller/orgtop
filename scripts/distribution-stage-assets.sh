#!/usr/bin/env bash
# Stage the exact source release assets emitted by GoReleaser.
#
# GoReleaser's release names and its on-disk artifact paths are different for
# binary-format artifacts. This helper resolves the six gh-extension records
# from dist/artifacts.json, verifies every path is a real file beneath dist, and
# copies archives, raw binaries, and metadata into a clean upload directory.
# Nothing is replaced here: the upload guard owns remote create-if-absent
# reconciliation after this helper has produced a complete local set.
#
# Usage:
#   distribution-stage-assets.sh --version VERSION --dist DIR --output DIR
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/distribution-lib.sh
. "$script_dir/distribution-lib.sh"

usage="$0 --version <version> --dist <dir> --output <dir>"
version="" dist="" output=""

while [ $# -gt 0 ]; do
  case "$1" in
  --version) version="${2-}" && shift 2 ;;
  --dist) dist="${2-}" && shift 2 ;;
  --output) output="${2-}" && shift 2 ;;
  *) usage_error "$usage" ;;
  esac
done

require_version "$version"
[ -d "$dist" ] && [ -n "$output" ] || usage_error "$usage"

dist="$(realpath -e "$dist")" || die "distribution directory '$dist' could not be resolved"
output_parent="$(realpath -e "$(dirname "$output")")" ||
  die "output parent directory '$(dirname "$output")' could not be resolved"
output="$output_parent/$(basename "$output")"
case "$output" in
"$dist" | "$dist"/*) die "output must be outside the distribution directory" ;;
esac

matrix_output=""
if ! matrix_output="$(distribution_matrix_rows "$version" 2>&1)"; then
  die "distribution matrix failed: $matrix_output"
fi

mapfile -t matrix_rows <<<"$matrix_output"
[ "${#matrix_rows[@]}" -eq 6 ] ||
  die "distribution matrix returned ${#matrix_rows[@]} rows, want exactly 6"

archives=()
raw_assets=()
for row in "${matrix_rows[@]}"; do
  IFS=$'\t' read -r _ _ archive raw _ <<<"$row"
  archives+=("$archive")
  raw_assets+=("$raw")
done

artifacts_json="$dist/artifacts.json"
[ -f "$artifacts_json" ] || die "$artifacts_json not found"

safe_file() {
  local label="$1" candidate="$2" resolved
  [ -e "$candidate" ] || die "$label '$candidate' not found"
  resolved="$(realpath -e "$candidate")" || die "$label '$candidate' could not be resolved"
  case "$resolved" in
  "$dist"/*) ;;
  *) die "$label '$candidate' resolves outside distribution directory" ;
  esac
  [ -f "$resolved" ] || die "$label '$candidate' is not a regular file"
  printf '%s\n' "$resolved"
}

artifact_path() {
  local raw="$1" path candidate
  if ! path="$(jq -er --arg name "$raw" '
    [.[] | select(.name == $name and .type == "Binary" and .extra.ID == "gh-extension")]
    | if length == 1 then .[0].path else error("expected one gh-extension artifact") end
  ' "$artifacts_json" 2>&1)"; then
    die "could not resolve raw asset '$raw' from $artifacts_json: $path"
  fi
  case "$path" in
  dist/*) candidate="$dist/${path#dist/}" ;;
  /*) candidate="$path" ;;
  *) die "raw asset '$raw' has a path outside dist: $path" ;;
  esac
  case "$path" in
  *$'\n'* | *$'\r'*) die "raw asset '$raw' has a multiline artifact path" ;;
  esac
  safe_file "raw asset '$raw'" "$candidate"
}

archive_paths=()
raw_paths=()
for archive in "${archives[@]}"; do
  archive_paths+=("$(safe_file "source archive '$archive'" "$dist/$archive")")
done
for raw in "${raw_assets[@]}"; do
  raw_paths+=("$(artifact_path "$raw")")
done
checksums_path="$(safe_file "checksums asset" "$dist/$checksums_asset")"
provenance_path="$(safe_file "provenance asset" "$dist/$provenance_asset")"

work="$(mktemp -d "${TMPDIR:-/tmp}/distribution-stage.XXXXXX")"
trap 'rm -rf "$work"' EXIT
for i in "${!archives[@]}"; do
  cp "${archive_paths[$i]}" "$work/${archives[$i]}"
done
for i in "${!raw_assets[@]}"; do
  cp "${raw_paths[$i]}" "$work/${raw_assets[$i]}"
done
cp "$checksums_path" "$work/$checksums_asset"
cp "$provenance_path" "$work/$provenance_asset"

backup=""
if [ -e "$output" ] || [ -L "$output" ]; then
  backup="$(mktemp -d "$output_parent/.distribution-stage-backup.XXXXXX")"
  if ! mv "$output" "$backup/previous"; then
    rm -rf "$backup"
    die "could not move the existing output aside"
  fi
fi
if ! mv "$work" "$output"; then
  if [ -n "$backup" ]; then
    mv "$backup/previous" "$output" || die "could not restore the existing output"
  fi
  rm -rf "$backup"
  die "could not replace the staged output"
fi
rm -rf "$backup"
trap - EXIT
echo "guard: staged the source release assets in $output"
