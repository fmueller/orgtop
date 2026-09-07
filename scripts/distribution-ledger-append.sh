#!/usr/bin/env bash
# Append one RG-011 ledger event, or assert that a version may still publish.
#
# Usage:
#   distribution-ledger-append.sh --ledger FILE --event-file FILE
#   distribution-ledger-append.sh --ledger FILE --assert-publishable VERSION
#
# Appending is create-if-absent or compare-exactly. An event already recorded
# byte for byte is reused and nothing is written, which is what makes a retry of
# the same transition idempotent. A second, different event for the same version
# and event kind is the contradictory same-version state RG-011 refuses to
# reconcile, and fails closed.
#
# `--assert-publishable` enforces the reservation: a version whose staged event
# carries no completed or withdrawn event is a partial release, and no later
# version may publish while it is unresolved. A completed version is permanently
# non-reusable.
#
# The file is only read and appended to here; committing it to the protected
# default branch is the release workflow's protected pull-request step.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/distribution-lib.sh
. "$script_dir/distribution-lib.sh"

usage="$0 --ledger <file> --event-file <file> | --assert-publishable <version>"
ledger="" event_file="" publishable=""

while [ $# -gt 0 ]; do
  case "$1" in
  --ledger) ledger="${2-}" && shift 2 ;;
  --event-file) event_file="${2-}" && shift 2 ;;
  --assert-publishable) publishable="${2-}" && shift 2 ;;
  *) usage_error "$usage" ;;
  esac
done

[ -n "$ledger" ] || usage_error "$usage"
[ -f "$ledger" ] || die "$ledger not found"

# Every recorded line must be exactly one canonical object followed by one LF.
# A file that does not read back that way was written by something other than
# this guard, and comparing against it would compare against unknown bytes.
if [ -s "$ledger" ] && [ "$(tail -c 1 "$ledger" | od -An -c | tr -d ' ')" != '\n' ]; then
  die "$ledger must end every event with exactly one LF"
fi

# is_canonical <text> — true when the text already is the RFC 8785 bytes jq
# would produce for it, which is what every recorded line has to be.
is_canonical() {
  [ "$(printf '%s' "$1" | jq -cS . 2>/dev/null)" = "$1" ]
}

while IFS= read -r line; do
  [ -n "$line" ] || die "$ledger has a blank line"
  is_canonical "$line" || die "$ledger holds a line that is not canonical: $line"
done <"$ledger"

# recorded_state <version> prints the kinds of event already recorded for one
# version, one per line, in the order they were appended.
recorded_state() {
  jq -r --arg version "$1" 'select(.version == $version) | .event' "$ledger"
}

if [ -n "$publishable" ]; then
  require_version "$publishable"

  # A completed version is permanently reserved: a fix ships under a later
  # semantic version, never by restaging this one.
  if recorded_state "$publishable" | grep -qx completed; then
    die "version $publishable is already completed and is permanently non-reusable"
  fi

  # An earlier version left staged but neither completed nor withdrawn blocks
  # every later version until it completes or is withdrawn.
  while IFS= read -r other; do
    [ -n "$other" ] || continue
    [ "$other" != "$publishable" ] || continue
    states="$(recorded_state "$other")"
    grep -qx staged <<<"$states" || continue
    if ! grep -qx -e completed -e withdrawn <<<"$states"; then
      die "version $other is staged but neither completed nor withdrawn; complete or withdraw it before publishing $publishable"
    fi
  done < <(jq -r '.version' "$ledger" | sort -u)

  echo "guard: version $publishable may publish"
  exit 0
fi

[ -f "$event_file" ] || usage_error "$usage"

event="$(cat "$event_file")"
is_canonical "$event" || die "the event is not canonical RFC 8785 bytes"

kind="$(printf '%s' "$event" | jq -r '.event')"
version="$(printf '%s' "$event" | jq -r '.version')"
require_version "$version"

if grep -qxF "$event" "$ledger"; then
  echo "guard: the $kind event for $version is already recorded"
  exit 0
fi

existing="$(jq -c --arg version "$version" --arg kind "$kind" \
  'select(.version == $version and .event == $kind)' "$ledger")"
if [ -n "$existing" ]; then
  die "the ledger already holds a contradictory $kind event for $version$(printf '\n have: %s\n want: %s' "$existing" "$event")"
fi

printf '%s\n' "$event" >>"$ledger"
echo "guard: recorded the $kind event for $version"
