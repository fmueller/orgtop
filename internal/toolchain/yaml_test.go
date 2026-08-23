// Package toolchain holds assertions about this repository's own build and CI
// configuration. It has no production code: the tests here parse the committed
// workflow, action, and toolchain files and fail when an invariant that keeps CI
// cheap, reproducible, and complete is broken.
//
// These tests read committed, immutable repository configuration. They create no
// temporary files, touch no network, and start no processes, so they stay
// ordinary unit tests despite reading from disk.
package toolchain

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const repoRoot = "../.."

var (
	ciWorkflow       = filepath.Join(repoRoot, ".github", "workflows", "ci.yml")
	planningWorkflow = filepath.Join(repoRoot, ".github", "workflows", "planning.yml")
	mutationWorkflow = filepath.Join(repoRoot, ".github", "workflows", "mutation.yml")
	releaseWorkflow  = filepath.Join(repoRoot, ".github", "workflows", "release.yml")
	setupAction      = filepath.Join(repoRoot, ".github", "actions", "setup", "action.yml")
	taskfile         = filepath.Join(repoRoot, "Taskfile.yml")
	miseConfig       = filepath.Join(repoRoot, "mise.toml")
	goreleaserConfig = filepath.Join(repoRoot, ".goreleaser.yml")
	changelogPath    = filepath.Join(repoRoot, "CHANGELOG.md")
	changelogGuard   = filepath.Join(repoRoot, "scripts", "check-changelog-version.sh")
	changelogNotes   = filepath.Join(repoRoot, "scripts", "changelog-release-notes.sh")
)

// Traversal works on yaml.Node rather than a decoded map because GitHub resolves
// the `on:` key as a boolean under some YAML schemas; comparing the raw scalar
// Value sidesteps that entirely.

func loadYAML(t *testing.T, path string) *yaml.Node {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s failed: %v", path, err)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatalf("parsing %s failed: %v", path, err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		t.Fatalf("%s must hold a single YAML document", path)
	}
	if document.Content[0].Kind != yaml.MappingNode {
		t.Fatalf("%s must start with a mapping", path)
	}
	return document.Content[0]
}

// readFile returns a file's contents with line endings normalized to LF. A
// Windows checkout with core.autocrlf enabled hands these files back with CRLF
// endings, which silently breaks every guard that scans for a multi-line
// pattern. Normalizing here keeps the guards independent of checkout
// configuration rather than repeating the fix at each call site.
func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s failed: %v", path, err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

// child returns the value node for key in a mapping, or nil when it is absent.
func child(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// nodeAt walks a chain of mapping keys, returning nil if any link is missing.
func nodeAt(node *yaml.Node, keys ...string) *yaml.Node {
	for _, key := range keys {
		node = child(node, key)
	}
	return node
}

func stringSlice(node *yaml.Node) []string {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	values := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		values = append(values, item.Value)
	}
	return values
}

func mappingKeys(node *yaml.Node) []string {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	keys := make([]string, 0, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		keys = append(keys, node.Content[i].Value)
	}
	return keys
}

func jobNames(root *yaml.Node) []string {
	return mappingKeys(child(root, "jobs"))
}

func jobAt(root *yaml.Node, name string) *yaml.Node {
	return nodeAt(root, "jobs", name)
}

// jobStepValues collects one field from every step of a single named job.
func jobStepValues(root *yaml.Node, job, field string) []string {
	var values []string
	steps := child(jobAt(root, job), "steps")
	if steps == nil {
		return nil
	}
	for _, step := range steps.Content {
		if value := child(step, field); value != nil {
			values = append(values, value.Value)
		}
	}
	return values
}

// stepValues collects one field from every step of every job in a workflow.
func stepValues(root *yaml.Node, field string) []string {
	jobs := child(root, "jobs")
	if jobs == nil {
		return nil
	}

	var values []string
	for i := 1; i < len(jobs.Content); i += 2 {
		steps := child(jobs.Content[i], "steps")
		if steps == nil {
			continue
		}
		for _, step := range steps.Content {
			if value := child(step, field); value != nil {
				values = append(values, value.Value)
			}
		}
	}
	return values
}

// compositeUses collects the `uses:` of every step in a composite action.
func compositeUses(t *testing.T, root *yaml.Node) []string {
	t.Helper()

	steps := nodeAt(root, "runs", "steps")
	if steps == nil {
		t.Fatal("a composite action must declare runs.steps")
	}

	var values []string
	for _, step := range steps.Content {
		if value := child(step, "uses"); value != nil {
			values = append(values, value.Value)
		}
	}
	return values
}

func workflowPaths(t *testing.T) []string {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(repoRoot, ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatalf("globbing the workflow directory failed: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("expected at least one committed workflow")
	}
	return paths
}

func sorted(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
