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

<!-- docs:view-selection -->
## Views

OrgTop has three primary views: Overview summarizes each Scope, Stream lists
events and opens local event detail, and Rain shows the ambient field with its
Interesting Now strip.

<!-- docs:overview-counts -->
## Overview counts

Overview reports PR event counts and current PR evidence from the retained snapshot.
These are not counts of distinct, open, or waiting pull requests; a row with
zero confirmed activity and zero unknowns says No activity in retained snapshot,
while unknown evidence says No confirmed activity · U unknown. The source
fetches the newest 100 events per repository and retains the newest 500 globally
rather than promising complete history.

## Running it

~~~bash
orgtop --repo OWNER/REPOSITORY [--repo OWNER/REPOSITORY ...] [--path (PATTERN | OWNER/REPOSITORY:PATTERN) ...] [--no-cache]
orgtop --path OWNER/REPOSITORY:PATTERN [--path OWNER/REPOSITORY:PATTERN ...] [--repo OWNER/REPOSITORY ...] [--no-cache]
orgtop (--org ORGANIZATION | --repo 'ORGANIZATION/*') [...] [--repo OWNER/REPOSITORY ...] [--path OWNER/REPOSITORY:PATTERN ...] [--include-archived] [--include-forks] [--no-cache]
orgtop (--org ORGANIZATION | --repo 'ORGANIZATION/*') [...] --repo OWNER/REPOSITORY [--repo OWNER/REPOSITORY ...] --path PATTERN [--path PATTERN ...] [--path OWNER/REPOSITORY:PATTERN ...] [--include-archived] [--include-forks] [--no-cache]
orgtop --reset-cache
orgtop --version
~~~

Repeat it: ~orgtop --repo acme/backend --repo acme/frontend~.

<!-- docs:organization-selection -->
## Selecting an organization

Use ~--org acme~ or ~--repo 'ORGANIZATION/*'~. Exact repositories, qualified
paths, and organization selectors can be mixed. ~--include-archived~ and
~--include-forks~ widen every organization selector. A selection accepts at
most 20 distinct repositories and 100 total Scopes; expansion truncates deterministically at those capacities, records omitted eligible repositories and,
when applicable, that more may remain, and does not present the result as the
whole organization.

~~~bash
orgtop --org acme --repo other/api --path 'other/api:src/**'
~~~

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

<!-- docs:path-diagnostics -->
## When a path value is rejected

Pattern offsets are zero-based byte indexes counted from the quoted pattern, so
a qualified value's pattern begins at byte 0. A rejected repository prefix
quotes the prefix and carries no byte offset.

- ~--path: invalid path pattern "" at byte 0: empty pattern~
- ~--path: invalid path pattern "/src" at byte 0: empty segment~
- ~--path: invalid path pattern "src//api" at byte 4: empty segment~
- ~--path: invalid path pattern "src/" at byte 3: empty segment~
- ~--path: invalid repository identifier "acme/*": repository contains an unsupported character "*"~

## Keys

<!-- docs:stream-controls -->
## Stream controls

Stream uses a visible focus marker. ~enter detail~ opens local event detail;
~esc back~ returns to that event and its viewport. The ~enter~ and ~esc~ keys are
kept beside ~q quit~ at narrow widths.

~1~ ~2~ ~3~ ~tab~ ~up~ ~down~ ~pgup~ ~pgdown~ ~enter~ ~esc~ ~-~ ~+~ ~p~ ~[~ ~]~ ~q~ ~ctrl+c~

<!-- docs:rain-windows -->
## The Rain field

Rain keeps an event while it is inside the selected window and defaults to ~24h~.

- ~15m~ ~30m~ ~60m~ ~6h~ ~24h~ ~7d~
- ~available~ shows the newest 100 events per repository the last refresh
  returned, which is not complete repository history.
Interesting Now samples the last 15 minutes as a Scope-fair recent-event sample,
not an importance ranking. Its fixed 15-minute window is independent of Rain's selected window and its -/+ controls.

<!-- docs:rain-membership -->
## Columns and pausing

An event of several visible Scopes is drawn in every matching Scope column and
stays one normalized event. ~p~ freezes motion, ageing, and expiry.
Pausing does not stop polling; arrivals are queued deterministically
and admitted when motion resumes.

<!-- docs:enrichment-cache -->
## The local cache

Evidence is reused from ~orgtop/enrichment-v1.db~ in the user cache directory,
beside ~enrichment-v1.lock~. No credential is stored. A record stays
usable for 30 days, and bounded cleanup removes invalid and expired records
first, then the least recently used, at 10,000 records, 250,000 paths, or
128 MiB. The cache is disposable: ~--no-cache~ skips it and ~--reset-cache~
removes it, and an unusable one is bypassed with CACHE DEGRADED.

<!-- docs:github-requests -->
## What a refresh spends

One request per selected repository per refresh, out of 5000 an hour, plus at
most 20 changed-file requests. An exhausted budget is shown as RATE LIMITED
with its retry time, and membership it could not decide stays unknown and is
never guessed.

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
		"invocation":             invocationProblems(readme, documentedInvocation(t)),
		"credentials":            credentialContractProblems(readme),
		"polling":                pollingAndControlProblems(readme),
		"views":                  viewSelectionProblems(readme),
		"Overview counts":        overviewCountsProblems(readme),
		"organization selection": organizationSelectionProblems(readme),
		"Stream controls":        streamDetailControlProblems(readme),
		"contributor":            contributorClaimProblems(docs),
		"flags":                  versionAndHelpFlagProblems(readme),
		"columns":                streamColumnProblems(readme),
		"diagnostics":            pathDiagnosticProblems(readme, documentedPathDiagnostics(t)),
		"Rain windows":           rainWindowProblems(readme),
		"Rain membership":        rainMembershipProblems(readme),
		"enrichment cache":       enrichmentCacheProblems(readme),
		"GitHub requests":        githubRequestProblems(readme),
		"cache help":             helpCacheProblems(documentedInvocation(t)),
		"deferred":               deferredClaimProblems(readme, deferredClaims(t, "v0.2.0")),
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
	diagnostics := documentedPathDiagnostics(t)
	for name, testCase := range map[string]struct {
		drop   string
		mutate func(string) string
		check  func(string) []string
	}{
		"a control":          {drop: "`pgdown`", check: pollingAndControlProblems},
		"the view inventory": {drop: "three primary views", check: viewSelectionProblems},
		"the Overview label": {drop: "PR event counts", check: overviewCountsProblems},
		"the mixed selection example": {
			mutate: func(readme string) string {
				return mixedOrganizationSelectionExamplePattern.ReplaceAllString(readme, "")
			},
			check: organizationSelectionProblems,
		},
		"the selection bound":               {drop: "100 total Scopes", check: organizationSelectionProblems},
		"the deterministic expansion claim": {drop: "truncates deterministically", check: organizationSelectionProblems},
		"the more-may-remain claim":         {drop: "more may remain", check: organizationSelectionProblems},
		"the Stream focus":                  {drop: "visible focus marker", check: streamDetailControlProblems},
		"the Stream section":                {drop: "<!-- docs:stream-controls -->", check: streamDetailControlProblems},
		"the Stream viewport return":        {drop: "returns to that event and its viewport", check: streamDetailControlProblems},
		"the unknown empty state":           {drop: "No confirmed activity · U unknown", check: overviewCountsProblems},
		"the polling label":                 {drop: "POLLING", check: pollingAndControlProblems},
		"a credential step":                 {drop: "GITHUB_TOKEN", check: credentialContractProblems},
		"the short help flag":               {drop: "`-h`", check: versionAndHelpFlagProblems},
		"a Stream column":                   {drop: "`repository`", check: streamColumnProblems},
		"a Rain window":                     {drop: "`7d`", check: rainWindowProblems},
		"the available bound":               {drop: "not complete repository history", check: rainWindowProblems},
		"the window section":                {drop: "<!-- docs:rain-windows -->", check: rainWindowProblems},
		"the column section":                {drop: "<!-- docs:stream-columns -->", check: streamColumnProblems},
		"the cache bound":                   {drop: "128 MiB", check: enrichmentCacheProblems},
		"the cache eviction order":          {drop: "least recently used", check: enrichmentCacheProblems},
		"the cache database name":           {drop: "enrichment-v1.db", check: enrichmentCacheProblems},
		"the cache section":                 {drop: "<!-- docs:enrichment-cache -->", check: enrichmentCacheProblems},
		"the enrichment budget":             {drop: "20 changed-file requests", check: githubRequestProblems},
		"the unknown honesty claim":         {drop: "never guessed", check: githubRequestProblems},
		"the request section":               {drop: "<!-- docs:github-requests -->", check: githubRequestProblems},
		"the Rain overlap claim":            {drop: "every matching Scope column", check: rainMembershipProblems},
		"the Rain pause claim":              {drop: "does not stop polling", check: rainMembershipProblems},
		"the membership section":            {drop: "<!-- docs:rain-membership -->", check: rainMembershipProblems},
		"a quoted diagnostic": {
			drop:  `at byte 4: empty segment`,
			check: func(readme string) []string { return pathDiagnosticProblems(readme, diagnostics) },
		},
		"the offset origin": {
			drop:  "counted from",
			check: func(readme string) []string { return pathDiagnosticProblems(readme, diagnostics) },
		},
		"the repeated --repo": {
			drop:  "orgtop --repo acme/backend --repo acme/frontend",
			check: func(readme string) []string { return invocationProblems(readme, usage) },
		},
	} {
		readme := restructuredDocs()["README.md"]
		if testCase.mutate != nil {
			readme = testCase.mutate(readme)
		} else {
			readme = strings.Replace(readme, testCase.drop, "", 1)
		}
		if problems := testCase.check(readme); len(problems) == 0 {
			t.Errorf("dropping %s from the README did not fail its check", name)
		}
	}
}

// TestHelpRainWindowCheckFailsOnADroppedOrReorderedPreset keeps the help-text
// check able to fail: a usage string that stops naming a preset, states them out
// of preset order, or drops the honest `available` bound is rejected, while the
// shipped usage passes.
func TestHelpRainWindowCheckFailsOnADroppedOrReorderedPreset(t *testing.T) {
	t.Parallel()

	complete := "windows 15m, 30m, 60m, 6h, 24h, 7d, available; a session starts at 24h; " +
		"available keeps the newest 100 events per repository, not complete repository history; " +
		"Interesting Now samples the last 15m as a Scope-fair recent-event sample, not an importance ranking"
	if problems := helpRainWindowProblems(complete); len(problems) != 0 {
		t.Errorf("a complete help text failed the Rain window check: %v", problems)
	}
	for name, broken := range map[string]string{
		"a dropped preset":    strings.Replace(complete, "7d, ", "", 1),
		"a reordered list":    strings.Replace(complete, "15m, 30m", "30m, 15m", 1),
		"the dropped bound":   strings.Replace(complete, "not complete repository history", "", 1),
		"the dropped default": strings.Replace(complete, "a session starts at 24h; ", "", 1),
	} {
		if problems := helpRainWindowProblems(broken); len(problems) == 0 {
			t.Errorf("%s in the help text did not fail its check", name)
		}
	}
	if problems := helpRainWindowProblems(documentedInvocation(t)); len(problems) != 0 {
		t.Errorf("the shipped help text failed the Rain window check: %v", problems)
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

// TestDeferredClaimsCatchSpellingVariants covers the escape the word-boundary
// match leaves open: a deferred capability written with a hyphen, split into two
// words, or run solid is the same promise to a reader, so each spelling must
// trip the gate under every keyed spec version.
func TestDeferredClaimsCatchSpellingVariants(t *testing.T) {
	t.Parallel()

	for _, version := range []string{"v0.1.0", "v0.2.0"} {
		claims := deferredClaims(t, version)
		for _, prose := range []string{
			"Every row supports drill-down.\n",
			"Every row supports drill down.\n",
			"The view updates in realtime.\n",
		} {
			if problems := deferredClaimProblems(prose, claims); len(problems) == 0 {
				t.Errorf("claiming %q under %s, which defers it, did not fail", prose, version)
			}
		}
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

// TestOrderedClaimChecksAssertClaimSequence covers the ordering half of the
// claim checks: a document that states the presets in order passes even though
// its prose names a later preset ahead of the list, and one that reorders an
// adjacent pair fails and names the preset that is out of order. Matching each
// claim from position zero would decide both cases on where a preset's text
// first happens to appear rather than on the sequence of the list itself.
func TestOrderedClaimChecksAssertClaimSequence(t *testing.T) {
	t.Parallel()

	readme := restructuredDocs()["README.md"]
	if problems := rainWindowProblems(readme); len(problems) != 0 {
		t.Errorf("a section whose prose names `24h` before the ordered list failed: %v", problems)
	}
	for name, testCase := range map[string]struct {
		from, to, named string
	}{
		"the first pair": {
			from:  "`15m` `30m`",
			to:    "`30m` `15m`",
			named: "`30m`",
		},
		"a pair the prose shadows": {
			from:  "`6h` `24h`",
			to:    "`24h` `6h`",
			named: "`24h`",
		},
	} {
		reordered := strings.Replace(readme, testCase.from, testCase.to, 1)
		if reordered == readme {
			t.Fatalf("%s: the fixture no longer contains %s", name, testCase.from)
		}
		problems := rainWindowProblems(reordered)
		if len(problems) == 0 {
			t.Errorf("reordering %s of Rain presets did not fail the check", name)
			continue
		}
		if !strings.Contains(strings.Join(problems, "\n"), testCase.named) {
			t.Errorf("reordering %s did not name %s: %v", name, testCase.named, problems)
		}
	}
}

// TestCredentialCheckFailsOnAReorderedPrecedence keeps the credential gate's
// ordering half able to fail, and keeps it about the precedence list rather
// than about the whole document: a README whose prose names the fallback step
// ahead of the list still passes while the list is in order, and still fails
// when two steps are swapped. Matching each step from position zero would
// reject the first document on the prose alone.
func TestCredentialCheckFailsOnAReorderedPrecedence(t *testing.T) {
	t.Parallel()

	readme := restructuredDocs()["README.md"]
	shadowed := strings.Replace(
		readme,
		"## Credentials\n",
		"## Credentials\n\nWith no variable set, `gh auth login` is what a reader has to run.\n",
		1,
	)
	if shadowed == readme {
		t.Fatal("the fixture no longer contains the credentials heading")
	}
	if problems := credentialContractProblems(shadowed); len(problems) != 0 {
		t.Errorf("prose naming `gh auth login` before the precedence list failed the check: %v", problems)
	}
	for name, document := range map[string]string{
		"the precedence list":              readme,
		"a prose-shadowed precedence list": shadowed,
	} {
		reordered := strings.Replace(
			document,
			"1. `GH_TOKEN`\n2. `GITHUB_TOKEN`",
			"1. `GITHUB_TOKEN`\n2. `GH_TOKEN`",
			1,
		)
		if reordered == document {
			t.Fatalf("%s: the fixture no longer contains the credential precedence list", name)
		}
		if problems := credentialContractProblems(reordered); len(problems) == 0 {
			t.Errorf("reordering %s did not fail the check", name)
		}
	}
}

// TestHelpRainWindowCheckReadsThePresetListNotTheProse pins the help-text half
// of the same property: usage that names a later preset in prose ahead of the
// `-/+` list passes while the list is in order, and fails when the list is
// reordered. First-occurrence matching would reject the in-order usage because
// `available` is mentioned before the presets that precede it.
func TestHelpRainWindowCheckReadsThePresetListNotTheProse(t *testing.T) {
	t.Parallel()

	shadowed := "the widest choice is available. " +
		"windows 15m, 30m, 60m, 6h, 24h, 7d, available; a session starts at 24h; " +
		"available keeps the newest 100 events per repository, not complete repository history; " +
		"Interesting Now samples the last 15m as a Scope-fair recent-event sample, not an importance ranking"
	if problems := helpRainWindowProblems(shadowed); len(problems) != 0 {
		t.Errorf("prose naming available before the preset list failed the check: %v", problems)
	}
	reordered := strings.Replace(shadowed, "15m, 30m", "30m, 15m", 1)
	if problems := helpRainWindowProblems(reordered); len(problems) == 0 {
		t.Error("a prose-shadowed help text with a reordered preset list did not fail the check")
	}
}
