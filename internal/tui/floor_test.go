package tui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
	"time"
)

// adapterSchedule is the GitHub adapter file that carries the adapter's own copy
// of the FR-004 polling floor, and adapterFloorName the constant holding it. The
// lifecycle must not import the adapter (NFR-004), so the constant is read from
// source rather than from its package and the boundary stays intact.
const (
	adapterSchedule  = "../github/schedule.go"
	adapterFloorName = "defaultInterval"
)

// specFloor is the polling floor FR-004 fixes for v0.1.0.
const specFloor = 60 * time.Second

// durationUnits are the time units a duration constant may be written in.
var durationUnits = map[string]time.Duration{
	"Nanosecond":  time.Nanosecond,
	"Microsecond": time.Microsecond,
	"Millisecond": time.Millisecond,
	"Second":      time.Second,
	"Minute":      time.Minute,
	"Hour":        time.Hour,
}

// TestTheLifecycleFloorIsTheSpecifiedPollingFloor anchors the lifecycle copy to
// the delay FR-004 fixes, so the adapter and the lifecycle cannot agree on a
// floor the spec does not allow.
func TestTheLifecycleFloorIsTheSpecifiedPollingFloor(t *testing.T) {
	if defaultDelay != specFloor {
		t.Errorf("defaultDelay = %s, want the FR-004 polling floor %s", defaultDelay, specFloor)
	}
}

// TestTheAdapterAndLifecycleFloorsAgree fails when the deliberately duplicated
// FR-004 polling floor drifts apart between the GitHub adapter and the refresh
// lifecycle.
func TestTheAdapterAndLifecycleFloorsAgree(t *testing.T) {
	if floor := constDuration(t, adapterSchedule, adapterFloorName); floor != defaultDelay {
		t.Errorf("%s is %s while defaultDelay is %s; the FR-004 polling floor must stay one delay", adapterFloorName, floor, defaultDelay)
	}
}

// constDuration evaluates the named duration constant of the given file. It
// compares delays rather than source text, so the two floors may be written in
// different idioms.
func constDuration(t *testing.T, path, name string) time.Duration {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s failed: %v", path, err)
	}
	for _, spec := range constSpecs(file) {
		for index, identifier := range spec.Names {
			if identifier.Name != name || index >= len(spec.Values) {
				continue
			}
			value, err := evalDuration(spec.Values[index])
			if err != nil {
				t.Fatalf("evaluating %s of %s failed: %v", name, path, err)
			}
			return value
		}
	}
	t.Fatalf("%s declares no constant %s", path, name)
	return 0
}

// constSpecs returns the constant declarations of a parsed file.
func constSpecs(file *ast.File) []*ast.ValueSpec {
	var specs []*ast.ValueSpec
	for _, decl := range file.Decls {
		declaration, isGenDecl := decl.(*ast.GenDecl)
		if !isGenDecl || declaration.Tok != token.CONST {
			continue
		}
		for _, spec := range declaration.Specs {
			if value, isValueSpec := spec.(*ast.ValueSpec); isValueSpec {
				specs = append(specs, value)
			}
		}
	}
	return specs
}

// evalDuration evaluates the duration idioms a floor constant may be written in:
// an integer literal, a `time.<Unit>` selector, and products of those in either
// operand order. Any other form is reported rather than guessed at.
func evalDuration(expr ast.Expr) (time.Duration, error) {
	switch node := expr.(type) {
	case *ast.BasicLit:
		if node.Kind != token.INT {
			break
		}
		factor, err := strconv.Atoi(node.Value)
		if err != nil {
			return 0, fmt.Errorf("reading the integer literal %s: %w", node.Value, err)
		}
		return time.Duration(factor), nil
	case *ast.SelectorExpr:
		if pkg, isIdent := node.X.(*ast.Ident); !isIdent || pkg.Name != "time" {
			break
		}
		unit, known := durationUnits[node.Sel.Name]
		if !known {
			return 0, fmt.Errorf("unknown time unit %s", node.Sel.Name)
		}
		return unit, nil
	case *ast.BinaryExpr:
		if node.Op != token.MUL {
			break
		}
		left, err := evalDuration(node.X)
		if err != nil {
			return 0, err
		}
		right, err := evalDuration(node.Y)
		if err != nil {
			return 0, err
		}
		return left * right, nil
	}
	return 0, fmt.Errorf("the %T form is no product of integer literals and time units; extend evalDuration or restate the constant", expr)
}

// specExpansionRetry is the minimum delay RG-010 fixes before a failed
// organization expansion may be retried.
const specExpansionRetry = 60 * time.Second

// TestTheExpansionRetryBoundIsTheSpecifiedOne anchors the lifecycle constant to
// the delay RG-010 fixes. It shares its value with the FR-004 polling floor but
// not its meaning, so the two are asserted apart.
func TestTheExpansionRetryBoundIsTheSpecifiedOne(t *testing.T) {
	if expansionRetry != specExpansionRetry {
		t.Errorf("expansionRetry = %s, want the RG-010 retry bound %s", expansionRetry, specExpansionRetry)
	}
}

// TestTheExpansionIntervalIsTheSpecifiedOne anchors the re-expansion interval to
// the one RG-010 fixes between successful attempts.
func TestTheExpansionIntervalIsTheSpecifiedOne(t *testing.T) {
	if want := 15 * time.Minute; expansionInterval != want {
		t.Errorf("expansionInterval = %s, want the RG-010 re-expansion interval %s", expansionInterval, want)
	}
}
