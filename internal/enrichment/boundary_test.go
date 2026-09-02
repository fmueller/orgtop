package enrichment_test

import (
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/fmueller/orgtop/internal/auth"
	"github.com/fmueller/orgtop/internal/cache"
	"github.com/fmueller/orgtop/internal/enrichment"
	"github.com/fmueller/orgtop/internal/github"
)

// TestCoordinationImportsNoRenderer guards the RG-009 placement rule: network,
// SQLite, enrichment, matching, and cleanup never run in a renderer or keyboard
// handler, so the coordinating service may not reach the TUI at all.
func TestCoordinationImportsNoRenderer(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the coordination directory failed: %v", err)
	}

	fileSet := token.NewFileSet()
	inspected := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s failed: %v", name, err)
		}
		inspected++
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("%s: unquoting import %s failed: %v", name, imported.Path.Value, err)
			}
			if strings.Contains(path, "/internal/tui") || path == "charm.land/bubbletea/v2" {
				t.Errorf("%s: coordination imports the renderer package %s", name, path)
			}
		}
	}
	if inspected == 0 {
		t.Fatal("no non-test Go files found in the coordination package")
	}
}

// The coordinator's seams stay honest only while the shipped GitHub enricher
// and version-1 store satisfy them without an adapter shim. These assertions
// fail the build, not a run, so the seams cannot drift unnoticed.
var (
	_ enrichment.Adapter = github.NewEnricher(auth.Credential{})
	_ enrichment.Cache   = (*cache.Store)(nil)
)
