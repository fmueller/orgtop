package tui

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// pathScope builds the path Scope of the repository from one leading literal
// segment followed by `/**`.
func pathScope(t *testing.T, repository, segment string) domain.Scope {
	t.Helper()
	matcher, err := domain.NewPathMatcher([]domain.MatcherToken{
		domain.LiteralToken(segment), domain.SeparatorToken(), domain.RecursiveToken(),
	})
	if err != nil {
		t.Fatalf("NewPathMatcher(%q) returned error: %v", segment, err)
	}
	scope, err := domain.NewPathScope(testRepository(t, repository), matcher)
	if err != nil {
		t.Fatalf("NewPathScope(%q, %q) returned error: %v", repository, segment, err)
	}
	return scope
}

// scopeSet builds the validated selection of the Scopes.
func scopeSet(t *testing.T, scopes ...domain.Scope) domain.ScopeSet {
	t.Helper()
	set, err := domain.NewScopeSet(scopes)
	if err != nil {
		t.Fatalf("NewScopeSet returned error: %v", err)
	}
	return set
}

// scopedModel returns a current model publishing the retained evidence against
// the selection, exactly as a successful refresh does.
func scopedModel(t *testing.T, scopes domain.ScopeSet, retained []domain.EventEvidence) Model {
	t.Helper()
	model, err := New(context.Background(), scopes, &fakeSource{})
	if err != nil {
		t.Fatalf("building the scoped test model failed: %v", err)
	}
	model.state.Scopes = scopes
	model.state.Scoped = domain.NewRetainedSnapshot(scopes, retained, false)
	model.state.Freshness = FreshnessCurrent
	return model
}

// evidenceFor builds count retained pushes of the repository, each settled at
// the outcome, with identifiers unique across the whole fixture.
func evidenceFor(t *testing.T, prefix, repository string, count int, outcome domain.EvidenceOutcome) []domain.EventEvidence {
	t.Helper()
	retained := make([]domain.EventEvidence, 0, count)
	for index := range count {
		event := testEvent(t, prefix+strconv.Itoa(index), repository, domain.CategoryPush, domain.EntityCommit)
		retained = append(retained, domain.EventEvidence{Event: event, Outcome: outcome})
	}
	return retained
}

// rowFor returns the rendered body row carrying the Scope token.
func rowFor(t *testing.T, rows []string, token string) string {
	t.Helper()
	for _, row := range rows {
		if strings.HasPrefix(strings.TrimSpace(row), token+" ") {
			return row
		}
	}
	t.Fatalf("no rendered row carries the Scope token %q:\n%v", token, rows)
	return ""
}

// TestOverviewRendersMixedScopesInPreparedOrder guards RG-012/A-075: Overview
// renders one row per selected Scope, labels it with the globally ordinal
// presentation token, and follows the prepared confirmed-activity order.
func TestOverviewRendersMixedScopesInPreparedOrder(t *testing.T) {
	api, web := "acme/api", "acme/web"
	scopes := scopeSet(t,
		domain.NewRepositoryScope(testRepository(t, web)),
		pathScope(t, api, "src"),
		domain.NewRepositoryScope(testRepository(t, api)),
		pathScope(t, api, "docs"),
	)
	retained := append(
		evidenceFor(t, "src", api, 3, completeEvidence(t, "src/main.go")),
		evidenceFor(t, "docs", api, 1, completeEvidence(t, "docs/readme.md"))...,
	)

	rows := bodyLines(t, renderAt(t, scopedModel(t, scopes, retained), 120, wideHeight))

	wantLabels := []string{
		"R1 acme/api",
		"P3 acme/api:src/**",
		"P2 acme/api:docs/**",
		"R4 acme/web",
	}
	if len(rows) != len(wantLabels) {
		t.Fatalf("overview rendered %d rows, want one per Scope:\n%v", len(rows), rows)
	}
	for index, want := range wantLabels {
		if !strings.HasPrefix(strings.TrimSpace(rows[index]), want) {
			t.Errorf("row %d is %q, want it to start with the Scope label %q", index, rows[index], want)
		}
	}
}

// TestOverviewNeverPresentsAPathScopeAsARepository guards FR-007: a path Scope
// stays distinguishable and is never spelled as a synthetic repository.
func TestOverviewNeverPresentsAPathScopeAsARepository(t *testing.T) {
	api := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, api)), pathScope(t, api, "src"))

	rows := bodyLines(t, renderAt(t, scopedModel(t, scopes,
		evidenceFor(t, "src", api, 2, completeEvidence(t, "src/main.go"))), 120, wideHeight))

	path := rowFor(t, rows, "P2")
	if !strings.Contains(path, "acme/api:src/**") {
		t.Errorf("the path Scope row %q does not carry its repository and pattern", path)
	}
	repository := rowFor(t, rows, "R1")
	if strings.Contains(repository, ":") {
		t.Errorf("the repository Scope row %q carries a path pattern", repository)
	}
}

// TestOverviewReportsLowerBoundActivityWithUnknownCoverage guards A-037: the
// activity count includes only confirmed members, the unknown total stays
// visible beside it, and neither empty state is spelled as the other.
func TestOverviewReportsLowerBoundActivityWithUnknownCoverage(t *testing.T) {
	api := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, api)), pathScope(t, api, "src"))
	retained := append(append(
		evidenceFor(t, "member", api, 12, completeEvidence(t, "src/main.go")),
		evidenceFor(t, "other", api, 5, completeEvidence(t, "docs/readme.md"))...),
		evidenceFor(t, "unknown", api, 3, domain.FailedOutcome("upstream failed"))...,
	)

	rows := bodyLines(t, renderAt(t, scopedModel(t, scopes, retained), 120, wideHeight))

	if path := rowFor(t, rows, "P2"); !strings.Contains(path, "12 activity"+separator+"3 unknown") {
		t.Errorf("the path Scope row %q does not report 12 activity · 3 unknown", path)
	}
}

func TestOverviewDistinguishesAllUnknownCoverageFromNoActivity(t *testing.T) {
	api := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, api)), pathScope(t, api, "src"))

	cases := []struct {
		name     string
		retained []domain.EventEvidence
		want     string
		unwanted string
	}{
		{
			name:     "all unknown",
			retained: evidenceFor(t, "unknown", api, 3, domain.FailedOutcome("upstream failed")),
			want:     "No confirmed activity" + separator + "3 unknown",
		},
		{
			name:     "complete evidence without a match",
			retained: evidenceFor(t, "other", api, 2, completeEvidence(t, "docs/readme.md")),
			want:     "No activity",
			unwanted: "unknown",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			rows := bodyLines(t, renderAt(t, scopedModel(t, scopes, testCase.retained), 120, wideHeight))
			path := rowFor(t, rows, "P2")
			if !strings.Contains(path, testCase.want) {
				t.Errorf("the path Scope row %q does not report %q", path, testCase.want)
			}
			if testCase.unwanted != "" && strings.Contains(path, testCase.unwanted) {
				t.Errorf("the path Scope row %q reports %q without any unknown evidence", path, testCase.unwanted)
			}
		})
	}
}

// TestOverviewQualifiesCurrentPRMembership guards A-039: qualified members are
// counted inside the activity total and disclosed separately.
func TestOverviewQualifiesCurrentPRMembership(t *testing.T) {
	api := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, api)), pathScope(t, api, "src"))
	current := domain.CompleteOutcome(domain.ProvenanceCurrentPR, mustChangedPaths(t, "src/main.go"))
	retained := append(
		evidenceFor(t, "member", api, 10, completeEvidence(t, "src/main.go")),
		evidenceFor(t, "current", api, 2, current)...,
	)

	rows := bodyLines(t, renderAt(t, scopedModel(t, scopes, retained), 120, wideHeight))

	if path := rowFor(t, rows, "P2"); !strings.Contains(path, "12 activity"+separator+"2 current PR") {
		t.Errorf("the path Scope row %q does not report 12 activity · 2 current PR", path)
	}
}

// TestOverviewComposesActivityCurrentPRAndUnknown guards RG-004's own worked
// example: the three clauses appear together, in the contract's order, with the
// qualified current-PR members counted inside the activity total rather than
// beside it.
func TestOverviewComposesActivityCurrentPRAndUnknown(t *testing.T) {
	api := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, api)), pathScope(t, api, "src"))
	current := domain.CompleteOutcome(domain.ProvenanceCurrentPR, mustChangedPaths(t, "src/main.go"))
	retained := append(append(
		evidenceFor(t, "member", api, 10, completeEvidence(t, "src/main.go")),
		evidenceFor(t, "current", api, 2, current)...),
		evidenceFor(t, "unknown", api, 3, domain.FailedOutcome("upstream failed"))...,
	)

	rows := bodyLines(t, renderAt(t, scopedModel(t, scopes, retained), 140, wideHeight))

	want := "12 activity" + separator + "2 current PR" + separator + "3 unknown"
	if path := rowFor(t, rows, "P2"); !strings.Contains(path, want) {
		t.Errorf("the path Scope row %q does not report %q", path, want)
	}
}

// TestOverviewKeepsMixedScopesReadableAtTheMinimumTerminal guards A-081: at
// 40x10 every Scope keeps its token, one primary row survives, and no line
// exceeds the width.
func TestOverviewKeepsMixedScopesReadableAtTheMinimumTerminal(t *testing.T) {
	api := "acme/platform-integration-services"
	scopes := scopeSet(t,
		domain.NewRepositoryScope(testRepository(t, api)),
		pathScope(t, api, "services-and-adapters"),
	)
	content := renderAt(t, scopedModel(t, scopes,
		evidenceFor(t, "member", api, 2, completeEvidence(t, "services-and-adapters/main.go"))),
		narrowWidth, narrowHeight)

	assertFits(t, content, narrowWidth, narrowHeight)
	rows := bodyLines(t, content)
	if len(rows) == 0 {
		t.Fatalf("the 40x10 overview rendered no Scope row:\n%s", content)
	}
	if !strings.HasPrefix(strings.TrimSpace(rows[0]), "R1 ") {
		t.Errorf("the 40x10 primary row %q loses its Scope token", rows[0])
	}
	for index, row := range rows {
		if rendered := lipgloss.Width(row); rendered > narrowWidth {
			t.Errorf("row %d is %d cells wide, want at most %d: %q", index, rendered, narrowWidth, row)
		}
	}
}
