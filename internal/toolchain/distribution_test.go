package toolchain

import (
	"path/filepath"
	"strings"
	"testing"
)

// The distribution guards, and the one Task target that exercises them. They
// only ever run inside the release workflow, against three repositories this
// suite must not touch, so what keeps them honest is the fixture suite plus the
// wiring asserted here.
var (
	distributionTest    = filepath.Join(repoRoot, "scripts", "distribution-test.sh")
	distributionLedger  = filepath.Join(repoRoot, "docs", "distribution-ledger.jsonl")
	distributionScripts = []string{
		"distribution-formula.sh",
		"distribution-ledger.sh",
		"distribution-ledger-append.sh",
		"distribution-manifest.sh",
		"distribution-manifest-asset.sh",
		"distribution-matrix.sh",
		"distribution-notice.sh",
		"distribution-protected-commit.sh",
		"distribution-stage-assets.sh",
		"distribution-upload-assets.sh",
		"distribution-verify.sh",
	}
	// The fixture suite covers every deterministic guard. These two only make
	// authenticated GitHub calls against repositories no test may touch, so the
	// workflow is the only place they run.
	workflowOnlyScripts = map[string]bool{
		"distribution-manifest-asset.sh":   true,
		"distribution-protected-commit.sh": true,
	}
)

// archiveIDs are the two publication routes one build feeds: the source
// archives v0.1.0 established, and the GitHub CLI extension's raw assets.
const (
	sourceArchiveID    = "orgtop"
	extensionArchiveID = "gh-extension"
)

// TestReleasePublishesTheDistributionMatrix pins the exact asset names RG-011
// fixes. The GitHub CLI extension installs by name: it asks for
// `gh-orgtop-<os>-<arch>` and nothing else, so a changed template does not fail
// the release, it publishes a release the extension channel cannot install
// from. The raw assets also have to come from the same build id as the
// archives, because the whole channel contract is that no channel rebuilds.
func TestReleasePublishesTheDistributionMatrix(t *testing.T) {
	t.Parallel()

	archives := child(loadYAML(t, goreleaserConfig), "archives")
	if archives == nil {
		t.Fatal(".goreleaser.yml must declare archives")
	}

	byID := map[string]*yamlArchive{}
	for _, archive := range archives.Content {
		id := value(child(archive, "id"))
		byID[id] = &yamlArchive{
			builds:  stringSlice(child(archive, "ids")),
			formats: stringSlice(child(archive, "formats")),
			name:    value(child(archive, "name_template")),
		}
	}

	source, ok := byID[sourceArchiveID]
	if !ok {
		t.Fatalf(".goreleaser.yml must declare an archive with id %q", sourceArchiveID)
	}
	if want := "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"; source.name != want {
		t.Errorf("the source archive name_template is %q, want %q", source.name, want)
	}
	if !equalStrings(source.formats, []string{"tar.gz"}) {
		t.Errorf("the source archive formats are %v, want [tar.gz] with a windows zip override", source.formats)
	}

	extension, ok := byID[extensionArchiveID]
	if !ok {
		t.Fatalf(".goreleaser.yml must declare an archive with id %q for the GitHub CLI extension assets", extensionArchiveID)
	}
	if want := "gh-orgtop-{{ .Os }}-{{ .Arch }}"; extension.name != want {
		t.Errorf("the extension raw asset name_template is %q, want %q", extension.name, want)
	}
	// `binary` publishes the built executable itself, renamed. Any container
	// format would make the extension assets archives, which the GitHub CLI
	// extension channel cannot install.
	if !equalStrings(extension.formats, []string{"binary"}) {
		t.Errorf("the extension raw asset formats are %v, want [binary]", extension.formats)
	}

	for id, archive := range map[string]*yamlArchive{sourceArchiveID: source, extensionArchiveID: extension} {
		if !equalStrings(archive.builds, []string{releaseBinary}) {
			t.Errorf("archive %q is built from %v, want the single %q build so no channel rebuilds", id, archive.builds, releaseBinary)
		}
	}

	// Windows is the only row whose archive is a zip, and the override has to
	// name it: a tar.gz on Windows would break the matrix and the formula's
	// implicit "Windows has no Homebrew channel" split.
	overrides := child(archives.Content[0], "format_overrides")
	if overrides == nil || len(overrides.Content) != 1 {
		t.Fatal("the source archive must declare exactly one format override")
	}
	if got := value(child(overrides.Content[0], "goos")); got != "windows" {
		t.Errorf("the source archive format override targets %q, want windows", got)
	}
	if got := stringSlice(child(overrides.Content[0], "formats")); !equalStrings(got, []string{"zip"}) {
		t.Errorf("the windows source archive formats are %v, want [zip]", got)
	}
}

type yamlArchive struct {
	builds  []string
	formats []string
	name    string
}

// TestReleaseStagesBeforeAnythingBecomesPublic keeps built assets out of
// anonymous reach until every channel reconciles. A published-by-default
// release would make the source archives downloadable before the extension
// draft, the formula branch, or the staged ledger event exist, which is exactly
// the partial state RG-011 forbids reporting as a release.
func TestReleaseStagesBeforeAnythingBecomesPublic(t *testing.T) {
	t.Parallel()

	release := child(loadYAML(t, goreleaserConfig), "release")
	if release == nil {
		t.Fatal(".goreleaser.yml must declare a release section")
	}
	if got := value(child(release, "draft")); got != "true" {
		t.Errorf(".goreleaser.yml release draft = %q, want true", got)
	}
	if got := value(child(release, "prerelease")); got != "false" {
		t.Errorf(".goreleaser.yml release prerelease = %q, want false", got)
	}
	// Assets are never overwritten: a retry of the same tag reconciles what is
	// already attached instead of replacing published bytes.
	if got := value(child(release, "mode")); got != "keep-existing" {
		t.Errorf(".goreleaser.yml release mode = %q, want keep-existing", got)
	}
}

// releaseTransitions is the publication order A-072 observes. Each entry is a
// distinguishing fragment of a step name in the tag job.
var releaseTransitions = []string{
	"Refuse to publish over an unresolved release",
	"Verify the companion repositories",
	"Build once and create or reconcile the source draft",
	"Inspect or reuse source provenance",
	"Attest the twelve build artifacts",
	"Stage source draft assets",
	"Verify staged source assets before upload",
	"Upload or reconcile the source draft assets",
	"Reconcile the source draft",
	"Create or reconcile the extension draft",
	"Reconcile the extension draft",
	"Create or reconcile the tap staging branch",
	"Append the staged ledger event",
	"Publish the source release",
	"Publish the extension release",
	"Merge the formula onto the verified tap base",
	"Reconcile every public asset",
	"Add the completion manifest to the source release",
	"Add the completion manifest to the extension release",
	"Append the completed ledger event",
	"Reconcile the completed release",
}

// TestReleaseWorkflowOrdersThePublicationTransitions pins the order RG-011
// fixes. Order is the contract here rather than an implementation detail:
// publishing the extension before the source, or merging the formula before
// either release is public, produces exactly the divergent public state the
// staged lifecycle exists to prevent, and no single step would fail.
func TestReleaseWorkflowOrdersThePublicationTransitions(t *testing.T) {
	t.Parallel()

	names := jobStepValues(loadYAML(t, releaseWorkflow), "release", "name")
	if len(names) == 0 {
		t.Fatal("release.yml must declare a `release` job with named steps")
	}

	position := 0
	for _, transition := range releaseTransitions {
		found := -1
		for i := position; i < len(names); i++ {
			if strings.Contains(names[i], transition) {
				found = i
				break
			}
		}
		if found < 0 {
			t.Fatalf("release.yml runs no step %q after the preceding transition; the publication order is:\n%s",
				transition, strings.Join(names, "\n"))
		}
		position = found + 1
	}
}

// TestReleaseWorkflowMintsTheCompanionTokenSeparately keeps the two credentials
// apart. The repository token publishes the source release only; every
// companion repository mutation uses the narrowly scoped App installation
// token. Handing the repository token to a companion step would either fail or,
// worse, succeed against the wrong repository.
func TestReleaseWorkflowMintsTheCompanionTokenSeparately(t *testing.T) {
	t.Parallel()

	workflow := loadYAML(t, releaseWorkflow)
	source := readFile(t, releaseWorkflow)

	steps := child(jobAt(workflow, "release"), "steps")
	if steps == nil {
		t.Fatal("release.yml must declare a `release` job with steps")
	}

	const appToken = "${{ steps.app.outputs.token }}"
	minted := false
	for _, step := range steps.Content {
		name := value(child(step, "name"))
		tokenNode := nodeAt(step, "env", "GH_TOKEN")

		if strings.Contains(name, "Mint the distribution App token") {
			minted = true
			for _, secret := range []string{"DISTRIBUTION_APP_ID", "DISTRIBUTION_APP_PRIVATE_KEY"} {
				if !strings.Contains(source, secret) {
					t.Errorf("release.yml must mint the App token from %s", secret)
				}
			}
			continue
		}

		companion := strings.Contains(name, "extension") || strings.Contains(name, "companion") ||
			strings.Contains(name, "tap") || strings.Contains(name, "formula")
		if companion && tokenNode != nil && tokenNode.Value != appToken {
			t.Errorf("step %q talks to a companion repository with %q, want the App installation token", name, tokenNode.Value)
		}
		if !minted && companion {
			t.Errorf("step %q touches a companion repository before the App token is minted", name)
		}
	}

	if !minted {
		t.Error("release.yml must mint the distribution App installation token")
	}
}

// TestReleaseWorkflowRequestsProvenancePermissions keeps the source release the
// only thing this repository's own token can do, and keeps the OIDC provenance
// issuable. A missing attestations or id-token permission does not fail the
// build; it fails the attestation step after the draft already exists.
func TestReleaseWorkflowRequestsProvenancePermissions(t *testing.T) {
	t.Parallel()

	workflow := loadYAML(t, releaseWorkflow)
	permissions := child(workflow, "permissions")
	if permissions == nil {
		t.Fatal("release.yml must declare workflow permissions")
	}

	for scope, level := range map[string]string{"contents": "read"} {
		if got := value(child(permissions, scope)); got != level {
			t.Errorf("release.yml default permissions %s = %q, want %q", scope, got, level)
		}
	}
	for _, scope := range []string{"id-token", "attestations"} {
		if child(permissions, scope) != nil {
			t.Errorf("release.yml default permissions must not grant %s outside the publishing job", scope)
		}
	}
	for _, scope := range mappingKeys(permissions) {
		if scope != "contents" {
			t.Errorf("release.yml default requests permission %q, which snapshot/withdraw do not need", scope)
		}
	}

	releasePermissions := child(jobAt(workflow, "release"), "permissions")
	if releasePermissions == nil {
		t.Fatal("the publishing release job must declare its elevated permissions explicitly")
	}
	for scope, level := range map[string]string{
		"contents":     "write",
		"id-token":     "write",
		"attestations": "write",
	} {
		if got := value(child(releasePermissions, scope)); got != level {
			t.Errorf("release job permissions %s = %q, want %q", scope, got, level)
		}
	}
	for _, scope := range mappingKeys(releasePermissions) {
		if scope != "contents" && scope != "id-token" && scope != "attestations" {
			t.Errorf("release job requests permission %q, which no RG-011 transition needs", scope)
		}
	}
}

// TestProvenanceBundleIsDecodedBeforeItIsSplit keeps the canonical bundle
// derivable from what the attestation action actually writes. Its bundle-path
// output is a Sigstore bundle: the in-toto statement, and so every subject, is
// base64 inside .dsseEnvelope.payload, and a jq program reading .subject off
// the bundle itself matches nothing. That produces an empty
// provenance.intoto.jsonl, which the upload rejects as HTTP 400 Bad
// Content-Length after the draft already holds twelve assets, so the failure
// arrives far from its cause and only against a real tag.
func TestProvenanceBundleIsDecodedBeforeItIsSplit(t *testing.T) {
	t.Parallel()

	var assembled string
	for _, step := range child(jobAt(loadYAML(t, releaseWorkflow), "release"), "steps").Content {
		if strings.Contains(value(child(step, "name")), "Assemble the canonical provenance bundle") {
			assembled = value(child(step, "run"))
			break
		}
	}
	if assembled == "" {
		t.Fatal("release.yml must assemble provenance.intoto.jsonl from the attestation bundle")
	}

	if !strings.Contains(assembled, "dsseEnvelope") {
		t.Error("the bundle carries its statement in .dsseEnvelope.payload, so an assembly that never reads that field splits nothing")
	}
	if !strings.Contains(assembled, "@base64d") {
		t.Error("the DSSE payload is base64, so the assembly has to decode it before it can read .subject")
	}
}

// TestReleaseReusesExistingSourceProvenance keeps a retry byte-stable. The
// attestation action includes its workflow invocation identity, so rerunning a
// failed companion publication would otherwise create a different provenance
// asset and collide with the exact source draft from the first attempt.
func TestReleaseReusesExistingSourceProvenance(t *testing.T) {
	t.Parallel()

	steps := child(jobAt(loadYAML(t, releaseWorkflow), "release"), "steps")
	if steps == nil {
		t.Fatal("release.yml must declare release steps")
	}

	var inspect, attest, assemble string
	for _, step := range steps.Content {
		name := value(child(step, "name"))
		run := value(child(step, "run"))
		switch {
		case strings.Contains(name, "Inspect or reuse source provenance"):
			inspect = run
		case strings.Contains(name, "Attest the twelve build artifacts"):
			attest = value(child(step, "if"))
		case strings.Contains(name, "Assemble the canonical provenance bundle"):
			assemble = value(child(step, "if"))
		}
	}

	if inspect == "" {
		t.Fatal("release.yml must inspect the existing source provenance asset before attestation")
	}
	for _, required := range []string{"gh release view", "gh release download", "provenance.intoto.jsonl", "GITHUB_OUTPUT"} {
		if !strings.Contains(inspect, required) {
			t.Errorf("source provenance inspection must contain %q", required)
		}
	}
	for step, condition := range map[string]string{"attestation": attest, "provenance assembly": assemble} {
		if condition == "" || !strings.Contains(condition, "source-provenance.outputs.exists") {
			t.Errorf("%s must run only when source provenance is absent, got condition %q", step, condition)
		}
	}
}

// TestTapStagingCreatesTheFormulaPath keeps the first formula publishable. A
// tap that has never carried one has no Formula directory, so copying into it
// fails, and `commit -a` stages no untracked file, so the very first
// Formula/orgtop.rb could not be committed even once the copy succeeded.
// Neither shows up against a tap that already holds a formula, which is every
// state after the first release.
func TestTapStagingCreatesTheFormulaPath(t *testing.T) {
	t.Parallel()

	var staging string
	for _, command := range jobStepValues(loadYAML(t, releaseWorkflow), "release", "run") {
		if strings.Contains(command, "release/orgtop-") && strings.Contains(command, "Formula/orgtop.rb") {
			staging = command
			break
		}
	}
	if staging == "" {
		t.Fatal("release.yml must stage Formula/orgtop.rb on a tap release branch")
	}

	if !strings.Contains(staging, "mkdir -p") {
		t.Error("tap staging must create the Formula directory; a tap without one cannot receive its first formula")
	}
	if !strings.Contains(staging, "add Formula/orgtop.rb") {
		t.Error("tap staging must add Formula/orgtop.rb explicitly; `commit -a` stages no untracked file")
	}
	if strings.Contains(staging, "commit -am") {
		t.Error("tap staging still commits with -a, which cannot commit the first, untracked formula")
	}
	// git status collapses a wholly untracked directory to `Formula/`, so the
	// guard comparing against the formula path needs the file listed.
	if !strings.Contains(staging, "--porcelain -uall") {
		t.Error("the changed-path guard must read --porcelain -uall, or a new Formula directory reports as `Formula/` and fails as a staging bug")
	}
}

// TestArchivesAreReproducibleAcrossRetries keeps the retry guarantee real. An
// archive records the modification time of the file it packs, so without a
// pinned mod_timestamp every rebuild of one tag produces the same executable
// inside a differently hashed archive. RG-011 requires a retry from the same
// tag to reconcile rather than republish, and reconciliation compares digests:
// the six raw executables would match and all six archives would not, which
// fails the release closed at its first retry and cannot be fixed by retrying
// again. The v0.0.1 rehearsal proved it, with six identical raw digests beside
// six divergent archive digests.
func TestArchivesAreReproducibleAcrossRetries(t *testing.T) {
	t.Parallel()

	builds := child(loadYAML(t, goreleaserConfig), "builds")
	if builds == nil || len(builds.Content) == 0 {
		t.Fatal(".goreleaser.yml must declare a build")
	}

	const want = "{{ .CommitTimestamp }}"
	for _, build := range builds.Content {
		if got := value(child(build, "mod_timestamp")); got != want {
			t.Errorf(".goreleaser.yml build mod_timestamp = %q, want %q: without it each rebuild of one tag archives the executable under a new mtime, and no retry can reconcile", got, want)
		}
	}

	// The executable is one member. An archive records an mtime, owner, and
	// group for each of them, and a checkout stamps the working tree with its
	// own clock, so an unpinned LICENSE or README moves the archive bytes even
	// while the executable stays put. v0.0.2 diverged on exactly that with
	// mod_timestamp already pinned.
	const wantDate = "{{ .CommitDate }}"
	archives := child(loadYAML(t, goreleaserConfig), "archives")
	if archives == nil || len(archives.Content) == 0 {
		t.Fatal(".goreleaser.yml must declare an archive")
	}
	for _, archive := range archives.Content {
		files := child(archive, "files")
		if files == nil {
			// A binary-format archive publishes the executable itself and packs
			// no other member, so it carries no archive metadata to pin.
			continue
		}

		if got := value(child(child(archive, "builds_info"), "mtime")); got != wantDate {
			t.Errorf(".goreleaser.yml archive builds_info.mtime = %q, want %q", got, wantDate)
		}
		for _, file := range files.Content {
			name := value(child(file, "src"))
			if name == "" || name == "<missing>" {
				t.Errorf(".goreleaser.yml archive packs %q as a bare path, which carries no pinned metadata", value(file))
				continue
			}
			info := child(file, "info")
			if got := value(child(info, "mtime")); got != wantDate {
				t.Errorf(".goreleaser.yml archive file %s info.mtime = %q, want %q", name, got, wantDate)
			}
			for _, field := range []string{"owner", "group"} {
				if got := value(child(info, field)); got == "" || got == "<missing>" {
					t.Errorf(".goreleaser.yml archive file %s pins no %s, so the archive records the build account's and is not reproducible off another host", name, field)
				}
			}
		}
	}
}

// TestWithdrawalToleratesAnUnpublishedFormula keeps the staging-only
// withdrawal a withdrawal. RG-011 names the case: a version staged but never
// completed records a null tap_commit, and the tap default branch carries no
// formula for it — on a tap publishing its first formula, no formula at all.
// Reading that file unconditionally 404s, and decoding the empty body fails the
// step, which strands the releases and tags the following steps exist to
// delete. The v0.0.1 rehearsal stranded exactly those.
func TestWithdrawalToleratesAnUnpublishedFormula(t *testing.T) {
	t.Parallel()

	var revert string
	for _, command := range jobStepValues(loadYAML(t, releaseWorkflow), "withdraw", "run") {
		if strings.Contains(command, "Formula/orgtop.rb?ref=main") {
			revert = command
			break
		}
	}
	if revert == "" {
		t.Fatal("release.yml must reconcile the tap formula when a version is withdrawn")
	}

	if !strings.Contains(revert, "nothing to revert") {
		t.Error("the revert must treat an absent tap formula as nothing to revert; a 404 there fails the step and strands the releases and tags")
	}
}

// TestWithdrawalDeletesDraftsByIdentity keeps a never-published version
// removable. GitHub's get-release-by-tag endpoint does not see draft releases,
// so resolving a release by its tag name finds nothing for a version withdrawn
// while still staged — while `gh release view` does find it, because it lists
// releases instead. Deleting by tag therefore reports success, removes the git
// tag, and leaves the draft and its twelve assets in place. The v0.0.1
// withdrawal did exactly that, and the guard behind it correctly refused to
// call the version withdrawn.
func TestWithdrawalDeletesDraftsByIdentity(t *testing.T) {
	t.Parallel()

	var deletion string
	for _, command := range jobStepValues(loadYAML(t, releaseWorkflow), "withdraw", "run") {
		if strings.Contains(command, "still exists in") {
			deletion = command
			break
		}
	}
	if deletion == "" {
		t.Fatal("release.yml must delete the releases and tags of a withdrawn version")
	}

	if strings.Contains(deletion, "gh release delete") {
		t.Error("the withdrawal must not delete by tag name: that resolution cannot see a draft, so it removes the tag and keeps the release")
	}
	if !strings.Contains(deletion, "releases/$") && !strings.Contains(deletion, "releases/${") {
		t.Error("the withdrawal must delete the release by its own id, resolved from a listing that includes drafts")
	}
}

// TestReleaseReusesTheExistingDraft keeps a retry a retry. RG-011 requires
// expected drafts to be reused and assets never to be overwritten, but the
// release step resolves its release by tag, and GitHub's get-release-by-tag
// endpoint does not see a draft. Left to that, every retry of a tag whose
// release is still a draft creates a second draft for the same tag, which is
// the contradictory same-version state reconciliation exists to refuse — and
// then refuses it on a digest mismatch rather than on the duplication. The
// v0.0.2 retry produced two drafts for one tag, holding fourteen and thirteen
// assets.
func TestReleaseReusesTheExistingDraft(t *testing.T) {
	t.Parallel()

	release := child(loadYAML(t, goreleaserConfig), "release")
	if release == nil {
		t.Fatal(".goreleaser.yml must configure the release")
	}

	if got := value(child(release, "use_existing_draft")); got != "true" {
		t.Errorf(".goreleaser.yml release use_existing_draft = %q, want \"true\": without it a retry creates a second draft for the same tag", got)
	}
	if got := value(child(release, "draft")); got != "true" {
		t.Errorf(".goreleaser.yml release draft = %q, want \"true\": nothing built may be anonymously downloadable before the publication transition", got)
	}
	if got := value(child(release, "mode")); got != "keep-existing" {
		t.Errorf(".goreleaser.yml release mode = %q, want \"keep-existing\"", got)
	}
	if got := value(child(release, "skip_upload")); got != "true" {
		t.Errorf(".goreleaser.yml release skip_upload = %q, want \"true\": GoReleaser must not retry asset uploads outside the compare-before-upload guard", got)
	}
	// Overwriting an asset is republishing bytes under a name that already
	// carried different ones, which no retry is allowed to do.
	if got := value(child(release, "replace_existing_artifacts")); got == "true" {
		t.Error(".goreleaser.yml release replace_existing_artifacts is true, so a retry would overwrite published bytes instead of comparing them")
	}
}

// TestReleaseWorkflowUsesTheUploadGuard keeps asset publication behind the
// create-if-absent boundary. Direct `gh release upload` or a clobber flag in the
// tag job would bypass digest comparison and could replace an existing asset on
// a retry.
func TestReleaseWorkflowUsesTheUploadGuard(t *testing.T) {
	t.Parallel()

	steps := child(jobAt(loadYAML(t, releaseWorkflow), "release"), "steps")
	sourceUpload := ""
	extensionUpload := ""
	for _, step := range steps.Content {
		name := value(child(step, "name"))
		run := value(child(step, "run"))
		switch {
		case strings.Contains(name, "Upload or reconcile the source draft assets"):
			sourceUpload = run
		case strings.Contains(name, "Create or reconcile the extension draft"):
			extensionUpload = run
		}
	}
	for name, command := range map[string]string{
		"source":    sourceUpload,
		"extension": extensionUpload,
	} {
		if !strings.Contains(command, "distribution-upload-assets.sh") ||
			!strings.Contains(command, "--channel "+name) {
			t.Errorf("the %s publication step must call the upload guard with its channel, got %q", name, command)
		}
	}
	commands := strings.Join(jobStepValues(loadYAML(t, releaseWorkflow), "release", "run"), "\n")
	if strings.Contains(commands, "gh release upload") {
		t.Error("release.yml must not upload assets directly; direct gh release upload bypasses compare-before-upload semantics")
	}
	if strings.Contains(commands, "release upload") && strings.Contains(commands, "--clobber") {
		t.Error("release.yml must not clobber release assets")
	}
}

// TestReleaseStagesGoReleaserArtifactsForUpload keeps the guard's input
// complete. GoReleaser's binary-format artifacts retain their target-directory
// paths on disk even though their release names are the six gh-orgtop-* names;
// uploading the raw `dist` directory directly would therefore omit them.
func TestReleaseStagesGoReleaserArtifactsForUpload(t *testing.T) {
	t.Parallel()

	var staging string
	for _, step := range child(jobAt(loadYAML(t, releaseWorkflow), "release"), "steps").Content {
		if strings.Contains(value(child(step, "name")), "Stage source draft assets") {
			staging = value(child(step, "run"))
			break
		}
	}
	if staging == "" {
		t.Fatal("release.yml must stage the source draft assets before upload")
	}
	for _, required := range []string{"distribution-stage-assets.sh", "source-upload"} {
		if !strings.Contains(staging, required) {
			t.Errorf("source asset staging must contain %q", required)
		}
	}
	helper := readFile(t, filepath.Join(repoRoot, "scripts", "distribution-stage-assets.sh"))
	for _, required := range []string{"artifacts.json", "distribution_matrix_rows", "gh-extension", "checksums_asset", "provenance_asset", "realpath -e"} {
		if !strings.Contains(helper, required) {
			t.Errorf("source asset staging helper must contain %q", required)
		}
	}
}

// TestReleaseRefusesDuplicateSameVersionReleases keeps the duplication
// diagnosable. Two releases for one tag is non-reconcilable same-version state
// under RG-011, and it has to be named where it happens: found only through a
// later digest mismatch, it reads as divergent bytes from a rebuild, which is a
// different defect with a different fix.
func TestReleaseRefusesDuplicateSameVersionReleases(t *testing.T) {
	t.Parallel()

	var guarded bool
	for _, command := range jobStepValues(loadYAML(t, releaseWorkflow), "release", "run") {
		if strings.Contains(command, "more than one release") {
			guarded = true
			break
		}
	}
	if !guarded {
		t.Error("release.yml must refuse a tag carrying more than one release, in the channel it built, before it reconciles digests")
	}
}

// TestSnapshotRehearsalPublishesNothing keeps the manual dispatch a rehearsal.
// The snapshot exists so the release path can be exercised off a tag; a `gh
// release` or a token in that job would make the rehearsal a publication.
func TestSnapshotRehearsalPublishesNothing(t *testing.T) {
	t.Parallel()

	workflow := loadYAML(t, releaseWorkflow)
	for _, command := range jobStepValues(workflow, "snapshot", "run") {
		for _, forbidden := range []string{"gh release", "gh pr", "gh api"} {
			if strings.Contains(command, forbidden) {
				t.Errorf("the snapshot rehearsal must publish nothing, but runs %q: %q", forbidden, command)
			}
		}
	}

	steps := child(jobAt(workflow, "snapshot"), "steps")
	if steps == nil {
		t.Fatal("release.yml must declare a `snapshot` job")
	}
	for _, step := range steps.Content {
		if env := child(step, "env"); env != nil {
			for _, key := range mappingKeys(env) {
				if strings.Contains(key, "TOKEN") {
					t.Errorf("snapshot step %q receives %s; the rehearsal needs no credential",
						value(child(step, "name")), key)
				}
			}
		}
	}
}

// TestWithdrawalRecordsBeforeItRemoves pins the order A-074 fixes. The durable
// notice and the withdrawal event have to be on the default branch before any
// release, tag, or formula is removed: they are what survives the deletions, and
// a removal-first order can leave a version withdrawn with no public record of
// why.
func TestWithdrawalRecordsBeforeItRemoves(t *testing.T) {
	t.Parallel()

	names := jobStepValues(loadYAML(t, releaseWorkflow), "withdraw", "name")
	if len(names) == 0 {
		t.Fatal("release.yml must declare a `withdraw` job with named steps")
	}

	record, remove := -1, -1
	for i, name := range names {
		if record < 0 && strings.Contains(name, "Append the withdrawal event and notice") {
			record = i
		}
		if remove < 0 && (strings.Contains(name, "Delete the releases") || strings.Contains(name, "Revert the tap formula")) {
			remove = i
		}
	}
	if record < 0 {
		t.Fatal("the withdrawal must append its event and notice through the protected pull request")
	}
	if remove < 0 {
		t.Fatal("the withdrawal must remove the published releases and tags")
	}
	if record > remove {
		t.Errorf("release.yml removes published state at step %d before recording the withdrawal at step %d", remove, record)
	}
}

// TestWithdrawalRestoresEveryChannel keeps the withdrawal a withdrawal. Ordering
// alone does not prove the public state is actually reverted: a step that
// reports what a human should do next, or a deletion whose failure is swallowed,
// leaves the version half withdrawn while the workflow reports success — the
// partial state RG-011 requires the withdrawal to end.
func TestWithdrawalRestoresEveryChannel(t *testing.T) {
	t.Parallel()

	workflow := loadYAML(t, releaseWorkflow)
	commands := jobStepValues(workflow, "withdraw", "run")
	if len(commands) == 0 {
		t.Fatal("release.yml must declare a `withdraw` job with run steps")
	}
	script := strings.Join(commands, "\n")

	// The formula revert lands through the tap's own protected pull request, the
	// same route the publication used.
	for _, required := range []string{"revert --no-edit", "gh pr merge"} {
		if !strings.Contains(script, required) {
			t.Errorf("the withdrawal must revert the published formula itself; no step runs %q", required)
		}
	}

	// The public staging branch is deleted whether or not the formula was ever
	// merged: an incomplete publication leaves exactly that branch behind.
	if !strings.Contains(script, `branch="release/orgtop-v${VERSION}"`) ||
		!strings.Contains(script, `-X DELETE "repos/${TAP_REPOSITORY}/git/refs/heads/${branch}"`) {
		t.Error("the withdrawal must delete the tap staging branch it created")
	}

	// A swallowed deletion failure is indistinguishable from success.
	for _, command := range commands {
		for _, line := range strings.Split(command, "\n") {
			if strings.Contains(line, "gh release delete") && strings.Contains(line, "|| true") {
				t.Errorf("the withdrawal must not swallow a deletion failure: %q", strings.TrimSpace(line))
			}
		}
	}
	if !strings.Contains(script, "still exists in") {
		t.Error("the withdrawal must verify the releases and tags are actually gone")
	}
}

// TestExtensionReconciliationComparesAgainstTheSource keeps the two channels
// compared to each other and not only to themselves. Each draft is internally
// consistent on its own, so a stale extension asset from an earlier partial run
// carries matching metadata and reconciles — while redistributing bytes the
// source never published. A-073 names that mismatch and requires it closed.
func TestExtensionReconciliationComparesAgainstTheSource(t *testing.T) {
	t.Parallel()

	for _, command := range jobStepValues(loadYAML(t, releaseWorkflow), "release", "run") {
		if !strings.Contains(command, "distribution-verify.sh") {
			continue
		}
		if !strings.Contains(command, "--channel extension") && !strings.Contains(command, "extension") {
			continue
		}
		if !strings.Contains(command, "--against") {
			t.Errorf("an extension reconciliation runs without --against, so it compares the channel only to itself: %q", command)
		}
	}
}

// TestDistributionGuardsAreExercised keeps every guard under the fixture suite
// and keeps that suite in the gate. A guard nothing runs is a guard that first
// reports its regression halfway through a real release, against repositories
// no test may touch.
func TestDistributionGuardsAreExercised(t *testing.T) {
	t.Parallel()

	fixtures := readFile(t, distributionTest)
	workflow := readFile(t, releaseWorkflow)

	for _, script := range distributionScripts {
		if !strings.Contains(fixtures, script) && !strings.Contains(workflow, script) {
			t.Errorf("scripts/%s is referenced by neither the fixture suite nor release.yml", script)
		}
		if !workflowOnlyScripts[script] && !strings.Contains(fixtures, script) {
			t.Errorf("scripts/%s has no fixture coverage in scripts/distribution-test.sh", script)
		}
	}

	if !strings.Contains(readFile(t, taskfile), "scripts/distribution-test.sh") {
		t.Error("Taskfile.yml must run the distribution fixture suite")
	}
}

// TestDistributionLedgerIsCanonical keeps the committed ledger readable by the
// guard that appends to it. The ledger is authoritative for same-version reuse
// and for the reservation that blocks a later tag while an earlier release is
// partial, so a hand-edited or reformatted line would silently change which
// versions may publish.
func TestDistributionLedgerIsCanonical(t *testing.T) {
	t.Parallel()

	ledger := readFile(t, distributionLedger)
	if ledger == "" {
		return // No release has been staged yet.
	}
	if !strings.HasSuffix(ledger, "\n") {
		t.Error("docs/distribution-ledger.jsonl must end every event with exactly one LF")
	}
	for i, line := range strings.Split(strings.TrimSuffix(ledger, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			t.Errorf("docs/distribution-ledger.jsonl line %d is blank", i+1)
		}
		if strings.Contains(line, ": ") || strings.HasPrefix(line, "{ ") {
			t.Errorf("docs/distribution-ledger.jsonl line %d is not canonical RFC 8785 bytes: %q", i+1, line)
		}
	}
}
