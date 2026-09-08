package cache

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// windowsLockSource is the Windows half of the maintenance lock. This guard
// reads it as source rather than linking it, so the binding it uses is pinned
// from every host the suite runs on and not only from a Windows runner.
const windowsLockSource = "lock_windows.go"

// parseWindowsLock parses the Windows lock file so a guard can read both its
// imports and the calls it makes.
func parseWindowsLock(t *testing.T) (*token.FileSet, *ast.File) {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, windowsLockSource, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s failed: %v", windowsLockSource, err)
	}
	return fileSet, file
}

// TestWindowsLockUsesTheMaintainedSyscallWrappers pins the binding the Windows
// maintenance lock takes its byte-range locks through. golang.org/x/sys is an
// approved direct requirement that wraps LockFileEx and UnlockFileEx, so the
// file must not carry a hand-resolved kernel32 handle again: that form needs
// unsafe.Pointer to hand an OVERLAPPED across a uintptr argument, which the
// wrappers type for the caller.
func TestWindowsLockUsesTheMaintainedSyscallWrappers(t *testing.T) {
	t.Parallel()

	_, file := parseWindowsLock(t)

	imports := map[string]bool{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("%s imports an unparsable path %s: %v", windowsLockSource, spec.Path.Value, err)
		}
		imports[path] = true
	}

	if !imports["golang.org/x/sys/windows"] {
		t.Errorf("%s must lock through golang.org/x/sys/windows, imports are %v", windowsLockSource, imports)
	}
	for _, rejected := range []string{"unsafe", "syscall"} {
		if imports[rejected] {
			t.Errorf("%s must not import %q: the x/sys wrappers replace the hand-rolled binding", windowsLockSource, rejected)
		}
	}
}

// TestWindowsLockResolvesNoDLLHandle rejects the lazy kernel32 resolution the
// x/sys wrappers replace, independently of which package it is spelled through,
// so the import guard above cannot be satisfied while the old call form stays.
func TestWindowsLockResolvesNoDLLHandle(t *testing.T) {
	t.Parallel()

	fileSet, file := parseWindowsLock(t)

	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch selector.Sel.Name {
		case "NewLazyDLL", "NewLazySystemDLL", "LoadDLL", "NewProc":
			t.Errorf("%s resolves a DLL handle at %s: use the golang.org/x/sys/windows wrappers", windowsLockSource, fileSet.Position(selector.Pos()))
		}
		return true
	})
}
