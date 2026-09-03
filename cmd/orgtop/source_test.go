package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
	"github.com/fmueller/orgtop/internal/github"
)

// roundTripper serves one canned response or transport failure per request.
type roundTripper struct {
	response *http.Response
	err      error
}

func (r roundTripper) Do(*http.Request) (*http.Response, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.response, nil
}

// response builds a GitHub response with the given status, headers, and body.
func response(status int, header http.Header, body string) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestAdapterMapsASuccessfulRefreshOntoTheLifecycleResult(t *testing.T) {
	header := http.Header{}
	header.Set("X-Poll-Interval", "180")
	adapter := sourceAdapter{source: github.Source{
		Client:  roundTripper{response: response(http.StatusOK, header, "[]")},
		BaseURL: "https://github.test",
	}}

	result, err := adapter.Refresh(context.Background(), mustScope(t, "acme/backend"))
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if len(result.Repositories) != 1 {
		t.Fatalf("Refresh returned %d repositories, want 1", len(result.Repositories))
	}
	if want := 180 * time.Second; result.Delay != want {
		t.Errorf("Refresh delay = %v, want %v", result.Delay, want)
	}
}

func TestAdapterReportsTheSourceRetryDelayOnFailure(t *testing.T) {
	header := http.Header{}
	header.Set("X-RateLimit-Remaining", "0")
	header.Set("Retry-After", "600")
	adapter := sourceAdapter{source: github.Source{
		Client:  roundTripper{response: response(http.StatusForbidden, header, "")},
		BaseURL: "https://github.test",
		Now:     func() time.Time { return time.Unix(0, 0) },
	}}

	result, err := adapter.Refresh(context.Background(), mustScope(t, "acme/backend"))
	if !errors.Is(err, github.ErrRateLimited) {
		t.Fatalf("Refresh error = %v, want %v", err, github.ErrRateLimited)
	}
	if len(result.Repositories) != 0 {
		t.Errorf("a failed refresh reported %d repositories, want none", len(result.Repositories))
	}
	if want := 600 * time.Second; result.Delay != want {
		t.Errorf("Refresh retry delay = %v, want %v", result.Delay, want)
	}
}

func TestAdapterReportsNoDelayForAFailureWithoutSchedulingMetadata(t *testing.T) {
	adapter := sourceAdapter{source: github.Source{BaseURL: "https://github.test"}}

	result, err := adapter.Refresh(context.Background(), domain.ScopeSet{})
	if !errors.Is(err, domain.ErrEmptyScope) {
		t.Fatalf("Refresh error = %v, want %v", err, domain.ErrEmptyScope)
	}
	if result.Delay != 0 {
		t.Errorf("Refresh delay = %v, want 0 so the lifecycle applies its own floor", result.Delay)
	}
}

// listingBody is one organization listing page for the expansion adapter tests.
const listingBody = `[{"full_name":"acme/backend","name":"backend","owner":{"login":"acme","type":"Organization"},"archived":false,"disabled":false,"fork":false}]`

func TestExpansionAdapterMapsASuccessfulExpansionOntoTheLifecycleSelection(t *testing.T) {
	adapter := expansionAdapter{
		source: github.Source{
			Client:  roundTripper{response: response(http.StatusOK, nil, listingBody)},
			BaseURL: "https://github.test",
		},
		request: github.ExpansionRequest{Organizations: []string{"acme"}},
	}

	expansion, err := adapter.Expand(context.Background())
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}
	selection := expansion.Selection
	if got := selection.Scopes.Len(); got != 1 {
		t.Fatalf("Expand returned %d scopes, want 1", got)
	}
	if selection.ExpandedScopes != 1 || selection.TotalScopes != 1 || selection.DistinctRepositories != 1 {
		t.Errorf("selection counts are %+v, want one expanded scope of one repository", selection)
	}
	if got := len(selection.Selectors); got != 1 || selection.Selectors[0].Organization != "acme" {
		t.Errorf("selection reports %d selectors, want the acme disclosure retained", got)
	}
	if selection.Selectors[0].Listed != 1 || selection.Selectors[0].Retained != 1 {
		t.Errorf("selector disclosure is %+v, want one listed and retained repository", selection.Selectors[0])
	}
	if got := len(selection.Provenance); got != 1 || selection.Provenance[0].Selector != "acme" {
		t.Errorf("selection reports %d provenance entries, want the contributing selector retained", got)
	}
	if selection.Provenance[0].Exact {
		t.Error("an expansion-created scope is reported as exact")
	}
}

func TestExpansionAdapterReportsTheRetryBoundAndRateLimitOfAFailure(t *testing.T) {
	header := http.Header{}
	header.Set("X-RateLimit-Remaining", "0")
	header.Set("Retry-After", "600")
	adapter := expansionAdapter{
		source: github.Source{
			Client:  roundTripper{response: response(http.StatusForbidden, header, "")},
			BaseURL: "https://github.test",
			Now:     func() time.Time { return time.Unix(0, 0) },
		},
		request: github.ExpansionRequest{Organizations: []string{"acme"}},
	}

	expansion, err := adapter.Expand(context.Background())
	if !errors.Is(err, github.ErrRateLimited) {
		t.Fatalf("Expand error = %v, want %v", err, github.ErrRateLimited)
	}
	if expansion.Selection.Scopes.Len() != 0 {
		t.Error("a failed expansion published a partial selection")
	}
	if want := 600 * time.Second; expansion.RetryDelay != want {
		t.Errorf("Expand retry delay = %v, want %v", expansion.RetryDelay, want)
	}
	if !expansion.RateLimited {
		t.Error("a rate-limited expansion does not report the dispatch bound")
	}
}

func TestExpansionAdapterReportsNoRetryBoundForAFailureWithoutSchedulingMetadata(t *testing.T) {
	adapter := expansionAdapter{
		source:  github.Source{BaseURL: "https://github.test"},
		request: github.ExpansionRequest{Organizations: []string{"-invalid"}},
	}

	expansion, err := adapter.Expand(context.Background())
	if !errors.Is(err, github.ErrInvalidSelector) {
		t.Fatalf("Expand error = %v, want %v", err, github.ErrInvalidSelector)
	}
	if expansion.RetryDelay != 0 || expansion.RateLimited {
		t.Errorf("Expand reported %+v, want no scheduling metadata so the lifecycle applies its own bound", expansion)
	}
}
