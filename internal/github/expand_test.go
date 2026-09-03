package github_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
	"github.com/fmueller/orgtop/internal/github"
)

// listingPage is one canned organization listing response.
type listingPage struct {
	status int
	body   string
	header map[string]string
	// next is the page number the response advertises through a documented
	// rel="next" Link header. Zero advertises no next page.
	next int
	// link overrides the advertised Link header verbatim. One %s is replaced by
	// the fixture host, so a test can advertise a deliberately invalid link.
	link string
}

// orgPage is the fixture key of one listing page request.
func orgPage(organization string, page int) string {
	return fmt.Sprintf("/orgs/%s/repos?page=%d", organization, page)
}

func newListingServer(t *testing.T, pages map[string]listingPage) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		key := request.URL.Path + "?page=" + request.URL.Query().Get("page")
		page, known := pages[key]
		if !known {
			t.Errorf("unexpected listing request %q", key)
			writer.WriteHeader(http.StatusNotImplemented)
			return
		}
		for name, value := range page.header {
			writer.Header().Set(name, value)
		}
		switch {
		case page.link != "":
			writer.Header().Set("Link", fmt.Sprintf(page.link, request.Host))
		case page.next > 0:
			writer.Header().Set("Link", fmt.Sprintf(
				`<http://%s%s?type=all&sort=full_name&direction=asc&per_page=100&page=%d>; rel="next"`,
				request.Host, request.URL.Path, page.next))
		}
		status := page.status
		if status == 0 {
			status = http.StatusOK
		}
		writer.WriteHeader(status)
		_, _ = io.WriteString(writer, page.body)
	}))
	t.Cleanup(server.Close)
	return server
}

// record is one listing record fixture.
type record struct {
	fullName  string
	owner     string
	ownerType string
	name      string
	archived  bool
	disabled  bool
	fork      bool
	raw       string
}

// sourceRecord returns an eligible active source repository record.
func sourceRecord(fullName string) record {
	owner, name, _ := strings.Cut(fullName, "/")
	return record{fullName: fullName, owner: owner, ownerType: "Organization", name: name}
}

func (r record) json() string {
	if r.raw != "" {
		return r.raw
	}
	return fmt.Sprintf(
		`{"full_name":%q,"name":%q,"owner":{"login":%q,"type":%q},"archived":%t,"disabled":%t,"fork":%t}`,
		r.fullName, r.name, r.owner, r.ownerType, r.archived, r.disabled, r.fork)
}

func listingBody(records ...record) string {
	encoded := make([]string, 0, len(records))
	for _, entry := range records {
		encoded = append(encoded, entry.json())
	}
	return "[" + strings.Join(encoded, ",") + "]"
}

// sourceListing returns a one-page listing body of eligible repositories.
func sourceListing(fullNames ...string) string {
	records := make([]record, 0, len(fullNames))
	for _, fullName := range fullNames {
		records = append(records, sourceRecord(fullName))
	}
	return listingBody(records...)
}

func newListingSource(t *testing.T, pages map[string]listingPage) (github.Source, *recordingClient) {
	t.Helper()
	return newTestSource(t, newListingServer(t, pages))
}

// expandOrganizations expands the selectors against the fixture pages with no
// exact selection and no inclusion flag.
func expandOrganizations(t *testing.T, pages map[string]listingPage, organizations ...string) (github.Expansion, *recordingClient) {
	t.Helper()
	source, client := newListingSource(t, pages)
	expansion, err := source.Expand(context.Background(), github.ExpansionRequest{Organizations: organizations})
	if err != nil {
		t.Fatalf("expanding %v failed: %v", organizations, err)
	}
	return expansion, client
}

// expandExpectingError expands one selector whose fixture must fail the whole
// attempt and asserts that no partial selection is published.
func expandExpectingError(t *testing.T, pages map[string]listingPage, organizations ...string) *github.ExpansionError {
	t.Helper()
	source, _ := newListingSource(t, pages)
	expansion, err := source.Expand(context.Background(), github.ExpansionRequest{Organizations: organizations})
	if err == nil {
		t.Fatalf("expansion succeeded with %d scopes, want an error", expansion.Scopes.Len())
	}
	if expansion.Scopes.Len() != 0 || len(expansion.Selection.Selectors) != 0 {
		t.Errorf("failed expansion published %d scopes and %d selector reports, want no partial selection",
			expansion.Scopes.Len(), len(expansion.Selection.Selectors))
	}
	var failure *github.ExpansionError
	if !errors.As(err, &failure) {
		t.Fatalf("error %v is not a *github.ExpansionError", err)
	}
	return failure
}

// assertSanitized keeps every reported expansion cause free of the credential
// the fixture source authenticates with (NFR-003).
func assertSanitized(t *testing.T, message string) {
	t.Helper()
	if strings.Contains(message, sentinelToken) {
		t.Error("a reported expansion failure carries the credential value")
	}
}

func scopeStrings(scopes []domain.Scope) []string {
	rendered := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		rendered = append(rendered, scope.String())
	}
	return rendered
}

func assertScopes(t *testing.T, expansion github.Expansion, want ...string) {
	t.Helper()
	got := scopeStrings(expansion.Scopes.Scopes())
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("expanded scopes are %v, want %v", got, want)
	}
}

func selectorReport(t *testing.T, expansion github.Expansion, organization string) github.SelectorSelection {
	t.Helper()
	for _, report := range expansion.Selection.Selectors {
		if report.Organization == organization {
			return report
		}
	}
	t.Fatalf("no selector report for %q in %+v", organization, expansion.Selection.Selectors)
	return github.SelectorSelection{}
}

// mustPathScopes returns the exact selection of one path Scope per repository
// and pattern literal, so a capacity test can fill Scope capacity without
// filling repository capacity.
func mustPathScopes(t *testing.T, repositories, patterns int) domain.ScopeSet {
	t.Helper()
	scopes := make([]domain.Scope, 0, repositories*patterns)
	for r := range repositories {
		repository, err := domain.ParseRepository(fmt.Sprintf("exact/repo-%02d", r))
		if err != nil {
			t.Fatalf("building the exact repository failed: %v", err)
		}
		for p := range patterns {
			matcher, err := domain.NewPathMatcher([]domain.MatcherToken{domain.LiteralToken(fmt.Sprintf("dir%02d", p))})
			if err != nil {
				t.Fatalf("building the exact matcher failed: %v", err)
			}
			scope, err := domain.NewPathScope(repository, matcher)
			if err != nil {
				t.Fatalf("building the exact path scope failed: %v", err)
			}
			scopes = append(scopes, scope)
		}
	}
	set, err := domain.NewScopeSet(scopes)
	if err != nil {
		t.Fatalf("building the exact selection failed: %v", err)
	}
	return set
}

func TestExpansionRequestsTheDocumentedListingPage(t *testing.T) {
	expansion, client := expandOrganizations(t, map[string]listingPage{
		orgPage("Acme", 1): {body: sourceListing("acme/api")},
	}, "Acme")

	requests := client.recorded()
	if len(requests) != 1 {
		t.Fatalf("expansion sent %d requests, want exactly one", len(requests))
	}
	request := requests[0]
	if request.method != http.MethodGet || request.path != "/orgs/Acme/repos" {
		t.Errorf("expansion sent %s %s, want GET /orgs/Acme/repos", request.method, request.path)
	}
	want := map[string]string{"type": "all", "sort": "full_name", "direction": "asc", "per_page": "100", "page": "1"}
	for key, value := range want {
		if request.query.Get(key) != value {
			t.Errorf("listing query %s is %q, want %q", key, request.query.Get(key), value)
		}
	}
	if len(request.query) != len(want) {
		t.Errorf("listing query is %v, want exactly %v", request.query, want)
	}
	if request.header.Get("Accept") != "application/vnd.github+json" {
		t.Errorf("listing Accept is %q, want the documented media type", request.header.Get("Accept"))
	}
	if request.header.Get("X-GitHub-Api-Version") != "2026-03-10" {
		t.Errorf("listing API version is %q, want 2026-03-10", request.header.Get("X-GitHub-Api-Version"))
	}
	if request.header.Get("Authorization") != "Bearer "+sentinelToken {
		t.Error("listing request does not carry the resolved credential")
	}
	if !request.hasDeadline {
		t.Error("listing request carries no deadline")
	}
	if expansion.Requests != 1 {
		t.Errorf("expansion recorded %d requests, want 1", expansion.Requests)
	}
}

func TestExpansionRetainsEligibleRepositoriesByTheirListingBooleans(t *testing.T) {
	pages := map[string]listingPage{
		orgPage("acme", 1): {body: listingBody(
			sourceRecord("acme/active"),
			record{fullName: "acme/archived", owner: "acme", ownerType: "Organization", name: "archived", archived: true},
			record{fullName: "acme/forked", owner: "acme", ownerType: "Organization", name: "forked", fork: true},
			record{fullName: "acme/disabled", owner: "acme", ownerType: "Organization", name: "disabled", disabled: true},
			sourceRecord("acme/private-never-pushed"),
			sourceRecord("acme/old-pushed"),
		)},
	}

	expansion, client := expandOrganizations(t, pages, "acme")

	assertScopes(t, expansion, "acme/active", "acme/old-pushed", "acme/private-never-pushed")
	report := selectorReport(t, expansion, "acme")
	if report.Listed != 6 || report.Eligible != 3 || report.Retained != 3 || report.Omitted != 0 {
		t.Errorf("selector report is %+v, want listed 6, eligible 3, retained 3, omitted 0", report)
	}
	if report.Disabled != 1 || report.Archived != 1 || report.Fork != 1 {
		t.Errorf("exclusion buckets are %+v, want one disabled, one archived, one fork", report)
	}
	if report.HasMore {
		t.Error("selector reports a remaining page although no next link was advertised")
	}
	if got := len(client.recorded()); got != 1 {
		t.Errorf("eligibility spent %d requests, want exactly the one listing page and no probe", got)
	}
}

func TestExpansionInclusionFlagsAdmitArchivedAndForkedRepositories(t *testing.T) {
	pages := map[string]listingPage{
		orgPage("acme", 1): {body: listingBody(
			sourceRecord("acme/active"),
			record{fullName: "acme/archived", owner: "acme", ownerType: "Organization", name: "archived", archived: true},
			record{fullName: "acme/forked", owner: "acme", ownerType: "Organization", name: "forked", fork: true},
			record{fullName: "acme/disabled", owner: "acme", ownerType: "Organization", name: "disabled", disabled: true},
		)},
	}
	source, _ := newListingSource(t, pages)

	expansion, err := source.Expand(context.Background(), github.ExpansionRequest{
		Organizations:   []string{"acme"},
		IncludeArchived: true,
		IncludeForks:    true,
	})
	if err != nil {
		t.Fatalf("expansion failed: %v", err)
	}

	assertScopes(t, expansion, "acme/active", "acme/archived", "acme/forked")
	report := selectorReport(t, expansion, "acme")
	if report.Eligible != 3 || report.Disabled != 1 || report.Archived != 0 || report.Fork != 0 {
		t.Errorf("selector report is %+v, want three eligible and only the disabled exclusion", report)
	}
}

func TestExpansionExclusionBucketsStayDisjointInContractPrecedence(t *testing.T) {
	pages := map[string]listingPage{
		orgPage("acme", 1): {body: listingBody(
			record{fullName: "acme/all", owner: "acme", ownerType: "Organization", name: "all", archived: true, disabled: true, fork: true},
			record{fullName: "acme/both", owner: "acme", ownerType: "Organization", name: "both", archived: true, fork: true},
		)},
	}

	expansion, _ := expandOrganizations(t, pages, "acme")

	report := selectorReport(t, expansion, "acme")
	if report.Disabled != 1 || report.Archived != 1 || report.Fork != 0 {
		t.Errorf("exclusion buckets are %+v, want disabled before archived before fork", report)
	}
	if report.Disabled+report.Archived+report.Fork+report.Eligible != report.Listed {
		t.Errorf("exclusion buckets plus eligible are %d, want the listed count %d",
			report.Disabled+report.Archived+report.Fork+report.Eligible, report.Listed)
	}
}

func TestExpansionRetainsTheFirstValidFullNameSpelling(t *testing.T) {
	pages := map[string]listingPage{
		orgPage("acme", 1): {body: listingBody(
			record{fullName: "Acme/API", owner: "acme", ownerType: "Organization", name: "api"},
		)},
	}

	expansion, _ := expandOrganizations(t, pages, "acme")

	assertScopes(t, expansion, "Acme/API")
}

func TestExpansionFailsOnAMalformedListingRecord(t *testing.T) {
	tests := map[string]record{
		"owner login mismatch":  {fullName: "other/api", owner: "other", ownerType: "Organization", name: "api"},
		"owner is a user":       {fullName: "acme/api", owner: "acme", ownerType: "User", name: "api"},
		"full name mismatch":    {fullName: "acme/api", owner: "acme", ownerType: "Organization", name: "other"},
		"extra slash":           {fullName: "acme/team/api", owner: "acme", ownerType: "Organization", name: "api"},
		"non-ascii component":   {fullName: "acme/äpi", owner: "acme", ownerType: "Organization", name: "äpi"},
		"missing archived flag": {raw: `{"full_name":"acme/api","name":"api","owner":{"login":"acme","type":"Organization"},"disabled":false,"fork":false}`},
		"missing owner":         {raw: `{"full_name":"acme/api","name":"api","archived":false,"disabled":false,"fork":false}`},
		"empty full name":       {raw: `{"full_name":"","name":"api","owner":{"login":"acme","type":"Organization"},"archived":false,"disabled":false,"fork":false}`},
	}

	for name, malformed := range tests {
		t.Run(name, func(t *testing.T) {
			failure := expandExpectingError(t, map[string]listingPage{
				orgPage("acme", 1): {body: listingBody(sourceRecord("acme/valid"), malformed)},
			}, "acme")

			if !errors.Is(failure, github.ErrInvalidListing) {
				t.Errorf("failure %v is not an invalid listing", failure)
			}
			assertSanitized(t, failure.Error())
		})
	}
}

func TestExpansionCollapsesConsistentDuplicatesAndFailsConflictingOnes(t *testing.T) {
	consistent, _ := expandOrganizations(t, map[string]listingPage{
		orgPage("acme", 1): {body: listingBody(
			sourceRecord("Acme/API"),
			record{fullName: "acme/api", owner: "ACME", ownerType: "Organization", name: "API"},
		)},
	}, "acme")

	assertScopes(t, consistent, "Acme/API")
	if report := selectorReport(t, consistent, "acme"); report.Listed != 1 || report.Retained != 1 {
		t.Errorf("consistent duplicate report is %+v, want one listed and one retained record", report)
	}

	failure := expandExpectingError(t, map[string]listingPage{
		orgPage("acme", 1): {body: listingBody(
			sourceRecord("acme/api"),
			record{fullName: "acme/api", owner: "acme", ownerType: "Organization", name: "api", archived: true},
		)},
	}, "acme")
	if !errors.Is(failure, github.ErrInvalidListing) {
		t.Errorf("conflicting duplicate failure %v is not an invalid listing", failure)
	}
}

func TestExpansionSpendsFiveRequestsInFairRoundRobinOrder(t *testing.T) {
	pages := map[string]listingPage{
		orgPage("a", 1): {body: sourceListing("a/one"), next: 2},
		orgPage("a", 2): {body: sourceListing("a/two"), next: 3},
		orgPage("b", 1): {body: sourceListing("b/one"), next: 2},
		orgPage("b", 2): {body: sourceListing("b/two"), next: 3},
		orgPage("c", 1): {body: sourceListing("c/one")},
	}

	expansion, client := expandOrganizations(t, pages, "a", "b", "c")

	var order []string
	for _, request := range client.recorded() {
		order = append(order, request.path+"?page="+request.query.Get("page"))
	}
	want := []string{orgPage("a", 1), orgPage("b", 1), orgPage("c", 1), orgPage("a", 2), orgPage("b", 2)}
	if strings.Join(order, " ") != strings.Join(want, " ") {
		t.Errorf("request order is %v, want %v", order, want)
	}
	if expansion.Requests != 5 {
		t.Errorf("expansion recorded %d requests, want the closed five", expansion.Requests)
	}
	if !selectorReport(t, expansion, "a").HasMore || !selectorReport(t, expansion, "b").HasMore {
		t.Error("selectors with an unconsumed next link do not report a remaining page")
	}
	if selectorReport(t, expansion, "c").HasMore {
		t.Error("the exhausted selector reports a remaining page")
	}
	if !expansion.Selection.PaginationRemains {
		t.Error("a bounded expansion with unconsumed next links does not report remaining pagination")
	}
}

func TestExpansionWithFiveSelectorsDispatchesOnlyFirstPages(t *testing.T) {
	organizations := []string{"a", "b", "c", "d", "e"}
	pages := make(map[string]listingPage, len(organizations))
	for _, organization := range organizations {
		pages[orgPage(organization, 1)] = listingPage{body: sourceListing(organization + "/one"), next: 2}
	}

	expansion, client := expandOrganizations(t, pages, organizations...)

	if got := len(client.recorded()); got != 5 {
		t.Fatalf("five selectors spent %d requests, want exactly five first pages", got)
	}
	for index, request := range client.recorded() {
		if request.query.Get("page") != "1" || request.path != "/orgs/"+organizations[index]+"/repos" {
			t.Errorf("request %d is %s?page=%s, want the first page of %s", index, request.path, request.query.Get("page"), organizations[index])
		}
	}
	for _, organization := range organizations {
		if !selectorReport(t, expansion, organization).HasMore {
			t.Errorf("selector %q does not disclose its unconsumed next link", organization)
		}
	}
}

func TestExpansionRejectsInvalidPagination(t *testing.T) {
	const query = "type=all&sort=full_name&direction=asc&per_page=100"
	tests := map[string]string{
		"cyclic":            `<http://%s/orgs/acme/repos?` + query + `&page=1>; rel="next"`,
		"regressing":        `<http://%s/orgs/acme/repos?` + query + `&page=0>; rel="next"`,
		"skipped page":      `<http://%s/orgs/acme/repos?` + query + `&page=3>; rel="next"`,
		"cross host":        `<https://evil.example.com/orgs/acme/repos?` + query + `&page=2>; rel="next"`,
		"cross organizaton": `<http://%s/orgs/other/repos?` + query + `&page=2>; rel="next"`,
		"other endpoint":    `<http://%s/orgs/acme/members?` + query + `&page=2>; rel="next"`,
		"extra query":       `<http://%s/orgs/acme/repos?` + query + `&page=2&affiliation=direct>; rel="next"`,
		"changed sort":      `<http://%s/orgs/acme/repos?type=all&sort=updated&direction=asc&per_page=100&page=2>; rel="next"`,
		"changed per page":  `<http://%s/orgs/acme/repos?type=all&sort=full_name&direction=asc&per_page=50&page=2>; rel="next"`,
		"malformed":         `not-a-link; rel="next"`,
	}

	for name, link := range tests {
		t.Run(name, func(t *testing.T) {
			failure := expandExpectingError(t, map[string]listingPage{
				orgPage("acme", 1): {body: sourceListing("acme/api"), link: link},
			}, "acme")

			if !errors.Is(failure, github.ErrInvalidPagination) {
				t.Errorf("failure %v is not an invalid pagination failure", failure)
			}
			assertSanitized(t, failure.Error())
		})
	}
}

// listingRecords returns count eligible records in canonical order.
func listingRecords(count int) []record {
	records := make([]record, 0, count)
	for index := range count {
		records = append(records, sourceRecord(fmt.Sprintf("acme/repo-%03d", index)))
	}
	return records
}

// TestExpansionBoundsAListingPageByTheRequestedPageSize keeps the malformed-page
// bound tied to the per_page value the request actually asks for: a full page is
// valid and one record more is malformed, so the two closed RG-010 facts cannot
// drift apart.
func TestExpansionBoundsAListingPageByTheRequestedPageSize(t *testing.T) {
	_, client := expandOrganizations(t, map[string]listingPage{
		orgPage("acme", 1): {body: `[]`},
	}, "acme")
	requested := client.recorded()[0].query.Get("per_page")
	size, err := strconv.Atoi(requested)
	if err != nil {
		t.Fatalf("the listing request asked for per_page %q, want a number: %v", requested, err)
	}

	full, _ := expandOrganizations(t, map[string]listingPage{
		orgPage("acme", 1): {body: listingBody(listingRecords(size)...)},
	}, "acme")
	if report := selectorReport(t, full, "acme"); report.Listed != size {
		t.Errorf("a full listing page listed %d records, want the requested %d", report.Listed, size)
	}

	failure := expandExpectingError(t, map[string]listingPage{
		orgPage("acme", 1): {body: listingBody(listingRecords(size + 1)...)},
	}, "acme")
	if !errors.Is(failure, github.ErrInvalidListing) {
		t.Errorf("failure %v is not an invalid listing", failure)
	}
}

func TestExpansionTruncatesWithinCapacityAndDisclosesTheRemainder(t *testing.T) {
	pages := make(map[string]listingPage, 5)
	for page := 1; page <= 5; page++ {
		records := make([]record, 0, 100)
		for index := range 100 {
			records = append(records, sourceRecord(fmt.Sprintf("acme/repo-%03d", (page-1)*100+index)))
		}
		pages[orgPage("acme", page)] = listingPage{body: listingBody(records...), next: page + 1}
	}

	expansion, client := expandOrganizations(t, pages, "acme")

	if got := len(client.recorded()); got != 5 {
		t.Fatalf("bounded expansion spent %d requests, want five", got)
	}
	want := make([]string, 0, domain.MaxSelectedRepositories)
	for index := range domain.MaxSelectedRepositories {
		want = append(want, fmt.Sprintf("acme/repo-%03d", index))
	}
	assertScopes(t, expansion, want...)
	report := selectorReport(t, expansion, "acme")
	if report.Listed != 500 || report.Eligible != 500 || report.Retained != 20 || report.Omitted != 480 {
		t.Errorf("selector report is %+v, want 500 listed, 500 eligible, 20 retained, 480 omitted", report)
	}
	if !report.HasMore || !expansion.Selection.PaginationRemains {
		t.Error("a bounded five-page attempt with a further next link does not disclose the remainder")
	}
	selection := expansion.Selection
	if selection.ExactScopes != 0 || selection.ExpandedScopes != 20 || selection.TotalScopes != 20 || selection.DistinctRepositories != 20 {
		t.Errorf("selection counts are %+v, want 0 exact, 20 expanded, 20 scopes, 20 repositories", selection)
	}
	if len(selection.Provenance) != 20 {
		t.Fatalf("selection carries %d provenance entries, want one per scope", len(selection.Provenance))
	}
	for _, entry := range selection.Provenance {
		if entry.Exact || entry.Selector != "acme" {
			t.Errorf("provenance %+v does not attribute the scope to the acme selector", entry)
		}
	}
}

func TestExpansionAllocatesFairlyWithinTheRemainingRepositoryCapacity(t *testing.T) {
	exact := make([]string, 0, 18)
	for index := range 18 {
		exact = append(exact, fmt.Sprintf("exact/repo-%02d", index))
	}
	set, err := domain.NewRepositoryScopeSet(exact)
	if err != nil {
		t.Fatalf("building the exact selection failed: %v", err)
	}
	source, _ := newListingSource(t, map[string]listingPage{
		orgPage("a", 1): {body: sourceListing("a/a1", "a/a2")},
		orgPage("b", 1): {body: sourceListing("b/b1", "b/b2")},
	})

	expansion, err := source.Expand(context.Background(), github.ExpansionRequest{
		Organizations: []string{"a", "b"},
		Exact:         set,
	})
	if err != nil {
		t.Fatalf("expansion failed: %v", err)
	}

	assertScopes(t, expansion, append(exact, "a/a1", "b/b1")...)
	for _, organization := range []string{"a", "b"} {
		report := selectorReport(t, expansion, organization)
		if report.Eligible != 2 || report.Retained != 1 || report.Omitted != 1 {
			t.Errorf("selector %q report is %+v, want two eligible, one retained, one omitted", organization, report)
		}
	}
	if expansion.Selection.DistinctRepositories != domain.MaxSelectedRepositories {
		t.Errorf("expansion selected %d distinct repositories, want the closed %d",
			expansion.Selection.DistinctRepositories, domain.MaxSelectedRepositories)
	}
}

func TestExpansionExactDuplicateGainsProvenanceWithoutASecondScope(t *testing.T) {
	set, err := domain.NewRepositoryScopeSet([]string{"Acme/API"})
	if err != nil {
		t.Fatalf("building the exact selection failed: %v", err)
	}
	source, _ := newListingSource(t, map[string]listingPage{
		orgPage("acme", 1): {body: sourceListing("acme/api", "acme/zeta")},
	})

	expansion, err := source.Expand(context.Background(), github.ExpansionRequest{
		Organizations: []string{"acme"},
		Exact:         set,
	})
	if err != nil {
		t.Fatalf("expansion failed: %v", err)
	}

	assertScopes(t, expansion, "Acme/API", "acme/zeta")
	report := selectorReport(t, expansion, "acme")
	if report.Retained != 2 || report.Omitted != 0 {
		t.Errorf("selector report is %+v, want both eligible repositories retained", report)
	}
	selection := expansion.Selection
	if selection.ExactScopes != 1 || selection.ExpandedScopes != 1 || selection.TotalScopes != 2 {
		t.Errorf("selection counts are %+v, want one exact and one expanded scope", selection)
	}
	duplicate := selection.Provenance[0]
	if !duplicate.Exact || duplicate.Selector != "acme" {
		t.Errorf("exact duplicate provenance is %+v, want an exact scope carrying acme provenance", duplicate)
	}
}

func TestExpansionOfARepositoryHeldOnlyByAPathScopeConsumesOneScopeSlot(t *testing.T) {
	matcher, err := domain.NewPathMatcher([]domain.MatcherToken{domain.LiteralToken("src")})
	if err != nil {
		t.Fatalf("building the matcher failed: %v", err)
	}
	repository, err := domain.ParseRepository("acme/api")
	if err != nil {
		t.Fatalf("building the repository failed: %v", err)
	}
	scope, err := domain.NewPathScope(repository, matcher)
	if err != nil {
		t.Fatalf("building the path scope failed: %v", err)
	}
	set, err := domain.NewScopeSet([]domain.Scope{scope})
	if err != nil {
		t.Fatalf("building the exact selection failed: %v", err)
	}
	source, _ := newListingSource(t, map[string]listingPage{
		orgPage("acme", 1): {body: sourceListing("acme/api")},
	})

	expansion, err := source.Expand(context.Background(), github.ExpansionRequest{
		Organizations: []string{"acme"},
		Exact:         set,
	})
	if err != nil {
		t.Fatalf("expansion failed: %v", err)
	}

	assertScopes(t, expansion, "acme/api:src", "acme/api")
	selection := expansion.Selection
	if selection.TotalScopes != 2 || selection.DistinctRepositories != 1 {
		t.Errorf("selection counts are %+v, want two scopes over one distinct repository", selection)
	}
}

// TestExpansionOmitsAReferencedRepositoryWithoutScopeCapacity closes A-064's
// combined case: the repository already appears in an exact path Scope, so it
// needs no distinct-repository slot, but the exhausted Scope capacity alone
// still records the omission and creates no repository Scope.
func TestExpansionOmitsAReferencedRepositoryWithoutScopeCapacity(t *testing.T) {
	set := mustPathScopes(t, 10, 10)
	source, _ := newListingSource(t, map[string]listingPage{
		orgPage("exact", 1): {body: sourceListing("exact/repo-00")},
	})

	expansion, err := source.Expand(context.Background(), github.ExpansionRequest{
		Organizations: []string{"exact"},
		Exact:         set,
	})
	if err != nil {
		t.Fatalf("expansion failed: %v", err)
	}

	if expansion.Scopes.Len() != set.Len() {
		t.Errorf("expansion published %d scopes, want the %d exact scopes unchanged", expansion.Scopes.Len(), set.Len())
	}
	selection := expansion.Selection
	if selection.ExpandedScopes != 0 || selection.DistinctRepositories != 10 {
		t.Errorf("selection counts are %+v, want no expanded scope over the ten exact repositories", selection)
	}
	report := selectorReport(t, expansion, "exact")
	if report.Eligible != 1 || report.Retained != 0 || report.Omitted != 1 {
		t.Errorf("selector report is %+v, want the eligible candidate omitted by scope capacity alone", report)
	}
}

func TestExpansionRespectsTheRemainingScopeCapacityAlone(t *testing.T) {
	tests := map[string]struct {
		repositories int
		patterns     int
		wantScopes   []string
		wantRetained int
		wantOmitted  int
	}{
		"one scope slot left": {repositories: 11, patterns: 9, wantScopes: []string{"acme/repo-1"}, wantRetained: 1, wantOmitted: 1},
		"no scope slot left":  {repositories: 10, patterns: 10, wantRetained: 0, wantOmitted: 2},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			set := mustPathScopes(t, test.repositories, test.patterns)
			source, _ := newListingSource(t, map[string]listingPage{
				orgPage("acme", 1): {body: sourceListing("acme/repo-1", "acme/repo-2")},
			})

			expansion, err := source.Expand(context.Background(), github.ExpansionRequest{
				Organizations: []string{"acme"},
				Exact:         set,
			})
			if err != nil {
				t.Fatalf("expansion failed: %v", err)
			}

			expanded := scopeStrings(expansion.Scopes.Scopes())[set.Len():]
			if strings.Join(expanded, ",") != strings.Join(test.wantScopes, ",") {
				t.Errorf("expansion admitted %v, want %v", expanded, test.wantScopes)
			}
			report := selectorReport(t, expansion, "acme")
			if report.Retained != test.wantRetained || report.Omitted != test.wantOmitted {
				t.Errorf("selector report is %+v, want %d retained and %d omitted", report, test.wantRetained, test.wantOmitted)
			}
			if expansion.Selection.TotalScopes > domain.MaxScopes {
				t.Errorf("expansion published %d scopes, want at most %d", expansion.Selection.TotalScopes, domain.MaxScopes)
			}
		})
	}
}

func TestExpansionFailureClassesStayDistinctAndSanitized(t *testing.T) {
	tests := map[string]struct {
		page            listingPage
		want            error
		wantRetry       time.Duration
		wantRateLimited bool
		wantStatus      string
	}{
		"unknown organization": {page: listingPage{status: http.StatusNotFound, body: `{}`}, want: github.ErrOrganizationNotFound, wantRetry: defaultInterval},
		"denied credential":    {page: listingPage{status: http.StatusUnauthorized, body: `{}`}, want: github.ErrAuthentication, wantRetry: defaultInterval},
		"denied access":        {page: listingPage{status: http.StatusForbidden, body: `{}`}, want: github.ErrOrganizationAccessDenied, wantRetry: defaultInterval},
		"rate limited": {
			page:            listingPage{status: http.StatusForbidden, body: `{}`, header: map[string]string{"X-RateLimit-Remaining": "0", "Retry-After": "120"}},
			want:            github.ErrRateLimited,
			wantRetry:       120 * time.Second,
			wantRateLimited: true,
		},
		"server failure": {page: listingPage{status: http.StatusBadGateway, body: `{}`}, want: github.ErrUnexpectedResponse, wantRetry: defaultInterval},
		"malformed body": {page: listingPage{body: `{"repositories":[]}`}, want: github.ErrInvalidListing, wantRetry: defaultInterval},
		"truncated body": {page: listingPage{body: `[{"full_name":`}, want: github.ErrInvalidListing, wantRetry: defaultInterval},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			failure := expandExpectingError(t, map[string]listingPage{orgPage("acme", 1): test.page}, "acme")

			if !errors.Is(failure, test.want) {
				t.Errorf("failure %v is not %v", failure, test.want)
			}
			if failure.Organization != "acme" {
				t.Errorf("failure names organization %q, want acme", failure.Organization)
			}
			if failure.RetryDelay != test.wantRetry {
				t.Errorf("failure retry delay is %s, want %s", failure.RetryDelay, test.wantRetry)
			}
			if failure.RateLimited != test.wantRateLimited {
				t.Errorf("failure reports rate limited %t, want %t", failure.RateLimited, test.wantRateLimited)
			}
			assertSanitized(t, failure.Error())
		})
	}
}

func TestExpansionCancellationSpendsItsBudgetAndPublishesNothing(t *testing.T) {
	source, client := newListingSource(t, map[string]listingPage{
		orgPage("acme", 1): {body: sourceListing("acme/api")},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	expansion, err := source.Expand(ctx, github.ExpansionRequest{Organizations: []string{"acme"}})
	if err == nil {
		t.Fatalf("cancelled expansion succeeded with %d scopes", expansion.Scopes.Len())
	}
	if !errors.Is(err, context.Canceled) || !errors.Is(err, github.ErrTransport) {
		t.Errorf("cancelled expansion failed with %v, want a cancelled transport failure", err)
	}
	if expansion.Scopes.Len() != 0 || len(expansion.Selection.Selectors) != 0 {
		t.Error("cancelled expansion published a partial selection")
	}
	if got := len(client.recorded()); got != 1 {
		t.Errorf("cancelled expansion dispatched %d requests, want the one whose budget it spent", got)
	}
	var failure *github.ExpansionError
	if !errors.As(err, &failure) {
		t.Fatalf("error %v is not a *github.ExpansionError", err)
	}
	if failure.Requests != 1 {
		t.Errorf("cancelled expansion reports %d spent requests, want the one it did not refund", failure.Requests)
	}
	assertSanitized(t, err.Error())
}

func TestExpansionWithoutEligibleRepositoriesSucceedsEmpty(t *testing.T) {
	expansion, _ := expandOrganizations(t, map[string]listingPage{
		orgPage("acme", 1): {body: `[]`},
	}, "acme")

	if expansion.Scopes.Len() != 0 {
		t.Errorf("empty expansion published %d scopes, want none", expansion.Scopes.Len())
	}
	report := selectorReport(t, expansion, "acme")
	if report.Listed != 0 || report.Eligible != 0 || report.Retained != 0 || report.Omitted != 0 {
		t.Errorf("selector report is %+v, want an explicit zero result", report)
	}
	if expansion.Selection.PaginationRemains {
		t.Error("an exhausted empty listing reports remaining pagination")
	}
}

func TestExpansionWithoutSelectorsKeepsTheExactSelectionAndSpendsNothing(t *testing.T) {
	set, err := domain.NewRepositoryScopeSet([]string{"acme/api"})
	if err != nil {
		t.Fatalf("building the exact selection failed: %v", err)
	}
	source, client := newListingSource(t, map[string]listingPage{})

	expansion, err := source.Expand(context.Background(), github.ExpansionRequest{Exact: set})
	if err != nil {
		t.Fatalf("expansion failed: %v", err)
	}

	assertScopes(t, expansion, "acme/api")
	if expansion.Requests != 0 || len(client.recorded()) != 0 {
		t.Errorf("an expansion without selectors spent %d requests, want none", expansion.Requests)
	}
	if expansion.Selection.ExactScopes != 1 || expansion.Selection.ExpandedScopes != 0 {
		t.Errorf("selection counts are %+v, want the exact selection unchanged", expansion.Selection)
	}
	if len(expansion.Selection.Provenance) != 1 || !expansion.Selection.Provenance[0].Exact {
		t.Errorf("provenance is %+v, want one exact entry", expansion.Selection.Provenance)
	}
}

func TestExpansionRejectsAnUnusableSelectorBeforeAnyRequest(t *testing.T) {
	tests := map[string][]string{
		"invalid organization":  {"-acme"},
		"repeated organization": {"acme", "ACME"},
	}

	for name, organizations := range tests {
		t.Run(name, func(t *testing.T) {
			source, client := newListingSource(t, map[string]listingPage{})

			expansion, err := source.Expand(context.Background(), github.ExpansionRequest{Organizations: organizations})
			if err == nil {
				t.Fatalf("expansion of %v succeeded with %d scopes, want an error", organizations, expansion.Scopes.Len())
			}
			if !errors.Is(err, github.ErrInvalidSelector) {
				t.Errorf("failure %v is not an invalid selector", err)
			}
			if len(client.recorded()) != 0 {
				t.Error("an unusable selector reached the network")
			}
		})
	}
}
