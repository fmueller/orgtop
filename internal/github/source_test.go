package github_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/auth"
	"github.com/fmueller/orgtop/internal/domain"
	"github.com/fmueller/orgtop/internal/github"
)

// sentinelToken is the only credential these tests use. No captured request,
// error, or rendered value may contain it (NFR-003).
const sentinelToken = "ghp_orgtopsentinelvalue0123456789abcdef"

// testNow pins the instant retry scheduling is calculated from.
var testNow = time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

const defaultInterval = 60 * time.Second

func sentinelCredential(t *testing.T) auth.Credential {
	t.Helper()
	resolver := auth.Resolver{
		LookupEnv: func(key string) string {
			if key == "GH_TOKEN" {
				return sentinelToken
			}
			return ""
		},
		Run: func(context.Context, string, ...string) ([]byte, error) {
			t.Error("gh was invoked although GH_TOKEN is set")
			return nil, errors.New("unexpected gh invocation")
		},
	}
	credential, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("resolving the sentinel credential failed: %v", err)
	}
	return credential
}

func mustScope(t *testing.T, values ...string) domain.ScopeSet {
	t.Helper()
	scope, err := domain.NewRepositoryScopeSet(values)
	if err != nil {
		t.Fatalf("building the scope %v failed: %v", values, err)
	}
	return scope
}

// stubResponse is one canned repository events response.
type stubResponse struct {
	status int
	body   string
	header map[string]string
}

// recordedRequest is what the source actually sent.
type recordedRequest struct {
	method      string
	path        string
	query       url.Values
	header      http.Header
	hasDeadline bool
	deadline    time.Time
}

// recordingClient records every request before delegating to the fixture server.
type recordingClient struct {
	inner    *http.Client
	mu       sync.Mutex
	requests []recordedRequest
}

func (c *recordingClient) Do(request *http.Request) (*http.Response, error) {
	recorded := recordedRequest{
		method: request.Method,
		path:   request.URL.Path,
		query:  request.URL.Query(),
		header: request.Header.Clone(),
	}
	if deadline, ok := request.Context().Deadline(); ok {
		recorded.hasDeadline = true
		recorded.deadline = deadline
	}
	c.mu.Lock()
	c.requests = append(c.requests, recorded)
	c.mu.Unlock()
	return c.inner.Do(request)
}

func (c *recordingClient) recorded() []recordedRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]recordedRequest(nil), c.requests...)
}

func newFixtureServer(t *testing.T, responses map[string]stubResponse) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		response, known := responses[request.URL.Path]
		if !known {
			t.Errorf("unexpected request path %q", request.URL.Path)
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

func newTestSource(t *testing.T, server *httptest.Server) (github.Source, *recordingClient) {
	t.Helper()
	client := &recordingClient{inner: server.Client()}
	source := github.Source{
		Client:     client,
		BaseURL:    server.URL,
		Credential: sentinelCredential(t),
		Now:        func() time.Time { return testNow },
	}
	return source, client
}

func fixtureBody(t *testing.T, name string) string {
	t.Helper()
	return string(loadFixture(t, name))
}

// refreshError extracts the structured failure a failed refresh must carry.
func refreshError(t *testing.T, err error) *github.RefreshError {
	t.Helper()
	var failure *github.RefreshError
	if !errors.As(err, &failure) {
		t.Fatalf("error %v is not a *github.RefreshError", err)
	}
	return failure
}

// refreshExpectingError refreshes one repository whose response must fail and
// asserts that no consumable partial data is returned.
func refreshExpectingError(t *testing.T, response stubResponse) *github.RefreshError {
	t.Helper()
	server := newFixtureServer(t, map[string]stubResponse{"/repos/acme/backend/events": response})
	source, _ := newTestSource(t, server)
	return refreshSourceExpectingError(t, source)
}

// refreshSourceExpectingError refreshes one repository through source, whose
// response must fail, and asserts that no consumable partial data is returned.
func refreshSourceExpectingError(t *testing.T, source github.Source) *github.RefreshError {
	t.Helper()
	refresh, err := source.Refresh(context.Background(), mustScope(t, "acme/backend"))
	if err == nil {
		t.Fatalf("refresh succeeded with %d repositories, want an error", len(refresh.Repositories))
	}
	assertNoPartialData(t, refresh)
	return refreshError(t, err)
}

func assertNoPartialData(t *testing.T, refresh github.Refresh) {
	t.Helper()
	if len(refresh.Repositories) != 0 {
		t.Errorf("failed refresh exposes %d repositories, want no consumable partial data", len(refresh.Repositories))
	}
	if refresh.PollDelay != 0 {
		t.Errorf("failed refresh exposes a poll delay of %s, want none", refresh.PollDelay)
	}
}

func TestRefreshRequestsOneDocumentedPagePerScopeEntry(t *testing.T) {
	server := newFixtureServer(t, map[string]stubResponse{
		"/repos/acme/backend/events":  {body: fixtureBody(t, "page.json")},
		"/repos/acme/frontend/events": {body: fixtureBody(t, "empty.json")},
	})
	source, client := newTestSource(t, server)

	if _, err := source.Refresh(context.Background(), mustScope(t, "acme/backend", "acme/frontend")); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	requests := client.recorded()
	if len(requests) != 2 {
		t.Fatalf("got %d requests, want exactly one page per scope entry", len(requests))
	}
	wantPaths := []string{"/repos/acme/backend/events", "/repos/acme/frontend/events"}
	for index, request := range requests {
		if request.path != wantPaths[index] {
			t.Errorf("request %d path = %q, want %q", index, request.path, wantPaths[index])
		}
		if request.method != http.MethodGet {
			t.Errorf("request %d method = %q, want %q", index, request.method, http.MethodGet)
		}
		if got := request.query.Get("per_page"); got != "100" {
			t.Errorf("request %d per_page = %q, want %q", index, got, "100")
		}
		if len(request.query) != 1 {
			t.Errorf("request %d query = %v, want only per_page", index, request.query)
		}
		if got := request.header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("request %d Accept = %q, want %q", index, got, "application/vnd.github+json")
		}
		if got := request.header.Get("X-GitHub-Api-Version"); got != "2026-03-10" {
			t.Errorf("request %d X-GitHub-Api-Version = %q, want %q", index, got, "2026-03-10")
		}
		if request.header.Get("Authorization") != "Bearer "+sentinelToken {
			t.Errorf("request %d does not carry the resolved bearer credential", index)
		}
		if got := request.header.Get("User-Agent"); got == "" {
			t.Errorf("request %d has no user agent", index)
		}
		if !request.hasDeadline {
			t.Errorf("request %d has no deadline, want a request timeout", index)
		}
	}
}

func TestRefreshUsesAStableUserAgentForEveryRequest(t *testing.T) {
	server := newFixtureServer(t, map[string]stubResponse{
		"/repos/acme/backend/events":  {body: fixtureBody(t, "empty.json")},
		"/repos/acme/frontend/events": {body: fixtureBody(t, "empty.json")},
	})
	source, client := newTestSource(t, server)

	if _, err := source.Refresh(context.Background(), mustScope(t, "acme/backend", "acme/frontend")); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	requests := client.recorded()
	first := requests[0].header.Get("User-Agent")
	if !strings.Contains(strings.ToLower(first), "orgtop") {
		t.Errorf("User-Agent = %q, want a stable orgtop identity", first)
	}
	for index, request := range requests[1:] {
		if got := request.header.Get("User-Agent"); got != first {
			t.Errorf("request %d User-Agent = %q, want the stable %q", index+1, got, first)
		}
	}
}

func TestRefreshReturnsNormalizedEventsPerScopeEntry(t *testing.T) {
	server := newFixtureServer(t, map[string]stubResponse{
		"/repos/acme/backend/events":  {body: fixtureBody(t, "page.json")},
		"/repos/acme/frontend/events": {body: fixtureBody(t, "empty.json")},
	})
	source, _ := newTestSource(t, server)

	refresh, err := source.Refresh(context.Background(), mustScope(t, "acme/backend", "acme/frontend"))
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if len(refresh.Repositories) != 2 {
		t.Fatalf("got %d repository results, want one per scope entry", len(refresh.Repositories))
	}

	backend := refresh.Repositories[0]
	if len(backend.Events) != 2 {
		t.Fatalf("got %d events for acme/backend, want 2", len(backend.Events))
	}
	if backend.Events[0].ID != "8002" || backend.Events[1].ID != "8001" {
		t.Errorf("got event IDs %q, %q, want the returned page order %q, %q",
			backend.Events[0].ID, backend.Events[1].ID, "8002", "8001")
	}
	if got := backend.Events[0].Repository.Key(); got != "acme/backend" {
		t.Errorf("event repository key = %q, want %q", got, "acme/backend")
	}
	if frontend := refresh.Repositories[1]; len(frontend.Events) != 0 {
		t.Errorf("got %d events for the empty acme/frontend page, want none", len(frontend.Events))
	}
}

func TestRefreshSelectsOneDisplayIdentityPerScopeEntry(t *testing.T) {
	server := newFixtureServer(t, map[string]stubResponse{
		"/repos/acme/backend/events":  {body: fixtureBody(t, "repository_case_variation.json")},
		"/repos/Acme/Frontend/events": {body: fixtureBody(t, "empty.json")},
	})
	source, _ := newTestSource(t, server)

	refresh, err := source.Refresh(context.Background(), mustScope(t, "acme/backend", "Acme/Frontend"))
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if got, want := refresh.Repositories[0].Repository.String(), "Acme/Backend"; got != want {
		t.Errorf("display identity = %q, want the first matching returned spelling %q", got, want)
	}
	if got, want := refresh.Repositories[1].Repository.String(), "Acme/Frontend"; got != want {
		t.Errorf("display identity for an empty page = %q, want the requested spelling %q", got, want)
	}
}

func TestRefreshComputesThePollDelay(t *testing.T) {
	tests := []struct {
		name     string
		backend  string
		frontend string
		want     time.Duration
	}{
		{name: "no header falls back to the default interval", want: defaultInterval},
		{name: "the greatest advertised interval wins", backend: "90", frontend: "70", want: 90 * time.Second},
		{name: "a shorter interval keeps the default floor", backend: "30", want: defaultInterval},
		{name: "an unparseable interval is ignored", backend: "soon", want: defaultInterval},
		{name: "a negative interval is ignored", backend: "-120", want: defaultInterval},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newFixtureServer(t, map[string]stubResponse{
				"/repos/acme/backend/events":  {body: fixtureBody(t, "empty.json"), header: map[string]string{"X-Poll-Interval": test.backend}},
				"/repos/acme/frontend/events": {body: fixtureBody(t, "empty.json"), header: map[string]string{"X-Poll-Interval": test.frontend}},
			})
			source, _ := newTestSource(t, server)

			refresh, err := source.Refresh(context.Background(), mustScope(t, "acme/backend", "acme/frontend"))
			if err != nil {
				t.Fatalf("refresh failed: %v", err)
			}
			if refresh.PollDelay != test.want {
				t.Errorf("PollDelay = %s, want %s", refresh.PollDelay, test.want)
			}
		})
	}
}

func TestRefreshFailsAtomicallyWhenOneRepositoryFails(t *testing.T) {
	server := newFixtureServer(t, map[string]stubResponse{
		"/repos/acme/backend/events":  {body: fixtureBody(t, "page.json")},
		"/repos/acme/frontend/events": {status: http.StatusInternalServerError, body: `{"message":"boom"}`},
	})
	source, _ := newTestSource(t, server)

	refresh, err := source.Refresh(context.Background(), mustScope(t, "acme/backend", "acme/frontend"))
	if err == nil {
		t.Fatalf("refresh succeeded with %d repositories, want an error", len(refresh.Repositories))
	}
	assertNoPartialData(t, refresh)

	failure := refreshError(t, err)
	if got, want := failure.Repository.String(), "acme/frontend"; got != want {
		t.Errorf("failed repository = %q, want %q", got, want)
	}
	if !errors.Is(err, github.ErrUnexpectedResponse) {
		t.Errorf("error %v does not match ErrUnexpectedResponse", err)
	}
}

func TestRefreshClassifiesFailureStatusesAndRetryDelays(t *testing.T) {
	reset := func(offset time.Duration) string {
		return fmt.Sprintf("%d", testNow.Add(offset).Unix())
	}

	tests := []struct {
		name      string
		response  stubResponse
		wantErr   error
		wantDelay time.Duration
	}{
		{
			name:      "unauthorized",
			response:  stubResponse{status: http.StatusUnauthorized},
			wantErr:   github.ErrAuthentication,
			wantDelay: defaultInterval,
		},
		{
			name:      "forbidden without rate limit signals is denied access",
			response:  stubResponse{status: http.StatusForbidden, header: map[string]string{"X-RateLimit-Remaining": "42"}},
			wantErr:   github.ErrAccessDenied,
			wantDelay: defaultInterval,
		},
		{
			name: "forbidden with an exhausted rate limit",
			response: stubResponse{status: http.StatusForbidden, header: map[string]string{
				"X-RateLimit-Remaining": "0",
				"X-RateLimit-Reset":     reset(5 * time.Minute),
			}},
			wantErr:   github.ErrRateLimited,
			wantDelay: 5 * time.Minute,
		},
		{
			name:      "forbidden with a retry after header",
			response:  stubResponse{status: http.StatusForbidden, header: map[string]string{"Retry-After": "120"}},
			wantErr:   github.ErrRateLimited,
			wantDelay: 120 * time.Second,
		},
		{
			name:      "forbidden with a zero delta retry after",
			response:  stubResponse{status: http.StatusForbidden, header: map[string]string{"Retry-After": "0"}},
			wantErr:   github.ErrRateLimited,
			wantDelay: defaultInterval,
		},
		{
			name:      "not found",
			response:  stubResponse{status: http.StatusNotFound},
			wantErr:   github.ErrNotFound,
			wantDelay: defaultInterval,
		},
		{
			name:      "too many requests keeps the default floor",
			response:  stubResponse{status: http.StatusTooManyRequests, header: map[string]string{"Retry-After": "5"}},
			wantErr:   github.ErrRateLimited,
			wantDelay: defaultInterval,
		},
		{
			name:      "too many requests with an http date retry after",
			response:  stubResponse{status: http.StatusTooManyRequests, header: map[string]string{"Retry-After": testNow.Add(2 * time.Hour).Format(http.TimeFormat)}},
			wantErr:   github.ErrRateLimited,
			wantDelay: 2 * time.Hour,
		},
		{
			name: "the latest retry instant wins",
			response: stubResponse{status: http.StatusTooManyRequests, header: map[string]string{
				"Retry-After":       "100",
				"X-RateLimit-Reset": reset(15 * time.Minute),
			}},
			wantErr:   github.ErrRateLimited,
			wantDelay: 15 * time.Minute,
		},
		{
			name: "invalid scheduling headers are ignored safely",
			response: stubResponse{status: http.StatusTooManyRequests, header: map[string]string{
				"Retry-After":       "whenever",
				"X-RateLimit-Reset": "not-a-timestamp",
			}},
			wantErr:   github.ErrRateLimited,
			wantDelay: defaultInterval,
		},
		{
			name:      "an elapsed retry instant keeps the default floor",
			response:  stubResponse{status: http.StatusTooManyRequests, header: map[string]string{"X-RateLimit-Reset": reset(-time.Hour)}},
			wantErr:   github.ErrRateLimited,
			wantDelay: defaultInterval,
		},
		{
			name:      "a redirect status is not a successful page",
			response:  stubResponse{status: http.StatusMultipleChoices, body: `[]`},
			wantErr:   github.ErrUnexpectedResponse,
			wantDelay: defaultInterval,
		},
		{
			name:      "any other status",
			response:  stubResponse{status: http.StatusBadGateway},
			wantErr:   github.ErrUnexpectedResponse,
			wantDelay: defaultInterval,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			failure := refreshExpectingError(t, test.response)
			if !errors.Is(failure, test.wantErr) {
				t.Errorf("error %v does not match %v", failure, test.wantErr)
			}
			if failure.RetryDelay != test.wantDelay {
				t.Errorf("RetryDelay = %s, want %s", failure.RetryDelay, test.wantDelay)
			}
			if failure.Repository.String() != "acme/backend" {
				t.Errorf("failed repository = %q, want %q", failure.Repository, "acme/backend")
			}
		})
	}
}

func TestRefreshRejectsUnusableResponseData(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		wantErr error
	}{
		{name: "malformed page", fixture: "malformed.json", wantErr: github.ErrInvalidPayload},
		{name: "top level object", fixture: "not_an_array.json", wantErr: github.ErrInvalidPayload},
		{name: "missing event id", fixture: "missing_id.json", wantErr: github.ErrInvalidPayload},
		{name: "repository identity mismatch", fixture: "repository_mismatch.json", wantErr: github.ErrRepositoryMismatch},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			failure := refreshExpectingError(t, stubResponse{body: fixtureBody(t, test.fixture)})
			if !errors.Is(failure, test.wantErr) {
				t.Errorf("error %v does not match %v", failure, test.wantErr)
			}
			if failure.RetryDelay != defaultInterval {
				t.Errorf("RetryDelay = %s, want %s", failure.RetryDelay, defaultInterval)
			}
		})
	}
}

func TestRefreshAcceptsAnEmptyResponse(t *testing.T) {
	server := newFixtureServer(t, map[string]stubResponse{
		"/repos/acme/backend/events": {body: fixtureBody(t, "empty.json")},
	})
	source, _ := newTestSource(t, server)

	refresh, err := source.Refresh(context.Background(), mustScope(t, "acme/backend"))
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if len(refresh.Repositories) != 1 || len(refresh.Repositories[0].Events) != 0 {
		t.Fatalf("got %+v, want one repository result without events", refresh.Repositories)
	}
	if refresh.PollDelay != defaultInterval {
		t.Errorf("PollDelay = %s, want %s", refresh.PollDelay, defaultInterval)
	}
}

func TestRefreshStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		cancel()
		<-request.Context().Done()
	}))
	t.Cleanup(server.Close)
	source, _ := newTestSource(t, server)

	refresh, err := source.Refresh(ctx, mustScope(t, "acme/backend"))
	if err == nil {
		t.Fatalf("refresh succeeded with %d repositories, want a cancellation error", len(refresh.Repositories))
	}
	assertNoPartialData(t, refresh)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error %v does not match context.Canceled", err)
	}
}

func TestRefreshReportsTransportFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	source, _ := newTestSource(t, server)
	server.Close()

	refresh, err := source.Refresh(context.Background(), mustScope(t, "acme/backend"))
	if err == nil {
		t.Fatalf("refresh succeeded with %d repositories, want a transport error", len(refresh.Repositories))
	}
	assertNoPartialData(t, refresh)

	failure := refreshError(t, err)
	if !errors.Is(failure, github.ErrTransport) {
		t.Errorf("error %v does not match ErrTransport", failure)
	}
	if failure.RetryDelay != defaultInterval {
		t.Errorf("RetryDelay = %s, want %s", failure.RetryDelay, defaultInterval)
	}
	if !strings.Contains(failure.Error(), "acme/backend") {
		t.Errorf("error %q does not name the failed repository", failure.Error())
	}
}

func TestRefreshDoesNotRequestRepositoriesAfterAFailure(t *testing.T) {
	server := newFixtureServer(t, map[string]stubResponse{
		"/repos/acme/backend/events": {status: http.StatusNotFound},
	})
	source, client := newTestSource(t, server)

	if _, err := source.Refresh(context.Background(), mustScope(t, "acme/backend", "acme/frontend")); err == nil {
		t.Fatal("refresh succeeded, want an error")
	}
	if requests := client.recorded(); len(requests) != 1 {
		t.Errorf("got %d requests, want the refresh to stop after the first failure", len(requests))
	}
}

func TestSourceNeverExposesTheCredential(t *testing.T) {
	server := newFixtureServer(t, map[string]stubResponse{
		"/repos/acme/backend/events": {status: http.StatusUnauthorized, body: `{"message":"Bad credentials"}`},
	})
	source, client := newTestSource(t, server)

	_, err := source.Refresh(context.Background(), mustScope(t, "acme/backend"))
	if err == nil {
		t.Fatal("refresh succeeded, want an error")
	}

	rendered := []string{
		err.Error(),
		fmt.Sprintf("%v", err),
		fmt.Sprintf("%+v", err),
		fmt.Sprintf("%v", source),
		fmt.Sprintf("%+v", source),
		fmt.Sprintf("%#v", source),
		fmt.Sprintf("%v", refreshError(t, err)),
	}
	for _, value := range rendered {
		if strings.Contains(value, sentinelToken) {
			t.Errorf("rendered value %q contains the credential", value)
		}
		if strings.Contains(strings.ToLower(value), "authorization") {
			t.Errorf("rendered value %q contains an authenticated request header", value)
		}
	}

	requests := client.recorded()
	if len(requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(requests))
	}
	if got := requests[0].header.Get("Authorization"); got != "Bearer "+sentinelToken {
		t.Error("the request did not carry the resolved bearer credential")
	}
}

func TestRefreshRejectsAnEmptyScope(t *testing.T) {
	server := newFixtureServer(t, map[string]stubResponse{})
	source, client := newTestSource(t, server)

	refresh, err := source.Refresh(context.Background(), domain.ScopeSet{})
	if err == nil {
		t.Fatalf("refresh succeeded with %d repositories, want an error", len(refresh.Repositories))
	}
	assertNoPartialData(t, refresh)
	if len(client.recorded()) != 0 {
		t.Errorf("got %d requests for an empty scope, want none", len(client.recorded()))
	}
}

// instrumentedBody records how much of a response body was read and whether the
// read happened before Close.
type instrumentedBody struct {
	reader         io.Reader
	read           int64
	closed         bool
	readAfterClose bool
}

func (b *instrumentedBody) Read(buffer []byte) (int, error) {
	if b.closed {
		b.readAfterClose = true
	}
	n, err := b.reader.Read(buffer)
	b.read += int64(n)
	return n, err
}

func (b *instrumentedBody) Close() error {
	b.closed = true
	return nil
}

// stubClient answers every request with one canned response.
type stubClient struct{ response *http.Response }

func (c stubClient) Do(*http.Request) (*http.Response, error) { return c.response, nil }

// TestRefreshDrainsNon2xxBodiesWithinABound covers the keep-alive nit from the
// T-004 review: net/http can only reuse an idle connection once the body is
// consumed, and a hostile error body must not force an unbounded read.
func TestRefreshDrainsNon2xxBodiesWithinABound(t *testing.T) {
	const drainLimit = 4 << 10

	tests := []struct {
		name     string
		size     int
		wantRead int64
	}{
		{name: "a short error body is drained completely", size: 128, wantRead: 128},
		{name: "an oversized error body is bounded", size: 1 << 20, wantRead: drainLimit},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header := http.Header{}
			header.Set("X-RateLimit-Remaining", "42")
			body := &instrumentedBody{reader: strings.NewReader(strings.Repeat("x", test.size))}
			source := github.Source{
				Client: stubClient{response: &http.Response{
					StatusCode: http.StatusForbidden,
					Header:     header,
					Body:       body,
				}},
				BaseURL:    "https://api.test.invalid",
				Credential: sentinelCredential(t),
				Now:        func() time.Time { return testNow },
			}

			failure := refreshSourceExpectingError(t, source)
			if !errors.Is(failure, github.ErrAccessDenied) {
				t.Errorf("error %v does not match ErrAccessDenied", failure)
			}
			if body.read != test.wantRead {
				t.Errorf("drained %d bytes, want %d", body.read, test.wantRead)
			}
			if !body.closed {
				t.Error("the response body was not closed")
			}
			if body.readAfterClose {
				t.Error("the response body was read after it was closed")
			}
		})
	}
}

// midStreamFailureReader serves a prefix, fails once the way a reset connection
// does, then serves the rest so a drain read after the failure is observable.
type midStreamFailureReader struct {
	prefix    io.Reader
	remainder io.Reader
	failed    bool
}

func (r *midStreamFailureReader) Read(buffer []byte) (int, error) {
	if r.failed {
		return r.remainder.Read(buffer)
	}
	n, err := r.prefix.Read(buffer)
	if err == nil {
		return n, nil
	}
	r.failed = true
	return n, errors.New("connection reset by peer")
}

// TestRefreshDrainsAPartiallyReadSuccessBodyWithinABound covers T-018: a 2xx
// body whose read fails partway must still be drained within the shared bound
// so net/http can reuse the idle connection, and a hostile remainder must not
// force an unbounded read.
func TestRefreshDrainsAPartiallyReadSuccessBodyWithinABound(t *testing.T) {
	const (
		drainLimit  = 4 << 10
		prefixSize  = 64
		wantMessage = "reading the response failed"
	)

	tests := []struct {
		name          string
		remainderSize int
		wantRead      int64
	}{
		{name: "a short remainder is drained completely", remainderSize: 128, wantRead: prefixSize + 128},
		{name: "an oversized remainder is bounded", remainderSize: 1 << 20, wantRead: prefixSize + drainLimit},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := &instrumentedBody{reader: &midStreamFailureReader{
				prefix:    strings.NewReader(strings.Repeat("x", prefixSize)),
				remainder: strings.NewReader(strings.Repeat("y", test.remainderSize)),
			}}
			source := github.Source{
				Client: stubClient{response: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       body,
				}},
				BaseURL:    "https://api.test.invalid",
				Credential: sentinelCredential(t),
				Now:        func() time.Time { return testNow },
			}

			failure := refreshSourceExpectingError(t, source)
			if !errors.Is(failure, github.ErrTransport) {
				t.Errorf("error %v does not match ErrTransport", failure)
			}
			if !strings.Contains(failure.Error(), wantMessage) {
				t.Errorf("error %q does not report %q", failure, wantMessage)
			}
			if body.read != test.wantRead {
				t.Errorf("read %d bytes, want %d", body.read, test.wantRead)
			}
			if !body.closed {
				t.Error("the response body was not closed")
			}
			if body.readAfterClose {
				t.Error("the response body was read after it was closed")
			}
		})
	}
}

// requestBudget is the bound Refresh is documented to place on each request
// (FR-004). The test brackets it with caller deadlines on either side rather
// than measuring the remaining duration, so the outcome does not depend on
// machine speed (NFR-006).
const requestBudget = 30 * time.Second

// TestRefreshBoundsEachRequestWithAThirtySecondTimeout pins the budget through
// the only thing an observer can compare a derived deadline against without
// reading the clock: the caller deadline it was derived from. A caller deadline
// inside the budget must survive untouched, and one beyond it must be cut short.
func TestRefreshBoundsEachRequestWithAThirtySecondTimeout(t *testing.T) {
	tests := []struct {
		name              string
		callerBudget      time.Duration
		wantCallerHonored bool
	}{
		{
			name:              "a caller deadline inside the budget is not extended",
			callerBudget:      requestBudget - time.Second,
			wantCallerHonored: true,
		},
		{
			name:              "a caller deadline beyond the budget is shortened",
			callerBudget:      requestBudget + time.Second,
			wantCallerHonored: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newFixtureServer(t, map[string]stubResponse{
				"/repos/acme/backend/events": {body: fixtureBody(t, "empty.json")},
			})
			source, client := newTestSource(t, server)
			ctx, cancel := context.WithTimeout(t.Context(), tt.callerBudget)
			defer cancel()
			callerDeadline, hasCallerDeadline := ctx.Deadline()
			if !hasCallerDeadline {
				t.Fatal("the caller context carried no deadline")
			}

			if _, err := source.Refresh(ctx, mustScope(t, "acme/backend")); err != nil {
				t.Fatalf("refresh failed: %v", err)
			}

			requests := client.recorded()
			if len(requests) != 1 {
				t.Fatalf("got %d requests, want exactly one", len(requests))
			}
			request := requests[0]
			if !request.hasDeadline {
				t.Fatal("the request context carried no deadline")
			}
			if honored := request.deadline.Equal(callerDeadline); honored != tt.wantCallerHonored {
				t.Errorf("the request deadline equals the caller deadline = %t, want %t", honored, tt.wantCallerHonored)
			}
			if !tt.wantCallerHonored && !request.deadline.Before(callerDeadline) {
				t.Errorf("request deadline %v is not earlier than the caller deadline %v, so the budget was never applied",
					request.deadline, callerDeadline)
			}
		})
	}
}
