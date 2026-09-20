#!/usr/bin/env bash
# Delete the release and the git tag of a withdrawn version in every channel
# repository.
#
# Usage:
#   distribution-withdraw-delete.sh --version X.Y.Z --repository OWNER/NAME...
#
# The caller supplies one token, GH_TOKEN, and it must be able to delete in
# every repository named: the distribution App holds `contents: write` on all
# three, while the default workflow token is deliberately read-only. Reaching
# for the workflow token here is what made the source delete 403, and because
# that failure aborted the loop the extension repository was never reached,
# leaving a version the ledger already records as withdrawn published in both
# channels.
#
# Every repository is therefore attempted, whatever the ones before it did, and
# the ones that could not be cleaned are named together at the end. A partial
# withdrawal that reports success is the state the withdrawal exists to end, so
# the deletions are verified and a surviving release or tag fails closed.
set -uo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/distribution-lib.sh
. "$script_dir/distribution-lib.sh"

usage="$0 --version <x.y.z> --repository <owner/name> [--repository <owner/name>]..."
version=""
repositories=()

while [ $# -gt 0 ]; do
  case "$1" in
  --version) version="${2-}" && shift 2 ;;
  --repository) repositories+=("${2-}") && shift 2 ;;
  *) usage_error "$usage" ;;
  esac
done

[ -n "$version" ] || usage_error "$usage"
[ "${#repositories[@]}" -gt 0 ] || usage_error "$usage"
require_version "$version"

tag="v${version}"

# release_id <owner/name> prints the id of the release carrying the tag, or
# nothing. It reads a listing rather than the get-release-by-tag endpoint,
# which does not see a draft: deleting by tag name resolves nothing for a
# version withdrawn while still staged, reports success, takes the git tag with
# it, and leaves the draft and its assets behind.
release_id() {
  local repo="$1"
  gh api --paginate "repos/$repo/releases" \
    --jq ".[] | select(.tag_name == \"${tag}\") | .id" | head -n1
}

# clean_repository <owner/name> removes the release and the tag and verifies
# both are gone. It returns non-zero rather than exiting, so one repository
# refusing does not decide anything for the next.
clean_repository() {
  local repo="$1" id

  if ! id="$(release_id "$repo")"; then
    printf 'guard: could not read the releases of %s\n' "$repo" >&2
    return 1
  fi

  if [ -n "$id" ] && ! gh api -X DELETE "repos/$repo/releases/$id" >/dev/null; then
    printf 'guard: could not delete the %s release of %s\n' "$tag" "$repo" >&2
    return 1
  fi

  # The tag is its own ref, and a draft never had one. Delete it only when it
  # exists, so a draft-only withdrawal is not a failure, and so a withdrawal
  # retried after a partial one is not a failure either.
  if gh api "repos/$repo/git/ref/tags/${tag}" >/dev/null 2>&1 &&
    ! gh api -X DELETE "repos/$repo/git/refs/tags/${tag}" >/dev/null; then
    printf 'guard: could not delete the %s tag of %s\n' "$tag" "$repo" >&2
    return 1
  fi

  # Delete-if-present, then verify. A delete answered with success that leaves
  # the release in place would otherwise be reported as a complete withdrawal.
  if ! id="$(release_id "$repo")"; then
    printf 'guard: could not reread the releases of %s\n' "$repo" >&2
    return 1
  fi
  if [ -n "$id" ]; then
    printf 'guard: the %s release still exists in %s\n' "$tag" "$repo" >&2
    return 1
  fi
  if gh api "repos/$repo/git/ref/tags/${tag}" >/dev/null 2>&1; then
    printf 'guard: the %s tag still exists in %s\n' "$tag" "$repo" >&2
    return 1
  fi

  printf 'guard: %s carries no %s release or tag\n' "$repo" "$tag"
}

unclean=()
for repository in "${repositories[@]}"; do
  clean_repository "$repository" || unclean+=("$repository")
done

if [ "${#unclean[@]}" -gt 0 ]; then
  die "the ${tag} withdrawal could not clean: ${unclean[*]}"
fi
