package tui

import (
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// forbiddenShellImports lists the packages the shell must not reach for.
// Source I/O and process work belong to the adapter, and rendering must not
// perform normalization, filtering, or aggregation of its own (NFR-004).
var forbiddenShellImports = []string{
	"github.com/fmueller/orgtop/internal/github",
	"github.com/fmueller/orgtop/internal/auth",
	"net/http",
	"os/exec",
}

// forbiddenComponentImports lists the TUI dependencies v0.1.0 defers. Bubbles
// needs a demonstrated concrete need and Harmonica an active future spec, so
// neither may arrive with a view implementation.
var forbiddenComponentImports = []string{
	"charm.land/bubbles",
	"github.com/charmbracelet/bubbles",
	"github.com/charmbracelet/harmonica",
}

// TestShellDoesNotImportDeferredComponentPackages guards the v0.1.0 non-goals:
// the views render their own rows from application and domain state.
func TestShellDoesNotImportDeferredComponentPackages(t *testing.T) {
	assertNoShellImports(t, forbiddenComponentImports)
}

// TestShellDoesNotImportSourceOrTransportPackages guards NFR-004: the Bubble Tea
// shell consumes application and domain state only.
func TestShellDoesNotImportSourceOrTransportPackages(t *testing.T) {
	assertNoShellImports(t, forbiddenShellImports)
}

// assertNoShellImports fails when a non-test file of the package imports one of
// the forbidden paths or a package below it.
func assertNoShellImports(t *testing.T, forbiddenImports []string) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the tui package directory failed: %v", err)
	}

	fileSet := token.NewFileSet()
	parsed := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed++

		file, err := parser.ParseFile(fileSet, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s failed: %v", name, err)
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("%s: unquoting import %s failed: %v", name, spec.Path.Value, err)
			}
			for _, forbidden := range forbiddenImports {
				if path == forbidden || strings.HasPrefix(path, forbidden+"/") {
					t.Errorf("%s imports %q, which the application shell must not reach for", name, path)
				}
			}
		}
	}

	if parsed == 0 {
		t.Fatal("no non-test Go files found in the tui package")
	}
}
