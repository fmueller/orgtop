#!/usr/bin/env bash
# Pre-publish guard: fail unless CHANGELOG.md has a heading for the release tag.
#
# Usage: scripts/check-changelog-version.sh <tag> [changelog-path]
#
# CHANGELOG.md is the source of truth for release notes, so a tag without a
# documented section must not publish. CHANGELOG.md follows Keep a Changelog: a
# tag `v0.1.0` maps to a `## [0.1.0]` heading (optionally `## [0.1.0] -
# 2026-08-22`). Exits non-zero when the heading is missing.
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

found="$(
  awk -v version="$version" '
    /^## \[/ {
      line = $0
      sub(/^## \[/, "", line)
      sub(/\].*$/, "", line)
      if (line == version) { print "yes"; exit }
    }
  ' "$changelog"
)"

if [ "$found" != "yes" ]; then
  echo "guard: no '## [$version]' heading found in $changelog" >&2
  echo "Add a '## [$version]' section to $changelog before tagging $tag." >&2
  exit 1
fi

echo "guard: found '## [$version]' heading in $changelog"
