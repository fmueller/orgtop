#!/usr/bin/env bash
# Shared validation and canonicalization helpers for the RG-011 distribution
# guards. Sourced, never executed.
#
# The records these guards emit are compared byte for byte on every retry, so a
# value that is merely "close enough" — an upper-case digest, an abbreviated
# commit, a URL with a trailing slash — is a reconciliation failure later rather
# than an error now. Every field is therefore checked at the boundary.

# Bytewise ordering, not the ambient locale's collation: RG-011 sorts asset and
# subject names by byte, and a locale that ignores punctuation would accept a
# record the reconciliation step then rejects.
export LC_ALL=C

# The canonical repositories. RG-011 names them exactly; nothing derives them
# from the runner environment, so a workflow running elsewhere cannot quietly
# bind a record to a different repository.
readonly source_repository="fmueller/orgtop"
readonly extension_repository="fmueller/gh-orgtop"
readonly tap_repository="fmueller/homebrew-tap"
readonly formula_path="Formula/orgtop.rb"
readonly release_workflow_path=".github/workflows/release.yml"
readonly checksums_asset="checksums.txt"
readonly provenance_asset="provenance.intoto.jsonl"

# Every guard reads the platform matrix from this one script rather than
# restating names it would then have to keep in step.
matrix_script="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/distribution-matrix.sh"
readonly matrix_script

die() {
  printf 'guard: %s\n' "$*" >&2
  exit 1
}

usage_error() {
  printf 'usage: %s\n' "$*" >&2
  exit 2
}

# require_hex <label> <length> <value> — lowercase hexadecimal, exact length.
require_hex() {
  local label="$1" length="$2" value="$3" hex=yes
  case "$value" in
  *[!0-9a-f]*) hex=no ;;
  esac
  if [ "$hex" = no ] || [ "${#value}" -ne "$length" ]; then
    die "$label must be $length lowercase hexadecimal characters, got '$value'"
  fi
}

# require_sha256 <label> <value>
require_sha256() {
  require_hex "$1" 64 "$2"
}

# require_sha1 <label> <value>
require_sha1() {
  require_hex "$1" 40 "$2"
}

# require_reason <value> — the withdrawal reason RG-011 calls "nonempty
# sanitized public copy". It is copied verbatim into a canonical ledger record
# and into a permanent public notice, so it is held to one line of printable
# text rather than sanitized silently: rewriting an operator's words would
# publish a reason nobody wrote.
require_reason() {
  [ -n "$(printf '%s' "$1" | tr -d '[:space:]')" ] || die "a withdrawal reason must not be empty"
  case "$1" in
  *$'\n'* | *$'\r'*) die "a withdrawal reason must be a single line" ;;
  esac
  [ "$1" = "$(printf '%s' "$1" | tr -d '[:cntrl:]')" ] || die "a withdrawal reason must carry no control character"
  [ "${#1}" -le 500 ] || die "a withdrawal reason must be at most 500 characters, got ${#1}"
}

# require_version <value> — the bare version, without the leading `v`.
require_version() {
  case "$1" in
  v*) die "version '$1' must omit the leading 'v'" ;;
  esac
  [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "version '$1' is not MAJOR.MINOR.PATCH"
}

# require_tag <value> — the tag, which carries the leading `v`.
require_tag() {
  [[ "$1" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "'$1' is not an ASCII semantic version tag"
}

# canonical reads a JSON value on stdin and writes RFC 8785 canonical bytes with
# no trailing newline. Every record RG-011 defines holds only strings and the
# integer 1, so sorted keys and jq's compact output are those bytes exactly.
canonical() {
  local json
  json="$(jq -cS .)" || die "the record is not valid JSON"
  printf '%s' "$json"
}

# digest_of_stdin prints the SHA-256 of the bytes it reads, which for a ledger
# event is the canonical object without the LF that terminates its line.
digest_of_stdin() {
  sha256sum | cut -d' ' -f1
}

# digest_of_file <file> prints the SHA-256 of a file's exact bytes.
digest_of_file() {
  digest_of_stdin <"$1"
}

# require_ascending <file> <noun> <previous> <name> enforces the bytewise order
# RG-011 fixes for both metadata assets. An empty <previous> is the first line.
require_ascending() {
  local file="$1" noun="$2" previous="$3" name="$4"
  [ -n "$previous" ] || return 0
  [[ "$previous" < "$name" ]] ||
    die "$file must be sorted bytewise by $noun name; '$name' follows '$previous'"
}

# read_checksums <file> populates the caller's CHECKSUMS associative array from
# a `<sha256>  <name>` file, rejecting malformed and unsorted content.
# shellcheck disable=SC2034
read_checksums() {
  # The caller names the array to fill, so one reader serves both a channel's
  # own checksums.txt and the source reference a cross-channel comparison needs.
  local -n __checksums="${CHECKSUMS_TARGET:-CHECKSUMS}"
  local file="$1" line sha name previous=""
  [ -f "$file" ] || die "$file not found"

  while IFS= read -r line; do
    [ -n "$line" ] || die "$file has a blank line"
    sha="${line%%  *}"
    name="${line#*  }"
    [ "$sha" != "$line" ] || die "$file line '$line' is not '<sha256>  <name>'"
    require_sha256 "the $file digest for '$name'" "$sha"
    require_ascending "$file" asset "$previous" "$name"
    previous="$name"
    __checksums["$name"]="$sha"
  done <"$file"
}

# read_provenance <file> populates PROVENANCE with one subject digest per asset
# name, rejecting a bundle that is not the LF-delimited, subject-sorted, one
# object per line form RG-011 requires.
read_provenance() {
  local file="$1" line name sha previous=""
  [ -f "$file" ] || die "$file not found"

  while IFS= read -r line; do
    [ -n "$line" ] || die "$file has a blank line"
    name="$(printf '%s' "$line" | jq -er '
      if (.subject | length) != 1 then error("one subject per line") else .subject[0].name end
    ')" || die "$file line does not carry exactly one subject: $line"
    sha="$(printf '%s' "$line" | jq -r '.subject[0].digest.sha256 // ""')"
    require_sha256 "the $file subject digest for '$name'" "$sha"
    require_ascending "$file" subject "$previous" "$name"
    previous="$name"
    PROVENANCE["$name"]="$sha"
  done <"$file"
}

# pull_request_readiness <json> decides whether the ledger pull request may be
# merged, from the `gh pr view` fields named in distribution-protected-commit.sh.
# It prints `ready`, `waiting`, or `conflicting`, and returns non-zero when the
# state cannot be read at all.
#
# The approval is read from the reviews rather than from `reviewDecision`.
# GitHub populates `reviewDecision` only when a branch protection or ruleset
# requires review, so on a default branch carrying no such rule it stays null
# however many approvals a pull request has, and a release waiting on it would
# spend its whole poll bound and then fail with every precondition actually met.
# Reading the reviews gives the same answer under a rule and without one, and it
# is the stricter of the two: RG-011's independent approval is checked here even
# where no repository rule demands one.
#
# The refusal is a return rather than a die because the caller reads this
# through a command substitution, where an exit would end only the subshell and
# leave the poll loop retrying a state it never understood.
pull_request_readiness() {
  jq -er '
    # An approval GitHub can no longer attribute to an account is not an
    # independent human approval, and neither is one carrying the App'"'"'s own
    # login: the App opens the pull request.
    def independent: (.author.login // "") as $login
      | $login != "" and $login != $author;

    # latestReviews carries one entry per reviewer, so an approval that a later
    # objection from the same person replaced is already gone. An objection
    # standing from anyone else is not settled by somebody else'"'"'s approval.
    def approved_independently:
      ([.latestReviews[]? | select(independent and .state == "APPROVED")] | length > 0)
      and ([.latestReviews[]? | select(independent and .state == "CHANGES_REQUESTED")] | length == 0);

    # statusCheckRollup mixes check runs, which report a conclusion, with classic
    # commit statuses, which report only a state and never a conclusion. Reading
    # one field alone would hold a green pull request forever.
    def settled: if .conclusion != null then (.conclusion | ascii_upcase) as $c
        | $c == "SUCCESS" or $c == "NEUTRAL" or $c == "SKIPPED"
      elif .state != null then (.state | ascii_upcase) == "SUCCESS"
      else false
      end;
    def checks_settled: [.statusCheckRollup[]? | select(settled | not)] | length == 0;

    if .mergeable == "CONFLICTING" then "conflicting"
    elif .mergeable == "MERGEABLE" and approved_independently and checks_settled then "ready"
    else "waiting"
    end
  ' --arg author "$(printf '%s' "$1" | jq -r '.author.login // ""' 2>/dev/null)" <<<"$1" 2>/dev/null ||
    {
      printf 'guard: the pull request state could not be read\n' >&2
      return 1
    }
}

# artifact_names <version> <channel> prints the exact asset names a channel's
# draft carries, excluding the two metadata assets, sorted bytewise.
artifact_names() {
  local version="$1" channel="$2" os arch archive raw homebrew

  while IFS=$'\t' read -r os arch archive raw homebrew; do
    case "$channel" in
    source) printf '%s\n%s\n' "$archive" "$raw" ;;
    extension) printf '%s\n' "$raw" ;;
    *) die "unknown channel '$channel'" ;;
    esac
  done < <("$matrix_script" "$version") | LC_ALL=C sort
}
