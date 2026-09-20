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
retry_candidate_limit=10

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
if ! app_login="$(gh api graphql -f query='query { viewer { login } }' --jq '.data.viewer.login' 2>/dev/null)" ||
  [ -z "$app_login" ] || [[ "$app_login" == *$'\n'* ]]; then
  die "the distribution App login could not be identified"
fi

# A refusal here protects the ledger, so it has to leave something behind that
# outlives the job log: the next operator has to know which transition stopped,
# on which base, and why, without rerunning the release to find out.
verified_base=""
fail_closed() {
  local reason="$1"
  if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    {
      printf '## Distribution ledger transition refused\n\n'
      printf -- '- branch: `%s`\n' "$branch"
      printf -- '- event: `%s` for `%s`\n' "$event_kind" "$event_version"
      printf -- '- default branch: `%s`\n' "$default_branch"
      printf -- '- verified base: `%s`\n' "${verified_base:-unknown}"
      printf -- '- reason: %s\n' "$reason"
    } >>"$GITHUB_STEP_SUMMARY"
  fi
  die "$reason"
}

# An independent approval and green checks prove the pull request; neither
# proves that the base it was reviewed against is still the base it merges onto.
# Within the pull-request-only boundary GitHub enforces that one way: a rule
# requiring the branch to be up to date before merging, which makes GitHub
# itself reject the merge once the default branch has advanced. Without it an
# exact or contradictory event landing between the last base read and the merge
# would persist. The guard therefore refuses to transition a ledger on a default
# branch whose rules it cannot read as both up-to-date-requiring and
# review-requiring, rather than merging into a window it cannot close.
protection_proves_atomic_merge() {
  local rules protection
  if rules="$(gh api "repos/$source_repository/rules/branches/$default_branch" 2>/dev/null)" &&
    jq -e '
      type == "array"
      and any(.[]?; .type == "pull_request"
        and ((.parameters.required_approving_review_count // 0) >= 1))
      and any(.[]?; .type == "required_status_checks"
        and (.parameters.strict_required_status_checks_policy == true))
    ' <<<"$rules" >/dev/null 2>&1; then
    return 0
  fi

  # A repository still carrying classic branch protection expresses the same two
  # requirements under different names; reading it needs a permission the App
  # may not hold, so it is a fallback rather than the primary source.
  if protection="$(gh api "repos/$source_repository/branches/$default_branch/protection" 2>/dev/null)" &&
    jq -e '
      type == "object"
      and ((.required_pull_request_reviews.required_approving_review_count // 0) >= 1)
      and (.required_status_checks.strict == true)
    ' <<<"$protection" >/dev/null 2>&1; then
    return 0
  fi

  return 1
}

protection_proves_atomic_merge ||
  fail_closed "$default_branch must require an approving review and an up to date branch before merge; the ledger transition cannot be proven atomic"

git "${authenticated[@]}" fetch origin "$default_branch"

# The exact commit whose ledger this run validated. Every later decision — the
# merge, and the proof that the merge landed on the base that was reviewed — is
# taken against this value rather than against whatever the default branch holds
# at the moment it is read again.
read_verified_base() {
  verified_base="$(git rev-parse "origin/$default_branch")" ||
    fail_closed "the $default_branch head could not be read"
}

read_verified_base

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
pr_fields="number,state,mergedAt,author,headRefName,headRefOid,headRepository,headRepositoryOwner,baseRefName,reviews"
readiness_fields="mergeable,author,headRefOid,reviews,latestReviews,statusCheckRollup"

is_definitive_not_found() {
  local output="$1"
  output="${output%$'\n'}"
  [ "$output" = "no pull requests found" ] ||
    [ "$output" = "no pull requests found for branch" ] ||
    [ "$output" = "no pull requests found for branch \"$branch\"" ]
}

is_stale_reopen_error() {
  local output="$1" gh_error=no json_output marker='gh: Validation Failed (HTTP 422)'

  # gh emits the JSON diagnostic followed either immediately by its exact
  # marker or on the next line. Anything else containing the marker is
  # diagnostic noise, not a stale-branch signal.
  output="${output//$'\r\n'/$'\n'}"
  output="${output%$'\r'}"
  case "$output" in
  *"$marker")
    json_output="${output%"$marker"}"
    case "$json_output" in
    *"$marker"*) return 1 ;;
    *$'\n') json_output="${json_output%$'\n'}" ;;
    esac
    gh_error=yes
    ;;
  *"$marker"*)
    return 1
    ;;
  *)
    json_output="$output"
    ;;
  esac
  [[ -n "$json_output" && "$json_output" != [[:space:]]* && "$json_output" != *[[:space:]] ]] || return 1

  jq -s -e --arg gh_error "$gh_error" '
    if length != 1 then false
    else
      .[0] as $error
      | if (($error | type) != "object") then false
        elif (($error | has("status")) and (($error.status | tostring) != "422")) then false
        elif (($error.errors? | type) != "array") then false
        else
          (
            ((($error.message // "") | startswith("Validation Failed"))
              or $gh_error == "yes")
            and any($error.errors[];
              .resource == "PullRequest"
              and .code == "custom"
              and .field == "state"
              and ((.message // "") | contains("branch was force-pushed or recreated"))
            )
          )
        end
    end
  ' <<<"$json_output" >/dev/null 2>&1
}

read_pr_json() {
  local selector="$1" json number base base_repository base_sha
  if ! json="$(gh pr view "$selector" --repo "$source_repository" --json "$pr_fields" 2>&1)"; then
    printf '%s\n' "$json"
    return 1
  fi
  if ! number="$(jq -er '.number | select(type == "number")' <<<"$json")"; then
    printf '%s\n' "$json"
    return 2
  fi
  if ! base="$(gh api "repos/$source_repository/pulls/$number" \
    --jq '"\(.base.repo.full_name) \(.base.sha)"' 2>&1)"; then
    printf '%s\n' "$base"
    return 1
  fi
  base_repository="${base%% *}"
  base_sha="${base##* }"
  [[ "$base_repository" =~ ^[^/[:space:]]+/[^/[:space:]]+$ ]] || {
    printf '%s\n' "$base"
    return 1
  }
  [[ "$base_sha" =~ ^[0-9a-f]{40}$ ]] || {
    printf '%s\n' "$base"
    return 1
  }
  jq --arg base_repository "$base_repository" --arg base_sha "$base_sha" \
    '. + {baseRepository: {nameWithOwner: $base_repository}, baseSha: $base_sha}' <<<"$json"
}

if pr_json="$(read_pr_json "$branch")"; then
  :
else
  lookup_status="$?"
  if [ "$lookup_status" -eq 2 ]; then
    die "the ledger pull request number could not be read"
  fi
  if ! is_definitive_not_found "$pr_json"; then
    die "the ledger pull request could not be inspected"
  fi
  gh pr create --repo "$source_repository" --base "$default_branch" --head "$branch" --title "$title" \
    --body "Records one RG-011 distribution-ledger event. Merging this pull request is a required transition of the release workflow, which is waiting for it."
  if pr_json="$(read_pr_json "$branch")"; then
    :
  else
    lookup_status="$?"
    if [ "$lookup_status" -eq 2 ]; then
      die "the ledger pull request number could not be read after creation"
    fi
    die "the ledger pull request could not be inspected after creation"
  fi
fi

validate_pr_identity() {
  local json="$1" require_current_head="$2" expected_state="$3"
  if ! jq -e --arg branch "$branch" --arg repo "$source_repository" \
    --arg base "$default_branch" --arg head "$expected_head" \
    --arg require_head "$require_current_head" --arg state "$expected_state" \
    --arg app "$app_login" '
      def app_slug:
        if type != "string" then null
        elif test("^app/[^/]+$") then .[4:]
        elif test("^[^/\\[]+\\[bot\\]$") then .[0:-5]
        else null
        end;
      (.number | type == "number" and floor == . and . > 0)
      and .state == $state
      and (((.author.login // "") | app_slug) != null)
      and (($app | app_slug) != null)
      and (((.author.login // "") | app_slug) == ($app | app_slug))
      and ((.headRefOid // "") | test("^[0-9a-f]{40}$"))
      and .headRefName == $branch
      and .headRepository.nameWithOwner == $repo
      and .headRepositoryOwner.login == ($repo | split("/")[0])
      and .baseRefName == $base
      and .baseRepository.nameWithOwner == $repo
      and ((.baseSha // "") | test("^[0-9a-f]{40}$"))
      and (.state != "OPEN" or .mergedAt == null)
      and (if $require_head == "yes" then .headRefOid == $head else true end)
    ' <<<"$json" >/dev/null; then
    die "the ledger pull request identity does not match the protected transition"
  fi
}

read_pr_readiness() {
  local phase="$1" state
  if ! pr_json="$(read_pr_json "$pr_number")"; then
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
  if ! gh pr reopen "$pr_number" --repo "$source_repository" >/dev/null 2>&1; then
    # GitHub refuses to reopen a closed pull request after its head branch was
    # force-pushed or recreated. That is exactly what cleanup followed by a
    # same-tag retry does, so create a fresh, uniquely named branch and pull
    # request rather than treating the historical pull request as reusable.
    if ! rest_output="$(gh api -X PATCH "repos/$source_repository/pulls/$pr_number" -f state=open 2>&1)"; then
      if ! is_stale_reopen_error "$rest_output"; then
        die "the existing closed ledger pull request could not be reopened"
      fi
      original_branch="$branch"
      retry_suffix="${GITHUB_RUN_ID:-local}"
      retry_index=0
      while :; do
        if [ "$retry_index" -ge "$retry_candidate_limit" ]; then
          die "too many retry ledger pull-request candidates are already closed"
        fi
        retry_branch="${original_branch}-retry-${retry_suffix}"
        [ "$retry_index" -eq 0 ] || retry_branch+="-$retry_index"

        branch="$retry_branch"
        if candidate_json="$(read_pr_json "$branch")"; then
          candidate_state="$(jq -r '.state // empty' <<<"$candidate_json")"
          case "$candidate_state" in
          OPEN)
            # Reusing an open retry pull request is safe only after its identity
            # is checked. Its head is then refreshed and checked again below, so
            # a rerun cannot retain an approval for an older commit.
            validate_pr_identity "$candidate_json" no OPEN
            git "${authenticated[@]}" fetch origin "$branch"
            git branch -f "$branch" "$expected_head"
            git "${authenticated[@]}" push --force-with-lease origin "$branch"
            ;;
          CLOSED)
            retry_index=$((retry_index + 1))
            continue
            ;;
          *)
            die "the retry ledger pull request is $candidate_state; refusing to reuse it"
            ;;
          esac
        else
          lookup_status="$?"
          if [ "$lookup_status" -ne 1 ] || ! is_definitive_not_found "$candidate_json"; then
            die "the retry ledger pull request could not be inspected"
          fi
          git branch -f "$branch" "$expected_head"
          git "${authenticated[@]}" push --force-with-lease origin "$branch"
          gh pr create --repo "$source_repository" --base "$default_branch" --head "$branch" --title "$title" \
            --body "Records one RG-011 distribution-ledger event. Merging this pull request is a required transition of the release workflow, which is waiting for it."
        fi

        if ! git "${authenticated[@]}" push \
          --force-with-lease="refs/heads/$original_branch:$expected_head" \
          origin ":refs/heads/$original_branch"; then
          die "the obsolete ledger branch changed before retry cleanup"
        fi
        if ! pr_json="$(read_pr_json "$branch")"; then
          die "the retry ledger pull request could not be inspected after refresh"
        fi
        pr_number="$(jq -er '.number | select(type == "number")' <<<"$pr_json")" ||
          die "the retry ledger pull request number could not be read"
        break
      done
    fi
  fi
  if ! pr_json="$(read_pr_json "$pr_number")"; then
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
  read_verified_base
  if default_branch_event; then
    echo "guard: the event is already on $default_branch"
    exit 0
  fi

  read_pr_readiness " during the poll"
  case "$readiness" in
  ready)
    # The approval and the checks were proven against the base read above, not
    # against the base the merge will land on. Read the default branch once more
    # here: an exact or a contradictory event may have landed in that window,
    # and merging afterwards would persist a duplicate or a contradiction that
    # no later check can take back.
    git "${authenticated[@]}" fetch origin "$default_branch"
    read_verified_base
    if default_branch_event; then
      echo "guard: the event is already on $default_branch"
      exit 0
    fi

    # Re-read both identity and readiness immediately before merge. The
    # --match-head-commit guard closes the remaining race between this check
    # and GitHub's merge operation without ever accepting a changed head.
    read_pr_readiness " before merge"
    [ "$readiness" = ready ] ||
      die "the ledger pull request was no longer ready before merge"

    # GitHub's own view of the base has to be the base this run validated. When
    # the two disagree the default branch moved underneath the transition, and
    # the up-to-date rule has to reject the merge anyway; refusing here keeps
    # the refusal explicit and evidenced rather than reading it out of a merge
    # error string.
    pr_base_sha="$(jq -er '.baseSha // empty' <<<"$pr_json")" ||
      fail_closed "the ledger pull request base commit could not be read"
    [ "$pr_base_sha" = "$verified_base" ] ||
      fail_closed "$default_branch advanced from $verified_base to $pr_base_sha; refusing to merge onto a base this run has not validated"
    merge_base="$verified_base"

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

    # A squash merge has exactly one parent, and it must be the commit whose
    # ledger this run validated. Anything else means the merge was rebased onto
    # a base nobody here checked, so the ledger it produced is not the ledger
    # that was reviewed.
    merged_head="$(git rev-parse "origin/$default_branch")" ||
      fail_closed "the merged $default_branch head could not be read"
    merged_parent="$(git rev-parse --verify --quiet "$merged_head^1" || true)"
    [ "$merged_parent" = "$merge_base" ] ||
      fail_closed "the merge landed on '$merged_parent' rather than the verified base $merge_base"
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
