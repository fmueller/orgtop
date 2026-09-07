#!/usr/bin/env bash
# Land one distribution-ledger event on the protected default branch.
#
# Usage:
#   distribution-protected-commit.sh --branch B --title T --ledger-event FILE
#                                    [--extra-path PATH]
#
# The release App can create a branch and open a narrowly scoped pull request,
# but it cannot approve its own pull request and cannot push to the protected
# default branch. This script therefore opens the pull request and waits: the
# merge happens only once the repository's required checks pass and an
# independent human approval exists.
#
# It is idempotent. When the default branch already carries the exact event the
# step is complete and nothing is created; when the pull request already exists
# it is reused rather than reopened.
#
# Requires GH_TOKEN to be the App installation token and the working tree to be
# a checkout of the source repository.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/distribution-lib.sh
. "$script_dir/distribution-lib.sh"

ledger_path="${LEDGER_PATH:-docs/distribution-ledger.jsonl}"
poll_seconds="${DISTRIBUTION_POLL_SECONDS:-30}"
poll_attempts="${DISTRIBUTION_POLL_ATTEMPTS:-60}"

usage="$0 --branch <branch> --title <title> --ledger-event <file> [--extra-path <path>]"
branch="" title="" event_file="" extra_path=""

while [ $# -gt 0 ]; do
  case "$1" in
  --branch) branch="${2-}" && shift 2 ;;
  --title) title="${2-}" && shift 2 ;;
  --ledger-event) event_file="${2-}" && shift 2 ;;
  --extra-path) extra_path="${2-}" && shift 2 ;;
  *) usage_error "$usage" ;;
  esac
done

[ -n "$branch" ] && [ -n "$title" ] && [ -f "$event_file" ] ||
  usage_error "$usage"

event="$(cat "$event_file")"
default_branch="$(gh api "repos/$source_repository" --jq '.default_branch')"

# Every network git operation authenticates as the App, not as whatever
# credential actions/checkout persisted in .git/config. RG-011 reserves the
# repository GITHUB_TOKEN for the source release, and the ledger branch is the
# App's to create; relying on the ambient token would both violate that boundary
# and silently bypass a branch protection that names the App. The header is
# passed per command rather than configured, so the token never reaches disk.
[ -n "${GH_TOKEN:-}" ] || die "GH_TOKEN must be the distribution App installation token"
authenticated=(-c "http.extraheader=AUTHORIZATION: basic $(printf 'x-access-token:%s' "$GH_TOKEN" | base64 -w0)")

git "${authenticated[@]}" fetch origin "$default_branch"

# Already landed: the transition is complete and creating anything would be a
# second, contradictory record.
if git show "origin/$default_branch:$ledger_path" 2>/dev/null | grep -qxF "$event"; then
  echo "guard: the event is already on $default_branch"
  exit 0
fi

git checkout -B "$branch" "origin/$default_branch"
"$script_dir/distribution-ledger-append.sh" --ledger "$ledger_path" --event-file "$event_file"

# The App commits under its own identity rather than a maintainer's; the
# protected pull request, not the author line, is what authorizes the change.
committer=(-c user.name=orgtop-distribution -c user.email=distribution@users.noreply.github.com)

git "${committer[@]}" add -- "$ledger_path" ${extra_path:+"$extra_path"}

# The pull request stays narrowly scoped: the ledger, and for a withdrawal the
# durable notice committed in the same commit.
changed="$(git diff --cached --name-only)"
expected="$ledger_path${extra_path:+$'\n'$extra_path}"
if [ "$(printf '%s\n' "$changed" | sort)" != "$(printf '%s\n' "$expected" | sort)" ]; then
  die "the protected pull request must change only $expected; it changes: $changed"
fi

git "${committer[@]}" commit -m "$title"
git "${authenticated[@]}" push --force-with-lease origin "$branch"

if ! gh pr view "$branch" --json number >/dev/null 2>&1; then
  gh pr create --base "$default_branch" --head "$branch" --title "$title" \
    --body "Records one RG-011 distribution-ledger event. Merging this pull request is a required transition of the release workflow, which is waiting for it."
fi

# Wait for the required checks and the independent approval the branch
# protection demands. The App cannot supply either.
attempt=0
while [ "$attempt" -lt "$poll_attempts" ]; do
  state="$(gh pr view "$branch" --json mergeable,reviewDecision,statusCheckRollup \
    --jq '[.mergeable, .reviewDecision, ([.statusCheckRollup[]? | select(.conclusion != null and .conclusion != "SUCCESS" and .conclusion != "NEUTRAL" and .conclusion != "SKIPPED")] | length | tostring)] | join(" ")')"
  case "$state" in
  "MERGEABLE APPROVED 0")
    gh pr merge "$branch" --squash --delete-branch
    git "${authenticated[@]}" fetch origin "$default_branch"
    git show "origin/$default_branch:$ledger_path" | grep -qxF "$event" ||
      die "the merge did not land the exact event on $default_branch"
    echo "guard: the event is on $default_branch"
    exit 0
    ;;
  "CONFLICTING"*)
    die "the ledger pull request conflicts with $default_branch; rebase it and rerun"
    ;;
  esac
  attempt=$((attempt + 1))
  sleep "$poll_seconds"
done

die "the ledger pull request was not approved and green within $((poll_attempts * poll_seconds))s; the release stays partial until it merges"
