#!/usr/bin/env bash
# Tests for the multi-channel distribution guards (RG-011).
#
# These guards only ever run inside the release workflow, on a tag, against
# repositories this suite must not touch. Everything they decide is therefore
# exercised here against fixtures: the platform matrix, the canonical ledger and
# completion records, and the pre-publication reconciliation of a staged asset
# directory. A regression in any of them is otherwise invisible until a release
# is already half public.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
matrix="$script_dir/distribution-matrix.sh"
ledger="$script_dir/distribution-ledger.sh"
manifest="$script_dir/distribution-manifest.sh"
manifest_asset="$script_dir/distribution-manifest-asset.sh"
verify="$script_dir/distribution-verify.sh"
upload="$script_dir/distribution-upload-assets.sh"
stage_assets="$script_dir/distribution-stage-assets.sh"
formula_script="$script_dir/distribution-formula.sh"
append="$script_dir/distribution-ledger-append.sh"
notice_script="$script_dir/distribution-notice.sh"
# shellcheck source=scripts/distribution-lib.sh
. "$script_dir/distribution-lib.sh"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

assert_equal() {
  local name="$1" got="$2" want="$3"
  if [ "$got" != "$want" ]; then
    fail "$name = $(printf '%q' "$got"), want $(printf '%q' "$want")"
  fi
}

assert_contains() {
  local name="$1" got="$2" want="$3"
  if [[ "$got" != *"$want"* ]]; then
    fail "$name did not report '$want': $got"
  fi
}

# assert_rejects <name> <expected message> -- <command...>
assert_rejects() {
  local name="$1" expected="$2" output
  shift 3
  if output="$("$@" 2>&1)"; then
    fail "$name was accepted: $output"
  fi
  assert_contains "$name" "$output" "$expected"
}

# ---------------------------------------------------------------------------
# Platform matrix
# ---------------------------------------------------------------------------

# The exact RG-011 table. Every other guard, the formula, and the workflow read
# the matrix from this one script, so this fixture is the contract.
read -r -d '' want_matrix <<'EOF' || true
darwin	amd64	orgtop_0.2.0_darwin_amd64.tar.gz	gh-orgtop-darwin-amd64	yes
darwin	arm64	orgtop_0.2.0_darwin_arm64.tar.gz	gh-orgtop-darwin-arm64	yes
linux	amd64	orgtop_0.2.0_linux_amd64.tar.gz	gh-orgtop-linux-amd64	yes
linux	arm64	orgtop_0.2.0_linux_arm64.tar.gz	gh-orgtop-linux-arm64	yes
windows	amd64	orgtop_0.2.0_windows_amd64.zip	gh-orgtop-windows-amd64.exe	no
windows	arm64	orgtop_0.2.0_windows_arm64.zip	gh-orgtop-windows-arm64.exe	no
EOF

assert_equal "matrix rows" "$("$matrix" 0.2.0)" "$want_matrix"

assert_rejects "matrix without a version" "usage:" -- "$matrix"
assert_rejects "matrix with a tag" "must omit the leading 'v'" -- "$matrix" v0.2.0

# ---------------------------------------------------------------------------
# Fixture artifacts
# ---------------------------------------------------------------------------

# digest_of <text> -> the sha256 a fixture artifact of that content would carry.
digest_of() {
  printf '%s' "$1" | sha256sum | cut -d' ' -f1
}

# digest_of_file <path> -> the sha256 of a file's exact bytes.
digest_of_file() {
  sha256sum "$1" | cut -d' ' -f1
}

# seal_checksums <dir> writes <dir>/checksums.txt over whatever artifacts the
# directory currently holds, sorted bytewise by name with two spaces before it.
# The file is built outside the directory, so the listing it is made from can
# never include the file being produced.
seal_checksums() {
  local dir="$1" scratch="$tmp_dir/sealed-checksums.txt" name
  (cd "$dir" && ls | LC_ALL=C sort | while read -r name; do
    printf '%s  %s\n' "$(digest_of_file "$name")" "$name"
  done) >"$scratch"
  mv "$scratch" "$dir/checksums.txt"
}

version=0.2.0
tag=v0.2.0
commit=1111111111111111111111111111111111111111
workflow_commit=2222222222222222222222222222222222222222
tap_commit=3333333333333333333333333333333333333333

stage="$tmp_dir/stage"
mkdir -p "$stage"

# Build the twelve artifacts as real archives whose contained executable is
# byte-identical to the matching raw asset, which is the parity RG-011 requires.
# The archive container's own bytes differ from the executable's, so a guard that
# compared containers would pass while the redistributed bytes diverged.
build_dir="$tmp_dir/build"
mkdir -p "$build_dir"
while IFS=$'\t' read -r os arch archive raw _; do
  executable="executable for $os/$arch"
  printf '%s' "$executable" >"$stage/$raw"

  rm -rf "$build_dir/content"
  mkdir -p "$build_dir/content"
  binary=orgtop
  if [ "$os" = windows ]; then binary=orgtop.exe; fi
  printf '%s' "$executable" >"$build_dir/content/$binary"
  printf 'license\n' >"$build_dir/content/LICENSE"
  case "$archive" in
    *.tar.gz) tar -czf "$stage/$archive" -C "$build_dir/content" . ;;
    *.zip) (cd "$build_dir/content" && zip -q -r "$stage/$archive" .) ;;
  esac
done < <("$matrix" "$version")

seal_checksums "$stage"

# provenance.intoto.jsonl: one statement per line, bytewise sorted by subject
# name, LF-delimited with no blank line.
provenance_for() {
  local dir="$1" name
  while read -r name; do
    jq -cn --arg name "$name" --arg sha "$(digest_of_file "$dir/$name")" \
      '{"_type":"https://in-toto.io/Statement/v1",subject:[{name:$name,digest:{sha256:$sha}}]}'
  done < <(cd "$dir" && ls | LC_ALL=C sort | grep -v -e '^checksums.txt$' -e '^provenance.intoto.jsonl$')
}
provenance_for "$stage" >"$stage/provenance.intoto.jsonl"

# reseal regenerates a fixture's metadata from its current artifacts, so a test
# that mutates one artifact fails on the invariant it targets rather than on the
# stale checksum that mutation also breaks.
reseal() {
  local dir="$1"
  rm -f "$dir/checksums.txt" "$dir/provenance.intoto.jsonl"
  seal_checksums "$dir"
  provenance_for "$dir" >"$dir/provenance.intoto.jsonl"
}

checksums_sha="$(digest_of_file "$stage/checksums.txt")"
provenance_sha="$(digest_of_file "$stage/provenance.intoto.jsonl")"

# ---------------------------------------------------------------------------
# Reconciliation of a staged asset directory
# ---------------------------------------------------------------------------

assert_verifies() {
  local name="$1" output
  shift
  if ! output="$("$verify" "$@" 2>&1)"; then
    fail "$name was rejected: $output"
  fi
}

assert_verifies "the exact source asset set" --dir "$stage" --version "$version" --channel source

# The extension draft carries the six raw assets and its own raw-only
# checksums.txt, with the source provenance lines for those six unchanged.
extension="$tmp_dir/extension"
mkdir -p "$extension"
while IFS=$'\t' read -r _ _ _ raw _; do
  cp "$stage/$raw" "$extension/$raw"
done < <("$matrix" "$version")
seal_checksums "$extension"
grep -F -e '-linux-' -e '-darwin-' -e '-windows-' "$stage/provenance.intoto.jsonl" >"$extension/provenance.intoto.jsonl"

assert_verifies "the exact extension asset set" --dir "$extension" --version "$version" --channel extension

# ---------------------------------------------------------------------------
# Create-if-absent release-asset uploads
# ---------------------------------------------------------------------------

# The upload guard talks to GitHub only through `gh`, so these fixtures replace
# that command with a tiny release-asset store. The store records remote bytes,
# not just names: a retry has to compare what the draft actually holds before it
# writes anything, and an upload failure may leave a deterministic partial draft
# for the next attempt to resume.
fake_gh_bin="$tmp_dir/fake-gh-bin"
mkdir -p "$fake_gh_bin"
cat >"$fake_gh_bin/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

state="${FAKE_GH_STATE:?FAKE_GH_STATE is required}"
assets="$state/assets"
mkdir -p "$assets"
log="$state/operations.log"

case "${1-}" in
release)
  subcommand="${2-}"
  shift 2
  case "$subcommand" in
  view)
    # The fixture only needs the asset-name projection. Release/tag/repository
    # arguments are deliberately ignored: the production guard validates the
    # repository and tag at its own boundary, while this fake isolates the
    # create-if-absent behavior.
    if [ "${FAKE_GH_FAIL_VIEW:-}" = yes ]; then
      printf 'gh-orgtop-linux-amd64\n'
      printf 'fixture: asset inventory request failed\n' >&2
      exit 1
    fi
    find "$assets" -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort
    ;;
  download)
    pattern=""
    output=""
    while [ "$#" -gt 0 ]; do
      case "$1" in
      --pattern) pattern="$2"; shift 2 ;;
      --output) output="$2"; shift 2 ;;
      --repo) shift 2 ;;
      --clobber) shift ;;
      *) shift ;;
      esac
    done
    [ -n "$pattern" ] && [ -n "$output" ]
    printf 'download %s\n' "$pattern" >>"$log"
    [ -f "$assets/$pattern" ]
    cp "$assets/$pattern" "$output"
    ;;
  upload)
    file="${2-}" # tag and local asset path
    shift 2
    name="$(basename "$file")"
    printf 'upload %s\n' "$name" >>"$log"
    if [ "${FAKE_GH_FAIL_UPLOAD_NAME:-}" = "$name" ]; then
      printf 'fixture: refusing upload of %s\n' "$name" >&2
      exit 1
    fi
    if [ -e "$assets/$name" ]; then
      printf 'fixture: asset %s already exists\n' "$name" >&2
      exit 1
    fi
    cp "$file" "$assets/$name"
    ;;
  *)
    printf 'fixture: unsupported gh release subcommand %s\n' "$subcommand" >&2
    exit 2
    ;;
  esac
  ;;
*)
  printf 'fixture: unsupported gh command %s\n' "${1-}" >&2
  exit 2
  ;;
esac
EOF
chmod +x "$fake_gh_bin/gh"

expected_source_assets="$(
  {
    artifact_names "$version" source
    printf '%s\n%s\n' "$checksums_asset" "$provenance_asset"
  } | LC_ALL=C sort
)"
expected_extension_assets="$(
  {
    artifact_names "$version" extension
    printf '%s\n%s\n' "$checksums_asset" "$provenance_asset"
  } | LC_ALL=C sort
)"

remote_assets() {
  find "$FAKE_GH_STATE/assets" -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort
}

new_upload_state() {
  FAKE_GH_STATE="$tmp_dir/$1"
  export FAKE_GH_STATE
  rm -rf "$FAKE_GH_STATE"
  mkdir -p "$FAKE_GH_STATE/assets"
  : >"$FAKE_GH_STATE/operations.log"
}

copy_upload_asset() {
  copy_upload_asset_from "$stage" "$1"
}

copy_upload_asset_from() {
  local dir="$1" name="$2"
  cp "$dir/$name" "$FAKE_GH_STATE/assets/$name"
}

(
  export PATH="$fake_gh_bin:$PATH"

  # Empty draft: every expected source asset is uploaded, including both
  # metadata assets, and the final remote set is exactly the 14-item source
  # draft. This is the first-publication path without a real tag.
  new_upload_state upload-empty
  "$upload" --tag "$tag" --repo fmueller/orgtop --dir "$stage" --channel source
  assert_equal "an empty draft receives the exact source asset set" "$(remote_assets)" "$expected_source_assets"
  assert_equal "an empty draft uploads every source asset once" \
    "$(grep -c '^upload ' "$FAKE_GH_STATE/operations.log")" "14"

  # Partial draft: existing bytes are saved before reconciliation and must be
  # unchanged; only absent assets are uploaded. The copied bytes prove the
  # guard does not replace an existing asset while making the set complete.
  new_upload_state upload-partial
  partial_names=(gh-orgtop-darwin-amd64 gh-orgtop-linux-amd64 checksums.txt)
  for name in "${partial_names[@]}"; do copy_upload_asset "$name"; done
  before_partial="$(sha256sum "$FAKE_GH_STATE/assets/gh-orgtop-linux-amd64" | cut -d' ' -f1)"
  "$upload" --tag "$tag" --repo fmueller/orgtop --dir "$stage" --channel source
  assert_equal "a partial draft is completed without replacing existing bytes" "$(remote_assets)" "$expected_source_assets"
  assert_equal "a partial draft keeps its existing digest" \
    "$(sha256sum "$FAKE_GH_STATE/assets/gh-orgtop-linux-amd64" | cut -d' ' -f1)" "$before_partial"
  assert_equal "a partial draft uploads only missing assets" \
    "$(grep -c '^upload ' "$FAKE_GH_STATE/operations.log")" "11"

  # Fully populated draft: all assets compare successfully and no upload is
  # attempted. This catches a regression to the old keep-existing behavior,
  # which retried every upload and received an already_exists response.
  new_upload_state upload-full
  while IFS= read -r name; do copy_upload_asset "$name"; done <<<"$expected_source_assets"
  "$upload" --tag "$tag" --repo fmueller/orgtop --dir "$stage" --channel source
  assert_equal "a full draft receives no replacement uploads" \
    "$(grep -c '^upload ' "$FAKE_GH_STATE/operations.log" || true)" "0"
  assert_equal "a full draft remains byte-complete" "$(remote_assets)" "$expected_source_assets"

  # Extension channel retries use the same immutable reconciliation rules as
  # source assets. Keep separate fixtures so a source-only implementation
  # cannot accidentally satisfy this contract.
  new_upload_state upload-extension-empty
  "$upload" --tag "$tag" --repo fmueller/orgtop --dir "$extension" --channel extension
  assert_equal "an empty extension draft receives the exact asset set" "$(remote_assets)" "$expected_extension_assets"
  assert_equal "an empty extension draft uploads every asset once" \
    "$(grep -c '^upload ' "$FAKE_GH_STATE/operations.log")" "8"

  new_upload_state upload-extension-partial
  extension_partial_names=(gh-orgtop-darwin-amd64 gh-orgtop-linux-amd64 checksums.txt)
  for name in "${extension_partial_names[@]}"; do
    copy_upload_asset_from "$extension" "$name"
  done
  extension_before_partial="$(sha256sum "$FAKE_GH_STATE/assets/gh-orgtop-linux-amd64" | cut -d' ' -f1)"
  "$upload" --tag "$tag" --repo fmueller/orgtop --dir "$extension" --channel extension
  assert_equal "a partial extension draft is completed without replacement" "$(remote_assets)" "$expected_extension_assets"
  assert_equal "a partial extension draft keeps its existing digest" \
    "$(sha256sum "$FAKE_GH_STATE/assets/gh-orgtop-linux-amd64" | cut -d' ' -f1)" "$extension_before_partial"
  assert_equal "a partial extension draft uploads only missing assets" \
    "$(grep -c '^upload ' "$FAKE_GH_STATE/operations.log")" "5"

  new_upload_state upload-extension-full
  while IFS= read -r name; do copy_upload_asset_from "$extension" "$name"; done <<<"$expected_extension_assets"
  "$upload" --tag "$tag" --repo fmueller/orgtop --dir "$extension" --channel extension
  assert_equal "a full extension draft receives no replacement uploads" \
    "$(grep -c '^upload ' "$FAKE_GH_STATE/operations.log" || true)" "0"
  assert_equal "a full extension draft remains byte-complete" "$(remote_assets)" "$expected_extension_assets"

  new_upload_state upload-extension-divergent
  copy_upload_asset_from "$extension" gh-orgtop-linux-amd64
  printf 'different extension bytes' >"$FAKE_GH_STATE/assets/gh-orgtop-linux-amd64"
  extension_divergent_expected="$(sha256sum "$extension/gh-orgtop-linux-amd64" | cut -d' ' -f1)"
  extension_divergent_actual="$(sha256sum "$FAKE_GH_STATE/assets/gh-orgtop-linux-amd64" | cut -d' ' -f1)"
  extension_divergent_output=""
  if extension_divergent_output="$("$upload" --tag "$tag" --repo fmueller/orgtop --dir "$extension" --channel extension 2>&1)"; then
    fail "a digest-divergent extension draft was accepted: $extension_divergent_output"
  fi
  assert_contains "a divergent extension draft names the asset" "$extension_divergent_output" "gh-orgtop-linux-amd64"
  assert_contains "a divergent extension draft names the expected digest" "$extension_divergent_output" "$extension_divergent_expected"
  assert_contains "a divergent extension draft names the actual digest" "$extension_divergent_output" "$extension_divergent_actual"
  assert_equal "a divergent extension draft uploads nothing" \
    "$(grep -c '^upload ' "$FAKE_GH_STATE/operations.log" || true)" "0"

  # The workflow must verify the complete staged set before it asks GitHub to
  # create anything. A locally inconsistent checksum/provenance pair therefore
  # fails with zero remote operations rather than publishing bad bytes and
  # discovering the contradiction during the later reconciliation step.
  new_upload_state upload-invalid-local
  invalid_local="$tmp_dir/invalid-local-stage"
  rm -rf "$invalid_local"
  cp -r "$stage" "$invalid_local"
  printf 'rebuilt before upload' >"$invalid_local/gh-orgtop-linux-amd64"
  invalid_local_output=""
  if invalid_local_output="$(
    {
      "$verify" --dir "$invalid_local" --version "$version" --channel source &&
      "$upload" --tag "$tag" --repo fmueller/orgtop --dir "$invalid_local" --channel source
    } 2>&1
  )" 2>&1; then
    fail "an inconsistent staged set reached upload: $invalid_local_output"
  fi
  assert_contains "an inconsistent staged set fails before upload" "$invalid_local_output" "checksums.txt"
  assert_equal "an inconsistent staged set uploads nothing" \
    "$(grep -c '^upload ' "$FAKE_GH_STATE/operations.log" || true)" "0"

  # A failed release inventory must not fall through the conditional pipeline
  # into a new manifest upload. This is a separate guard from the asset-set
  # uploader because the completion manifest is attached after publication.
  new_upload_state manifest-inventory-failure
  export FAKE_GH_FAIL_VIEW=yes
  manifest_inventory_output=""
  if manifest_inventory_output="$("$manifest_asset" --tag "$tag" --repo fmueller/orgtop \
    --manifest "$stage/checksums.txt" 2>&1)"; then
    fail "a failed manifest inventory was accepted: $manifest_inventory_output"
  fi
  unset FAKE_GH_FAIL_VIEW
  assert_contains "a failed manifest inventory is closed before upload" "$manifest_inventory_output" "asset inventory"
  assert_equal "a failed manifest inventory uploads nothing" \
    "$(grep -c '^upload ' "$FAKE_GH_STATE/operations.log" || true)" "0"

  # Inventory failure: partial API output is not a trustworthy draft state.
  # The guard must surface the read failure before it uploads any omitted name.
  new_upload_state upload-inventory-failure
  copy_upload_asset gh-orgtop-linux-amd64
  export FAKE_GH_FAIL_VIEW=yes
  assert_rejects "a failed asset inventory is closed before upload" "asset inventory" -- \
    "$upload" --tag "$tag" --repo fmueller/orgtop --dir "$stage" --channel source
  unset FAKE_GH_FAIL_VIEW
  assert_equal "a failed asset inventory uploads nothing" \
    "$(grep -c '^upload ' "$FAKE_GH_STATE/operations.log" || true)" "0"

  # Divergent draft: preflight compares every existing asset before the upload
  # phase, so a mismatched name reports both digests and leaves even the missing
  # assets untouched. A retry cannot silently repair a published name.
  new_upload_state upload-divergent
  copy_upload_asset gh-orgtop-linux-amd64
  printf 'different bytes' >"$FAKE_GH_STATE/assets/gh-orgtop-linux-amd64"
  divergent_expected="$(sha256sum "$stage/gh-orgtop-linux-amd64" | cut -d' ' -f1)"
  divergent_actual="$(sha256sum "$FAKE_GH_STATE/assets/gh-orgtop-linux-amd64" | cut -d' ' -f1)"
  divergent_output=""
  if divergent_output="$("$upload" --tag "$tag" --repo fmueller/orgtop --dir "$stage" --channel source 2>&1)"; then
    fail "a digest-divergent draft was accepted: $divergent_output"
  fi
  assert_contains "a digest-divergent draft names the asset" "$divergent_output" "gh-orgtop-linux-amd64"
  assert_contains "a digest-divergent draft names the expected digest" "$divergent_output" "$divergent_expected"
  assert_contains "a digest-divergent draft names the actual digest" "$divergent_output" "$divergent_actual"
  assert_equal "a digest-divergent draft uploads nothing" \
    "$(grep -c '^upload ' "$FAKE_GH_STATE/operations.log" || true)" "0"
  assert_equal "a digest-divergent draft keeps its unexpected bytes" \
    "$(sha256sum "$FAKE_GH_STATE/assets/gh-orgtop-linux-amd64" | cut -d' ' -f1)" "$divergent_actual"

  # Partial API failure: the first attempt may leave exact assets behind, but a
  # later attempt compares those and uploads only the remaining names.
  new_upload_state upload-retry
  export FAKE_GH_FAIL_UPLOAD_NAME=orgtop_0.2.0_linux_amd64.tar.gz
  assert_rejects "an upload failure leaves a retryable partial draft" "refusing upload" -- \
    "$upload" --tag "$tag" --repo fmueller/orgtop --dir "$stage" --channel source
  unset FAKE_GH_FAIL_UPLOAD_NAME
  retry_before="$(remote_assets)"
  "$upload" --tag "$tag" --repo fmueller/orgtop --dir "$stage" --channel source
  assert_equal "a retry reconciles an upload-failure partial draft" "$(remote_assets)" "$expected_source_assets"
  assert_contains "a retry preserves the partial draft" "$retry_before" "gh-orgtop-darwin-amd64"
)

# GoReleaser records raw binaries in target-specific paths. The staging helper
# must select the six extension records by type/ID, map their dist-relative
# paths into an arbitrary fixture directory, and copy the complete immutable
# upload set without allowing traversal or symlink escape.
release_dist="$tmp_dir/goreleaser-dist"
release_output="$tmp_dir/goreleaser-output"
mkdir -p "$release_dist"
cp "$stage"/orgtop_0.2.0_*.tar.gz "$release_dist/"
cp "$stage"/orgtop_0.2.0_windows_*.zip "$release_dist/"
cp "$stage/checksums.txt" "$release_dist/"
cp "$stage/provenance.intoto.jsonl" "$release_dist/"

release_artifacts='[]'
matrix_output="$("$matrix" "$version")"
while IFS=$'\t' read -r os arch archive raw homebrew; do
  raw_relative="targets/$os-$arch/$raw"
  mkdir -p "$release_dist/$(dirname "$raw_relative")"
  cp "$stage/$raw" "$release_dist/$raw_relative"
  release_artifacts="$(jq --arg name "$raw" --arg path "dist/$raw_relative" \
    '. + [{name: $name, type: "Binary", extra: {ID: "gh-extension"}, path: $path}]' \
    <<<"$release_artifacts")"
done <<<"$matrix_output"
release_artifacts="$(jq '. + [{name: "ignored", type: "Binary", extra: {ID: "other"}, path: "dist/ignored"}]' <<<"$release_artifacts")"
printf '%s\n' "$release_artifacts" >"$release_dist/artifacts.json"

"$stage_assets" --version "$version" --dist "$release_dist" --output "$release_output"
assert_equal "artifact staging produces the complete source set" \
  "$(find "$release_output" -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort)" "$expected_source_assets"
while IFS=$'\t' read -r os arch archive raw homebrew; do
  assert_equal "artifact staging maps the $os/$arch raw binary" \
    "$(sha256sum "$release_output/$raw" | cut -d' ' -f1)" "$(sha256sum "$stage/$raw" | cut -d' ' -f1)"
done <<<"$matrix_output"

outside="$tmp_dir/outside-artifact"
printf 'outside distribution root' >"$outside"
traversal_dist="$tmp_dir/goreleaser-traversal"
cp -a "$release_dist" "$traversal_dist"
jq --arg name gh-orgtop-linux-amd64 --arg path 'dist/../outside-artifact' \
  'map(if .name == $name then .path = $path else . end)' \
  "$traversal_dist/artifacts.json" >"$traversal_dist/artifacts.tmp"
mv "$traversal_dist/artifacts.tmp" "$traversal_dist/artifacts.json"
assert_rejects "artifact staging rejects a traversal path" "outside distribution directory" -- \
  "$stage_assets" --version "$version" --dist "$traversal_dist" --output "$tmp_dir/traversal-output"

symlink_dist="$tmp_dir/goreleaser-symlink"
cp -a "$release_dist" "$symlink_dist"
ln -s "$outside" "$symlink_dist/escaped-raw"
jq --arg name gh-orgtop-linux-amd64 --arg path 'dist/escaped-raw' \
  'map(if .name == $name then .path = $path else . end)' \
  "$symlink_dist/artifacts.json" >"$symlink_dist/artifacts.tmp"
mv "$symlink_dist/artifacts.tmp" "$symlink_dist/artifacts.json"
assert_rejects "artifact staging rejects a symlink escape" "outside distribution directory" -- \
  "$stage_assets" --version "$version" --dist "$symlink_dist" --output "$tmp_dir/symlink-output"

# A failed matrix producer must not be converted into an incomplete six-row
# inventory by process substitution. Exercise both the workflow-facing staging
# helper and the upload guard through a copied script directory.
matrix_failure_dir="$tmp_dir/matrix-failure-scripts"
mkdir -p "$matrix_failure_dir"
cp "$stage_assets" "$upload" "$script_dir/distribution-lib.sh" "$matrix_failure_dir/"
cat >"$matrix_failure_dir/distribution-matrix.sh" <<'EOF'
#!/usr/bin/env bash
printf 'darwin\tamd64\torgtop_0.2.0_darwin_amd64.tar.gz\tgh-orgtop-darwin-amd64\tyes\n'
printf '%s\n' 'matrix fixture failed' >&2
exit 1
EOF
chmod +x "$matrix_failure_dir/distribution-matrix.sh"
assert_rejects "artifact staging surfaces a failed matrix producer" "distribution matrix" -- \
  "$matrix_failure_dir/distribution-stage-assets.sh" --version "$version" --dist "$release_dist" --output "$tmp_dir/matrix-output"

export PATH="$fake_gh_bin:$PATH"
new_upload_state upload-matrix-failure
assert_rejects "asset upload surfaces a failed matrix producer" "distribution matrix" -- \
  "$matrix_failure_dir/distribution-upload-assets.sh" --tag "$tag" --repo fmueller/orgtop \
  --dir "$stage" --channel source

matrix_bad_name_dir="$tmp_dir/matrix-bad-name-scripts"
mkdir -p "$matrix_bad_name_dir"
cp "$stage_assets" "$upload" "$script_dir/distribution-lib.sh" "$matrix_bad_name_dir/"
cat >"$matrix_bad_name_dir/distribution-matrix.sh" <<EOF
#!/usr/bin/env bash
"$matrix" "\$1" | sed '1s/orgtop_/wrong_/'
EOF
chmod +x "$matrix_bad_name_dir/distribution-matrix.sh"
assert_rejects "artifact staging rejects malformed matrix names" "archive" -- \
  "$matrix_bad_name_dir/distribution-stage-assets.sh" --version "$version" --dist "$release_dist" --output "$tmp_dir/bad-name-output"
new_upload_state upload-matrix-bad-name
assert_rejects "asset upload rejects malformed matrix names" "archive" -- \
  "$matrix_bad_name_dir/distribution-upload-assets.sh" --tag "$tag" --repo fmueller/orgtop \
  --dir "$stage" --channel source

assert_rejects "artifact staging rejects output beneath dist" "output must be outside" -- \
  "$stage_assets" --version "$version" --dist "$release_dist" --output "$release_dist/nested-output"
assert_equal "invalid staging preserves the distribution directory" \
  "$(test -f "$release_dist/artifacts.json" && printf present)" "present"

preserved_dist="$tmp_dir/goreleaser-preserved"
cp -a "$release_dist" "$preserved_dist"
jq --arg name gh-orgtop-linux-amd64 --arg path 'dist/../outside-artifact' \
  'map(if .name == $name then .path = $path else . end)' \
  "$preserved_dist/artifacts.json" >"$preserved_dist/artifacts.tmp"
mv "$preserved_dist/artifacts.tmp" "$preserved_dist/artifacts.json"
preserved_stage="$tmp_dir/preserved-stage"
mkdir -p "$preserved_stage"
printf 'keep existing output' >"$preserved_stage/sentinel"
assert_rejects "failed artifact staging preserves existing output" "outside distribution directory" -- \
  "$stage_assets" --version "$version" --dist "$preserved_dist" --output "$preserved_stage"
assert_equal "failed artifact staging keeps existing output bytes" \
  "$(cat "$preserved_stage/sentinel")" "keep existing output"

# Failure paths: every one of these must fail before anything is published.
copy_stage() {
  local dir="$tmp_dir/$1"
  rm -rf "$dir"
  cp -r "$stage" "$dir"
  printf '%s' "$dir"
}

missing="$(copy_stage missing)"
rm "$missing/gh-orgtop-linux-arm64"
assert_rejects "a missing raw asset" "gh-orgtop-linux-arm64" -- \
  "$verify" --dir "$missing" --version "$version" --channel source

extra="$(copy_stage extra)"
printf 'x' >"$extra/orgtop_0.2.0_plan9_amd64.tar.gz"
assert_rejects "an extra asset" "unexpected asset" -- \
  "$verify" --dir "$extra" --version "$version" --channel source

renamed="$(copy_stage renamed)"
mv "$renamed/gh-orgtop-windows-amd64.exe" "$renamed/gh-orgtop-windows-amd64"
assert_rejects "a raw Windows asset without .exe" "gh-orgtop-windows-amd64.exe" -- \
  "$verify" --dir "$renamed" --version "$version" --channel source

diverged="$(copy_stage diverged)"
printf 'rebuilt executable' >"$diverged/gh-orgtop-linux-amd64"
reseal "$diverged"
assert_rejects "a raw asset rebuilt away from its archive" "does not match the executable" -- \
  "$verify" --dir "$diverged" --version "$version" --channel source

corrupt="$(copy_stage corrupt)"
printf 'rebuilt executable' >"$corrupt/gh-orgtop-linux-amd64"
assert_rejects "an asset that contradicts checksums.txt" "checksums.txt" -- \
  "$verify" --dir "$corrupt" --version "$version" --channel source

# The last fixture archive built above holds a Windows executable, so reusing it
# under the Linux archive name is an archive with no orgtop inside it.
wrongname="$(copy_stage wrongname)"
tar -czf "$wrongname/orgtop_0.2.0_linux_amd64.tar.gz" -C "$build_dir/content" .
reseal "$wrongname"
assert_rejects "an archive that holds no orgtop executable" "does not contain an orgtop executable" -- \
  "$verify" --dir "$wrongname" --version "$version" --channel source

unsorted="$(copy_stage unsorted)"
LC_ALL=C sort -r "$stage/checksums.txt" >"$unsorted/checksums.txt"
assert_rejects "an unsorted checksums.txt" "sorted" -- \
  "$verify" --dir "$unsorted" --version "$version" --channel source

uncovered="$(copy_stage uncovered)"
grep -v 'gh-orgtop-darwin-arm64' "$stage/provenance.intoto.jsonl" >"$uncovered/provenance.intoto.jsonl"
assert_rejects "an artifact no provenance subject covers" "gh-orgtop-darwin-arm64" -- \
  "$verify" --dir "$uncovered" --version "$version" --channel source

blank="$(copy_stage blank)"
printf '\n' >>"$blank/provenance.intoto.jsonl"
assert_rejects "a provenance bundle with a blank line" "blank line" -- \
  "$verify" --dir "$blank" --version "$version" --channel source

# Cross-channel parity: the extension draft must redistribute the source bytes.
# Each channel is internally consistent on its own, so a stale extension asset
# left by an earlier partial run — with its own matching checksums.txt and
# provenance lines — reconciles against itself and would publish different bytes
# under the same names. A-073 names that "extension raw mismatch" and requires it
# to fail closed.
stale="$(copy_stage stale-extension)"
rm -f "$stale"/orgtop_0.2.0_* "$stale/checksums.txt" "$stale/provenance.intoto.jsonl"
printf 'an executable from an earlier run' >"$stale/gh-orgtop-linux-amd64"
seal_checksums "$stale"
provenance_for "$stale" >"$stale/provenance.intoto.jsonl"

assert_verifies "an extension draft that mirrors the source" \
  --dir "$extension" --version "$version" --channel extension --against "$stage"

assert_rejects "an extension asset that diverges from the source" "gh-orgtop-linux-amd64" -- \
  "$verify" --dir "$stale" --version "$version" --channel extension --against "$stage"

# A reference draft that never published an artifact the compared draft carries
# is itself a divergence, not a pass.
assert_rejects "a reference draft missing a compared artifact" "no entry" -- \
  "$verify" --dir "$stage" --version "$version" --channel source --against "$extension"

# ---------------------------------------------------------------------------
# Ledger records
# ---------------------------------------------------------------------------

staged="$("$ledger" staged --version "$version" --tag "$tag" --commit "$commit" \
  --artifacts "$stage/checksums.txt" --checksums-sha "$checksums_sha" --provenance-sha "$provenance_sha")"

# Canonical bytes: sorted keys, no whitespace, no trailing newline.
assert_equal "staged record is one line" "$(printf '%s' "$staged" | wc -l | tr -d ' ')" "0"
assert_contains "staged record" "$staged" '"event":"staged"'
assert_contains "staged record" "$staged" '"source_repository":"fmueller/orgtop"'
assert_equal "staged record key order" \
  "$(printf '%s' "$staged" | jq -r 'keys_unsorted | join(",")')" \
  "artifacts,checksums_sha256,event,provenance_sha256,schema_version,source_commit,source_repository,source_tag,version"
assert_equal "staged artifact count" "$(printf '%s' "$staged" | jq '.artifacts | length')" "12"
assert_equal "staged artifacts are sorted by name" \
  "$(printf '%s' "$staged" | jq -r '[.artifacts[].name] == ([.artifacts[].name] | sort) | tostring')" "true"

# The staged digest hashes the canonical object bytes and excludes the LF the
# ledger line ends with.
staged_sha="$("$ledger" staged --version "$version" --tag "$tag" --commit "$commit" \
  --artifacts "$stage/checksums.txt" --checksums-sha "$checksums_sha" --provenance-sha "$provenance_sha" --digest)"
assert_equal "staged digest excludes the trailing LF" "$staged_sha" "$(digest_of "$staged")"

assert_rejects "a staged record whose version carries a v" "must omit the leading 'v'" -- \
  "$ledger" staged --version "v$version" --tag "$tag" --commit "$commit" \
  --artifacts "$stage/checksums.txt" --checksums-sha "$checksums_sha" --provenance-sha "$provenance_sha"

assert_rejects "a staged record whose tag does not match its version" "does not match" -- \
  "$ledger" staged --version "$version" --tag v0.2.1 --commit "$commit" \
  --artifacts "$stage/checksums.txt" --checksums-sha "$checksums_sha" --provenance-sha "$provenance_sha"

assert_rejects "a staged record with an upper-case digest" "64 lowercase" -- \
  "$ledger" staged --version "$version" --tag "$tag" --commit "$commit" \
  --artifacts "$stage/checksums.txt" --checksums-sha "${checksums_sha^^}" --provenance-sha "$provenance_sha"

assert_rejects "a staged record missing artifacts" "exactly 12" -- \
  "$ledger" staged --version "$version" --tag "$tag" --commit "$commit" \
  --artifacts "$extension/checksums.txt" --checksums-sha "$checksums_sha" --provenance-sha "$provenance_sha"

source_manifest_sha="$(digest_of source-manifest)"
extension_manifest_sha="$(digest_of extension-manifest)"

completed="$("$ledger" completed --version "$version" --staged-sha "$staged_sha" \
  --source-manifest-sha "$source_manifest_sha" --extension-manifest-sha "$extension_manifest_sha" \
  --tap-commit "$tap_commit")"
assert_equal "completed record key order" \
  "$(printf '%s' "$completed" | jq -r 'keys_unsorted | join(",")')" \
  "event,extension_manifest_sha256,schema_version,source_manifest_sha256,staged_sha256,tap_commit,version"
assert_contains "completed record" "$completed" '"event":"completed"'

assert_rejects "a completed record without a tap commit" "40 lowercase" -- \
  "$ledger" completed --version "$version" --staged-sha "$staged_sha" \
  --source-manifest-sha "$source_manifest_sha" --extension-manifest-sha "$extension_manifest_sha" \
  --tap-commit ""

# Withdrawal: the notice path and URL are derived, never supplied, so a record
# can never bind a notice the protected PR did not create.
withdrawn="$("$ledger" withdrawn --version "$version" --staged-sha "$staged_sha" \
  --state incomplete --reason "extension publication failed" --tap-commit "")"
assert_equal "withdrawn notice path" "$(printf '%s' "$withdrawn" | jq -r '.notice_path')" "docs/withdrawals/v0.2.0.md"
assert_equal "withdrawn notice url" "$(printf '%s' "$withdrawn" | jq -r '.notice_url')" \
  "https://github.com/fmueller/orgtop/blob/main/docs/withdrawals/v0.2.0.md"
assert_equal "a staging-only withdrawal has no tap commit" \
  "$(printf '%s' "$withdrawn" | jq -r '.tap_commit')" "null"
assert_equal "withdrawn record key order" \
  "$(printf '%s' "$withdrawn" | jq -r 'keys_unsorted | join(",")')" \
  "event,notice_path,notice_url,publication_state,reason,schema_version,staged_sha256,tap_commit,version"

published_withdrawal="$("$ledger" withdrawn --version "$version" --staged-sha "$staged_sha" \
  --state completed --reason "wrong bytes" --tap-commit "$tap_commit")"
assert_equal "a completed withdrawal records the pre-withdrawal formula commit" \
  "$(printf '%s' "$published_withdrawal" | jq -r '.tap_commit')" "$tap_commit"

assert_rejects "a withdrawal with no reason" "reason" -- \
  "$ledger" withdrawn --version "$version" --staged-sha "$staged_sha" \
  --state completed --reason "" --tap-commit "$tap_commit"

assert_rejects "a withdrawal in an unknown publication state" "publication state" -- \
  "$ledger" withdrawn --version "$version" --staged-sha "$staged_sha" \
  --state partial --reason "wrong bytes" --tap-commit "$tap_commit"

# ---------------------------------------------------------------------------
# Completion manifest
# ---------------------------------------------------------------------------

complete_json="$("$manifest" --tag "$tag" --commit "$commit" --workflow-commit "$workflow_commit" \
  --tap-commit "$tap_commit" --checksums "$stage/checksums.txt" --provenance "$stage/provenance.intoto.jsonl")"

assert_equal "manifest schema version" "$(printf '%s' "$complete_json" | jq -r '.schema_version')" "1"
assert_equal "manifest source url" "$(printf '%s' "$complete_json" | jq -r '.source.release_url')" \
  "https://github.com/fmueller/orgtop/releases/tag/v0.2.0"
assert_equal "manifest extension url" "$(printf '%s' "$complete_json" | jq -r '.extension.release_url')" \
  "https://github.com/fmueller/gh-orgtop/releases/tag/v0.2.0"
assert_equal "manifest workflow path" "$(printf '%s' "$complete_json" | jq -r '.source.workflow.path')" \
  ".github/workflows/release.yml"
assert_equal "manifest formula path" "$(printf '%s' "$complete_json" | jq -r '.tap.formula_path')" \
  "Formula/orgtop.rb"
assert_equal "manifest target count" "$(printf '%s' "$complete_json" | jq '.targets | length')" "6"
assert_equal "manifest target order" \
  "$(printf '%s' "$complete_json" | jq -r '[.targets[] | .os + "/" + .architecture] | join(",")')" \
  "darwin/amd64,darwin/arm64,linux/amd64,linux/arm64,windows/amd64,windows/arm64"
assert_equal "manifest ends without a newline" "$(printf '%s' "$complete_json" | tail -c 1)" "}"

# Each target binds its archive and raw asset to separate provenance subject
# digests, and each equals the published artifact digest.
assert_equal "manifest archive name" \
  "$(printf '%s' "$complete_json" | jq -r '.targets[2].archive.name')" "orgtop_0.2.0_linux_amd64.tar.gz"
assert_equal "manifest raw name" \
  "$(printf '%s' "$complete_json" | jq -r '.targets[2].raw.name')" "gh-orgtop-linux-amd64"
assert_equal "manifest archive provenance subject digest matches the archive digest" \
  "$(printf '%s' "$complete_json" | jq -r '.targets[2] | .archive.sha256 == .archive.provenance_subject_sha256 | tostring')" "true"
assert_equal "manifest raw provenance subject digest matches the raw digest" \
  "$(printf '%s' "$complete_json" | jq -r '.targets[2] | .raw.sha256 == .raw.provenance_subject_sha256 | tostring')" "true"
assert_equal "manifest executable digest is the raw asset digest" \
  "$(printf '%s' "$complete_json" | jq -r '.targets[2] | .executable_sha256 == .raw.sha256 | tostring')" "true"

# The manifest is byte-identical across runs, which is what create-or-compare
# retry semantics depend on.
again="$("$manifest" --tag "$tag" --commit "$commit" --workflow-commit "$workflow_commit" \
  --tap-commit "$tap_commit" --checksums "$stage/checksums.txt" --provenance "$stage/provenance.intoto.jsonl")"
assert_equal "manifest is byte-identical on a retry" "$again" "$complete_json"

assert_rejects "a manifest whose tag is not a semantic version tag" "semantic version tag" -- \
  "$manifest" --tag 0.2.0 --commit "$commit" --workflow-commit "$workflow_commit" \
  --tap-commit "$tap_commit" --checksums "$stage/checksums.txt" --provenance "$stage/provenance.intoto.jsonl"

assert_rejects "a manifest whose commit is not a full sha" "40 lowercase" -- \
  "$manifest" --tag "$tag" --commit deadbeef --workflow-commit "$workflow_commit" \
  --tap-commit "$tap_commit" --checksums "$stage/checksums.txt" --provenance "$stage/provenance.intoto.jsonl"

short="$tmp_dir/short-checksums.txt"
grep -v 'gh-orgtop-windows-arm64.exe' "$stage/checksums.txt" >"$short"
assert_rejects "a manifest missing an artifact checksum" "gh-orgtop-windows-arm64.exe" -- \
  "$manifest" --tag "$tag" --commit "$commit" --workflow-commit "$workflow_commit" \
  --tap-commit "$tap_commit" --checksums "$short" --provenance "$stage/provenance.intoto.jsonl"

divergent="$tmp_dir/divergent.jsonl"
jq -c 'if .subject[0].name == "gh-orgtop-linux-amd64" then .subject[0].digest.sha256 = "'"$(digest_of other)"'" else . end' \
  "$stage/provenance.intoto.jsonl" >"$divergent"
assert_rejects "a manifest whose provenance subject digest diverges" "gh-orgtop-linux-amd64" -- \
  "$manifest" --tag "$tag" --commit "$commit" --workflow-commit "$workflow_commit" \
  --tap-commit "$tap_commit" --checksums "$stage/checksums.txt" --provenance "$divergent"

# ---------------------------------------------------------------------------
# Homebrew formula
# ---------------------------------------------------------------------------

formula="$("$formula_script" --tag "$tag" --commit "$commit" --checksums "$stage/checksums.txt")"

assert_contains "the formula" "$formula" 'class Orgtop < Formula'
assert_equal "the formula version is the tag without its v" \
  "$(printf '%s\n' "$formula" | sed -n 's/^  version "\(.*\)"$/\1/p')" "$version"
assert_contains "the formula test" "$formula" 'assert_match "0.2.0", shell_output("#{bin}/orgtop --version")'
assert_contains "the formula install" "$formula" 'bin.install "orgtop"'

# Exactly the four Linux/macOS rows, each pinned to the canonical versioned URL
# and the digest checksums.txt records. Windows has no Homebrew channel.
assert_equal "the formula pins four archives" \
  "$(printf '%s\n' "$formula" | grep -c '^      url ')" "4"
assert_equal "the formula pins no Windows archive" \
  "$(printf '%s\n' "$formula" | grep -c 'windows')" "0"

while IFS=$'\t' read -r os arch archive raw homebrew; do
  expected_url="      url \"https://github.com/fmueller/orgtop/releases/download/$tag/$archive\""
  expected_sha="      sha256 \"$(digest_of_file "$stage/$archive")\""
  if [ "$homebrew" = yes ]; then
    assert_contains "the formula url for $os/$arch" "$formula" "$expected_url"
    assert_contains "the formula sha for $os/$arch" "$formula" "$expected_sha"
  else
    if [[ "$formula" == *"$archive"* ]]; then
      fail "the formula names the Windows archive $archive"
    fi
  fi
done < <("$matrix" "$version")

# The formula identifies the canonical repository and the attestation route, so
# the tap publishes a provenance reference rather than a second build claim.
assert_contains "the formula provenance reference" "$formula" "Source tag:    $tag"
assert_contains "the formula provenance reference" "$formula" "Source commit: $commit"
assert_contains "the formula provenance reference" "$formula" "gh attestation verify"

assert_equal "the formula is byte-identical on a retry" \
  "$("$formula_script" --tag "$tag" --commit "$commit" --checksums "$stage/checksums.txt")" "$formula"

no_darwin="$tmp_dir/no-darwin-checksums.txt"
grep -v 'orgtop_0.2.0_darwin_arm64.tar.gz' "$stage/checksums.txt" >"$no_darwin"
assert_rejects "a formula missing an archive checksum" "orgtop_0.2.0_darwin_arm64.tar.gz" -- \
  "$formula_script" --tag "$tag" --commit "$commit" --checksums "$no_darwin"

assert_rejects "a formula for a bare version" "semantic version tag" -- \
  "$formula_script" --tag "$version" --commit "$commit" --checksums "$stage/checksums.txt"

# ---------------------------------------------------------------------------
# Ledger append, reuse, and the partial-release reservation
# ---------------------------------------------------------------------------

# The ledger is the durable record the protected pull request commits. Appending
# is create-if-absent or compare-exactly: a retry of the same transition reuses
# the existing line, and a same-version event with different bytes is the
# contradictory state RG-011 refuses to reconcile.
ledger_file="$tmp_dir/distribution-ledger.jsonl"
: >"$ledger_file"

staged_event="$tmp_dir/staged.json"
printf '%s' "$staged" >"$staged_event"

assert_appends() {
  local name="$1" output
  shift
  if ! output="$("$append" "$@" 2>&1)"; then
    fail "$name was rejected: $output"
  fi
  printf '%s' "$output"
}

assert_appends "the first staged event" --ledger "$ledger_file" --event-file "$staged_event" >/dev/null
assert_equal "the ledger holds one line" "$(wc -l <"$ledger_file" | tr -d ' ')" "1"
assert_equal "the ledger line is the canonical record" "$(head -c -1 "$ledger_file")" "$staged"

reused="$(assert_appends "the same staged event again" --ledger "$ledger_file" --event-file "$staged_event")"
assert_equal "an exact retry appends nothing" "$(wc -l <"$ledger_file" | tr -d ' ')" "1"
assert_contains "an exact retry" "$reused" "already recorded"

contradiction="$tmp_dir/contradiction.json"
"$ledger" staged --version "$version" --tag "$tag" --commit "$(digest_of other | cut -c1-40)" \
  --artifacts "$stage/checksums.txt" --checksums-sha "$checksums_sha" --provenance-sha "$provenance_sha" >"$contradiction"
assert_rejects "a second, different staged event for one version" "contradictory" -- \
  "$append" --ledger "$ledger_file" --event-file "$contradiction"

assert_rejects "an event that is not canonical bytes" "canonical" -- \
  "$append" --ledger "$ledger_file" --event-file "$stage/checksums.txt"

# The reservation rule: a version whose staged event has no completed or
# withdrawn event is partial, and no later version may publish over it.
assert_rejects "publishing 0.2.1 over a partial 0.2.0" "0.2.0" -- \
  "$append" --ledger "$ledger_file" --assert-publishable 0.2.1

assert_appends "retrying the partial version itself" --ledger "$ledger_file" --assert-publishable "$version" >/dev/null

completed_event="$tmp_dir/completed.json"
printf '%s' "$completed" >"$completed_event"
assert_appends "the completed event" --ledger "$ledger_file" --event-file "$completed_event" >/dev/null
assert_equal "the ledger holds two lines" "$(wc -l <"$ledger_file" | tr -d ' ')" "2"

assert_appends "publishing 0.2.1 over a completed 0.2.0" --ledger "$ledger_file" --assert-publishable 0.2.1 >/dev/null

# A completed version is permanently non-reusable.
assert_rejects "restaging a completed version" "completed" -- \
  "$append" --ledger "$ledger_file" --assert-publishable "$version"

# A withdrawn partial release also clears the reservation.
withdrawn_ledger="$tmp_dir/withdrawn-ledger.jsonl"
printf '%s\n' "$staged" >"$withdrawn_ledger"
assert_rejects "publishing over a partial release" "0.2.0" -- \
  "$append" --ledger "$withdrawn_ledger" --assert-publishable 0.3.0
printf '%s' "$withdrawn" >"$tmp_dir/withdrawn.json"
assert_appends "the withdrawal event" --ledger "$withdrawn_ledger" --event-file "$tmp_dir/withdrawn.json" >/dev/null
assert_appends "publishing after a withdrawal" --ledger "$withdrawn_ledger" --assert-publishable 0.3.0 >/dev/null

# Every line has to end with exactly one LF; a truncated final line means the
# ledger was written by something other than this guard.
truncated="$tmp_dir/truncated.jsonl"
printf '%s' "$staged" >"$truncated"
assert_rejects "a ledger whose last line has no LF" "LF" -- \
  "$append" --ledger "$truncated" --assert-publishable 0.3.0

# ---------------------------------------------------------------------------
# Withdrawal notice
# ---------------------------------------------------------------------------

notice="$("$notice_script" --version "$version" --state completed --reason "wrong bytes" \
  --staged-sha "$staged_sha" --replacement 0.2.1)"
assert_contains "the notice heading" "$notice" "# Withdrawal notice: v0.2.0"
assert_contains "the notice state" "$notice" "Publication state at withdrawal: completed"
assert_contains "the notice staged digest" "$notice" "$staged_sha"
assert_contains "the notice reason" "$notice" "wrong bytes"
assert_contains "the notice replacement" "$notice" "Replacement version: 0.2.1"
assert_contains "the notice records the irrecoverable limitation" "$notice" "cannot be revoked"
assert_contains "the notice reserves a completed version" "$notice" "permanently reserved"

staging_notice="$("$notice_script" --version "$version" --state incomplete --reason "extension failed" \
  --staged-sha "$staged_sha")"
assert_contains "an incomplete notice permits an identical restage" "$staging_notice" "identical source commit"

assert_rejects "a notice with no reason" "reason" -- \
  "$notice_script" --version "$version" --state completed --reason "" --staged-sha "$staged_sha"

# RG-011 requires the reason to be sanitized public copy. It is committed
# verbatim to a permanent public document and copied into the ledger record, so
# control characters and line breaks are rejected rather than published.
assert_rejects "a reason carrying a line break" "single line" -- \
  "$notice_script" --version "$version" --state completed --reason "first
second" --staged-sha "$staged_sha"

assert_rejects "a reason carrying a control character" "control character" -- \
  "$notice_script" --version "$version" --state completed --reason "$(printf 'bad\tcopy')" --staged-sha "$staged_sha"

assert_rejects "a ledger reason carrying a line break" "single line" -- \
  "$ledger" withdrawn --version "$version" --staged-sha "$staged_sha" \
  --state completed --reason "first
second" --tap-commit "$tap_commit"

# ---------------------------------------------------------------------------
# Ledger pull-request readiness
# ---------------------------------------------------------------------------

# RG-011 requires an independent human approval before any built asset becomes
# public. The workflow cannot read that from `reviewDecision`: GitHub leaves it
# null unless a branch protection or ruleset requires review, so on a repository
# whose default branch carries no rule an approved, green pull request would
# never satisfy it and the release would stall until the poll bound expired.
# The approval is therefore read from the reviews themselves, which is the same
# answer under a rule and without one.
pr_state() {
  local mergeable="$1" author="$2" reviews="$3" checks="${4:-[]}"
  printf '{"mergeable":"%s","author":{"login":"%s"},"latestReviews":%s,"statusCheckRollup":%s}' \
    "$mergeable" "$author" "$reviews" "$checks"
}

approved_by_human="$(pr_state MERGEABLE orgtop-distribution '[{"state":"APPROVED","author":{"login":"fmueller"}}]')"
assert_equal "an approved green pull request is ready" \
  "$(pull_request_readiness "$approved_by_human")" ready

assert_equal "an unreviewed pull request waits" \
  "$(pull_request_readiness "$(pr_state MERGEABLE orgtop-distribution '[]')")" waiting

assert_equal "a commented-on pull request is not approved" \
  "$(pull_request_readiness "$(pr_state MERGEABLE orgtop-distribution '[{"state":"COMMENTED","author":{"login":"fmueller"}}]')")" waiting

assert_equal "a changes-requested pull request waits" \
  "$(pull_request_readiness "$(pr_state MERGEABLE orgtop-distribution '[{"state":"CHANGES_REQUESTED","author":{"login":"fmueller"}}]')")" waiting

# The App opens the pull request; an approval carrying its own login is not the
# independent approval RG-011 requires, whatever GitHub would allow.
assert_equal "a self-approval is not independent" \
  "$(pull_request_readiness "$(pr_state MERGEABLE orgtop-distribution '[{"state":"APPROVED","author":{"login":"orgtop-distribution"}}]')")" waiting

# A later approval by a second reviewer still counts even when the App itself
# appears among the reviewers.
assert_equal "an independent approval beside a self-approval is ready" \
  "$(pull_request_readiness "$(pr_state MERGEABLE orgtop-distribution '[{"state":"APPROVED","author":{"login":"orgtop-distribution"}},{"state":"APPROVED","author":{"login":"fmueller"}}]')")" ready

assert_equal "a failed check blocks an approved pull request" \
  "$(pull_request_readiness "$(pr_state MERGEABLE orgtop-distribution '[{"state":"APPROVED","author":{"login":"fmueller"}}]' '[{"conclusion":"FAILURE"}]')")" waiting

assert_equal "a pending check blocks an approved pull request" \
  "$(pull_request_readiness "$(pr_state MERGEABLE orgtop-distribution '[{"state":"APPROVED","author":{"login":"fmueller"}}]' '[{"conclusion":null}]')")" waiting

assert_equal "a skipped check does not block" \
  "$(pull_request_readiness "$(pr_state MERGEABLE orgtop-distribution '[{"state":"APPROVED","author":{"login":"fmueller"}}]' '[{"conclusion":"SKIPPED"},{"conclusion":"SUCCESS"},{"conclusion":"NEUTRAL"}]')")" ready

# A conflict is not something waiting will resolve, so it is reported apart from
# an unfinished review and fails the release rather than spending the poll bound.
assert_equal "a conflicting pull request is reported apart" \
  "$(pull_request_readiness "$(pr_state CONFLICTING orgtop-distribution '[{"state":"APPROVED","author":{"login":"fmueller"}}]')")" conflicting

# One reviewer's approval does not settle another reviewer's outstanding
# objection. Without a branch protection this loop is the only gate, so an
# unaddressed CHANGES_REQUESTED has to block the merge the way GitHub's own
# reviewDecision would under a rule.
assert_equal "an outstanding objection blocks an approval" \
  "$(pull_request_readiness "$(pr_state MERGEABLE orgtop-distribution '[{"state":"APPROVED","author":{"login":"fmueller"}},{"state":"CHANGES_REQUESTED","author":{"login":"reviewer"}}]')")" waiting

assert_equal "a dismissed review is not an approval" \
  "$(pull_request_readiness "$(pr_state MERGEABLE orgtop-distribution '[{"state":"DISMISSED","author":{"login":"fmueller"}}]')")" waiting

# An approval GitHub can no longer attribute to an account is not the
# independent human approval RG-011 requires.
assert_equal "an unattributed approval is not independent" \
  "$(pull_request_readiness "$(pr_state MERGEABLE orgtop-distribution '[{"state":"APPROVED","author":null}]')")" waiting

# statusCheckRollup carries classic commit statuses as well as check runs, and a
# commit status has no conclusion at all. Reading only the conclusion would hold
# a green pull request forever, which is the failure this whole function exists
# to remove.
assert_equal "a successful commit status settles" \
  "$(pull_request_readiness "$(pr_state MERGEABLE orgtop-distribution '[{"state":"APPROVED","author":{"login":"fmueller"}}]' '[{"state":"SUCCESS","context":"ci/legacy"}]')")" ready

assert_equal "a failed commit status blocks" \
  "$(pull_request_readiness "$(pr_state MERGEABLE orgtop-distribution '[{"state":"APPROVED","author":{"login":"fmueller"}}]' '[{"state":"FAILURE","context":"ci/legacy"}]')")" waiting

# The poll loop reads this through a command substitution, where an exit would
# only end the subshell and leave the loop spending its whole bound on a state
# it never understood. A refusal is a non-zero return the caller can see.
if pull_request_readiness 'not json' >/dev/null 2>&1; then
  fail "an unreadable pull request state was accepted"
fi

assert_equal "an unknown mergeable state waits" \
  "$(pull_request_readiness "$(pr_state UNKNOWN orgtop-distribution '[{"state":"APPROVED","author":{"login":"fmueller"}}]')")" waiting

echo "PASS: distribution guards"
