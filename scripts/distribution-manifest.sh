#!/usr/bin/env bash
# Emit the canonical RG-011 `distribution-complete.json` for a published tag.
#
# Usage:
#   distribution-manifest.sh --tag T --commit SHA1 --workflow-commit SHA1
#                            --tap-commit SHA1 --checksums FILE --provenance FILE
#
# The manifest is written to stdout as RFC 8785 canonical bytes with no BOM and
# no trailing newline, because the same bytes are added as a release asset to
# both channels and compared on every retry.
#
# Workflow identity is the stable workflow path plus the commit of the workflow
# file, never a run id or attempt: a retry of the same tag must produce the same
# manifest bytes.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/distribution-lib.sh
. "$script_dir/distribution-lib.sh"

usage="$0 --tag <tag> --commit <sha> --workflow-commit <sha> --tap-commit <sha> --checksums <file> --provenance <file>"
tag="" commit="" workflow_commit="" tap_commit="" checksums_file="" provenance_file=""

while [ $# -gt 0 ]; do
  case "$1" in
  --tag) tag="${2-}" && shift 2 ;;
  --commit) commit="${2-}" && shift 2 ;;
  --workflow-commit) workflow_commit="${2-}" && shift 2 ;;
  --tap-commit) tap_commit="${2-}" && shift 2 ;;
  --checksums) checksums_file="${2-}" && shift 2 ;;
  --provenance) provenance_file="${2-}" && shift 2 ;;
  *) usage_error "$usage" ;;
  esac
done

require_tag "$tag"
require_sha1 "the source commit" "$commit"
require_sha1 "the workflow commit" "$workflow_commit"
require_sha1 "the tap commit" "$tap_commit"

version="${tag#v}"

declare -A CHECKSUMS=()
declare -A PROVENANCE=()
read_checksums "$checksums_file"
read_provenance "$provenance_file"

# artifact_record <name> emits the {name, sha256, provenance_subject_sha256}
# object for one published artifact, failing closed when the artifact is absent
# from either metadata asset or when the two disagree.
artifact_record() {
  local name="$1" published subject
  published="${CHECKSUMS[$name]-}"
  [ -n "$published" ] || die "$checksums_file has no entry for '$name'"
  subject="${PROVENANCE[$name]-}"
  [ -n "$subject" ] || die "$provenance_file has no subject for '$name'"
  [ "$published" = "$subject" ] || die "the provenance subject digest for '$name' does not match its published digest"

  jq -cn --arg name "$name" --arg sha "$published" '{name:$name,sha256:$sha,provenance_subject_sha256:$sha}'
}

# One target object per matrix row, in matrix order; `jq -s` slurps them into
# the `targets` array RG-011 requires that order in.
while IFS=$'\t' read -r os arch archive raw _; do
  # Resolved before use: a `die` inside a command substitution would end only
  # that subshell, so the record has to fail the assignment instead.
  archive_json="$(artifact_record "$archive")"
  raw_json="$(artifact_record "$raw")"

  jq -cn --arg os "$os" --arg arch "$arch" \
    --argjson archive "$archive_json" --argjson raw "$raw_json" \
    '{os:$os,architecture:$arch,archive:$archive,raw:$raw,
      executable_sha256:$raw.sha256}'
done < <("$matrix_script" "$version") |
  jq -cs --arg tag "$tag" --arg commit "$commit" --arg workflow_commit "$workflow_commit" \
    --arg tap_commit "$tap_commit" --arg source_repository "$source_repository" \
    --arg extension_repository "$extension_repository" --arg tap_repository "$tap_repository" \
    --arg formula_path "$formula_path" --arg workflow_path "$release_workflow_path" \
    '{schema_version:1,
      source:{repository:$source_repository,tag:$tag,commit:$commit,
        release_url:("https://github.com/" + $source_repository + "/releases/tag/" + $tag),
        workflow:{path:$workflow_path,commit:$workflow_commit}},
      extension:{repository:$extension_repository,tag:$tag,
        release_url:("https://github.com/" + $extension_repository + "/releases/tag/" + $tag)},
      tap:{repository:$tap_repository,formula_path:$formula_path,commit:$tap_commit},
      targets:.}' | canonical
