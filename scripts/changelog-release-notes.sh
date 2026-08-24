#!/usr/bin/env bash
# Extract the CHANGELOG.md section for a given release tag.
#
# Usage: scripts/changelog-release-notes.sh <tag> [changelog-path]
#
# Prints the body of the matching `## [<version>]` section to stdout, with the
# heading line, `[label]: url` link references, and surrounding blank lines
# removed. Exits non-zero when the section is missing or empty, so a release can
# never publish manufactured notes in place of documented ones.
#
# CHANGELOG.md follows Keep a Changelog: a tag `v0.1.0` matches a heading
# `## [0.1.0]` or `## [0.1.0] - 2026-08-22`, but not `## [0.1.0-rc1]`.
set -euo pipefail

tag="${1:-}"
changelog="${2:-CHANGELOG.md}"

if [ -z "$tag" ]; then
  echo "usage: $0 <tag> [changelog-path]" >&2
  exit 2
fi

if [ ! -f "$changelog" ]; then
  echo "guard: $changelog not found" >&2
  exit 1
fi

# Normalize: strip a leading 'v' from the tag to match the bracketed version.
version="${tag#v}"

notes="$(
  awk -v version="$version" '
    /^## / {
      if (capture) { capture = 0 }
      if ($0 ~ /^## \[/) {
        token = $0
        sub(/^## \[/, "", token)
        sub(/\].*$/, "", token)
        if (token == version) { capture = 1; next }
      }
    }
    # Print every line but a `[label]: url` link reference: Keep a Changelog
    # collects those at the foot of the file, inside the newest version section,
    # and they render as nothing. Mirrors isLinkReference in
    # internal/toolchain/release_test.go, so the trim matches its TrimSpace.
    capture {
      line = $0
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", line)
      if (line !~ /^\[.*\]: ./) { print }
    }
  ' "$changelog"
)"
# Trim leading and trailing blank lines.
notes="$(printf '%s\n' "$notes" | sed -e '/[^[:space:]]/,$!d' | sed -e ':a' -e '/^[[:space:]]*$/{$d;N;ba}')"

if [ -z "$notes" ]; then
  echo "guard: no non-empty '## [$version]' section found in $changelog" >&2
  exit 1
fi

printf '%s\n' "$notes"
