package github_test

import (
	"context"
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

// enrichmentPhaseBudget stands in for the closed 60-second enrichment phase
// deadline. The 30-second request budget must always fall strictly inside it.
const enrichmentPhaseBudget = 60 * time.Second

const (
	enrichOwner = "Acme/Web"
	commitSHA   = "2222222222222222222222222222222222222222"
	parentSHA   = "1111111111111111111111111111111111111111"
)

// enrichFixtureServer keys canned responses by request URI so a paginated commit
// walk can serve a different page per query.
func enrichFixtureServer(t *testing.T, responses map[string]stubResponse) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		response, known := responses[request.URL.RequestURI()]
		if !known {
			t.Errorf("unexpected request URI %q", request.URL.RequestURI())
			writer.WriteHeader(http.StatusNotImplemented)
			return
		}
		for key, value := range response.header {
			writer.Header().Set(key, value)
		}
		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		writer.WriteHeader(status)
		_, _ = io.WriteString(writer, response.body)
	}))
	t.Cleanup(server.Close)
	return server
}

func newTestEnricher(t *testing.T, server *httptest.Server) (github.Enricher, *recordingClient) {
	t.Helper()
	// The injected client mirrors the adapter's own redirect policy, so a moved
	// entity cannot silently change evidence identity in a fixture either.
	inner := server.Client()
	inner.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client := &recordingClient{inner: inner}
	return github.Enricher{
		Client:     client,
		BaseURL:    server.URL,
		Credential: sentinelCredential(t),
		Now:        func() time.Time { return testNow },
	}, client
}

func mustCommitEvidence(t *testing.T, head string) domain.EvidenceDescriptor {
	t.Helper()
	descriptor, err := domain.NewCommitEvidence(mustParseRepository(t, enrichOwner), head)
	if err != nil {
		t.Fatalf("building commit evidence failed: %v", err)
	}
	return descriptor
}

func mustCompareEvidence(t *testing.T, base, head string) domain.EvidenceDescriptor {
	t.Helper()
	descriptor, err := domain.NewCompareEvidence(mustParseRepository(t, enrichOwner), base, head, domain.ProvenanceEventTime)
	if err != nil {
		t.Fatalf("building compare evidence failed: %v", err)
	}
	return descriptor
}

func mustPullRequestEvidence(t *testing.T, number int) domain.EvidenceDescriptor {
	t.Helper()
	descriptor, err := domain.NewPullRequestEvidence(mustParseRepository(t, enrichOwner), number)
	if err != nil {
		t.Fatalf("building pull request evidence failed: %v", err)
	}
	return descriptor
}

// fileRecords renders count file records with distinct valid paths, starting at
// the given offset so several pages of one commit stay free of duplicates.
func fileRecords(count int) string { return fileRecordsFrom(0, count) }

func fileRecordsFrom(offset, count int) string {
	records := make([]string, 0, count)
	for index := offset; index < offset+count; index++ {
		records = append(records, fmt.Sprintf(`{"filename":"src/pkg%d/file.go","status":"modified"}`, index))
	}
	return strings.Join(records, ",")
}

func commitPage(files string) string {
	return fmt.Sprintf(`{"sha":%q,"parents":[{"sha":%q}],"files":[%s]}`, commitSHA, parentSHA, files)
}

func commitPath(page int) string {
	if page <= 1 {
		return "/repos/Acme/Web/commits/" + commitSHA + "?per_page=100"
	}
	return "/repos/Acme/Web/commits/" + commitSHA + "?page=" + strconv.Itoa(page) + "&per_page=100"
}

func nextLink(server *httptest.Server, page int) map[string]string {
	return map[string]string{"Link": fmt.Sprintf(`<%s%s>; rel="next"`, server.URL, commitPath(page))}
}

func comparePath(base, head string) string {
	return "/repos/Acme/Web/compare/" + base + "..." + head
}

func compareBody(server *httptest.Server, base, head, files string) string {
	return fmt.Sprintf(`{"url":"%s%s","base_commit":{"sha":%q},"files":[%s]}`,
		server.URL, comparePath(base, head), base, files)
}

func changed(t *testing.T, enricher github.Enricher, descriptor domain.EvidenceDescriptor) domain.EvidenceOutcome {
	t.Helper()
	return enricher.Changed(context.Background(), descriptor)
}

func pathStrings(outcome domain.EvidenceOutcome) []string {
	values := make([]string, 0, len(outcome.Paths()))
	for _, path := range outcome.Paths() {
		values = append(values, path.String())
	}
	return values
}

// TestEnrichmentRequestContract guards the RG-003 request contract every
// enrichment endpoint shares: GET, the exact endpoint, the upgraded API version,
// the safe user agent, and a bounded request context.
func TestEnrichmentRequestContract(t *testing.T) {
	server := enrichFixtureServer(t, map[string]stubResponse{
		commitPath(1):                     {body: commitPage(fileRecords(1))},
		comparePath(parentSHA, commitSHA): {body: ""},
		"/repos/Acme/Web/pulls/12":        {body: ""},
	})
	enricher, client := newTestEnricher(t, server)

	// The phase context stands in for the 60-second enrichment deadline, so each
	// request deadline must fall strictly inside it (A-035).
	phaseCtx, cancelPhase := context.WithTimeout(t.Context(), enrichmentPhaseBudget)
	defer cancelPhase()
	phaseDeadline, hasPhaseDeadline := phaseCtx.Deadline()
	if !hasPhaseDeadline {
		t.Fatal("the phase context carried no deadline")
	}
	enricher.Changed(phaseCtx, mustCommitEvidence(t, commitSHA))
	enricher.Changed(phaseCtx, mustCompareEvidence(t, parentSHA, commitSHA))
	enricher.CurrentPullRequest(phaseCtx, mustPullRequestEvidence(t, 12))

	recorded := client.recorded()
	if len(recorded) != 3 {
		t.Fatalf("dispatched %d requests, want 3", len(recorded))
	}
	wantPaths := []string{
		"/repos/Acme/Web/commits/" + commitSHA,
		comparePath(parentSHA, commitSHA),
		"/repos/Acme/Web/pulls/12",
	}
	for index, request := range recorded {
		if request.method != http.MethodGet {
			t.Errorf("request %d method = %s, want GET", index, request.method)
		}
		if request.path != wantPaths[index] {
			t.Errorf("request %d path = %q, want %q", index, request.path, wantPaths[index])
		}
		if got := request.header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("request %d Accept = %q", index, got)
		}
		if got := request.header.Get("X-GitHub-Api-Version"); got != "2026-03-10" {
			t.Errorf("request %d API version = %q, want 2026-03-10", index, got)
		}
		if got := request.header.Get("User-Agent"); !strings.HasPrefix(got, "orgtop/") {
			t.Errorf("request %d User-Agent = %q", index, got)
		}
		if !request.hasDeadline {
			t.Errorf("request %d has no deadline", index)
		} else if !request.deadline.Before(phaseDeadline) {
			t.Errorf("request %d deadline %v is not inside the enrichment phase deadline %v",
				index, request.deadline, phaseDeadline)
		}
	}
	if got := recorded[0].query.Get("per_page"); got != "100" {
		t.Errorf("commit per_page = %q, want 100", got)
	}
	for _, index := range []int{1, 2} {
		if len(recorded[index].query) != 0 {
			t.Errorf("request %d carries query %v, want none", index, recorded[index].query)
		}
	}
}

// TestSettledEvidenceNeedsNoRequest guards that evidence already terminal at
// classification never reaches GitHub.
func TestSettledEvidenceNeedsNoRequest(t *testing.T) {
	server := enrichFixtureServer(t, map[string]stubResponse{})
	enricher, client := newTestEnricher(t, server)

	path, err := domain.NewChangedPath("a/b.go")
	if err != nil {
		t.Fatalf("building a changed path failed: %v", err)
	}
	for _, descriptor := range []domain.EvidenceDescriptor{
		domain.NewDirectEvidence(path),
		domain.NewUnchangedEvidence(),
		domain.NewUnsupportedEvidence("issue-only activity"),
		domain.NewIncompleteEvidence("malformed comment path"),
	} {
		settled, _ := descriptor.Settled()
		if got := changed(t, enricher, descriptor); got.Kind() != settled.Kind() {
			t.Errorf("settled %v became %v", settled.Kind(), got.Kind())
		}
	}
	if len(client.recorded()) != 0 {
		t.Errorf("settled evidence dispatched %d requests", len(client.recorded()))
	}
}

// TestCompletePaginatedCommitEvidence guards A-031: identical commit identity
// across every page, same-host next links, and one complete event-time set whose
// verified sole parent decides per-event applicability.
func TestCompletePaginatedCommitEvidence(t *testing.T) {
	var server *httptest.Server
	responses := map[string]stubResponse{}
	server = enrichFixtureServer(t, responses)
	responses[commitPath(1)] = stubResponse{body: commitPage(fileRecordsFrom(0, 100)), header: nextLink(server, 2)}
	responses[commitPath(2)] = stubResponse{body: commitPage(fileRecordsFrom(100, 100)), header: nextLink(server, 3)}
	responses[commitPath(3)] = stubResponse{body: commitPage(fileRecordsFrom(200, 50))}
	enricher, client := newTestEnricher(t, server)

	outcome := changed(t, enricher, mustCommitEvidence(t, commitSHA))
	if !outcome.IsComplete() {
		t.Fatalf("outcome = %v (%s), want complete", outcome.Kind(), outcome.Reason())
	}
	if got := len(outcome.Paths()); got != 250 {
		t.Errorf("paths = %d, want 250 unique across three pages", got)
	}
	if outcome.Provenance() != domain.ProvenanceEventTime {
		t.Errorf("provenance = %v", outcome.Provenance())
	}
	if outcome.SoleParent() != parentSHA {
		t.Errorf("sole parent = %q, want %q", outcome.SoleParent(), parentSHA)
	}
	if got := len(client.recorded()); got != 3 {
		t.Errorf("dispatched %d requests, want 3", got)
	}
	if outcome.ForSoleParent(parentSHA).Kind() != domain.OutcomeComplete {
		t.Error("the event whose before object is the verified sole parent must be complete")
	}
	if outcome.ForSoleParent(commitSHA).Kind() != domain.OutcomeIncomplete {
		t.Error("an event sharing the head with a different before object must be incomplete")
	}
}

// TestCommitCompletenessProofFailures guards the RG-003 commit proof: every page
// repeats one identity, pagination advances safely, and any violation discards
// every path already collected.
func TestCommitCompletenessProofFailures(t *testing.T) {
	otherSHA := "3333333333333333333333333333333333333333"
	tests := []struct {
		name  string
		pages func(server *httptest.Server) map[string]stubResponse
	}{
		{
			name: "a cyclic next link never repeats a page",
			pages: func(server *httptest.Server) map[string]stubResponse {
				return map[string]stubResponse{
					commitPath(1): {body: commitPage(fileRecords(1)), header: map[string]string{
						"Link": fmt.Sprintf(`<%s%s>; rel="next"`, server.URL, commitPath(1))}},
				}
			},
		},
		{
			name: "a changed parent list fails parent verification",
			pages: func(server *httptest.Server) map[string]stubResponse {
				return map[string]stubResponse{
					commitPath(1): {body: commitPage(fileRecords(1)), header: nextLink(server, 2)},
					commitPath(2): {body: fmt.Sprintf(`{"sha":%q,"parents":[{"sha":%q}],"files":[]}`, commitSHA, otherSHA)},
				}
			},
		},
		{
			name: "a changed commit identity is not mixed across pages",
			pages: func(server *httptest.Server) map[string]stubResponse {
				return map[string]stubResponse{
					commitPath(1): {body: commitPage(fileRecords(1)), header: nextLink(server, 2)},
					commitPath(2): {body: fmt.Sprintf(`{"sha":%q,"parents":[{"sha":%q}],"files":[]}`, otherSHA, parentSHA)},
				}
			},
		},
		{
			name: "a cross-host next link is not followed",
			pages: func(*httptest.Server) map[string]stubResponse {
				return map[string]stubResponse{
					commitPath(1): {body: commitPage(fileRecords(1)), header: map[string]string{
						"Link": `<https://evil.example.com` + commitPath(2) + `>; rel="next"`}},
				}
			},
		},
		{
			name: "a cross-repository next link is not followed",
			pages: func(server *httptest.Server) map[string]stubResponse {
				return map[string]stubResponse{
					commitPath(1): {body: commitPage(fileRecords(1)), header: map[string]string{
						"Link": fmt.Sprintf(`<%s/repos/Acme/Other/commits/%s?page=2&per_page=100>; rel="next"`, server.URL, commitSHA)}},
				}
			},
		},
		{
			name: "a next link for another commit is not followed",
			pages: func(server *httptest.Server) map[string]stubResponse {
				return map[string]stubResponse{
					commitPath(1): {body: commitPage(fileRecords(1)), header: map[string]string{
						"Link": fmt.Sprintf(`<%s/repos/Acme/Web/commits/%s?page=2&per_page=100>; rel="next"`, server.URL, otherSHA)}},
				}
			},
		},
		{
			name: "an unexpected query member is not followed",
			pages: func(server *httptest.Server) map[string]stubResponse {
				return map[string]stubResponse{
					commitPath(1): {body: commitPage(fileRecords(1)), header: map[string]string{
						"Link": fmt.Sprintf(`<%s/repos/Acme/Web/commits/%s?page=2&per_page=100&since=2020>; rel="next"`, server.URL, commitSHA)}},
				}
			},
		},
		{
			name: "a non-advancing next link is not followed",
			pages: func(server *httptest.Server) map[string]stubResponse {
				return map[string]stubResponse{
					commitPath(2): {body: commitPage(fileRecords(1))},
					commitPath(1): {body: commitPage(fileRecords(1)), header: map[string]string{
						"Link": fmt.Sprintf(`<%s/repos/Acme/Web/commits/%s?page=0&per_page=100>; rel="next"`, server.URL, commitSHA)}},
				}
			},
		},
		{
			name: "a malformed next link is incomplete",
			pages: func(*httptest.Server) map[string]stubResponse {
				return map[string]stubResponse{
					commitPath(1): {body: commitPage(fileRecords(1)), header: map[string]string{"Link": `<::not a url>; rel="next"`}},
				}
			},
		},
		{
			name: "an eleventh page exceeds the closed page limit",
			pages: func(server *httptest.Server) map[string]stubResponse {
				pages := map[string]stubResponse{}
				for page := 1; page <= 11; page++ {
					pages[commitPath(page)] = stubResponse{body: commitPage(fileRecordsFrom(page, 1)), header: nextLink(server, page+1)}
				}
				return pages
			},
		},
		{
			name: "more than one parent proves no sole parent",
			pages: func(*httptest.Server) map[string]stubResponse {
				return map[string]stubResponse{
					commitPath(1): {body: fmt.Sprintf(`{"sha":%q,"parents":[{"sha":%q},{"sha":%q}],"files":[]}`,
						commitSHA, parentSHA, otherSHA)},
				}
			},
		},
		{
			name: "no parent proves no sole parent",
			pages: func(*httptest.Server) map[string]stubResponse {
				return map[string]stubResponse{
					commitPath(1): {body: fmt.Sprintf(`{"sha":%q,"parents":[],"files":[]}`, commitSHA)},
				}
			},
		},
		{
			name: "a response for another commit is not admitted",
			pages: func(*httptest.Server) map[string]stubResponse {
				return map[string]stubResponse{
					commitPath(1): {body: fmt.Sprintf(`{"sha":%q,"parents":[{"sha":%q}],"files":[]}`, otherSHA, parentSHA)},
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responses := map[string]stubResponse{}
			server := enrichFixtureServer(t, responses)
			for uri, response := range test.pages(server) {
				responses[uri] = response
			}
			enricher, _ := newTestEnricher(t, server)

			outcome := changed(t, enricher, mustCommitEvidence(t, commitSHA))
			if outcome.Kind() != domain.OutcomeIncomplete {
				t.Fatalf("outcome = %v, want incomplete", outcome.Kind())
			}
			if len(outcome.Paths()) != 0 {
				t.Errorf("incomplete evidence leaked %d paths", len(outcome.Paths()))
			}
		})
	}
}

// TestCommitEvidenceStopsAtThePageBound guards that ten pages are admitted while
// a next link at the bound still proves incompleteness.
func TestCommitEvidenceStopsAtThePageBound(t *testing.T) {
	responses := map[string]stubResponse{}
	server := enrichFixtureServer(t, responses)
	for page := 1; page < 10; page++ {
		responses[commitPath(page)] = stubResponse{body: commitPage(fileRecordsFrom(page, 1)), header: nextLink(server, page+1)}
	}
	responses[commitPath(10)] = stubResponse{body: commitPage(fileRecordsFrom(10, 1))}
	enricher, client := newTestEnricher(t, server)

	if outcome := changed(t, enricher, mustCommitEvidence(t, commitSHA)); !outcome.IsComplete() {
		t.Fatalf("ten pages ending without a next link = %v, want complete", outcome.Kind())
	}
	if got := len(client.recorded()); got != 10 {
		t.Errorf("dispatched %d requests, want 10", got)
	}
}

// TestChangedFileRecordNormalization guards the RG-003 file-record rules and the
// RG-002 normalization every admitted path passes.
func TestChangedFileRecordNormalization(t *testing.T) {
	tests := []struct {
		name     string
		files    string
		complete bool
		paths    []string
	}{
		{
			name:     "a rename contributes both normalized names",
			files:    `{"filename":"b/new.go","status":"renamed","previous_filename":"a/old.go"}`,
			complete: true,
			paths:    []string{"b/new.go", "a/old.go"},
		},
		{
			name:     "a copy contributes its filename",
			files:    `{"filename":"b/new.go","status":"copied"}`,
			complete: true,
			paths:    []string{"b/new.go"},
		},
		{
			name:     "every documented status is admitted",
			files:    `{"filename":"a.go","status":"added"},{"filename":"b.go","status":"removed"},{"filename":"c.go","status":"modified"},{"filename":"d.go","status":"changed"},{"filename":"e.go","status":"unchanged"}`,
			complete: true,
			paths:    []string{"a.go", "b.go", "c.go", "d.go", "e.go"},
		},
		{name: "an unknown status is incomplete", files: `{"filename":"a.go","status":"exploded"}`},
		{name: "a rename without its old name is incomplete", files: `{"filename":"b/new.go","status":"renamed"}`},
		{name: "a rename with a malformed old name is incomplete", files: `{"filename":"b/new.go","status":"renamed","previous_filename":"a//old.go"}`},
		{name: "a malformed path is incomplete", files: `{"filename":"a//b.go","status":"modified"}`},
		{name: "an empty path is incomplete", files: `{"filename":"","status":"modified"}`},
		{name: "a traversal segment is incomplete", files: `{"filename":"a/../b.go","status":"modified"}`},
		{name: "a duplicate file record is incomplete", files: `{"filename":"a.go","status":"modified"},{"filename":"a.go","status":"added"}`},
		{name: "a missing status is incomplete", files: `{"filename":"a.go"}`},
		{name: "a wrong filename type is incomplete", files: `{"filename":7,"status":"modified"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responses := map[string]stubResponse{}
			server := enrichFixtureServer(t, responses)
			responses[comparePath(parentSHA, commitSHA)] = stubResponse{
				body: compareBody(server, parentSHA, commitSHA, test.files)}
			enricher, _ := newTestEnricher(t, server)

			outcome := changed(t, enricher, mustCompareEvidence(t, parentSHA, commitSHA))
			if outcome.IsComplete() != test.complete {
				t.Fatalf("outcome = %v (%s), want complete = %t", outcome.Kind(), outcome.Reason(), test.complete)
			}
			if !test.complete {
				if outcome.Kind() != domain.OutcomeIncomplete {
					t.Errorf("outcome = %v, want incomplete", outcome.Kind())
				}
				return
			}
			if got := pathStrings(outcome); fmt.Sprint(got) != fmt.Sprint(test.paths) {
				t.Errorf("paths = %v, want %v", got, test.paths)
			}
		})
	}
}

// bulkyFileRecords renders records that stay inside the path-count and
// per-path-byte limits while together exceeding the bytes one evidence identity
// may hold, so only the aggregate budget can reject them.
func bulkyFileRecords() string {
	const perPath = domain.MaxChangedPathBytes / 2
	count := domain.MaxEvidenceBytes/perPath + 1
	records := make([]string, 0, count)
	for index := range count {
		name := fmt.Sprintf("%d", index)
		records = append(records, fmt.Sprintf(`{"filename":"src/%s%s.go","status":"modified"}`,
			strings.Repeat("a", perPath-len(name)-len("src/.go")), name))
	}
	return strings.Join(records, ",")
}

// TestEvidenceCapacityBounds guards the closed per-entity RG-009 capacities: a
// result that exceeds one of them is incomplete rather than truncated.
func TestEvidenceCapacityBounds(t *testing.T) {
	longPath := "a/" + strings.Repeat("b", domain.MaxChangedPathBytes)
	tests := []struct {
		name  string
		files string
	}{
		{name: "more than the per-entity path limit", files: fileRecords(domain.MaxEvidencePaths + 1)},
		{name: "a path longer than the byte limit", files: fmt.Sprintf(`{"filename":%q,"status":"modified"}`, longPath)},
		{name: "more bytes than one evidence identity may hold", files: bulkyFileRecords()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responses := map[string]stubResponse{}
			server := enrichFixtureServer(t, responses)
			responses[commitPath(1)] = stubResponse{
				body: fmt.Sprintf(`{"sha":%q,"parents":[{"sha":%q}],"files":[%s]}`, commitSHA, parentSHA, test.files)}
			enricher, _ := newTestEnricher(t, server)

			outcome := changed(t, enricher, mustCommitEvidence(t, commitSHA))
			if outcome.Kind() != domain.OutcomeIncomplete {
				t.Fatalf("outcome = %v, want incomplete", outcome.Kind())
			}
			if len(outcome.Paths()) != 0 {
				t.Errorf("an over-bound result leaked %d paths", len(outcome.Paths()))
			}
		})
	}
}
