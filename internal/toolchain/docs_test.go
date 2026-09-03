package toolchain

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/fmueller/orgtop/internal/cli"
)

// documentSet is the documentation FR-011 and FR-012 govern, keyed by the
// repository-relative path of each document. User-facing claims stay scoped to
// README.md; contributor-facing claims are satisfied by any document in the set,
// so material can live with the audience that reads it.
type documentSet map[string]string

// userDocument is the document a user reads before running the binary. Claims
// about invocation, credentials, polling, controls, Stream columns, and the
// version and help flags are checked against it alone.
const userDocument = "README.md"

// documentPaths are the documents the checks resolve over.
var documentPaths = []string{userDocument, "CONTRIBUTING.md"}

// stateFile carries the active spec version the deferred-capability list is
// keyed to. It is Taskrail-managed state, not prose, so reading the version from
// it keeps the list a function of the release being built.
var stateFile = filepath.Join(repoRoot, "planning", "STATE.md")

// activeSpecVersionPattern matches the active spec version in STATE.md's
// frontmatter.
var activeSpecVersionPattern = regexp.MustCompile(`(?m)^active_spec_version:\s*"?([^"\s]+)"?\s*$`)

// deferredClaimsBySpec are the capabilities each spec version does not ship. The
// documentation must not promise one of them under the spec that defers it.
// Every entry is keyed to a spec version rather than being a constant, because a
// deny list outlives its release otherwise: v0.1.0 defers Rain, v0.2.0 ships it
// as a primary view (FR-008). Add a key when a spec version becomes active, and
// review the entries it inherits while doing so.
var deferredClaimsBySpec = map[string][]string{
	"v0.1.0": {
		"filtering", "search", "clustering", "inspect", "rain", "drilldown",
		"live activity", "real time", "real-time", "webhook",
	},
	"v0.2.0": {
		"filtering", "search", "clustering", "inspect", "drilldown",
		"live activity", "real time", "real-time", "webhook",
	},
}

// documentedControls are the key bindings the footer advertises, scrolling
// included. Documentation that stops naming one of them misdescribes the shipped
// shell to the operator who reads it.
var documentedControls = []string{
	"`1`", "`2`", "`tab`", "`up`", "`down`", "`pgup`", "`pgdown`", "`q`", "`ctrl+c`",
}

// streamColumnsSectionID marks the section that describes Stream's columns. The
// column checks are scoped to it, because names as ordinary as "age" and
// "repository" also occur in the usage prose, and a whole-document search for
// them would pass whether or not the columns are documented at all. The marker
// is a stable identifier rather than the heading's text, so the section can be
// renamed, promoted, or nested without changing what is searched.
const streamColumnsSectionID = "stream-columns"

// documentedStreamColumns are the columns Stream renders, in the order its
// heading row names them. Like documentedControls this repeats the shipped
// spelling rather than deriving it, because the heading row is internal to
// internal/tui; documentation that stops naming a column misdescribes the view
// an operator is looking at (FR-010, FR-011). Each is quoted as the README
// quotes it, so the check cannot be satisfied by the same word used as ordinary
// prose inside the section, such as the sentence explaining that times are ages.
var documentedStreamColumns = []string{"`age`", "`repository`", "`category`", "`actor · description`"}

// sectionMarker is the comment that identifies a section by id.
func sectionMarker(id string) string {
	return "<!-- docs:" + id + " -->"
}

// documentSection returns the part of doc that belongs to the section marked
// with id: its heading and everything up to the next heading of the same or
// shallower depth. It reports whether the marked section exists, so a check
// scoped to a missing section fails rather than searching an empty window.
func documentSection(doc, id string) (string, bool) {
	marker := sectionMarker(id)
	start := strings.Index(doc, marker)
	if start < 0 {
		return "", false
	}

	lines := strings.Split(doc[start+len(marker):], "\n")
	depth := 0
	fenced := false
	var section []string
	for _, line := range lines {
		if isFence(line) {
			fenced = !fenced
		}
		level := 0
		if !fenced {
			level = headingDepth(line)
		}
		if depth == 0 {
			if level == 0 {
				continue
			}
			depth = level
			section = append(section, line)
			continue
		}
		if level > 0 && level <= depth {
			break
		}
		section = append(section, line)
	}
	if depth == 0 {
		return "", false
	}
	return strings.Join(section, "\n"), true
}

// isFence reports whether line opens or closes a fenced code block. A fence may
// be indented by up to three spaces, like a heading.
func isFence(line string) bool {
	trimmed := indentedContent(line)
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

// headingDepth returns the ATX heading depth of line, or 0 when it is not a
// heading. A line indented by four or more spaces is an indented code block
// rather than a heading, and a `#` inside a code block is not a heading at all;
// treating either as one would silently truncate the section window a check
// searches.
func headingDepth(line string) int {
	trimmed := indentedContent(line)
	depth := len(trimmed) - len(strings.TrimLeft(trimmed, "#"))
	if depth == 0 || depth > 6 || !strings.HasPrefix(trimmed[depth:], " ") {
		return 0
	}
	return depth
}

// indentedContent returns line without its leading indentation, or the empty
// string when the indentation is four or more spaces and the line is therefore
// code rather than markup.
func indentedContent(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) >= 4 {
		return ""
	}
	return trimmed
}

// readDocumentationSet reads the documents the checks resolve over.
func readDocumentationSet(t *testing.T) documentSet {
	t.Helper()

	docs := make(documentSet, len(documentPaths))
	for _, path := range documentPaths {
		docs[path] = readFile(t, filepath.Join(repoRoot, path))
	}
	return docs
}

// activeSpecVersion returns the spec version the repository is building against.
func activeSpecVersion(t *testing.T) string {
	t.Helper()

	match := activeSpecVersionPattern.FindStringSubmatch(readFile(t, stateFile))
	if match == nil {
		t.Fatalf("%s declares no active_spec_version", stateFile)
	}
	return match[1]
}

// deferredClaims returns the capabilities version does not ship. An unlisted
// version fails loudly: a new active spec has to be reviewed against the deny
// list rather than inheriting the previous release's silently.
func deferredClaims(t *testing.T, version string) []string {
	t.Helper()

	claims, ok := deferredClaimsBySpec[version]
	if !ok {
		t.Fatalf("no deferred-capability list is declared for the active spec %s", version)
	}
	return claims
}

// documentedInvocation returns the usage the binary itself declares.
func documentedInvocation(t *testing.T) string {
	t.Helper()

	var usage strings.Builder
	if _, err := cli.ParseArgs(releaseBinary, nil, &usage); err == nil {
		t.Fatal("parsing an empty argument list must be rejected")
	}
	return usage.String()
}

// invocationProblems reports the invocation lines the binary declares that the
// user document does not show. Deriving them from the parser rather than
// repeating the string means a renamed or reshaped flag fails here instead of
// misleading a first-time user.
func invocationProblems(readme, usage string) []string {
	var problems []string
	for _, line := range strings.Split(usage, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Usage:") {
			continue
		}
		if !strings.Contains(readme, strings.TrimPrefix(line, "Usage: ")) {
			problems = append(problems, "the invocation "+line+" is not documented")
		}
	}
	if !strings.Contains(readme, "--repo acme/backend --repo acme/frontend") {
		problems = append(problems, "the repeated --repo form that selects several repositories is not shown")
	}
	return problems
}

// credentialContractProblems guards FR-011 and A-004: the credential precedence
// is stated in the order the resolver applies it, and the gh fallback a user has
// to set up is named.
func credentialContractProblems(readme string) []string {
	var problems []string
	position := -1
	for _, step := range []string{"GH_TOKEN", "GITHUB_TOKEN", "gh auth token", "gh auth login"} {
		next := strings.Index(readme, step)
		if next < 0 {
			problems = append(problems, step+" is not documented")
			continue
		}
		if next < position {
			problems = append(problems, step+" is documented before the step that precedes it")
		}
		position = next
	}
	return problems
}

// pollingAndControlProblems guards the polling semantics that explain why data
// is not live and the controls the shell implements.
func pollingAndControlProblems(readme string) []string {
	var problems []string
	for _, required := range []string{"POLLING", "not live"} {
		if !strings.Contains(readme, required) {
			problems = append(problems, required+" is not documented")
		}
	}
	for _, control := range documentedControls {
		if !strings.Contains(readme, control) {
			problems = append(problems, "the "+control+" control is not documented")
		}
	}
	return problems
}

// contributorClaimProblems guards the contributor-facing claims. They are
// satisfied by any document in the set, so the local gate command can live in
// CONTRIBUTING.md with the rest of the contributor material.
func contributorClaimProblems(docs documentSet) []string {
	var problems []string
	for _, required := range []string{"task check"} {
		found := false
		for _, doc := range docs {
			if strings.Contains(doc, required) {
				found = true
				break
			}
		}
		if !found {
			problems = append(problems, required+" is documented in no document of the set")
		}
	}
	return problems
}

// deferredClaimProblems reports the deferred capabilities a document promises.
// Claims match on word boundaries, so ordinary prose about a "constraint" or
// "research" does not fail the gate for a capability it does not mention.
func deferredClaimProblems(doc string, claims []string) []string {
	var problems []string
	for _, claim := range claims {
		if regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(claim) + `\b`).MatchString(doc) {
			problems = append(problems, "the deferred capability "+claim+" is claimed")
		}
	}
	return problems
}

// staleClaimProblems keeps the documentation from describing an implemented
// milestone as unbuilt.
func staleClaimProblems(doc string) []string {
	var problems []string
	lowered := strings.ToLower(doc)
	for _, stale := range []string{"not implemented yet", "design document"} {
		if strings.Contains(lowered, stale) {
			problems = append(problems, "it still says "+stale)
		}
	}
	return problems
}

// versionAndHelpFlagProblems guards FR-011 for the two flags a bug reporter is
// asked to run first: both spellings of each are named, or a downloaded binary
// cannot be identified from its documentation.
func versionAndHelpFlagProblems(readme string) []string {
	var problems []string
	for _, flag := range []string{"--version", "-v", "--help", "-h"} {
		if !strings.Contains(readme, "`"+flag+"`") {
			problems = append(problems, "the "+flag+" flag is not documented")
		}
	}
	return problems
}

// streamColumnProblems guards FR-011 for the surface T-020 through T-023
// changed: Stream's columns are named in their own section, times are described
// as ages at the last successful refresh rather than clock times, and the two
// affordances that explain an incomplete row or list — the coverage disclosure
// and the shortening mark — are documented.
func streamColumnProblems(readme string) []string {
	lowered := strings.ToLower(readme)
	section, ok := documentSection(lowered, streamColumnsSectionID)
	if !ok {
		return []string{"there is no " + sectionMarker(streamColumnsSectionID) + " section describing the Stream columns"}
	}

	var problems []string
	for _, column := range documentedStreamColumns {
		if !strings.Contains(section, column) {
			problems = append(problems, "the Stream "+column+" column is not described")
		}
	}
	for _, required := range []string{"age at the last successful refresh", "shortened", "showing"} {
		if !strings.Contains(lowered, required) {
			problems = append(problems, required+" is not documented")
		}
	}
	return problems
}

// TestDocumentationDescribesTheShippedSurface runs every check over the
// repository's own documentation set. It is the gate FR-011 and FR-012 rest on:
// documentation that stops describing the shipped surface, or starts promising
// an unshipped one, fails the build.
func TestDocumentationDescribesTheShippedSurface(t *testing.T) {
	t.Parallel()

	docs := readDocumentationSet(t)
	readme := docs[userDocument]
	checks := map[string][]string{
		"invocation":            invocationProblems(readme, documentedInvocation(t)),
		"credential contract":   credentialContractProblems(readme),
		"polling and controls":  pollingAndControlProblems(readme),
		"contributor claims":    contributorClaimProblems(docs),
		"version and help":      versionAndHelpFlagProblems(readme),
		"Stream columns":        streamColumnProblems(readme),
		"deferred capabilities": deferredClaimProblems(readme, deferredClaims(t, activeSpecVersion(t))),
		"stale claims":          staleClaimProblems(readme),
	}
	for name, problems := range checks {
		for _, problem := range problems {
			t.Errorf("%s: %s check failed: %s", userDocument, name, problem)
		}
	}
}
