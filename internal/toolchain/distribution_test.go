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
	"Attest the twelve build artifacts",
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

	permissions := child(loadYAML(t, releaseWorkflow), "permissions")
	if permissions == nil {
		t.Fatal("release.yml must declare workflow permissions")
	}

	want := map[string]string{
		"contents":     "write",
		"id-token":     "write",
		"attestations": "write",
	}
	for scope, level := range want {
		if got := value(child(permissions, scope)); got != level {
			t.Errorf("release.yml permissions %s = %q, want %q", scope, got, level)
		}
	}
	// Nothing beyond those three: the App token, not this workflow, carries
	// every permission the companion repositories need.
	for _, scope := range mappingKeys(permissions) {
		if _, ok := want[scope]; !ok {
			t.Errorf("release.yml requests permission %q, which no RG-011 transition needs", scope)
		}
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
