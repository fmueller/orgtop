package toolchain

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// goModule is the module file whose requirement set the dependency guards read.
var goModule = filepath.Join(repoRoot, "go.mod")

// v020Dependencies is the direct module requirement set v0.2.0 ships with.
// v0.1.0 pinned only the shell's three libraries; RG-005 closes a bounded local
// SQLite enrichment cache, which is the sole reason this list grows.
//
// modernc.org/sqlite is the selected SQLite approach. It is a pure-Go
// transliteration of SQLite, so it links under `CGO_ENABLED=0` — the setting
// every build path in this repository pins and the only way the six GOOS/GOARCH
// pairs .goreleaser.yml publishes can be produced from one Linux runner. A
// cgo-backed driver would require six cross-toolchains and would break the
// single-command local build, so the choice is a toolchain constraint rather
// than a preference.
var v020Dependencies = []string{
	"charm.land/bubbletea/v2",
	"charm.land/lipgloss/v2",
	"gopkg.in/yaml.v3",
	"modernc.org/sqlite",
}

// deferredDependencies rejects, by module path prefix, the dependency categories
// the v0.2.0 non-goals defer. The direct-requirement allowlist above already
// pins what may be imported; this reads every module providing a package this
// module or its tests import, because a deferred capability can also arrive
// transitively, where no allowlist sees it. Module-graph pruning is what makes
// go.mod's own require blocks that complete set.
var deferredDependencies = []struct {
	prefix string
	reason string
}{
	{"github.com/charmbracelet/harmonica", "physics-based animation is a v0.2.0 non-goal"},
	{"github.com/mattn/go-sqlite3", "a cgo SQLite driver breaks the CGO_ENABLED=0 cross-compile"},
	{"crawshaw.io/sqlite", "a cgo SQLite driver breaks the CGO_ENABLED=0 cross-compile"},
	{"zombiezen.com/go/sqlite", "a cgo SQLite driver breaks the CGO_ENABLED=0 cross-compile"},
	{"gorm.io/", "RG-005 closes a narrow enrichment cache, not a generic persistence layer"},
	{"entgo.io/", "RG-005 closes a narrow enrichment cache, not a generic persistence layer"},
	{"github.com/jmoiron/sqlx", "RG-005 closes a narrow enrichment cache, not a generic persistence layer"},
	{"github.com/uptrace/bun", "RG-005 closes a narrow enrichment cache, not a generic persistence layer"},
	{"github.com/hashicorp/go-plugin", "a generic plugin or source framework is a v0.2.0 non-goal"},
	{"go.opentelemetry.io/", "telemetry and analytics storage are v0.2.0 non-goals"},
	{"github.com/getsentry/", "telemetry and analytics storage are v0.2.0 non-goals"},
	{"github.com/prometheus/client_golang", "telemetry and analytics storage are v0.2.0 non-goals"},
}

// TestModuleKeepsTheV020DependencySurface guards the v0.2.0 non-goals from the
// dependency side. The shell's own import guard rejects a deferred component
// library inside internal/tui; this rejects one arriving in go.mod at all.
//
// Bubbles is deliberately absent: the spec admits it only for a concrete
// pagination or viewport component, which no closed contract has yet named.
func TestModuleKeepsTheV020DependencySurface(t *testing.T) {
	t.Parallel()

	direct := sorted(directRequirements(t))
	want := sorted(v020Dependencies)
	if !equalStrings(direct, want) {
		t.Errorf("go.mod direct requirements are %v, want %v", direct, want)
	}
}

// TestModuleExcludesDeferredDependencyCategories reads every requirement, direct
// and indirect. A deferred capability that arrives through another module's
// requirement still ships in the binary, and the allowlist above never sees it.
func TestModuleExcludesDeferredDependencyCategories(t *testing.T) {
	t.Parallel()

	for _, module := range moduleRequirements(t) {
		for _, deferred := range deferredDependencies {
			if strings.HasPrefix(module.path, deferred.prefix) {
				t.Errorf("go.mod must not require %q: %s", module.path, deferred.reason)
			}
		}
	}
}

// TestSelectedSQLiteDriverIsRegisteredAndPureGo proves the selected driver is
// linked and usable rather than only named in go.mod. An in-memory database
// keeps this a unit test: no file, no network, no process.
//
// The application ID round-trip is the cheapest exercise that reaches SQLite's
// header rather than only its SQL parser, so a driver that registers but cannot
// open a database fails here instead of inside the first cache write.
func TestSelectedSQLiteDriverIsRegisteredAndPureGo(t *testing.T) {
	t.Parallel()

	// applicationID is RG-005's `ORGT` SQLite application id, 0x4f524754.
	const applicationID = 1330136148

	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening an in-memory database failed: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing the database failed: %v", err)
		}
	})

	if _, err := database.Exec(fmt.Sprintf("PRAGMA application_id = %d", applicationID)); err != nil {
		t.Fatalf("setting the application ID failed: %v", err)
	}

	var stored int64
	if err := database.QueryRow("PRAGMA application_id").Scan(&stored); err != nil {
		t.Fatalf("reading the application ID back failed: %v", err)
	}
	if stored != applicationID {
		t.Errorf("application_id = %d, want %d", stored, applicationID)
	}
}

// TestEveryBuildPathDisablesCGO keeps the pure-Go SQLite choice enforced rather
// than merely intended. With cgo enabled the toolchain is free to prefer a cgo
// implementation of a dependency, and the cross-compile targets stop being
// reachable from one runner without six C toolchains.
//
// `test` is in this list for a reason the build targets cannot cover: until the
// RG-005 cache lands, no non-test file imports the driver, so `build` and
// `build:cross` compile a binary that never reaches SQLite. `go test ./...` is
// the only gate that compiles the import at all, and running it with the
// ambient cgo default would prove the driver works in a mode the released
// binary never uses. Pinning it here makes every gate exercise the same linkage
// the published artifact ships with.
func TestEveryBuildPathDisablesCGO(t *testing.T) {
	t.Parallel()

	tasks := loadYAML(t, taskfile)
	for _, target := range []string{"build", "build:cross", "test"} {
		cgo := nodeAt(tasks, "tasks", target, "env", "CGO_ENABLED")
		if cgo == nil || cgo.Value != "0" {
			t.Errorf("task %q must pin CGO_ENABLED=0, got %q", target, value(cgo))
		}
	}

	builds := child(loadYAML(t, goreleaserConfig), "builds")
	if builds == nil || len(builds.Content) == 0 {
		t.Fatal(".goreleaser.yml must declare at least one build")
	}
	for _, build := range builds.Content {
		if !contains(stringSlice(child(build, "env")), "CGO_ENABLED=0") {
			t.Error(".goreleaser.yml must publish every target with CGO_ENABLED=0")
		}
	}
}

// TestPinnedGoToolchainAgreesWithTheModule keeps one Go version across the
// module and the provisioned toolchain. When they drift, CI compiles with a
// version the module never claims, so a language or standard-library feature
// can pass CI and fail for a contributor building with the pinned toolchain.
func TestPinnedGoToolchainAgreesWithTheModule(t *testing.T) {
	t.Parallel()

	module := regexp.MustCompile(`(?m)^go\s+(\S+)\s*$`).FindStringSubmatch(readFile(t, goModule))
	if module == nil {
		t.Fatal("go.mod must declare a go directive")
	}

	mise := regexp.MustCompile(`(?m)^go\s*=\s*"([^"]+)"`).FindStringSubmatch(readFile(t, miseConfig))
	if mise == nil {
		t.Fatal("mise.toml must pin a go version")
	}

	if module[1] != mise[1] {
		t.Errorf("go.mod pins Go %s but mise.toml provisions %s; the two must agree", module[1], mise[1])
	}
}

// requirement is one module go.mod requires, and whether go.mod marks it
// indirect. The direct/indirect split is the only thing separating the
// allowlisted import surface from the transitive graph, so both guards read the
// same parse rather than each scanning go.mod their own way.
type requirement struct {
	path     string
	indirect bool
}

// moduleRequirements returns every requirement go.mod declares, indirect
// included, in file order.
func moduleRequirements(t *testing.T) []requirement {
	t.Helper()

	var (
		requirements []requirement
		inBlock      bool
	)
	for _, line := range strings.Split(readFile(t, goModule), "\n") {
		line = strings.TrimSpace(line)

		var path string
		switch {
		case line == "require (":
			inBlock = true
		case inBlock && line == ")":
			inBlock = false
		case inBlock && line != "":
			path = strings.Fields(line)[0]
		case !inBlock && strings.HasPrefix(line, "require "):
			path = strings.Fields(line)[1]
		}
		if path == "" {
			continue
		}

		requirements = append(requirements, requirement{
			path:     path,
			indirect: strings.Contains(line, "// indirect"),
		})
	}
	if len(requirements) == 0 {
		t.Fatal("go.mod declares no requirement")
	}
	return requirements
}

// directRequirements returns the module paths go.mod requires directly, which
// are the requirements not marked indirect.
func directRequirements(t *testing.T) []string {
	t.Helper()

	var paths []string
	for _, module := range moduleRequirements(t) {
		if !module.indirect {
			paths = append(paths, module.path)
		}
	}
	if len(paths) == 0 {
		t.Fatal("go.mod declares no direct requirement")
	}
	return paths
}

// TestLicenseCheckCoversTestOnlyDependencies keeps the license gate honest for a
// dependency that reaches the module through a test. go-licenses walks the
// non-test import graph by default, so a requirement pinned for a guard — as
// modernc.org/sqlite is until the cache lands — is invisible to the gate and its
// license is never actually approved.
func TestLicenseCheckCoversTestOnlyDependencies(t *testing.T) {
	t.Parallel()

	cmds := nodeAt(loadYAML(t, taskfile), "tasks", "licenses:check", "cmds")
	if cmds == nil || len(cmds.Content) == 0 {
		t.Fatal("Taskfile.yml must declare a `licenses:check` target")
	}

	for _, command := range cmds.Content {
		if !strings.Contains(command.Value, "--include_tests") {
			t.Errorf("licenses:check must pass --include_tests so test-only requirements are approved too: %q", command.Value)
		}
	}
}
