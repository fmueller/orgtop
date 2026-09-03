package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// expansionRequests is the closed RG-009 organization-listing share of one
// refresh. Together with the documented pageSize it bounds the listed records
// one attempt admits.
const expansionRequests = 5

// organizationOwnerType is the only owner type expansion admits. A listed
// repository owned by a user is malformed rather than silently skipped.
const organizationOwnerType = "Organization"

// Failure classes a bounded organization expansion reports. None of them
// carries a credential value or an authenticated request header (NFR-003).
var (
	// ErrInvalidSelector reports an organization selector the adapter cannot
	// request a listing for.
	ErrInvalidSelector = errors.New("invalid organization selector")

	// ErrInvalidListing reports a 2xx listing page that cannot be read: an
	// unusable body, an oversized page, a malformed record, or a duplicate
	// record with conflicting required facts.
	ErrInvalidListing = errors.New("invalid github organization listing")

	// ErrInvalidPagination reports a next page link OrgTop must not follow.
	ErrInvalidPagination = errors.New("invalid github organization listing pagination")

	// ErrOrganizationNotFound reports an organization that does not exist or is
	// inaccessible to the credential.
	ErrOrganizationNotFound = errors.New("organization not found or inaccessible")

	// ErrOrganizationAccessDenied reports a denied listing that is not rate
	// limiting.
	ErrOrganizationAccessDenied = errors.New("github denied access to the organization")
)

// ExpansionRequest is one bounded expansion input. Organizations holds the
// validated, deduplicated selectors in their first-request order and their
// retained spelling; Exact is the canonicalized selection that has
// unconditional capacity precedence and may be empty for organization-only
// input (RG-010).
type ExpansionRequest struct {
	Organizations   []string
	Exact           domain.ScopeSet
	IncludeArchived bool
	IncludeForks    bool
}

// Expansion is one completely successful bounded expansion. Scopes carries the
// exact selection followed by the expansion-created repository Scopes in their
// deterministic allocation order, so downstream code receives ordinary
// canonical Scopes and no organization model.
type Expansion struct {
	Scopes    domain.ScopeSet
	Selection Selection
	// Requests is the number of listing requests the attempt spent.
	Requests int
}

// Selection is the prepared disclosure and provenance state of one expansion.
// It never changes Scope identity or behavior.
type Selection struct {
	// ExactScopes and ExpandedScopes partition TotalScopes.
	ExactScopes    int
	ExpandedScopes int
	TotalScopes    int
	// DistinctRepositories is the number of distinct repositories the published
	// Scopes reference.
	DistinctRepositories int
	// Selectors reports one entry per selector in first-request order.
	Selectors []SelectorSelection
	// Provenance carries one entry per published Scope, in Scope order.
	Provenance []ScopeProvenance
	// PaginationRemains is the derived global condition that at least one
	// selector left a valid next page link unconsumed, so further eligible
	// repositories may exist beyond the disclosed omissions.
	PaginationRemains bool
}

// SelectorSelection is the per-selector disclosure state. The disjoint Disabled,
// Archived, and Fork exclusion buckets plus Eligible sum to Listed, and Retained
// plus Omitted sum to Eligible.
type SelectorSelection struct {
	Organization string
	Listed       int
	Disabled     int
	Archived     int
	Fork         int
	Eligible     int
	Retained     int
	Omitted      int
	// HasMore reports an unconsumed valid next page link.
	HasMore bool
}

// ScopeProvenance records how one published Scope was selected. Exact marks a
// Scope the invocation named directly; Selector names the contributing
// organization selector, including for an exact Scope an expansion also reached.
type ScopeProvenance struct {
	Scope    domain.Scope
	Exact    bool
	Selector string
}

// ExpansionError reports the sanitized failure that failed one whole expansion
// attempt. No valid-looking partial selector set is published with it.
type ExpansionError struct {
	// Organization is the selector whose listing failed.
	Organization string
	// RetryDelay is the delay before the next attempt is eligible.
	RetryDelay time.Duration
	// Requests is the number of listing requests the failed attempt spent. A
	// dispatch spends budget before the request and cancellation refunds none.
	Requests int

	cause error
}

// Error implements error with a sanitized cause.
func (e *ExpansionError) Error() string {
	return fmt.Sprintf("expanding %s: %v", e.Organization, e.cause)
}

// Unwrap exposes the failure class for errors.Is.
func (e *ExpansionError) Unwrap() error { return e.cause }

// Expand lists every selector's organization within the closed request budget
// and expands the result into canonical repository Scopes. It is atomic: the
// first failing listing fails the whole attempt and publishes nothing. An
// expansion that finds no eligible repository is an explicit empty success, and
// a request without selectors keeps the exact selection and spends nothing.
func (s Source) Expand(ctx context.Context, request ExpansionRequest) (Expansion, error) {
	listings, err := newListings(s.baseURL(), request.Organizations)
	if err != nil {
		return Expansion{}, err
	}
	spent, err := s.list(ctx, listings)
	if err != nil {
		return Expansion{}, err
	}
	expansion, err := allocate(request, listings)
	if err != nil {
		return Expansion{}, err
	}
	expansion.Requests = spent
	return expansion, nil
}

// listing is the mutable per-selector state of one expansion attempt.
type listing struct {
	organization string
	// url is the next page to request and is empty once the list ended. A
	// non-empty url after the budget is spent is the disclosed remainder.
	url string
	// page is the page number url names.
	page    int
	records []listingRecord
	byKey   map[string]int
}

// listingRecord is one validated listing record after duplicate collapse.
type listingRecord struct {
	repository domain.Repository
	archived   bool
	disabled   bool
	fork       bool
}

// sameFacts reports whether two records of one repository agree on every
// required eligibility fact.
func (r listingRecord) sameFacts(other listingRecord) bool {
	return r.archived == other.archived && r.disabled == other.disabled && r.fork == other.fork
}

// newListings validates the selectors and prepares each one's first page
// request. The CLI already validated and deduplicated them; repeating both here
// keeps the adapter safe for any caller.
func newListings(root string, organizations []string) ([]*listing, error) {
	listings := make([]*listing, 0, len(organizations))
	seen := make(map[string]struct{}, len(organizations))
	for _, organization := range organizations {
		if err := domain.ValidateOwner(organization); err != nil {
			return nil, fmt.Errorf("%w %q: %w", ErrInvalidSelector, organization, err)
		}
		key := strings.ToLower(organization)
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("%w %q: the organization is already selected", ErrInvalidSelector, organization)
		}
		seen[key] = struct{}{}
		listings = append(listings, &listing{
			organization: organization,
			url:          listingURL(root, organization, 1),
			page:         1,
			byKey:        make(map[string]int),
		})
	}
	return listings, nil
}

// list fetches listing pages until the request budget is spent or every list
// ended. Distinct selectors receive page one in first-request order and the
// remaining slots follow one next link per selector round, skipping exhausted
// selectors.
func (s Source) list(ctx context.Context, listings []*listing) (int, error) {
	spent := 0
	for {
		pending := false
		for _, entry := range listings {
			if entry.url == "" {
				continue
			}
			if spent >= expansionRequests {
				return spent, nil
			}
			pending = true
			// The dispatch spends its budget before the request, so a failed or
			// cancelled page is never refunded.
			spent++
			if retry, err := s.fetchListingPage(ctx, entry); err != nil {
				return spent, &ExpansionError{
					Organization: entry.organization,
					RetryDelay:   retry,
					Requests:     spent,
					cause:        err,
				}
			}
		}
		if !pending {
			return spent, nil
		}
	}
}

// fetchListingPage retrieves, validates, and records one listing page and
// resolves the next page the selector may follow. A failed page reports the
// delay before the next attempt is eligible alongside its sanitized cause.
func (s Source) fetchListingPage(ctx context.Context, entry *listing) (time.Duration, error) {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	response, err := s.send(requestCtx, entry.url)
	if err != nil {
		return s.listingRetryDelay(http.Header{}, false), err
	}
	defer func() { _ = response.Body.Close() }()

	rateLimited, statusErr := s.listingStatusError(response)
	if statusErr != nil {
		drainBody(response.Body)
		return s.listingRetryDelay(response.Header, rateLimited), statusErr
	}
	retry := s.listingRetryDelay(response.Header, false)

	body, err := io.ReadAll(response.Body)
	if err != nil {
		drainBody(response.Body)
		return retry, fmt.Errorf("%w: reading the response failed", ErrTransport)
	}

	records, err := decodeListingPage(entry.organization, body)
	if err != nil {
		return retry, err
	}
	if err := entry.add(records); err != nil {
		return retry, err
	}

	next, err := s.nextListingPage(response.Header, entry)
	if err != nil {
		return retry, err
	}
	entry.url = next
	entry.page++
	return 0, nil
}

// listingRetryDelay is the delay before a failed listing attempt may be retried.
func (s Source) listingRetryDelay(header http.Header, rateLimited bool) time.Duration {
	return retryDelay(header, rateLimited, s.now())
}

// listingStatusError classifies a non-2xx listing response. It shares the RG-003
// status mapping with repository polling and only names the organization rather
// than a repository for the two identity-bearing classes.
func (s Source) listingStatusError(response *http.Response) (rateLimited bool, cause error) {
	rateLimited, cause = s.statusError(response)
	switch {
	case errors.Is(cause, ErrNotFound):
		return rateLimited, ErrOrganizationNotFound
	case errors.Is(cause, ErrAccessDenied):
		return rateLimited, ErrOrganizationAccessDenied
	default:
		return rateLimited, cause
	}
}

// add records the page's records, collapsing a consistent duplicate onto its
// first fetched occurrence and failing the attempt on a conflicting one.
func (l *listing) add(records []listingRecord) error {
	for _, record := range records {
		key := record.repository.Key()
		if index, duplicate := l.byKey[key]; duplicate {
			if !l.records[index].sameFacts(record) {
				return fmt.Errorf("%w: %s is listed twice with conflicting facts", ErrInvalidListing, key)
			}
			continue
		}
		l.byKey[key] = len(l.records)
		l.records = append(l.records, record)
	}
	return nil
}
