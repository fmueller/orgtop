package github_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
	"github.com/fmueller/orgtop/internal/github"
)

// TestEnrichmentStatusMapping guards the closed RG-003 status classification.
// Every unknown outcome stays distinguishable so a later refresh can recover.
func TestEnrichmentStatusMapping(t *testing.T) {
	tests := []struct {
		name     string
		response stubResponse
		kind     domain.OutcomeKind
	}{
		{name: "401 is denied", response: stubResponse{status: http.StatusUnauthorized}, kind: domain.OutcomeDenied},
		{
			name:     "403 without rate evidence is denied",
			response: stubResponse{status: http.StatusForbidden},
			kind:     domain.OutcomeDenied,
		},
		{name: "404 is unavailable", response: stubResponse{status: http.StatusNotFound}, kind: domain.OutcomeUnavailable},
		{name: "409 is unavailable", response: stubResponse{status: http.StatusConflict}, kind: domain.OutcomeUnavailable},
		{name: "422 is failed", response: stubResponse{status: http.StatusUnprocessableEntity}, kind: domain.OutcomeFailed},
		{name: "400 is failed", response: stubResponse{status: http.StatusBadRequest}, kind: domain.OutcomeFailed},
		{name: "406 is failed", response: stubResponse{status: http.StatusNotAcceptable}, kind: domain.OutcomeFailed},
		{name: "an unsolicited 304 is failed", response: stubResponse{status: http.StatusNotModified}, kind: domain.OutcomeFailed},
		{name: "500 is failed", response: stubResponse{status: http.StatusInternalServerError}, kind: domain.OutcomeFailed},
		{name: "502 is failed", response: stubResponse{status: http.StatusBadGateway}, kind: domain.OutcomeFailed},
		{
			name: "a redirect is failed and never followed",
			response: stubResponse{status: http.StatusMovedPermanently,
				header: map[string]string{"Location": comparePath(commitSHA, parentSHA)}},
			kind: domain.OutcomeFailed,
		},
		{
			name: "403 with an exhausted rate limit is rate limited",
			response: stubResponse{status: http.StatusForbidden,
				header: map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": "1755864000"}},
			kind: domain.OutcomeRateLimited,
		},
		{
			name: "429 with Retry-After is rate limited",
			response: stubResponse{status: http.StatusTooManyRequests,
				header: map[string]string{"Retry-After": "120"}},
			kind: domain.OutcomeRateLimited,
		},
		{
			name:     "429 without a usable time is rate limited",
			response: stubResponse{status: http.StatusTooManyRequests},
			kind:     domain.OutcomeRateLimited,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responses := map[string]stubResponse{comparePath(parentSHA, commitSHA): test.response}
			server := enrichFixtureServer(t, responses)
			enricher, client := newTestEnricher(t, server)

			outcome := changed(t, enricher, mustCompareEvidence(t, parentSHA, commitSHA))
			if outcome.Kind() != test.kind {
				t.Fatalf("outcome = %v, want %v", outcome.Kind(), test.kind)
			}
			if len(outcome.Paths()) != 0 {
				t.Errorf("%v carried %d paths", outcome.Kind(), len(outcome.Paths()))
			}
			if got := len(client.recorded()); got != 1 {
				t.Errorf("dispatched %d requests, want exactly one attempt without a same-refresh retry", got)
			}
		})
	}
}

// TestRateLimitedRetryTiming guards that the latest instructed time wins and that
// a rate stop without a usable time still delays by the polling floor.
func TestRateLimitedRetryTiming(t *testing.T) {
	tests := []struct {
		name    string
		header  map[string]string
		retryAt time.Time
	}{
		{
			name:    "a bare rate stop uses the 60-second floor",
			header:  map[string]string{},
			retryAt: testNow.Add(defaultInterval),
		},
		{
			name:    "Retry-After beyond the floor wins",
			header:  map[string]string{"Retry-After": "180"},
			retryAt: testNow.Add(3 * time.Minute),
		},
		{
			name: "the latest of Retry-After and the reset instant wins",
			header: map[string]string{
				"Retry-After":           "120",
				"X-RateLimit-Remaining": "0",
				"X-RateLimit-Reset":     fmt.Sprint(testNow.Add(10 * time.Minute).Unix()),
			},
			retryAt: testNow.Add(10 * time.Minute),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responses := map[string]stubResponse{comparePath(parentSHA, commitSHA): {
				status: http.StatusTooManyRequests, header: test.header}}
			server := enrichFixtureServer(t, responses)
			enricher, _ := newTestEnricher(t, server)

			outcome := changed(t, enricher, mustCompareEvidence(t, parentSHA, commitSHA))
			if outcome.Kind() != domain.OutcomeRateLimited {
				t.Fatalf("outcome = %v, want rate-limited", outcome.Kind())
			}
			if !outcome.RetryAt().Equal(test.retryAt) {
				t.Errorf("retry at = %v, want %v", outcome.RetryAt(), test.retryAt)
			}
		})
	}
}

// TestEnrichmentCancellation guards that canceled work publishes no partial
// evidence and is never reported as a timeout failure.
func TestEnrichmentCancellation(t *testing.T) {
	responses := map[string]stubResponse{comparePath(parentSHA, commitSHA): {
		body: `{"base_commit":{"sha":"` + parentSHA + `"},"files":[]}`}}
	server := enrichFixtureServer(t, responses)
	enricher, client := newTestEnricher(t, server)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outcome := enricher.Changed(ctx, mustCompareEvidence(t, parentSHA, commitSHA))
	if outcome.Kind() != domain.OutcomeCanceled {
		t.Fatalf("outcome = %v, want canceled", outcome.Kind())
	}
	if len(outcome.Paths()) != 0 {
		t.Errorf("canceled work published %d paths", len(outcome.Paths()))
	}
	if got := len(client.recorded()); got > 1 {
		t.Errorf("canceled work dispatched %d requests", got)
	}
}

// stubEnrichmentClient fails every request with a fixed transport error.
type stubEnrichmentClient struct{ err error }

func (c stubEnrichmentClient) Do(*http.Request) (*http.Response, error) { return nil, c.err }

// TestTransportFailureIsFailed guards that a request that never produced a
// response is a failed outcome rather than an empty change set.
func TestTransportFailureIsFailed(t *testing.T) {
	enricher := github.Enricher{
		Client:     stubEnrichmentClient{err: errors.New("dial tcp: connection refused")},
		BaseURL:    "https://api.github.com",
		Credential: sentinelCredential(t),
		Now:        func() time.Time { return testNow },
	}
	outcome := enricher.Changed(context.Background(), mustCompareEvidence(t, parentSHA, commitSHA))
	if outcome.Kind() != domain.OutcomeFailed {
		t.Fatalf("outcome = %v, want failed", outcome.Kind())
	}
}

// TestDefaultEnrichmentClientDoesNotFollowRedirects guards that the adapter's
// own HTTP client, not only an injected fixture client, refuses to follow a
// redirect for these operations.
func TestDefaultEnrichmentClientDoesNotFollowRedirects(t *testing.T) {
	var visited []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		visited = append(visited, request.URL.Path)
		if request.URL.Path == comparePath(parentSHA, commitSHA) {
			writer.Header().Set("Location", comparePath(commitSHA, parentSHA))
			writer.WriteHeader(http.StatusMovedPermanently)
			return
		}
		_, _ = io.WriteString(writer, `{"base_commit":{"sha":"`+parentSHA+`"},"files":[]}`)
	}))
	t.Cleanup(server.Close)

	enricher := github.Enricher{
		BaseURL:    server.URL,
		Credential: sentinelCredential(t),
		Now:        func() time.Time { return testNow },
	}
	if outcome := enricher.Changed(context.Background(), mustCompareEvidence(t, parentSHA, commitSHA)); outcome.Kind() != domain.OutcomeFailed {
		t.Fatalf("outcome = %v, want failed", outcome.Kind())
	}
	if len(visited) != 1 {
		t.Errorf("visited %v, want only the requested comparison", visited)
	}
}

// TestEnrichmentRejectsForeignResponseURLs guards that the JSON url invariant is
// evaluated against the configured API root, so a public-API response naming
// another host is never admitted.
func TestEnrichmentRejectsForeignResponseURLs(t *testing.T) {
	responses := map[string]stubResponse{comparePath(parentSHA, commitSHA): {
		body: fmt.Sprintf(`{"url":"https://evil.example.com%s","base_commit":{"sha":%q},"files":[]}`,
			comparePath(parentSHA, commitSHA), parentSHA)}}
	server := enrichFixtureServer(t, responses)
	enricher, _ := newTestEnricher(t, server)

	if outcome := changed(t, enricher, mustCompareEvidence(t, parentSHA, commitSHA)); outcome.Kind() != domain.OutcomeIncomplete {
		t.Fatalf("outcome = %v, want incomplete", outcome.Kind())
	}
}

// TestEnrichmentOutcomesNeverExposeTheCredential guards NFR-003 for every
// enrichment outcome: a sanitized reason names the operation and entity only.
func TestEnrichmentOutcomesNeverExposeTheCredential(t *testing.T) {
	secretBody := `{"message":"` + sentinelToken + ` is not authorized","documentation_url":"https://example.invalid"}`
	responses := map[string]stubResponse{
		comparePath(parentSHA, commitSHA): {status: http.StatusForbidden, body: secretBody},
		commitPath(1):                     {status: http.StatusInternalServerError, body: secretBody},
		"/repos/Acme/Web/pulls/12":        {status: http.StatusNotFound, body: secretBody},
	}
	server := enrichFixtureServer(t, responses)
	enricher, client := newTestEnricher(t, server)

	settled, _ := enricher.CurrentPullRequest(context.Background(), mustPullRequestEvidence(t, 12)).Settled()
	reasons := []string{
		changed(t, enricher, mustCompareEvidence(t, parentSHA, commitSHA)).Reason(),
		changed(t, enricher, mustCommitEvidence(t, commitSHA)).Reason(),
		settled.Reason(),
	}
	for _, reason := range reasons {
		if reason == "" {
			t.Error("a non-complete outcome reported no reason")
		}
		if strings.Contains(reason, sentinelToken) {
			t.Errorf("outcome reason exposed the credential: %q", reason)
		}
		if strings.Contains(reason, "documentation_url") || strings.Contains(reason, "not authorized") {
			t.Errorf("outcome reason exposed the response body: %q", reason)
		}
	}
	for _, request := range client.recorded() {
		if got := request.header.Get("Authorization"); got != "Bearer "+sentinelToken {
			t.Errorf("Authorization header = %q, want the resolved credential", got)
		}
	}
}
