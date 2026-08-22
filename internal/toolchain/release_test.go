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
