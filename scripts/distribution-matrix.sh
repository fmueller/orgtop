#!/usr/bin/env bash
# The RG-011 build and publication matrix, printed for one version.
#
# Usage: scripts/distribution-matrix.sh <version>
#
# The version omits the leading `v`: the tag is `v0.2.0`, the version is
# `0.2.0`, and archive names carry the version. One line per target, tab
# separated:
#
#   <os> <arch> <source archive> <extension raw asset> <homebrew: yes|no>
#
# Rows are sorted by OS then architecture in byte order, which is the order the
# completion manifest's `targets` array has to be in. This script is the single
# source of truth for the matrix: the release workflow, the reconciliation
# guards, and internal/toolchain all read it rather than restating the names.
set -euo pipefail

version="${1:-}"

if [ -z "$version" ]; then
  echo "usage: $0 <version>" >&2
  echo "The version omits the leading 'v', for example 0.2.0." >&2
  exit 2
fi

case "$version" in
v*)
  echo "guard: version '$version' must omit the leading 'v'" >&2
  exit 2
  ;;
esac

# Archive extension and Homebrew availability follow from the OS: Homebrew has
# no Windows channel, so Windows ships through source archives and the GitHub
# CLI extension only.
for os in darwin linux windows; do
  case "$os" in
  windows)
    archive_extension=zip
    executable_suffix=.exe
    homebrew=no
    ;;
  *)
    archive_extension=tar.gz
    executable_suffix=
    homebrew=yes
    ;;
  esac

  for arch in amd64 arm64; do
    printf '%s\t%s\torgtop_%s_%s_%s.%s\tgh-orgtop-%s-%s%s\t%s\n' \
      "$os" "$arch" \
      "$version" "$os" "$arch" "$archive_extension" \
      "$os" "$arch" "$executable_suffix" \
      "$homebrew"
  done
done
