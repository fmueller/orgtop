package toolchain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// hostClockReaders names the time package functions this guard rejects. A test
// that calls one of them makes its own outcome depend on when and how fast it
// runs; NFR-006 requires the opposite, so a test pins the instants it depends
// on and injects them through the seam the code under test exposes.
var hostClockReaders = []string{"Now", "Since", "Until"}

// clockRead is one host clock read a test file performs.
type clockRead struct {
	// position locates the call for the failure message.
	position token.Position
	// expression is the call as the file spells it, so a report names the
	// identifier a reader will find at that line.
	expression string
}

// TestNoTestReadsTheHostClock walks every _test.go file in the repository and
// fails on a call to any hostClockReaders entry, except inside a benchmark,
// where measuring elapsed real time is the point.
func TestNoTestReadsTheHostClock(t *testing.T) {
	t.Parallel()

	fileSet := token.NewFileSet()
	parsed := 0
	walkErr := filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			// repoRoot is a relative path whose own last element starts with a
			// dot, so the root itself is exempt from the hidden-directory skip
			// that keeps the walk out of .git and friends.
			if path != repoRoot && strings.HasPrefix(entry.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		parsed++

		file, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s failed: %v", path, err)
		}
		relativePath := strings.TrimPrefix(path, repoRoot+string(filepath.Separator))
		for _, read := range hostClockReadsIn(fileSet, file) {
			t.Errorf("%s:%d calls %s; a test must pin the instant it depends on rather than read the host clock (NFR-006)",
				relativePath, read.position.Line, read.expression)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking the repository failed: %v", walkErr)
	}
	if parsed == 0 {
		t.Fatal("no _test.go files found in the repository")
	}
}

// hostClockReadsIn reports every host clock read the file performs outside a
// benchmark. It resolves the name the file imports the time package under
// rather than matching the identifier "time", so neither a renamed nor a dot
// import hides a call. A file that only refers to a reader as a value, without
// calling it, is beyond a syntax-only guard and is not reported.
func hostClockReadsIn(fileSet *token.FileSet, file *ast.File) []clockRead {
	timePackage, isImported := timePackageName(file)
	if !isImported {
		return nil
	}

	var found []clockRead
	for _, decl := range file.Decls {
		if function, isFunction := decl.(*ast.FuncDecl); isFunction && strings.HasPrefix(function.Name.Name, "Benchmark") {
			continue
		}
		ast.Inspect(decl, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			if expression, isRead := clockReadExpression(call.Fun, timePackage); isRead {
				found = append(found, clockRead{position: fileSet.Position(call.Pos()), expression: expression})
			}
			return true
		})
	}
	return found
}

// timePackageName reports the identifier the file refers to the time package
// by. A dot import binds the readers into the file scope and so yields the
// empty name, which no qualified call can match and every bare call does.
func timePackageName(file *ast.File) (name string, isImported bool) {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != "time" {
			continue
		}
		if spec.Name == nil {
			return "time", true
		}
		if spec.Name.Name == "." {
			return "", true
		}
		return spec.Name.Name, true
	}
	return "", false
}

// clockReadExpression reports the call as written when it reads the host clock
// through the time package the file imports as timePackage.
func clockReadExpression(fun ast.Expr, timePackage string) (string, bool) {
	switch fun := fun.(type) {
	case *ast.SelectorExpr:
		pkg, isIdent := fun.X.(*ast.Ident)
		if timePackage == "" || !isIdent || pkg.Name != timePackage || !slices.Contains(hostClockReaders, fun.Sel.Name) {
			return "", false
		}
		return timePackage + "." + fun.Sel.Name, true
	case *ast.Ident:
		if timePackage != "" || !slices.Contains(hostClockReaders, fun.Name) {
			return "", false
		}
		return fun.Name, true
	default:
		return "", false
	}
}
