#!/usr/bin/env bash
# Extract the CHANGELOG.md section for a given release tag.
#
# Usage: scripts/changelog-release-notes.sh <tag> [changelog-path]
#
# Prints the body of the matching `## [<version>]` section (heading line
# excluded, leading and trailing blank lines trimmed) to stdout. If no matching
# section is found, prints the tag name so a release always carries notes.
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

# Normalize: strip a leading 'v' from the tag to match the bracketed version.
version="${tag#v}"

notes=""
if [ -f "$changelog" ]; then
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
      capture { print }
    ' "$changelog"
  )"
  # Trim leading and trailing blank lines.
  notes="$(printf '%s\n' "$notes" | sed -e '/./,$!d' | sed -e ':a' -e '/^[[:space:]]*$/{$d;N;ba}')"
fi

if [ -z "$notes" ]; then
  printf '%s\n' "$tag"
else
  printf '%s\n' "$notes"
fi
