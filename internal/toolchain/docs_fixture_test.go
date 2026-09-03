package toolchain

import (
	"strings"
	"testing"
)

// restructuredDocs is a documentation set whose user-facing claims live in a
// README with different headings than the repository's own, and whose
// contributor-facing claim lives in CONTRIBUTING.md. Every guarded claim is
// present, so every check must pass over it: the checks bind to claims, not to
// one file's prose or layout.
//
// The README is written as a raw literal with ~ standing in for the backtick a
// Go raw literal cannot contain. Splicing the real character in at each of its
// several dozen occurrences would bury the fixture's claims in quoting.
func restructuredDocs() documentSet {
	return documentSet{
		"README.md": inlineBackticks(`# Product

## Running it

~~~bash
orgtop --repo OWNER/REPOSITORY [--repo OWNER/REPOSITORY ...] [--path (PATTERN | OWNER/REPOSITORY:PATTERN) ...] [--no-cache]
orgtop --path OWNER/REPOSITORY:PATTERN [--path OWNER/REPOSITORY:PATTERN ...] [--repo OWNER/REPOSITORY ...] [--no-cache]
orgtop --reset-cache
orgtop --version
~~~

Repeat it: ~orgtop --repo acme/backend --repo acme/frontend~.

~--version~ (or ~-v~) and ~--help~ (or ~-h~) exit at once.

## Credentials

1. ~GH_TOKEN~
2. ~GITHUB_TOKEN~
3. ~gh auth token --hostname github.com~

Otherwise run ~gh auth login~.

## Refreshing

The header shows POLLING because the data is not live.

<!-- docs:stream-columns -->
## The columns Stream renders

| Column | Meaning |
|---|---|
| ~age~ | The event's age at the last successful refresh |
| ~repository~ | Its repository |
| ~category~ | Its category |
| ~actor · description~ | Who acted |

### A nested note

Wide content is shortened, and the count above the headings states what Stream
is showing.

## Keys

~1~ ~2~ ~tab~ ~up~ ~down~ ~pgup~ ~pgdown~ ~q~ ~ctrl+c~

A rate-limit constraint applies, and research into restraint and training continues.
`),
		"CONTRIBUTING.md": "Run `task check` before opening a pull request.\n",
	}
}

// inlineBackticks substitutes the ~ placeholders a raw fixture literal uses for
// backticks.
func inlineBackticks(document string) string {
	return strings.ReplaceAll(document, "~", "`")
}

// TestRestructuredDocumentationSetSatisfiesEveryCheck pins the decoupling: a
// documentation set that carries every guarded claim passes even though its
// headings, layout, and file split differ from the repository's own.
func TestRestructuredDocumentationSetSatisfiesEveryCheck(t *testing.T) {
	t.Parallel()

	docs := restructuredDocs()
	readme := docs["README.md"]
	for name, problems := range map[string][]string{
		"invocation":  invocationProblems(readme, documentedInvocation(t)),
		"credentials": credentialContractProblems(readme),
		"polling":     pollingAndControlProblems(readme),
		"contributor": contributorClaimProblems(docs),
		"flags":       versionAndHelpFlagProblems(readme),
		"columns":     streamColumnProblems(readme),
		"deferred":    deferredClaimProblems(readme, deferredClaims(t, "v0.2.0")),
	} {
		if len(problems) != 0 {
			t.Errorf("restructured documentation failed the %s checks: %v", name, problems)
		}
	}
}

// TestChecksFailWhenAGuardedClaimIsDropped keeps every check able to fail: the
// gate exists to catch documentation that stops describing the shipped surface.
func TestChecksFailWhenAGuardedClaimIsDropped(t *testing.T) {
	t.Parallel()

	usage := documentedInvocation(t)
	for name, testCase := range map[string]struct {
		drop  string
		check func(string) []string
	}{
		"a control":           {drop: "`pgdown`", check: pollingAndControlProblems},
		"the polling label":   {drop: "POLLING", check: pollingAndControlProblems},
		"a credential step":   {drop: "GITHUB_TOKEN", check: credentialContractProblems},
		"the short help flag": {drop: "`-h`", check: versionAndHelpFlagProblems},
		"a Stream column":     {drop: "`repository`", check: streamColumnProblems},
		"the column section":  {drop: "<!-- docs:stream-columns -->", check: streamColumnProblems},
		"the repeated --repo": {
			drop:  "orgtop --repo acme/backend --repo acme/frontend",
			check: func(readme string) []string { return invocationProblems(readme, usage) },
		},
	} {
		readme := strings.Replace(restructuredDocs()["README.md"], testCase.drop, "", 1)
		if problems := testCase.check(readme); len(problems) == 0 {
			t.Errorf("dropping %s from the README did not fail its check", name)
		}
	}
}

// TestContributorClaimsAreSatisfiedByAnyDocument evidences the file decoupling:
// the local gate command may live in CONTRIBUTING.md alone.
func TestContributorClaimsAreSatisfiedByAnyDocument(t *testing.T) {
	t.Parallel()

	docs := restructuredDocs()
	docs["README.md"] = strings.ReplaceAll(docs["README.md"], "task check", "")
	if problems := contributorClaimProblems(docs); len(problems) != 0 {
		t.Errorf("a contributor claim documented only in CONTRIBUTING.md failed: %v", problems)
	}

	docs["CONTRIBUTING.md"] = "Nothing about the gate.\n"
	if problems := contributorClaimProblems(docs); len(problems) == 0 {
		t.Error("dropping the local gate command from every document did not fail")
	}
}

// TestDeferredClaimsMatchOnWordBoundaries covers the substring regressions: the
// deny list may not fire inside unrelated words.
func TestDeferredClaimsMatchOnWordBoundaries(t *testing.T) {
	t.Parallel()

	prose := "A rate-limit constraint, a restraint, some training, and research.\n"
	if problems := deferredClaimProblems(prose, deferredClaims(t, "v0.1.0")); len(problems) != 0 {
		t.Errorf("ordinary prose failed the deferred-capability check: %v", problems)
	}
	claim := "Rain is a primary view.\n"
	if problems := deferredClaimProblems(claim, deferredClaims(t, "v0.1.0")); len(problems) == 0 {
		t.Error("claiming Rain under v0.1.0, which defers it, did not fail")
	}
	if problems := deferredClaimProblems(claim, deferredClaims(t, "v0.2.0")); len(problems) != 0 {
		t.Errorf("claiming Rain under v0.2.0, which ships it, failed: %v", problems)
	}
	if problems := deferredClaimProblems("We ship clustering.\n", deferredClaims(t, "v0.2.0")); len(problems) == 0 {
		t.Error("claiming clustering under v0.2.0, which defers it, did not fail")
	}
}

// TestSectionWindowFollowsHeadingDepth covers the layout regressions: the window
// a section-scoped check searches ends at the next heading of the same or
// shallower depth, and never silently widens or empties.
func TestSectionWindowFollowsHeadingDepth(t *testing.T) {
	t.Parallel()

	document := "# Title\n\n<!-- docs:columns -->\n### Renamed columns\n\nin section\n\n#### Nested\n\nstill in section\n\n### After\n\nout of section\n\n## Shallower\n\nalso out\n"
	section, ok := documentSection(document, "columns")
	if !ok {
		t.Fatal("a marked section was not found")
	}
	if !strings.Contains(section, "in section") || !strings.Contains(section, "still in section") {
		t.Errorf("the section window dropped its own content: %q", section)
	}
	if strings.Contains(section, "out of section") || strings.Contains(section, "also out") {
		t.Errorf("the section window ran past the next heading of equal or shallower depth: %q", section)
	}

	promoted := strings.Replace(document, "### Renamed columns", "## Renamed columns", 1)
	section, ok = documentSection(promoted, "columns")
	if !ok {
		t.Fatal("a promoted section heading was not found")
	}
	if !strings.Contains(section, "out of section") {
		t.Errorf("a `##` section stopped at a deeper `###` heading: %q", section)
	}
	if strings.Contains(section, "also out") {
		t.Errorf("a `##` section ran past the next `##` heading: %q", section)
	}

	if _, ok := documentSection(document, "absent"); ok {
		t.Error("an absent section reported as found")
	}
}

// TestSectionWindowIgnoresNonHeadings covers the two ways a line can look like a
// heading without being one: inside a fenced code block, and indented far enough
// to be a code block itself. Treating either as a heading would silently
// truncate the searched window, which is the failure this task exists to remove.
func TestSectionWindowIgnoresNonHeadings(t *testing.T) {
	t.Parallel()

	document := inlineBackticks(`# Title

<!-- docs:columns -->
### Columns

~~~bash
# not a heading
orgtop --repo acme/backend
~~~

    # not a heading either

still in section

### After

out of section
`)
	section, ok := documentSection(document, "columns")
	if !ok {
		t.Fatal("a marked section was not found")
	}
	if !strings.Contains(section, "still in section") {
		t.Errorf("a fenced or indented line starting with # truncated the section window: %q", section)
	}
	if strings.Contains(section, "out of section") {
		t.Errorf("the section window ran past the next heading of equal depth: %q", section)
	}
}
