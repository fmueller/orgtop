#!/usr/bin/env bash
# Delete the tap staging branch a completed publication leaves behind.
#
# Usage:
#   distribution-tap-staging-delete.sh --repository OWNER/NAME --branch REF \
#     --base REF --formula FILE
#
# The publication deletes its staging branch through `gh pr merge
# --delete-branch`. That deletes nothing when the pull request is already
# merged, which is the state a retry of a completed publication restages into:
# the branch is recreated, the merge reports `! Pull request ... was already
# merged`, the step exits 0, and a public tap keeps a dangling release branch.
# RG-011 expects the staging branch to exist only between staging and the
# transition, so the deletion is requested explicitly here.
#
# The branch is public and this deletes it unattended, so it deletes only what
# this tag staged: the branch must carry byte-for-byte the formula the caller
# rendered and must change nothing else against the default branch. Anything
# else is somebody else's branch and fails closed rather than disappearing
# silently. Finding no branch at all is the first publication's own merge
# having already done this, and is success.
set -uo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/distribution-lib.sh
. "$script_dir/distribution-lib.sh"

usage="$0 --repository <owner/name> --branch <ref> --base <ref> --formula <file>"
repository=""
branch=""
base=""
formula=""

while [ $# -gt 0 ]; do
  case "$1" in
  --repository) repository="${2-}" && shift 2 ;;
  --branch) branch="${2-}" && shift 2 ;;
  --base) base="${2-}" && shift 2 ;;
  --formula) formula="${2-}" && shift 2 ;;
  *) usage_error "$usage" ;;
  esac
done

[ -n "$repository" ] || usage_error "$usage"
[ -n "$branch" ] || usage_error "$usage"
[ -n "$base" ] || usage_error "$usage"
[ -n "$formula" ] || usage_error "$usage"
[ -f "$formula" ] || die "no rendered formula at $formula"

if ! gh api "repos/$repository/git/ref/heads/${branch}" >/dev/null 2>&1; then
  printf 'guard: the tap staging branch %s is already gone\n' "$branch"
  exit 0
fi

# The branch is read through the API rather than a clone: the comparison is
# against the bytes the caller published, so the only thing needed from the tap
# is the blob and the file list.
if ! encoded="$(gh api "repos/$repository/contents/Formula/orgtop.rb?ref=${branch}" --jq '.content' 2>/dev/null)" ||
  [ -z "$encoded" ]; then
  die "the tap staging branch ${branch} carries no formula to match against"
fi
if [ "$(printf '%s' "$encoded" | base64 -d | digest_of_stdin)" != "$(digest_of_file "$formula")" ]; then
  die "the tap staging branch ${branch} does not carry the formula this tag published"
fi

# A branch whose formula matches can still carry something else. The compare is
# what says the branch introduces nothing beyond the formula against the base
# the merge verified.
if ! changed="$(gh api "repos/$repository/compare/${base}...${branch}" --jq '.files[].filename' 2>/dev/null)"; then
  die "could not compare the tap staging branch ${branch} against ${base}"
fi
while IFS= read -r file; do
  [ -n "$file" ] || continue
  [ "$file" = "Formula/orgtop.rb" ] ||
    die "the tap staging branch ${branch} changes more than the formula: ${file}"
done <<<"$changed"

if ! gh api -X DELETE "repos/$repository/git/refs/heads/${branch}" >/dev/null; then
  die "could not delete the tap staging branch ${branch} in ${repository}"
fi

# Delete, then verify. A delete answered with success that leaves the branch in
# place is the residue this exists to remove, reported as its removal.
if gh api "repos/$repository/git/ref/heads/${branch}" >/dev/null 2>&1; then
  die "the tap staging branch ${branch} still exists in ${repository}"
fi

printf 'guard: deleted the tap staging branch %s in %s\n' "$branch" "$repository"
