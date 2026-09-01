package github_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fmueller/orgtop/internal/domain"
)

// The compare and pull request halves of the RG-003 enrichment contract. The
// shared fixtures, helpers, and commit-evidence cases live in enrich_test.go.

// TestImmutableCompareCompleteness guards A-032: the documented 300-file cap is
// always incomplete while everything below it proves a complete set.
func TestImmutableCompareCompleteness(t *testing.T) {
	tests := []struct {
		name     string
		files    int
		complete bool
	}{
		{name: "an empty comparison is complete", files: 0, complete: true},
		{name: "299 files are complete", files: 299, complete: true},
		{name: "exactly 300 files are always incomplete", files: 300},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var server *httptest.Server
			responses := map[string]stubResponse{}
			server = enrichFixtureServer(t, responses)
			responses[comparePath(parentSHA, commitSHA)] = stubResponse{
				body: compareBody(server, parentSHA, commitSHA, fileRecords(test.files))}
			enricher, _ := newTestEnricher(t, server)

			outcome := changed(t, enricher, mustCompareEvidence(t, parentSHA, commitSHA))
			if outcome.IsComplete() != test.complete {
				t.Fatalf("outcome = %v (%s), complete = %t, want %t", outcome.Kind(), outcome.Reason(), outcome.IsComplete(), test.complete)
			}
			if !test.complete {
				if outcome.Kind() != domain.OutcomeIncomplete {
					t.Errorf("outcome = %v, want incomplete", outcome.Kind())
				}
				return
			}
			if got := len(outcome.Paths()); got != test.files {
				t.Errorf("paths = %d, want %d", got, test.files)
			}
			if outcome.Provenance() != domain.ProvenanceEventTime {
				t.Errorf("provenance = %v", outcome.Provenance())
			}
		})
	}
}

// TestCompareResponseIdentity guards the RG-003 identity proof for compare
// responses, including the URL invariants that keep evidence bound to its
// immutable request.
func TestCompareResponseIdentity(t *testing.T) {
	tests := []struct {
		name string
		body func(server *httptest.Server) string
	}{
		{
			name: "a mismatched base commit is incomplete",
			body: func(server *httptest.Server) string {
				return fmt.Sprintf(`{"url":"%s%s","base_commit":{"sha":%q},"files":[]}`,
					server.URL, comparePath(parentSHA, commitSHA), commitSHA)
			},
		},
		{
			name: "a JSON url for another repository is incomplete",
			body: func(server *httptest.Server) string {
				return fmt.Sprintf(`{"url":"%s/repos/Acme/Other/compare/%s...%s","base_commit":{"sha":%q},"files":[]}`,
					server.URL, parentSHA, commitSHA, parentSHA)
			},
		},
		{
			name: "a JSON url for another object pair is incomplete",
			body: func(server *httptest.Server) string {
				return fmt.Sprintf(`{"url":"%s%s","base_commit":{"sha":%q},"files":[]}`,
					server.URL, comparePath(commitSHA, parentSHA), parentSHA)
			},
		},
		{
			name: "a percent-encoded alternate path is incomplete",
			body: func(server *httptest.Server) string {
				return fmt.Sprintf(`{"url":"%s/repos/Acme/%%57eb/compare/%s...%s","base_commit":{"sha":%q},"files":[]}`,
					server.URL, parentSHA, commitSHA, parentSHA)
			},
		},
		{
			name: "a trailing slash is incomplete",
			body: func(server *httptest.Server) string {
				return fmt.Sprintf(`{"url":"%s%s/","base_commit":{"sha":%q},"files":[]}`,
					server.URL, comparePath(parentSHA, commitSHA), parentSHA)
			},
		},
		{
			name: "a query member is incomplete",
			body: func(server *httptest.Server) string {
				return fmt.Sprintf(`{"url":"%s%s?page=1","base_commit":{"sha":%q},"files":[]}`,
					server.URL, comparePath(parentSHA, commitSHA), parentSHA)
			},
		},
		{
			name: "embedded credentials are incomplete",
			body: func(server *httptest.Server) string {
				return fmt.Sprintf(`{"url":"%s","base_commit":{"sha":%q},"files":[]}`,
					strings.Replace(server.URL, "://", "://user:pass@", 1)+comparePath(parentSHA, commitSHA), parentSHA)
			},
		},
		{
			name: "a missing url is incomplete",
			body: func(*httptest.Server) string {
				return fmt.Sprintf(`{"base_commit":{"sha":%q},"files":[]}`, parentSHA)
			},
		},
		{
			name: "a wrong required type is incomplete",
			body: func(server *httptest.Server) string {
				return fmt.Sprintf(`{"url":"%s%s","base_commit":{"sha":7},"files":[]}`,
					server.URL, comparePath(parentSHA, commitSHA))
			},
		},
		{
			name: "invalid JSON is incomplete",
			body: func(*httptest.Server) string { return `{"url":` },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responses := map[string]stubResponse{}
			server := enrichFixtureServer(t, responses)
			responses[comparePath(parentSHA, commitSHA)] = stubResponse{body: test.body(server)}
			enricher, _ := newTestEnricher(t, server)

			outcome := changed(t, enricher, mustCompareEvidence(t, parentSHA, commitSHA))
			if outcome.Kind() != domain.OutcomeIncomplete {
				t.Fatalf("outcome = %v, want incomplete", outcome.Kind())
			}
			if len(outcome.Paths()) != 0 {
				t.Errorf("incomplete evidence leaked %d paths", len(outcome.Paths()))
			}
		})
	}
}

// TestCompareAcceptsCanonicalOwnerSpelling guards that only case differs: the
// returned owner spelling may differ in case from the requested one and still
// identifies the same repository.
func TestCompareAcceptsCanonicalOwnerSpelling(t *testing.T) {
	responses := map[string]stubResponse{}
	server := enrichFixtureServer(t, responses)
	responses[comparePath(parentSHA, commitSHA)] = stubResponse{
		body: fmt.Sprintf(`{"url":"%s/repos/acme/web/compare/%s...%s","base_commit":{"sha":%q},"files":[],"unknown":{"ignored":true}}`,
			server.URL, parentSHA, commitSHA, parentSHA)}
	enricher, _ := newTestEnricher(t, server)

	if outcome := changed(t, enricher, mustCompareEvidence(t, parentSHA, commitSHA)); !outcome.IsComplete() {
		t.Fatalf("outcome = %v (%s), want complete", outcome.Kind(), outcome.Reason())
	}
}

// TestCurrentPullRequestCapture guards A-033: one metadata read captures a pull
// request's current objects and yields the immutable compare that serves every
// comment on it, with current-PR provenance retained.
func TestCurrentPullRequestCapture(t *testing.T) {
	responses := map[string]stubResponse{}
	server := enrichFixtureServer(t, responses)
	responses["/repos/Acme/Web/pulls/12"] = stubResponse{body: fmt.Sprintf(
		`{"url":"%s/repos/Acme/Web/pulls/12","number":12,"base":{"sha":%q,"repo":{"full_name":"acme/web"}},"head":{"sha":%q}}`,
		server.URL, parentSHA, commitSHA)}
	responses[comparePath(parentSHA, commitSHA)] = stubResponse{
		body: compareBody(server, parentSHA, commitSHA, `{"filename":"a.go","status":"modified"}`)}
	enricher, client := newTestEnricher(t, server)

	derived := enricher.CurrentPullRequest(context.Background(), mustPullRequestEvidence(t, 12))
	if derived.Operation() != domain.EvidenceCompare {
		settled, _ := derived.Settled()
		t.Fatalf("derived operation = %v (%s), want compare", derived.Operation(), settled.Reason())
	}
	if want := "compare(acme/web," + parentSHA + "," + commitSHA + ")"; derived.Key() != want {
		t.Errorf("derived key = %q, want %q", derived.Key(), want)
	}
	if derived.Provenance() != domain.ProvenanceCurrentPR {
		t.Errorf("derived provenance = %v, want current-pr", derived.Provenance())
	}

	outcome := changed(t, enricher, derived)
	if !outcome.IsComplete() {
		t.Fatalf("outcome = %v (%s), want complete", outcome.Kind(), outcome.Reason())
	}
	if outcome.Provenance() != domain.ProvenanceCurrentPR {
		t.Errorf("outcome provenance = %v, want current-pr", outcome.Provenance())
	}
	if got := len(client.recorded()); got != 2 {
		t.Errorf("dispatched %d requests, want one metadata read and one compare", got)
	}
}

// TestCurrentPullRequestIdentity guards the metadata identity proof and the
// unchanged pull request whose objects are equal.
func TestCurrentPullRequestIdentity(t *testing.T) {
	tests := []struct {
		name string
		body func(server *httptest.Server) string
		kind domain.OutcomeKind
	}{
		{
			name: "a mismatched number is incomplete",
			body: func(server *httptest.Server) string {
				return fmt.Sprintf(`{"url":"%s/repos/Acme/Web/pulls/12","number":13,"base":{"sha":%q,"repo":{"full_name":"acme/web"}},"head":{"sha":%q}}`,
					server.URL, parentSHA, commitSHA)
			},
			kind: domain.OutcomeIncomplete,
		},
		{
			name: "a base repository mismatch is incomplete",
			body: func(server *httptest.Server) string {
				return fmt.Sprintf(`{"url":"%s/repos/Acme/Web/pulls/12","number":12,"base":{"sha":%q,"repo":{"full_name":"acme/other"}},"head":{"sha":%q}}`,
					server.URL, parentSHA, commitSHA)
			},
			kind: domain.OutcomeIncomplete,
		},
		{
			name: "an invalid object is incomplete",
			body: func(server *httptest.Server) string {
				return fmt.Sprintf(`{"url":"%s/repos/Acme/Web/pulls/12","number":12,"base":{"sha":"nope","repo":{"full_name":"acme/web"}},"head":{"sha":%q}}`,
					server.URL, commitSHA)
			},
			kind: domain.OutcomeIncomplete,
		},
		{
			name: "a JSON url for another pull request is incomplete",
			body: func(server *httptest.Server) string {
				return fmt.Sprintf(`{"url":"%s/repos/Acme/Web/pulls/13","number":12,"base":{"sha":%q,"repo":{"full_name":"acme/web"}},"head":{"sha":%q}}`,
					server.URL, parentSHA, commitSHA)
			},
			kind: domain.OutcomeIncomplete,
		},
		{
			name: "equal current objects are a complete empty comparison",
			body: func(server *httptest.Server) string {
				return fmt.Sprintf(`{"url":"%s/repos/Acme/Web/pulls/12","number":12,"base":{"sha":%q,"repo":{"full_name":"acme/web"}},"head":{"sha":%q}}`,
					server.URL, commitSHA, commitSHA)
			},
			kind: domain.OutcomeComplete,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responses := map[string]stubResponse{}
			server := enrichFixtureServer(t, responses)
			responses["/repos/Acme/Web/pulls/12"] = stubResponse{body: test.body(server)}
			enricher, _ := newTestEnricher(t, server)

			derived := enricher.CurrentPullRequest(context.Background(), mustPullRequestEvidence(t, 12))
			settled, isSettled := derived.Settled()
			if !isSettled {
				t.Fatalf("derived operation = %v, want a settled outcome", derived.Operation())
			}
			if settled.Kind() != test.kind {
				t.Fatalf("outcome = %v, want %v", settled.Kind(), test.kind)
			}
			if test.kind == domain.OutcomeComplete {
				if len(settled.Paths()) != 0 {
					t.Errorf("an unchanged pull request reported %d paths", len(settled.Paths()))
				}
				if settled.Provenance() != domain.ProvenanceCurrentPR {
					t.Errorf("provenance = %v, want current-pr", settled.Provenance())
				}
			}
		})
	}
}

// TestChangedRejectsMetadataEvidence guards that a metadata descriptor is never
// mistaken for changed-file evidence.
func TestChangedRejectsMetadataEvidence(t *testing.T) {
	server := enrichFixtureServer(t, map[string]stubResponse{})
	enricher, client := newTestEnricher(t, server)

	if outcome := changed(t, enricher, mustPullRequestEvidence(t, 12)); outcome.Kind() != domain.OutcomeIncomplete {
		t.Errorf("outcome = %v, want incomplete", outcome.Kind())
	}
	if len(client.recorded()) != 0 {
		t.Error("a metadata descriptor dispatched a changed-file request")
	}
}
