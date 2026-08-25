package toolchain

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// releaseBinary is the executable name every build path must produce. The
// archive layout, the install instructions, and the `orgtop --repo` usage in
// README.md all assume it.
const releaseBinary = "orgtop"

// releaseMain is the only command package the module builds.
const releaseMain = "./cmd/orgtop"

// releaseVersion is the milestone this repository is preparing to tag. The
// release workflow extracts its notes from the matching CHANGELOG.md section,
// so the section has to exist before the tag is pushed.
const releaseVersion = "0.1.0"

// goModule is the module file whose direct requirements v0.1.0 pins.
var goModule = filepath.Join(repoRoot, "go.mod")

// v010Dependencies is the direct module requirement set v0.1.0 ships with. The
// spec defers component libraries and every other runtime dependency to a later
// milestone, so a new direct requirement is a scope change rather than an
// implementation detail.
var v010Dependencies = []string{
	"charm.land/bubbletea/v2",
	"charm.land/lipgloss/v2",
	"gopkg.in/yaml.v3",
}

// TestReleaseConfigurationProducesTheOrgtopBinary keeps the published artifact,
// the local build, and the cross-compile smoke pointed at one command package
// under one name. A renamed binary or a second build id would ship archives the
// documented invocation does not match.
func TestReleaseConfigurationProducesTheOrgtopBinary(t *testing.T) {
	t.Parallel()

	release := loadYAML(t, goreleaserConfig)
	if project := child(release, "project_name"); project == nil || project.Value != releaseBinary {
		t.Errorf(".goreleaser.yml project_name = %q, want %q", value(project), releaseBinary)
	}

	builds := child(release, "builds")
	if builds == nil {
		t.Fatal(".goreleaser.yml must declare a build")
	}
	if len(builds.Content) != 1 {
		t.Fatalf(".goreleaser.yml must declare exactly one build, got %d", len(builds.Content))
	}
	build := builds.Content[0]
	if binary := child(build, "binary"); binary == nil || binary.Value != releaseBinary {
		t.Errorf(".goreleaser.yml build binary = %q, want %q", value(binary), releaseBinary)
	}
	if main := child(build, "main"); main == nil || main.Value != releaseMain {
		t.Errorf(".goreleaser.yml build main = %q, want %q", value(main), releaseMain)
	}

	tasks := readFile(t, taskfile)
	if !strings.Contains(tasks, "go build "+releaseMain) {
		t.Errorf("`task build` must build %s so the local binary matches the published one", releaseMain)
	}
}

// TestModuleKeepsTheV010DependencySurface guards the v0.1.0 non-goals from the
// dependency side. The shell's own import guard rejects a deferred component
// library inside internal/tui; this rejects one arriving in go.mod at all.
func TestModuleKeepsTheV010DependencySurface(t *testing.T) {
	t.Parallel()

	direct := sorted(directRequirements(t))
	want := sorted(v010Dependencies)
	if !equalStrings(direct, want) {
		t.Errorf("go.mod direct requirements are %v, want %v", direct, want)
	}
}

// TestChangelogDocumentsTheReleaseVersion keeps the tag publishable. The guard
// scripts refuse a tag whose `## [<version>]` section is missing or empty, but
// they only run at tag time; this fails on every ordinary CI run instead, while
// the section can still be written.
func TestChangelogDocumentsTheReleaseVersion(t *testing.T) {
	t.Parallel()

	notes, documented := changelogSection(readFile(t, changelogPath), releaseVersion)
	if !documented {
		t.Fatalf("CHANGELOG.md has no '## [%s]' heading, so the release workflow would refuse the tag", releaseVersion)
	}
	if strings.TrimSpace(notes) == "" {
		t.Errorf("CHANGELOG.md's '## [%s]' section is empty, so the release would publish a blank body", releaseVersion)
	}
}

// directRequirements returns the module paths go.mod requires directly, which
// are the requirements not marked indirect.
func directRequirements(t *testing.T) []string {
	t.Helper()

	var (
		paths   []string
		inBlock bool
	)
	for _, line := range strings.Split(readFile(t, goModule), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "require (":
			inBlock = true
		case inBlock && line == ")":
			inBlock = false
		case inBlock && line != "" && !strings.Contains(line, "// indirect"):
			paths = append(paths, strings.Fields(line)[0])
		case !inBlock && strings.HasPrefix(line, "require ") && !strings.Contains(line, "// indirect"):
			paths = append(paths, strings.Fields(line)[1])
		}
	}
	if len(paths) == 0 {
		t.Fatal("go.mod declares no direct requirement")
	}
	return paths
}

// value reports a YAML scalar's value, or a placeholder for an absent node.
func value(node *yaml.Node) string {
	if node == nil {
		return "<missing>"
	}
	return node.Value
}

// changelogSection returns the release notes under the `## [version]` heading
// and whether that heading is present at all, which keeps a missing section
// distinguishable from one that says nothing.
//
// The link-reference block a Keep a Changelog file ends with falls under the
// last version heading, so it is skipped: those lines are addresses, not notes,
// and counting them would let an entryless section read as written.
func changelogSection(changelog, version string) (string, bool) {
	heading := "## [" + version + "]"
	var (
		notes      []string
		documented bool
	)
	for _, line := range strings.Split(changelog, "\n") {
		if !strings.HasPrefix(line, "## ") {
			if documented && !isLinkReference(line) {
				notes = append(notes, line)
			}
			continue
		}
		if documented {
			break
		}
		documented = line == heading || strings.HasPrefix(line, heading+" ")
	}
	return strings.Join(notes, "\n"), documented
}

// isLinkReference reports whether the line defines a markdown link reference,
// the `[label]: url` form Keep a Changelog collects at the foot of the file.
func isLinkReference(line string) bool {
	line = strings.TrimSpace(line)
	label, address, found := strings.Cut(line, "]: ")
	return found && strings.HasPrefix(label, "[") && address != ""
}

// TestChangelogSectionDistinguishesAbsentFromSaysNothing pins the two answers
// the release guard depends on. The link-reference block at the foot of a Keep
// a Changelog file sits under the last version heading, so a section whose
// entries were never written still has lines under it; those lines are not
// release notes and must not read as content.
func TestChangelogSectionDistinguishesAbsentFromSaysNothing(t *testing.T) {
	t.Parallel()

	const links = "[Unreleased]: https://example.test/compare\n[0.1.0]: https://example.test/tag"

	tests := []struct {
		name           string
		changelog      string
		wantDocumented bool
		wantEntries    bool
		// wantNotes, when set, pins the extracted notes exactly, so a stripped
		// link reference cannot take real entries with it.
		wantNotes string
	}{
		{
			name:      "heading absent",
			changelog: "## [Unreleased]\n\n- something\n",
		},
		{
			name:           "heading with no lines at all",
			changelog:      "## [0.1.0]\n## [0.0.9]\n\n- older\n",
			wantDocumented: true,
		},
		{
			name:           "heading followed only by the link block",
			changelog:      "## [0.1.0] - 2026-08-23\n\n" + links + "\n",
			wantDocumented: true,
		},
		{
			name:           "dated heading with entries",
			changelog:      "## [0.1.0] - 2026-08-23\n\n### Added\n\n- a feature\n\n" + links + "\n",
			wantDocumented: true,
			wantEntries:    true,
			wantNotes:      "### Added\n\n- a feature",
		},
		{
			name:           "a link reference inside the section goes, its entries stay",
			changelog:      "## [0.1.0] - 2026-08-23\n\n[0.1.0]: https://example.test/tag\n\n### Added\n\n- a feature\n\n## [0.0.9]\n\n- older\n",
			wantDocumented: true,
			wantEntries:    true,
			wantNotes:      "### Added\n\n- a feature",
		},
		{
			name:           "an entry that merely contains a bracket-colon stays",
			changelog:      "## [0.1.0]\n\n- an entry with a stray ]: sequence.\n- [a link](https://example.test) entry.\n\n" + links + "\n",
			wantDocumented: true,
			wantEntries:    true,
			wantNotes:      "- an entry with a stray ]: sequence.\n- [a link](https://example.test) entry.",
		},
		{
			name:           "entries end at the next version heading",
			changelog:      "## [0.1.0]\n\n- a feature\n\n## [0.0.9]\n\n- older\n",
			wantDocumented: true,
			wantEntries:    true,
		},
		{
			name:      "a longer version is not the requested one",
			changelog: "## [0.1.0-rc1]\n\n- a candidate\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			notes, documented := changelogSection(test.changelog, releaseVersion)
			if documented != test.wantDocumented {
				t.Errorf("documented = %t, want %t", documented, test.wantDocumented)
			}
			if entries := strings.TrimSpace(notes) != ""; entries != test.wantEntries {
				t.Errorf("has entries = %t, want %t, from notes %q", entries, test.wantEntries, notes)
			}
			if got := strings.TrimSpace(notes); test.wantNotes != "" && got != test.wantNotes {
				t.Errorf("notes = %q, want %q", got, test.wantNotes)
			}
		})
	}
}

// versionSymbol is the package-level variable, relative to the module path,
// that the release stamp fills and the CLI prints.
const versionSymbol = "internal/cli.Version"

// TestReleaseStampsTheVersionSymbolTheBinaryReads keeps published archives from
// reporting `dev`. GoReleaser's ldflags list replaces its default
// `-X main.version=...`, so the assignment has to be written here explicitly,
// and it has to name the symbol internal/cli actually declares: a rename on
// either side un-stamps every release silently otherwise (FR-001, A-012). The
// stamped variable's own `dev` default is guarded in internal/cli.
func TestReleaseStampsTheVersionSymbolTheBinaryReads(t *testing.T) {
	t.Parallel()

	builds := child(loadYAML(t, goreleaserConfig), "builds")
	if builds == nil || len(builds.Content) != 1 {
		t.Fatal(".goreleaser.yml must declare exactly one build")
	}
	flags := stringSlice(child(builds.Content[0], "ldflags"))
	for _, want := range []string{"-s", "-w"} {
		if !contains(strings.Fields(strings.Join(flags, " ")), want) {
			t.Errorf(".goreleaser.yml ldflags %v dropped %q", flags, want)
		}
	}

	target, template, ok := linkerAssignment(flags)
	if !ok {
		t.Fatalf(".goreleaser.yml ldflags %v carry no -X version assignment", flags)
	}
	if want := modulePath(t) + "/" + versionSymbol; target != want {
		t.Errorf("-X targets %q, want %q", target, want)
	}
	if normalized := strings.Join(strings.Fields(template), ""); normalized != "{{.Version}}" {
		t.Errorf("-X assigns %q, want GoReleaser's {{ .Version }}", template)
	}
}

// linkerAssignment splits the first `-X target=value` out of the ldflags list.
// The value is read from the whole entry rather than a whitespace field because
// GoReleaser's template spelling, `{{ .Version }}`, contains spaces.
func linkerAssignment(flags []string) (target, template string, ok bool) {
	for _, flag := range flags {
		assignment, found := strings.CutPrefix(strings.TrimSpace(flag), "-X")
		if !found {
			continue
		}
		assignment = strings.TrimSpace(strings.TrimPrefix(assignment, "="))
		target, template, ok = strings.Cut(assignment, "=")
		return target, strings.TrimSpace(template), ok
	}
	return "", "", false
}

// modulePath returns the module path go.mod declares.
func modulePath(t *testing.T) string {
	t.Helper()

	for _, line := range strings.Split(readFile(t, goModule), "\n") {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(path)
		}
	}
	t.Fatal("go.mod declares no module path")
	return ""
}
