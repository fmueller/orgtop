#!/usr/bin/env bash
# Reconcile one staged distribution channel before anything becomes public.
#
# Usage:
#   distribution-verify.sh --dir DIR --version V --channel source|extension
#                          [--against SOURCE_DIR]
#
# DIR holds the assets of one draft, downloaded through authenticated URLs. The
# guard fails closed on an absent, extra, renamed, rebuilt, digest-divergent,
# unsorted-checksum, or uncovered artifact, which are exactly the states RG-011
# refuses to publish. It reads only local files: it publishes nothing, mutates
# nothing, and needs no credential.
#
# `--against` compares every asset in DIR to the same name in the source draft.
# Each channel is internally consistent on its own, so an extension asset left
# by an earlier partial run — carrying its own matching checksums.txt and
# provenance lines — reconciles against itself while redistributing bytes the
# source never published. That is the "extension raw mismatch" A-073 requires to
# fail closed, and only a cross-channel comparison can see it.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/distribution-lib.sh
. "$script_dir/distribution-lib.sh"

usage="$0 --dir <dir> --version <version> --channel source|extension [--against <source dir>]"
dir="" version="" channel="" against=""

while [ $# -gt 0 ]; do
  case "$1" in
  --dir) dir="${2-}" && shift 2 ;;
  --version) version="${2-}" && shift 2 ;;
  --channel) channel="${2-}" && shift 2 ;;
  --against) against="${2-}" && shift 2 ;;
  *) usage_error "$usage" ;;
  esac
done

[ -d "$dir" ] || usage_error "$usage"
require_version "$version"
case "$channel" in
source | extension) ;;
*) die "unknown channel '$channel'" ;;
esac

expected="$(artifact_names "$version" "$channel")"
expected_with_metadata="$(printf '%s\n%s\n%s\n' "$expected" "$checksums_asset" "$provenance_asset" | sort)"
present="$(cd "$dir" && ls -A | sort)"

# Exact asset set first: a missing or extra asset makes every later comparison
# describe the wrong draft.
while IFS= read -r name; do
  [ -e "$dir/$name" ] || die "the $channel draft is missing '$name'"
done <<<"$expected_with_metadata"

while IFS= read -r name; do
  grep -qxF "$name" <<<"$expected_with_metadata" || die "the $channel draft carries an unexpected asset '$name'"
done <<<"$present"

declare -A CHECKSUMS=()
declare -A PROVENANCE=()
read_checksums "$dir/$checksums_asset"
read_provenance "$dir/$provenance_asset"

# checksums.txt and the provenance bundle describe the artifacts, not
# themselves, so both cover exactly the channel's artifact set.
listed="$(printf '%s\n' "${!CHECKSUMS[@]}" | sort)"
[ "$listed" = "$expected" ] || die "$checksums_asset must list exactly the $channel artifacts$(printf '\n got:\n%s\nwant:\n%s' "$listed" "$expected")"

while IFS= read -r name; do
  [ -n "${PROVENANCE[$name]-}" ] || die "$provenance_asset covers no subject named '$name'"
done <<<"$expected"

covered="$(printf '%s\n' "${!PROVENANCE[@]}" | sort)"
[ "$covered" = "$expected" ] || die "$provenance_asset must cover exactly the $channel artifacts$(printf '\n got:\n%s\nwant:\n%s' "$covered" "$expected")"

while IFS= read -r name; do
  actual="$(digest_of_file "$dir/$name")"
  [ "$actual" = "${CHECKSUMS[$name]}" ] || die "$checksums_asset records $name as ${CHECKSUMS[$name]}, which does not match the published $actual"
  [ "$actual" = "${PROVENANCE[$name]}" ] || die "the provenance subject for $name does not match its published digest"
done <<<"$expected"

# Archive-to-raw parity: the bytes a user extracts must be the bytes the
# extension channel redistributes. Only the source draft carries archives.
if [ "$channel" = source ]; then
  work="$(mktemp -d)"
  trap 'rm -rf "$work"' EXIT

  while IFS=$'\t' read -r os arch archive raw _; do
    rm -rf "$work/extracted"
    mkdir -p "$work/extracted"
    case "$archive" in
    *.tar.gz) tar -xzf "$dir/$archive" -C "$work/extracted" ;;
    *.zip) unzip -qq "$dir/$archive" -d "$work/extracted" ;;
    esac

    executable=orgtop
    if [ "$os" = windows ]; then executable=orgtop.exe; fi
    extracted="$(find "$work/extracted" -type f -name "$executable" -print -quit)"
    [ -n "$extracted" ] || die "$archive does not contain an orgtop executable named '$executable'"

    extracted_sha="$(digest_of_file "$extracted")"
    raw_sha="${CHECKSUMS[$raw]}"
    [ "$extracted_sha" = "$raw_sha" ] || die "$raw does not match the executable inside $archive ($raw_sha vs $extracted_sha)"
  done < <("$matrix_script" "$version")
fi

# Cross-channel parity. The source draft's own checksums.txt is the reference:
# it was reconciled against the archives it came from, so comparing to it proves
# the compared channel redistributes the source bytes rather than its own.
if [ -n "$against" ]; then
  [ -d "$against" ] || usage_error "$usage"

  declare -A SOURCE_CHECKSUMS=()
  CHECKSUMS_TARGET=SOURCE_CHECKSUMS read_checksums "$against/$checksums_asset"

  while IFS= read -r name; do
    reference="${SOURCE_CHECKSUMS[$name]-}"
    [ -n "$reference" ] || die "the source draft has no entry for '$name', so the $channel draft redistributes an artifact it never published"
    [ "${CHECKSUMS[$name]}" = "$reference" ] || die "the $channel asset $name is ${CHECKSUMS[$name]}, which does not match the source $reference"
  done <<<"$expected"
fi

echo "guard: the $channel draft for $version reconciles against the RG-011 matrix"
