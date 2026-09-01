package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fmueller/orgtop/internal/auth"
	"github.com/fmueller/orgtop/internal/domain"
)

// Sanitized outcome reasons. A user-visible reason names the operation class and
// the displayable entity only: never an authorization value, a response body, a
// request dump, or a transport struct.
const (
	reasonDenied          = "the credential may not read this entity"
	reasonUnavailable     = "github no longer serves this exact entity"
	reasonServerFailure   = "github reported a temporary failure"
	reasonUnexpectedReply = "github returned an unusable response"
	reasonTransport       = "the request did not reach github"
	reasonRequestBuild    = "the request could not be built"
	reasonUnreadableBody  = "the response could not be read"
	reasonMalformedBody   = "the response failed its completeness proof"
	reasonNotChangedFiles = "this descriptor names metadata, not changed files"
)

// Enricher performs the bounded RG-003 changed-file operations for one evidence
// descriptor at a time. It owns no queue, ledger, cache, or retry policy: those
// belong to the coordinating application service, so this type stays a pure
// request-and-prove boundary.
type Enricher struct {
	// Client performs requests. A nil Client uses a non-redirecting default.
	Client Doer
	// BaseURL is the GitHub API root. An empty value uses the public API.
	BaseURL string
	// Credential authenticates every request.
	Credential auth.Credential
	// Now supplies the instant rate-limit retry timing is calculated from.
	Now func() time.Time
}

// NewEnricher returns an Enricher bound to the public GitHub API.
func NewEnricher(credential auth.Credential) Enricher {
	return Enricher{Credential: credential}
}

// Changed returns the one normalized outcome of a descriptor's changed-file
// evidence. Evidence already settled at classification needs no request, and a
// pull request metadata descriptor is not changed-file evidence at all.
func (e Enricher) Changed(ctx context.Context, descriptor domain.EvidenceDescriptor) domain.EvidenceOutcome {
	if settled, ok := descriptor.Settled(); ok {
		return settled
	}
	switch descriptor.Operation() {
	case domain.EvidenceCommit:
		return e.commitEvidence(ctx, descriptor)
	case domain.EvidenceCompare:
		return e.compareEvidence(ctx, descriptor)
	default:
		return domain.IncompleteOutcome(reasonNotChangedFiles)
	}
}

// CurrentPullRequest captures a pull request's current base and head once and
// returns the immutable compare descriptor they define, so every comment on that
// pull request shares one evidence identity for the whole refresh. A failure or
// unchanged pull request comes back as a settled descriptor, so one caller shape
// handles both stages.
func (e Enricher) CurrentPullRequest(ctx context.Context, descriptor domain.EvidenceDescriptor) domain.EvidenceDescriptor {
	if descriptor.Operation() != domain.EvidencePullRequest {
		return domain.NewSettledEvidence(domain.IncompleteOutcome(reasonNotChangedFiles))
	}
	body, _, outcome, ok := e.read(ctx, e.pullRequestURL(descriptor))
	if !ok {
		return domain.NewSettledEvidence(outcome)
	}
	base, head, ok := currentPullRequestRefs(e.baseURL(), descriptor, body)
	if !ok {
		return domain.NewSettledEvidence(domain.IncompleteOutcome(reasonMalformedBody))
	}
	if base == head {
		// The pull request currently changes nothing. That is complete evidence,
		// and it still carries current-PR provenance because it was not read from
		// the objects the comment was written against.
		return domain.NewSettledEvidence(domain.CompleteOutcome(domain.ProvenanceCurrentPR, nil))
	}
	derived, err := domain.NewCompareEvidence(descriptor.Repository(), base, head, domain.ProvenanceCurrentPR)
	if err != nil {
		return domain.NewSettledEvidence(domain.IncompleteOutcome(reasonMalformedBody))
	}
	return derived
}

// read dispatches one bounded request and reports its body only for a success
// that the RG-003 status mapping admits. Otherwise it reports the terminal
// outcome for this evidence, so no caller sees a status, header, or body.
func (e Enricher) read(ctx context.Context, endpoint string) ([]byte, http.Header, domain.EvidenceOutcome, bool) {
	if ctx.Err() != nil {
		return nil, nil, domain.CanceledOutcome(), false
	}
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, domain.FailedOutcome(reasonRequestBuild), false
	}
	setDocumentedHeaders(request, e.Credential)

	response, err := e.client().Do(request)
	if err != nil {
		if outcome, canceled := cancellationOutcome(ctx, err); canceled {
			return nil, nil, outcome, false
		}
		return nil, nil, domain.FailedOutcome(reasonTransport), false
	}
	defer func() { _ = response.Body.Close() }()

	if outcome, failed := e.statusOutcome(response); failed {
		drainBody(response.Body)
		return nil, nil, outcome, false
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		drainBody(response.Body)
		if outcome, canceled := cancellationOutcome(ctx, err); canceled {
			return nil, nil, outcome, false
		}
		return nil, nil, domain.FailedOutcome(reasonUnreadableBody), false
	}
	return body, response.Header.Clone(), domain.EvidenceOutcome{}, true
}

// cancellationOutcome reports the canceled outcome when the parent refresh, not
// the request budget, ended the work. Cancellation publishes nothing and is
// never converted into a timeout failure.
func cancellationOutcome(ctx context.Context, err error) (domain.EvidenceOutcome, bool) {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return domain.CanceledOutcome(), true
	}
	return domain.EvidenceOutcome{}, false
}

// statusOutcome applies the closed RG-003 status mapping. It reports whether the
// response is already terminal for this evidence.
func (e Enricher) statusOutcome(response *http.Response) (domain.EvidenceOutcome, bool) {
	status := response.StatusCode
	if isRateLimited(status, response.Header, e.now()) {
		return domain.RateLimitedOutcome(e.now().Add(retryDelay(response.Header, true, e.now()))), true
	}
	switch {
	case status >= 200 && status < 300:
		return domain.EvidenceOutcome{}, false
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return domain.DeniedOutcome(reasonDenied), true
	case status == http.StatusNotFound, status == http.StatusConflict:
		return domain.UnavailableOutcome(reasonUnavailable), true
	case status >= 500:
		return domain.FailedOutcome(reasonServerFailure), true
	default:
		// Unexpected 3xx, an unsolicited 304, and 400/406/422 without rate
		// evidence are failed adapter or API operations. Redirects are never
		// followed, so a moved entity cannot silently change evidence identity.
		return domain.FailedOutcome(reasonUnexpectedReply), true
	}
}

// decode parses a response body into a private transport type, rejecting any
// unusable document. Unknown members are accepted; a wrong required type is not.
func decode(body []byte, into any) bool {
	return json.Unmarshal(body, into) == nil
}

func (e Enricher) commitURL(descriptor domain.EvidenceDescriptor) string {
	return e.repositoryURL(descriptor.Repository()) + "/commits/" + descriptor.Head() + "?per_page=" + perPage
}

func (e Enricher) compareURL(descriptor domain.EvidenceDescriptor) string {
	return e.repositoryURL(descriptor.Repository()) + "/compare/" + descriptor.Base() + "..." + descriptor.Head()
}

func (e Enricher) pullRequestURL(descriptor domain.EvidenceDescriptor) string {
	return e.repositoryURL(descriptor.Repository()) + "/pulls/" + strconv.Itoa(descriptor.Number())
}

func (e Enricher) repositoryURL(repository domain.Repository) string {
	return e.baseURL() + "/repos/" + url.PathEscape(repository.Owner()) + "/" + url.PathEscape(repository.Name())
}

func (e Enricher) baseURL() string {
	if e.BaseURL == "" {
		return defaultBaseURL
	}
	return strings.TrimSuffix(e.BaseURL, "/")
}

// client returns the request performer. The default never follows a redirect,
// because a redirected enrichment response could describe another entity.
func (e Enricher) client() Doer {
	if e.Client == nil {
		return &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	return e.Client
}

func (e Enricher) now() time.Time {
	if e.Now == nil {
		return time.Now()
	}
	return e.Now()
}
