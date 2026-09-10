#!/usr/bin/env bash
# Enforce the per-package mutation efficacy floor NFR-006 expects. gremlins
# thresholds the repository total only, so a single thin package can regress far
# below the floor while the aggregate stays green. This reads the report the
# weekly gate already wrote and thresholds each package on its own.
set -euo pipefail

floor=85
report=mutation-results.json

while [ "$#" -gt 0 ]; do
  case "$1" in
    --floor)
      floor="${2:-}"
      shift 2
      ;;
    --report)
      report="${2:-}"
      shift 2
      ;;
    *)
      echo "check-mutation-floor: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

if ! printf '%s' "$floor" | grep -qE '^[0-9]+(\.[0-9]+)?$'; then
  echo "check-mutation-floor: --floor must be a percentage, got: ${floor:-<empty>}" >&2
  exit 1
fi

if [ ! -f "$report" ]; then
  echo "check-mutation-floor: no mutation report at $report; run the gate first" >&2
  exit 1
fi

# Efficacy is killed / (killed + lived), the ratio gremlins publishes: timed-out
# and not-covered mutants return no verdict and stay outside it. A package whose
# every mutant timed out therefore has no ratio to fall below. The comparison is
# strictly below the floor, so a package sitting exactly on the floor holds it.
# That is one mutant looser than gremlins' own --threshold-efficacy, which reds
# on `<=`; the floor NFR-006 states is a minimum to reach, not one to exceed.
rows="$(
  jq -r --argjson floor "$floor" '
    [ .files[]
      | (.file_name | if test("/") then sub("/[^/]*$"; "") else "." end) as $package
      | .mutations[]
      | {package: $package, status: .status}
    ]
    | group_by(.package)
    | map(
        (map(select(.status == "KILLED")) | length) as $killed
        | (map(select(.status == "LIVED")) | length) as $lived
        | {
            package: .[0].package,
            killed: $killed,
            lived: $lived,
            timed_out: (map(select(.status == "TIMED OUT")) | length),
            efficacy: (if $killed + $lived == 0 then null else 100 * $killed / ($killed + $lived) end)
          }
      )
    | sort_by(.package)[]
    | [ .package,
        # Truncated rather than rounded: a package at 84.99% reported as 85.0%
        # reads as sitting on the floor the run just failed it for missing.
        # Truncation only ever moves a figure away from the floor it is compared
        # against, so the printed number and the verdict cannot contradict.
        (if .efficacy == null then "n/a" else ((.efficacy * 100 | floor) / 100 | tostring) end),
        .killed, .lived, .timed_out,
        (.efficacy != null and .efficacy < $floor)
      ]
    | @tsv
  ' "$report"
)" || {
  echo "check-mutation-floor: $report is not a readable gremlins report" >&2
  exit 1
}

if [ -z "$rows" ]; then
  echo "check-mutation-floor: $report names no mutated package; the gate produced no verdicts" >&2
  exit 1
fi

printf '| Package | Efficacy | Killed | Lived | Timed out |\n'
printf '|---|---:|---:|---:|---:|\n'

failures=()
while IFS=$'\t' read -r package efficacy killed lived timed_out under; do
  if [ "$efficacy" = "n/a" ]; then
    printf '| %s | n/a | %s | %s | %s |\n' "$package" "$killed" "$lived" "$timed_out"
    continue
  fi

  # jq already truncated for display; the comparison it made stands on the
  # unrounded ratio, so a package just under the floor cannot round its way past.
  measured="$(printf '%.2f' "$efficacy")"
  printf '| %s | %s%% | %s | %s | %s |\n' "$package" "$measured" "$killed" "$lived" "$timed_out"
  if [ "$under" = true ]; then
    failures+=("$package is at $measured% efficacy: killed $killed, lived $lived, timed out $timed_out")
  fi
done <<<"$rows"

if [ "${#failures[@]}" -gt 0 ]; then
  printf '\n'
  for failure in "${failures[@]}"; do
    echo "check-mutation-floor: $failure, below the ${floor}% floor" >&2
  done
  exit 1
fi

printf '\nEvery package holds the %s%% mutation efficacy floor.\n' "$floor"
