#!/usr/bin/env bash
# Render the Homebrew tap formula for a published tag.
#
# Usage:
#   distribution-formula.sh --tag T --commit SHA1 --checksums FILE
#
# The formula is printed to stdout and is the entire content of
# `Formula/orgtop.rb` in fmueller/homebrew-tap. It carries no build: each stanza
# downloads the canonical source archive from this repository's release for that
# tag and pins the exact SHA-256 that release's checksums.txt records. Only the
# four Linux/macOS matrix rows appear, because Homebrew has no Windows channel.
#
# Rendering is deterministic, so the staging branch can be rebuilt and compared
# byte for byte on every retry.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/distribution-lib.sh
. "$script_dir/distribution-lib.sh"

usage="$0 --tag <tag> --commit <sha> --checksums <file>"
tag="" commit="" checksums_file=""

while [ $# -gt 0 ]; do
  case "$1" in
  --tag) tag="${2-}" && shift 2 ;;
  --commit) commit="${2-}" && shift 2 ;;
  --checksums) checksums_file="${2-}" && shift 2 ;;
  *) usage_error "$usage" ;;
  esac
done

require_tag "$tag"
require_sha1 "the source commit" "$commit"

version="${tag#v}"

declare -A CHECKSUMS=()
read_checksums "$checksums_file"

declare -A ARCHIVES=()
while IFS=$'\t' read -r os arch archive _; do
  ARCHIVES["$os/$arch"]="$archive"
done < <("$matrix_script" "$version")

# stanza <os>/<arch> writes the versioned URL and pinned hash for one target,
# indented for a Homebrew `on_intel`/`on_arm` block. The URL is the canonical
# release download URL, so the formula can never point at a moving reference.
stanza() {
  local target="$1" archive sha
  archive="${ARCHIVES[$target]-}"
  [ -n "$archive" ] || die "the matrix has no $target row"
  sha="${CHECKSUMS[$archive]-}"
  [ -n "$sha" ] || die "$checksums_file has no entry for '$archive'"
  printf '      url "https://github.com/%s/releases/download/%s/%s"\n' "$source_repository" "$tag" "$archive"
  printf '      sha256 "%s"' "$sha"
}

# Every stanza is resolved before the template is written. A `die` inside a
# command substitution would only end that subshell, so a missing checksum would
# otherwise render a formula with an empty stanza and exit 0 — the opposite of
# failing closed.
darwin_amd64="$(stanza darwin/amd64)"
darwin_arm64="$(stanza darwin/arm64)"
linux_amd64="$(stanza linux/amd64)"
linux_arm64="$(stanza linux/arm64)"

cat <<RUBY
# frozen_string_literal: true

# orgtop is built and published by https://github.com/$source_repository.
# Source tag:    $tag
# Source commit: $commit
#
# This tap redistributes that release's archives byte for byte and performs no
# build of its own. Verify a download against the release's checksums.txt and
# its GitHub build attestation:
#
#   gh attestation verify <archive> --repo $source_repository
#
# Homebrew neither builds nor attests these bytes; the canonical repository above
# is the only publisher.
class Orgtop < Formula
  desc "Terminal dashboard for GitHub organization activity"
  homepage "https://github.com/$source_repository"
  version "$version"
  license "Apache-2.0"

  on_macos do
    on_intel do
${darwin_amd64}
    end
    on_arm do
${darwin_arm64}
    end
  end

  on_linux do
    on_intel do
${linux_amd64}
    end
    on_arm do
${linux_arm64}
    end
  end

  def install
    bin.install "orgtop"
  end

  test do
    assert_match "$version", shell_output("#{bin}/orgtop --version")
  end
end
RUBY
