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

	result, err := adapter.Refresh(context.Background(), domain.Scope{})
	if !errors.Is(err, domain.ErrEmptyScope) {
		t.Fatalf("Refresh error = %v, want %v", err, domain.ErrEmptyScope)
	}
	if result.Delay != 0 {
		t.Errorf("Refresh delay = %v, want 0 so the lifecycle applies its own floor", result.Delay)
	}
}
