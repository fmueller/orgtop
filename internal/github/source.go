package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/fmueller/orgtop/internal/auth"
	"github.com/fmueller/orgtop/internal/domain"
)

// The documented v0.1.0 request contract (FR-004). A retired contract requires a
// spec revision rather than an implicit behavior change here.
const (
	defaultBaseURL  = "https://api.github.com"
	acceptMediaType = "application/vnd.github+json"
	apiVersion      = "2022-11-28"
	perPage         = "100"
	userAgent       = "orgtop/0.1.0"
	requestTimeout  = 30 * time.Second
)

// bodyDrainLimit bounds how much of a response body the caller stopped reading,
// whether untouched or partially read, is consumed so the idle keep-alive
// connection can be reused. It is large enough for a GitHub error document and
// small enough that a hostile body cannot force a long read.
const bodyDrainLimit = 4 << 10

// Failure classes a repository refresh reports. None of them carries a
// credential value or an authenticated request header (NFR-003).
var (
	// ErrAuthentication reports a rejected credential.
	ErrAuthentication = errors.New("github rejected the credential: check the token or run gh auth login")

	// ErrAccessDenied reports a denied request that is not rate limiting.
	ErrAccessDenied = errors.New("github denied access to the repository")

	// ErrNotFound reports a repository that does not exist or is inaccessible.
	ErrNotFound = errors.New("repository not found or inaccessible")

	// ErrRateLimited reports an exhausted GitHub rate limit.
	ErrRateLimited = errors.New("github rate limit reached")

	// ErrUnexpectedResponse reports any other unusable response status.
	ErrUnexpectedResponse = errors.New("unexpected github response")

	// ErrTransport reports a request that never produced a response.
	ErrTransport = errors.New("github request failed")
)

// Doer performs one HTTP request. *http.Client satisfies it, and tests inject a
// recording or fixture-backed implementation.
type Doer interface {
	Do(request *http.Request) (*http.Response, error)
}

// Source retrieves one bounded repository events page per selected repository.
// It owns no state between refreshes and performs no retries of its own.
type Source struct {
	// Client performs requests. A nil Client uses a default http.Client; every
	// request is bound to a context with the FR-004 request timeout regardless.
	Client Doer
	// BaseURL is the GitHub API root. An empty value uses the public API.
	BaseURL string
	// Credential authenticates every request.
	Credential auth.Credential
	// Now supplies the instant retry scheduling is calculated from. A nil Now
	// uses time.Now.
	Now func() time.Time
}

// RepositoryActivity is the successful result for one Scope entry. It aliases the
// domain aggregation input, so a refresh needs no conversion step, and the FR-002
// display identity rule stays documented with the domain type.
type RepositoryActivity = domain.RepositoryActivity

// Refresh is one completely successful multi-repository refresh. Repositories
// holds exactly one entry per Scope entry in request order. PollDelay is
// scheduling metadata and never enters events or snapshots.
type Refresh struct {
	Repositories []RepositoryActivity
	PollDelay    time.Duration
}

// RefreshError reports the repository failure that failed the atomic refresh.
// RetryDelay is the earliest eligible next attempt; it is scheduling metadata
// and never enters events or snapshots.
type RefreshError struct {
	// Repository is the Scope entry whose refresh failed.
	Repository domain.Repository
	// RetryDelay is the delay before the next attempt is eligible.
	RetryDelay time.Duration

	cause error
}

// Error implements error with a sanitized cause.
func (e *RefreshError) Error() string {
	return fmt.Sprintf("refreshing %s: %v", e.Repository, e.cause)
}

// Unwrap exposes the failure class for errors.Is.
func (e *RefreshError) Unwrap() error { return e.cause }

// NewSource returns a Source bound to the public GitHub API and the default HTTP
// client.
func NewSource(credential auth.Credential) Source {
	return Source{Credential: credential}
}

// Refresh retrieves one events page for every Scope entry. It is atomic: the
// first repository failure fails the whole refresh, and no partial data is
// returned. The caller ctx cancels in-flight work.
func (s Source) Refresh(ctx context.Context, scopes domain.ScopeSet) (Refresh, error) {
	repositories := scopes.Repositories()
	if len(repositories) == 0 {
		return Refresh{}, domain.ErrEmptyScope
	}

	activities := make([]RepositoryActivity, 0, len(repositories))
	intervals := make([]time.Duration, 0, len(repositories))
	for _, repository := range repositories {
		activity, interval, err := s.fetch(ctx, repository)
		if err != nil {
			return Refresh{}, err
		}
		activities = append(activities, activity)
		intervals = append(intervals, interval)
	}
	return Refresh{Repositories: activities, PollDelay: pollDelay(intervals)}, nil
}

// fetch retrieves and normalizes one repository page and reports the poll
// interval that response advertised.
func (s Source) fetch(ctx context.Context, repository domain.Repository) (RepositoryActivity, time.Duration, error) {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	response, err := s.do(requestCtx, repository)
	if err != nil {
		return RepositoryActivity{}, 0, s.failure(repository, http.Header{}, false, err)
	}
	defer func() { _ = response.Body.Close() }()

	rateLimited, statusErr := s.statusError(response)
	if statusErr != nil {
		drainBody(response.Body)
		return RepositoryActivity{}, 0, s.failure(repository, response.Header, rateLimited, statusErr)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		drainBody(response.Body)
		return RepositoryActivity{}, 0, s.failure(repository, response.Header, false, fmt.Errorf("%w: reading the response failed", ErrTransport))
	}

	events, err := NormalizeEvents(repository, body)
	if err != nil {
		return RepositoryActivity{}, 0, s.failure(repository, response.Header, false, err)
	}
	return RepositoryActivity{
		Repository: displayIdentity(repository, events),
		Events:     events,
	}, pollInterval(response.Header), nil
}

// drainBody consumes a bounded prefix of a body the caller stopped reading, so
// net/http can reuse the idle connection once the deferred Close runs.
func drainBody(body io.Reader) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, bodyDrainLimit))
}

// do sends the documented bounded events request for one repository.
func (s Source) do(ctx context.Context, repository domain.Repository) (*http.Response, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/events?per_page=%s",
		s.baseURL(), url.PathEscape(repository.Owner()), url.PathEscape(repository.Name()), perPage)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: the request could not be built", ErrTransport)
	}
	request.Header.Set("Accept", acceptMediaType)
	request.Header.Set("X-GitHub-Api-Version", apiVersion)
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Authorization", "Bearer "+s.Credential.Token())

	response, err := s.client().Do(request)
	if err != nil {
		// Wrapping keeps cancellation and deadline causes matchable while
		// carrying no credential: the cause reports the request URL, never
		// request headers.
		return nil, fmt.Errorf("%w: %w", ErrTransport, err)
	}
	return response, nil
}

// statusError classifies a non-2xx response and reports whether it was denied
// because the rate limit is exhausted. FR-003 fixes the reported cause for 401,
// both 403 classes, 404, and 429.
func (s Source) statusError(response *http.Response) (rateLimited bool, cause error) {
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return false, nil
	}
	switch {
	case response.StatusCode == http.StatusUnauthorized:
		return false, ErrAuthentication
	case response.StatusCode == http.StatusNotFound:
		return false, ErrNotFound
	case isRateLimited(response.StatusCode, response.Header, s.now()):
		return true, ErrRateLimited
	case response.StatusCode == http.StatusForbidden:
		return false, ErrAccessDenied
	default:
		return false, fmt.Errorf("%w: status %d", ErrUnexpectedResponse, response.StatusCode)
	}
}

// failure wraps a sanitized cause with the next eligible retry delay.
func (s Source) failure(repository domain.Repository, header http.Header, rateLimited bool, cause error) error {
	return &RefreshError{
		Repository: repository,
		RetryDelay: retryDelay(header, rateLimited, s.now()),
		cause:      cause,
	}
}

// displayIdentity returns the identity shown for a Scope entry: the first
// matching returned spelling, or the requested spelling for an empty page.
func displayIdentity(requested domain.Repository, events []domain.Event) domain.Repository {
	if len(events) == 0 {
		return requested
	}
	return events[0].Repository
}

func (s Source) baseURL() string {
	if s.BaseURL == "" {
		return defaultBaseURL
	}
	return s.BaseURL
}

func (s Source) client() Doer {
	if s.Client == nil {
		return http.DefaultClient
	}
	return s.Client
}

func (s Source) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}
