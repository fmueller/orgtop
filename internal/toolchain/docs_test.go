package toolchain

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/fmueller/orgtop/internal/cli"
)

// readmePath is the user-facing documentation FR-011 governs.
var readmePath = filepath.Join(repoRoot, "README.md")

// deferredClaims are the capabilities v0.1.0 does not ship. The README must not
// promise any of them, and it must not describe the product as live.
var deferredClaims = []string{
	"filtering", "search", "clustering", "inspect", "rain", "drilldown",
	"live activity", "real time", "real-time", "webhook",
}

// documentedControls are the key bindings the footer advertises, scrolling
// included. A README that stops naming one of them misdescribes the shipped
// shell to the operator who reads it.
var documentedControls = []string{
	"`1`", "`2`", "`tab`", "`up`", "`down`", "`pgup`", "`pgdown`", "`q`", "`ctrl+c`",
}

// TestReadmeDocumentsTheDocumentedUsage keeps the documented invocation the one
// the binary actually accepts. Deriving it from the parser rather than repeating
// the string means a renamed or reshaped flag fails here instead of misleading a
// first-time user.
func TestReadmeDocumentsTheDocumentedUsage(t *testing.T) {
	t.Parallel()

	var usage strings.Builder
	if _, err := cli.ParseArgs(releaseBinary, nil, &usage); err == nil {
		t.Fatal("parsing an empty argument list must be rejected")
	}

	readme := readFile(t, readmePath)
	for _, line := range strings.Split(usage.String(), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Usage:") {
			continue
		}
		if !strings.Contains(readme, strings.TrimPrefix(line, "Usage: ")) {
			t.Errorf("README.md does not document the invocation %q", line)
		}
	}
	if !strings.Contains(readme, "--repo acme/backend --repo acme/frontend") {
		t.Error("README.md does not show the repeated --repo form that selects several repositories")
	}
}

// TestReadmeDocumentsTheCredentialContract guards FR-011 and A-004: the README
// states the credential precedence in the order the resolver applies it and
// names the gh fallback a user has to set up.
func TestReadmeDocumentsTheCredentialContract(t *testing.T) {
	t.Parallel()

	readme := readFile(t, readmePath)
	position := -1
	for _, step := range []string{"GH_TOKEN", "GITHUB_TOKEN", "gh auth token", "gh auth login"} {
		next := strings.Index(readme, step)
		if next < 0 {
			t.Errorf("README.md does not document %q", step)
			continue
		}
		if next < position {
			t.Errorf("README.md documents %q before the step that precedes it", step)
		}
		position = next
	}
}

// TestReadmeDocumentsPollingControlsAndChecks guards the rest of FR-011: the
// polling semantics that explain why data is not live, the controls the shell
// implements, and the development checks a contributor runs.
func TestReadmeDocumentsPollingControlsAndChecks(t *testing.T) {
	t.Parallel()

	readme := readFile(t, readmePath)
	for _, required := range []string{"POLLING", "not live", "task check"} {
		if !strings.Contains(readme, required) {
			t.Errorf("README.md does not document %q", required)
		}
	}
	for _, control := range documentedControls {
		if !strings.Contains(readme, control) {
			t.Errorf("README.md does not document the %s control", control)
		}
	}
}

// TestReadmeClaimsNoDeferredFunctionality keeps the README honest in both
// directions: it may not advertise a deferred capability, and it may not keep
// describing an implemented milestone as unbuilt.
func TestReadmeClaimsNoDeferredFunctionality(t *testing.T) {
	t.Parallel()

	readme := strings.ToLower(readFile(t, readmePath))
	for _, claim := range deferredClaims {
		if strings.Contains(readme, claim) {
			t.Errorf("README.md claims the deferred capability %q", claim)
		}
	}
	for _, stale := range []string{"not implemented yet", "design document"} {
		if strings.Contains(readme, stale) {
			t.Errorf("README.md still says %q", stale)
		}
	}
}

// TestReadmeDocumentsTheVersionAndHelpFlags guards FR-011 for the two flags a
// bug reporter is asked to run first: the README has to name both spellings of
// each, or a downloaded binary cannot be identified from its documentation.
func TestReadmeDocumentsTheVersionAndHelpFlags(t *testing.T) {
	t.Parallel()

	readme := readFile(t, readmePath)
	for _, flag := range []string{"--version", "-v", "--help", "-h"} {
		if !strings.Contains(readme, "`"+flag+"`") {
			t.Errorf("README.md does not document the %s flag", flag)
		}
	}
}
