package github_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// parseAdapterFiles parses the non-test files of the GitHub adapter package.
func parseAdapterFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the github adapter directory failed: %v", err)
	}

	fileSet := token.NewFileSet()
	files := make(map[string]*ast.File)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s failed: %v", name, err)
		}
		files[name] = file
	}
	if len(files) == 0 {
		t.Fatal("no non-test Go files found in the github adapter package")
	}
	return files
}

// TestTransportPayloadTypesDoNotEscapeTheAdapter guards NFR-004: GitHub
// request/response structs stay owned by the source adapter and never become
// domain or TUI models.
func TestTransportPayloadTypesDoNotEscapeTheAdapter(t *testing.T) {
	files := parseAdapterFiles(t)

	transport := make(map[string]struct{})
	for name, file := range files {
		for _, spec := range typeSpecs(file) {
			structType, isStruct := spec.Type.(*ast.StructType)
			if !isStruct || !hasJSONTag(structType) {
				continue
			}
			if spec.Name.IsExported() {
				t.Errorf("%s: transport type %s carries json tags and is exported", name, spec.Name.Name)
			}
			transport[spec.Name.Name] = struct{}{}
		}
	}
	if len(transport) == 0 {
		t.Fatal("no GitHub transport payload types found in the adapter")
	}

	for name, file := range files {
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Recv != nil || !fn.Name.IsExported() {
				continue
			}
			for _, identifier := range signatureIdentifiers(fn.Type) {
				if _, isTransport := transport[identifier]; isTransport {
					t.Errorf("%s: exported function %s exposes transport type %s", name, fn.Name.Name, identifier)
				}
			}
		}
	}
}

func typeSpecs(file *ast.File) []*ast.TypeSpec {
	var specs []*ast.TypeSpec
	ast.Inspect(file, func(node ast.Node) bool {
		if spec, isTypeSpec := node.(*ast.TypeSpec); isTypeSpec {
			specs = append(specs, spec)
		}
		return true
	})
	return specs
}

func hasJSONTag(structType *ast.StructType) bool {
	for _, field := range structType.Fields.List {
		if field.Tag != nil && strings.Contains(field.Tag.Value, "json:") {
			return true
		}
	}
	return false
}

func signatureIdentifiers(signature *ast.FuncType) []string {
	var names []string
	for _, list := range []*ast.FieldList{signature.Params, signature.Results} {
		if list == nil {
			continue
		}
		for _, field := range list.List {
			ast.Inspect(field.Type, func(node ast.Node) bool {
				if identifier, isIdent := node.(*ast.Ident); isIdent {
					names = append(names, identifier.Name)
				}
				return true
			})
		}
	}
	return names
}
