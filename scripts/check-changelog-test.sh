#!/usr/bin/env bash
# Tests for the changelog release guards. The guards only run on a tag push, so
# a regression in them is otherwise invisible until a release ships blank notes.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
checker="$script_dir/check-changelog-version.sh"
extractor="$script_dir/changelog-release-notes.sh"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

# write <name> <content> -> path of a changelog fixture
write() {
  local path="$tmp_dir/$1.md"
  printf '%s\n' "$2" >"$path"
  printf '%s' "$path"
}

assert_accepts() {
  local name="$1" tag="$2" changelog="$3" output

  if ! output="$("$checker" "$tag" "$changelog" 2>&1)"; then
    fail "$name was rejected: $output"
  fi
}

assert_rejects() {
  local name="$1" tag="$2" changelog="$3" expected="$4" output

  if output="$("$checker" "$tag" "$changelog" 2>&1)"; then
    fail "$name was accepted"
  fi
  if [[ "$output" != *"$expected"* ]]; then
    fail "$name did not report '$expected': $output"
  fi
}

assert_notes() {
  local name="$1" tag="$2" changelog="$3" expected="$4" output

  if ! output="$("$extractor" "$tag" "$changelog" 2>&1)"; then
    fail "$name failed to extract: $output"
  fi
  if [[ "$output" != "$expected" ]]; then
    fail "$name extracted $(printf '%q' "$output"), want $(printf '%q' "$expected")"
  fi
}

documented="$(write documented '# Changelog

## [Unreleased]

## [0.1.0] - 2026-08-22

### Added

- A documented change.

## [0.0.1] - 2026-08-01

- An older change.')"

heading_only="$(write heading-only '# Changelog

## [0.1.0] - 2026-08-22

## [0.0.1] - 2026-08-01

- An older change.')"

blank_section="$(write blank-section '# Changelog

## [0.1.0] - 2026-08-22

   

## [0.0.1] - 2026-08-01

- An older change.')"

trailing_heading="$(write trailing-heading '# Changelog

## [0.1.0] - 2026-08-22')"

link_footer="$(write link-footer '# Changelog

## [0.1.0] - 2026-08-22

### Added

- A documented change.

[Unreleased]: https://example.test/compare/v0.1.0...HEAD
[0.1.0]: https://example.test/releases/tag/v0.1.0')"

link_only="$(write link-only '# Changelog

## [0.1.0] - 2026-08-22

[Unreleased]: https://example.test/compare/v0.1.0...HEAD
[0.1.0]: https://example.test/releases/tag/v0.1.0')"

link_midsection="$(write link-midsection '# Changelog

## [0.1.0] - 2026-08-22

[0.1.0]: https://example.test/releases/tag/v0.1.0

### Added

- An entry whose text contains a stray ]: sequence.

## [0.0.1] - 2026-08-01

- An older change.')"

prerelease="$(write prerelease '# Changelog

## [0.1.0-rc1] - 2026-08-20

- A prerelease change.')"

assert_accepts documented v0.1.0 "$documented"
assert_accepts unprefixed-tag 0.1.0 "$documented"

assert_rejects missing-heading v0.2.0 "$documented" "no '## [0.2.0]' heading"
assert_rejects heading-only v0.1.0 "$heading_only" 'has no entries'
assert_rejects blank-section v0.1.0 "$blank_section" 'has no entries'
assert_rejects trailing-heading v0.1.0 "$trailing_heading" 'has no entries'
assert_rejects link-only v0.1.0 "$link_only" 'has no entries'
assert_rejects prerelease-mismatch v0.1.0 "$prerelease" "no '## [0.1.0]' heading"
assert_rejects missing-file v0.1.0 "$tmp_dir/absent.md" 'not found'

# The extracted body stops at the next heading and carries no blank padding.
assert_notes extract-body v0.1.0 "$documented" '### Added

- A documented change.'

# Link-reference definitions render as nothing, so they are not release notes.
# They live at the foot of a Keep a Changelog file, inside the newest section.
assert_notes strip-link-footer v0.1.0 "$link_footer" '### Added

- A documented change.'

# A reference in the middle of a section goes too, and an ordinary entry that
# merely contains `]: ` stays.
assert_notes strip-link-midsection v0.1.0 "$link_midsection" '### Added

- An entry whose text contains a stray ]: sequence.'

# A missing section must fail rather than fall back to notes naming the tag.
if "$extractor" v0.2.0 "$documented" >/dev/null 2>&1; then
  fail "the extractor invented notes for an undocumented tag"
fi

printf 'changelog guard checks passed\n'
