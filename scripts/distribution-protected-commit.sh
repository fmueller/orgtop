#!/usr/bin/env bash
# Land one distribution-ledger event on the protected default branch.
#
# Usage:
#   distribution-protected-commit.sh --branch B --title T --ledger-event FILE
#                                    [--extra-path PATH]
#
# The release App can create a branch and open a narrowly scoped pull request,
# but it never approves its own pull request and never pushes to the default
# branch. This script therefore opens the pull request and waits: the merge
# happens only once its checks have settled green and an independent approval
# exists. That decision is the workflow's own, taken from the reviews
# themselves, so it holds whether or not the default branch carries a protection
# or ruleset that would also require them.
#
# It is idempotent. When the default branch already carries the exact event the
# step is complete and nothing is created; when the pull request already exists
# and is open it is reused. A closed, unmerged rehearsal pull request is
# reopened so a retry still passes through the same independent-approval gate.
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
event_version="$(printf '%s' "$event" | jq -r '.version // empty')"
event_kind="$(printf '%s' "$event" | jq -r '.event // empty')"
ledger_snapshot="$(mktemp)"
ledger_validation="$(mktemp)"
trap 'rm -f "$ledger_snapshot" "$ledger_validation"' EXIT
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

default_branch_event() {
  local line line_version line_kind matches=0
  if ! git show "origin/$default_branch:$ledger_path" >"$ledger_snapshot" 2>/dev/null; then
    return 1
  fi

  while IFS= read -r line; do
    [ -n "$line" ] || continue
    if [ "$line" = "$event" ]; then
      matches=$((matches + 1))
      continue
    fi
    line_version="$(printf '%s' "$line" | jq -r '.version // empty' 2>/dev/null || true)"
    line_kind="$(printf '%s' "$line" | jq -r '.event // empty' 2>/dev/null || true)"
    if [ "$line_version" = "$event_version" ] && [ "$line_kind" = "$event_kind" ]; then
      die "the default branch holds a contradictory $event_kind event for $event_version"
    fi
  done <"$ledger_snapshot"

  [ "$matches" -le 1 ] ||
    die "the default branch holds duplicate $event_kind event for $event_version"

  # Reuse the append guard's complete ledger validation against a disposable
  # copy. It checks canonical JSON, blank lines, and exactly one LF per record;
  # the copy may receive the requested event when it is not present, while the
  # fetched default-branch bytes remain untouched for the exact match above.
  cp "$ledger_snapshot" "$ledger_validation"
  if ! "$script_dir/distribution-ledger-append.sh" \
    --ledger "$ledger_validation" --event-file "$event_file" >/dev/null 2>&1; then
    die "the default branch ledger is not a valid canonical ledger"
  fi

  [ "$matches" -eq 1 ]
}

# Already landed: the transition is complete and creating anything would be a
# second, contradictory record. The same check is repeated during the approval
# poll because the event may land out of band while this step is waiting.
if default_branch_event; then
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

expected_head="$(git rev-parse HEAD)"
pr_fields="number,state,mergedAt,headRefName,headRefOid,headRepository,headRepositoryOwner,baseRefName,baseRepository,reviews"
readiness_fields="mergeable,author,headRefOid,reviews,latestReviews,statusCheckRollup"

is_definitive_not_found() {
  local output="$1"
  output="${output%$'\n'}"
  [ "$output" = "no pull requests found" ] ||
    [ "$output" = "no pull requests found for branch" ] ||
    [ "$output" = "no pull requests found for branch \"$branch\"" ]
}

if ! pr_json="$(gh pr view "$branch" --repo "$source_repository" --json "$pr_fields" 2>&1)"; then
  if ! is_definitive_not_found "$pr_json"; then
    die "the ledger pull request could not be inspected"
  fi
  gh pr create --repo "$source_repository" --base "$default_branch" --head "$branch" --title "$title" \
    --body "Records one RG-011 distribution-ledger event. Merging this pull request is a required transition of the release workflow, which is waiting for it."
  if ! pr_json="$(gh pr view "$branch" --repo "$source_repository" --json "$pr_fields" 2>&1)"; then
    die "the ledger pull request could not be inspected after creation"
  fi
fi

validate_pr_identity() {
  local json="$1" require_current_head="$2" expected_state="$3"
  if ! jq -e --arg branch "$branch" --arg repo "$source_repository" \
    --arg base "$default_branch" --arg head "$expected_head" \
    --arg require_head "$require_current_head" --arg state "$expected_state" '
      (.number | type == "number" and floor == . and . > 0)
      and .state == $state
      and ((.headRefOid // "") | test("^[0-9a-f]{40}$"))
      and .headRefName == $branch
      and .headRepository.nameWithOwner == $repo
      and .headRepositoryOwner.login == ($repo | split("/")[0])
      and .baseRefName == $base
      and .baseRepository.nameWithOwner == $repo
      and (.state != "OPEN" or .mergedAt == null)
      and (if $require_head == "yes" then .headRefOid == $head else true end)
    ' <<<"$json" >/dev/null; then
    die "the ledger pull request identity does not match the protected transition"
  fi
}

read_pr_readiness() {
  local phase="$1" state
  if ! pr_json="$(gh pr view "$pr_number" --repo "$source_repository" --json "$pr_fields" 2>&1)"; then
    die "the ledger pull request could not be inspected$phase"
  fi
  validate_pr_identity "$pr_json" yes OPEN

  if ! state="$(gh pr view "$pr_number" --repo "$source_repository" --json "$readiness_fields" 2>&1)"; then
    die "the ledger pull request state could not be read$phase"
  fi
  readiness="$(pull_request_readiness "$state" "$expected_head")" ||
    die "the ledger pull request state could not be read$phase"
}

pr_number="$(jq -er '.number | select(type == "number")' <<<"$pr_json")" ||
  die "the ledger pull request number could not be read"
pr_state="$(jq -er '.state // empty' <<<"$pr_json")" ||
  die "the ledger pull request state could not be read"
case "$pr_state" in
OPEN)
  validate_pr_identity "$pr_json" yes OPEN
  ;;
CLOSED)
  if jq -e '.mergedAt != null' <<<"$pr_json" >/dev/null; then
    die "the existing ledger pull request is already merged; refusing to reuse it"
  fi
  validate_pr_identity "$pr_json" no CLOSED
  if jq -e '[.reviews[]? | select(.state == "APPROVED")] | length > 0' <<<"$pr_json" >/dev/null; then
    die "the existing closed ledger pull request carries prior approval; refusing to reuse it"
  fi
  if ! gh pr reopen "$pr_number" --repo "$source_repository" >/dev/null; then
    die "the existing closed ledger pull request could not be reopened"
  fi
  if ! pr_json="$(gh pr view "$pr_number" --repo "$source_repository" --json "$pr_fields" 2>&1)"; then
    die "the reopened ledger pull request could not be inspected"
  fi
  if [ "$(jq -r '.state // empty' <<<"$pr_json")" != OPEN ]; then
    die "the reopened ledger pull request is not open"
  fi
  validate_pr_identity "$pr_json" yes OPEN
  ;;
*)
  die "the existing ledger pull request is $pr_state; refusing to reuse it"
  ;;
esac

# Wait for the checks to settle green and for an independent approval. The App
# supplies neither: it is the pull request's author, and pull_request_readiness
# refuses an approval carrying its own login.
attempt=0
while [ "$attempt" -lt "$poll_attempts" ]; do
  git "${authenticated[@]}" fetch origin "$default_branch"
  if default_branch_event; then
    echo "guard: the event is already on $default_branch"
    exit 0
  fi

  read_pr_readiness " during the poll"
  case "$readiness" in
  ready)
    # Re-read both identity and readiness immediately before merge. The
    # --match-head-commit guard closes the remaining race between this check
    # and GitHub's merge operation without ever accepting a changed head.
    read_pr_readiness " before merge"
    [ "$readiness" = ready ] ||
      die "the ledger pull request was no longer ready before merge"

    if ! gh pr merge "$pr_number" --repo "$source_repository" --match-head-commit "$expected_head" --squash --delete-branch; then
      # The pull request may have merged out of band after the poll check. A
      # failed merge is complete only when the exact event is now present.
      git "${authenticated[@]}" fetch origin "$default_branch"
      if default_branch_event; then
        echo "guard: the event is already on $default_branch"
        exit 0
      fi
      die "the ledger pull request could not be merged"
    fi
    git "${authenticated[@]}" fetch origin "$default_branch"
    default_branch_event ||
      die "the merge did not land the exact event on $default_branch"
    echo "guard: the event is on $default_branch"
    exit 0
    ;;
  conflicting)
    die "the ledger pull request conflicts with $default_branch; rebase it and rerun"
    ;;
  esac
  attempt=$((attempt + 1))
  sleep "$poll_seconds"
done

die "the ledger pull request was not approved and green within $((poll_attempts * poll_seconds))s; the release stays partial until it merges"
