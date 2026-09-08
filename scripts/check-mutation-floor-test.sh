#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
checker="$script_dir/check-mutation-floor.sh"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

# report writes a gremlins-shaped mutation-results.json from `file:status xN`
# triples, so a fixture names only the statuses a case turns on.
report() {
  local name="$1"
  shift
  local path="$tmp_dir/$name.json"
  local spec file status count index

  for spec in "$@"; do
    file="${spec%%:*}"
    status="${spec#*:}"
    count="${status##*x}"
    status="${status%x*}"
    for ((index = 0; index < count; index++)); do
      printf '{"file":"%s","status":"%s"}\n' "$file" "$status"
    done
  done | jq -s '{
    go_module: "example.com/orgtop",
    test_efficacy: 0,
    files: (group_by(.file) | map({
      file_name: .[0].file,
      mutations: map({type: "CONDITIONALS_BOUNDARY", status: .status, line: 1, column: 1})
    }))
  }' >"$path"
  printf '%s' "$path"
}

assert_accepts() {
  local name="$1"
  local path="$2"
  shift 2
  local output expected

  if ! output="$("$checker" --floor 90 --report "$path" 2>&1)"; then
    fail "$name was rejected: $output"
  fi
  for expected in "$@"; do
    if [[ "$output" != *"$expected"* ]]; then
      fail "$name did not report '$expected': $output"
    fi
  done
}

assert_rejects() {
  local name="$1"
  local path="$2"
  shift 2
  local output expected

  if output="$("$checker" --floor 90 --report "$path" 2>&1)"; then
    fail "$name was accepted: $output"
  fi
  for expected in "$@"; do
    if [[ "$output" != *"$expected"* ]]; then
      fail "$name did not report '$expected': $output"
    fi
  done
}

# Every package at or above the floor passes, and the table names each package
# with the counts the floor is computed from.
assert_accepts at-and-above-floor \
  "$(report at-and-above-floor \
    'internal/auth/token.go:KILLEDx10' \
    'internal/domain/scope.go:KILLEDx9' \
    'internal/domain/scope.go:LIVEDx1')" \
  'internal/auth' 'internal/domain' '100.0' '90.0'

# A single thin package under the floor reds the run even though the repository
# total stays well above it: the blind spot the aggregate threshold leaves.
assert_rejects one-package-under-floor \
  "$(report one-package-under-floor \
    'internal/auth/token.go:KILLEDx100' \
    'internal/github/client.go:KILLEDx8' \
    'internal/github/client.go:LIVEDx2')" \
  'internal/github' '80.0' 'killed 8' 'lived 2'

# Timed-out mutants stay outside the ratio, exactly as gremlins reports it, so a
# package killing 9 of its 10 verdicts holds the floor however many timed out.
assert_accepts timeouts-outside-the-ratio \
  "$(report timeouts-outside-the-ratio \
    'internal/cache/store.go:KILLEDx9' \
    'internal/cache/store.go:LIVEDx1' \
    'internal/cache/store.go:TIMED OUTx90')" \
  'internal/cache' '90.0' '90'

# A package with no runnable mutants has no ratio to fall below, so it is
# reported rather than failed.
assert_accepts no-runnable-mutants \
  "$(report no-runnable-mutants \
    'internal/auth/token.go:KILLEDx10' \
    'internal/enrichment/files.go:NOT COVEREDx4' \
    'internal/enrichment/files.go:TIMED OUTx2')" \
  'internal/enrichment' 'n/a'

# A package just under the floor must not print a figure that reads as at or
# above it: rounding 89.99% to 90.0% tells the reader they are on the floor when
# the run failed them for being under it, and the verdict has to be actionable
# without a second run.
assert_rejects just-under-the-floor \
  "$(report just-under-the-floor \
    'internal/github/client.go:KILLEDx8999' \
    'internal/github/client.go:LIVEDx1001')" \
  'internal/github' '89.99' 'below the 90% floor'

# A Go file at the module root has no directory to group by, so it forms its own
# package rather than colliding with the file name or dropping out of the table.
assert_accepts module-root-package \
  "$(report module-root-package \
    'main.go:KILLEDx10' \
    'internal/auth/token.go:KILLEDx10')" \
  '| . |' 'internal/auth'

# The floor is a parameter, not a constant baked into the check.
raised="$(report raised-floor \
  'internal/auth/token.go:KILLEDx9' \
  'internal/auth/token.go:LIVEDx1')"
if "$checker" --floor 95 --report "$raised" >/dev/null 2>&1; then
  fail "raised-floor was accepted at 95% by a 90% package"
fi

# A missing or unreadable report must red the gate rather than pass vacuously.
assert_rejects missing-report "$tmp_dir/absent.json" 'absent.json'

printf 'not json\n' >"$tmp_dir/malformed.json"
assert_rejects malformed-report "$tmp_dir/malformed.json" 'not a readable gremlins report'

# A report gremlins wrote before mutating anything must not read as a pass.
printf '{"go_module":"example.com/orgtop","files":[]}\n' >"$tmp_dir/empty.json"
assert_rejects empty-report "$tmp_dir/empty.json" 'no mutated package'

printf 'mutation floor checks passed\n'
