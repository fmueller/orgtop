#!/usr/bin/env bash
# Emit one canonical RG-011 distribution-ledger event.
#
# Usage:
#   distribution-ledger.sh staged    --version V --tag T --commit SHA1
#                                    --artifacts CHECKSUMS --checksums-sha SHA256
#                                    --provenance-sha SHA256 [--digest]
#   distribution-ledger.sh completed --version V --staged-sha SHA256
#                                    --source-manifest-sha SHA256
#                                    --extension-manifest-sha SHA256
#                                    --tap-commit SHA1 [--digest]
#   distribution-ledger.sh withdrawn --version V --staged-sha SHA256
#                                    --state incomplete|completed --reason TEXT
#                                    --tap-commit SHA1|"" [--digest]
#
# The event is written to stdout as RFC 8785 canonical bytes with no trailing
# newline; the appending step adds exactly one LF. `--digest` prints the SHA-256
# of those same bytes instead, which is the digest later events reference and
# which therefore must exclude that LF.
#
# The withdrawal notice path and URL are derived here rather than accepted as
# arguments, so a record can never bind a notice the protected pull request did
# not create at the exact canonical location.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/distribution-lib.sh
. "$script_dir/distribution-lib.sh"

usage="$0 staged|completed|withdrawn --version <version> ..."
event="${1:-}"
shift || true

version="" tag="" commit="" artifacts="" checksums_sha="" provenance_sha=""
staged_sha="" source_manifest_sha="" extension_manifest_sha="" tap_commit=""
state="" reason="" digest_only=no

while [ $# -gt 0 ]; do
  case "$1" in
  --version) version="${2-}" && shift 2 ;;
  --tag) tag="${2-}" && shift 2 ;;
  --commit) commit="${2-}" && shift 2 ;;
  --artifacts) artifacts="${2-}" && shift 2 ;;
  --checksums-sha) checksums_sha="${2-}" && shift 2 ;;
  --provenance-sha) provenance_sha="${2-}" && shift 2 ;;
  --staged-sha) staged_sha="${2-}" && shift 2 ;;
  --source-manifest-sha) source_manifest_sha="${2-}" && shift 2 ;;
  --extension-manifest-sha) extension_manifest_sha="${2-}" && shift 2 ;;
  --tap-commit) tap_commit="${2-}" && shift 2 ;;
  --state) state="${2-}" && shift 2 ;;
  --reason) reason="${2-}" && shift 2 ;;
  --digest) digest_only=yes && shift ;;
  *) usage_error "$usage" ;;
  esac
done

[ -n "$version" ] || usage_error "$0 $event --version <version> ..."
require_version "$version"

# emit writes the record, or its digest, from the canonical bytes on stdin.
emit() {
  local record
  record="$(canonical)"
  if [ "$digest_only" = yes ]; then
    printf '%s' "$record" | digest_of_stdin
    return
  fi
  printf '%s' "$record"
}

case "$event" in
staged)
  require_tag "$tag"
  [ "$tag" = "v$version" ] || die "tag '$tag' does not match version '$version'"
  require_sha1 "the source commit" "$commit"
  require_sha256 "the checksums digest" "$checksums_sha"
  require_sha256 "the provenance digest" "$provenance_sha"

  declare -A CHECKSUMS=()
  read_checksums "$artifacts"

  # Exactly the twelve build artifacts, and only those: the two metadata assets
  # are hashed by their own fields, not listed as artifacts.
  expected="$(artifact_names "$version" source)"
  listed="$(printf '%s\n' "${!CHECKSUMS[@]}" | grep -v -e "^$checksums_asset\$" -e "^$provenance_asset\$" | sort)"
  [ "$listed" = "$expected" ] || die "the staged artifacts must be exactly 12 matrix artifacts$(printf '\n got:\n%s\nwant:\n%s' "$listed" "$expected")"

  # One object per artifact, already in bytewise name order; `jq -s` slurps
  # them into the `artifacts` array that order is required in.
  while IFS= read -r name; do
    jq -cn --arg name "$name" --arg sha "${CHECKSUMS[$name]}" '{name:$name,sha256:$sha}'
  done <<<"$expected" |
    jq -cs --arg version "$version" --arg tag "$tag" --arg commit "$commit" \
      --arg repository "$source_repository" --arg checksums "$checksums_sha" --arg provenance "$provenance_sha" \
      '{schema_version:1,event:"staged",version:$version,source_repository:$repository,
        source_tag:$tag,source_commit:$commit,artifacts:.,
        checksums_sha256:$checksums,provenance_sha256:$provenance}' | emit
  ;;

completed)
  require_sha256 "the staged digest" "$staged_sha"
  require_sha256 "the source manifest digest" "$source_manifest_sha"
  require_sha256 "the extension manifest digest" "$extension_manifest_sha"
  require_sha1 "the tap commit" "$tap_commit"

  jq -cn --arg version "$version" --arg staged "$staged_sha" --arg source "$source_manifest_sha" \
    --arg extension "$extension_manifest_sha" --arg tap "$tap_commit" \
    '{schema_version:1,event:"completed",version:$version,staged_sha256:$staged,
      source_manifest_sha256:$source,extension_manifest_sha256:$extension,tap_commit:$tap}' | emit
  ;;

withdrawn)
  require_sha256 "the staged digest" "$staged_sha"
  case "$state" in
  incomplete | completed) ;;
  *) die "publication state must be 'incomplete' or 'completed', got '$state'" ;;
  esac
  require_reason "$reason"

  # A formula that was never published leaves tap_commit null; a formula that
  # was published records the commit that published it, never the later revert.
  if [ -n "$tap_commit" ]; then
    require_sha1 "the pre-withdrawal tap commit" "$tap_commit"
  fi

  notice_path="docs/withdrawals/v$version.md"
  jq -cn --arg version "$version" --arg staged "$staged_sha" --arg state "$state" \
    --arg reason "$reason" --arg tap "$tap_commit" --arg path "$notice_path" \
    --arg url "https://github.com/$source_repository/blob/main/$notice_path" \
    '{schema_version:1,event:"withdrawn",version:$version,staged_sha256:$staged,
      publication_state:$state,reason:$reason,
      tap_commit:(if $tap == "" then null else $tap end),
      notice_path:$path,notice_url:$url}' | emit
  ;;

*)
  usage_error "$usage"
  ;;
esac
