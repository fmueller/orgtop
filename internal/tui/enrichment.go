package tui

import (
	"context"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// The sanitized reasons the lifecycle settles evidence it never obtained at.
// Each one names the missing coordination and nothing about a credential, a
// request, or a response.
const (
	reasonNoEnricher = "no changed-file evidence coordination is wired"
	reasonNoOutcome  = "the enrichment reported no outcome for the event"
)

// Evidence is the settled changed-file evidence of one refresh and the
// secondary conditions its coordination reported. It carries no request,
// response, or pagination state: the adapter normalized all of that away.
type Evidence struct {
	// Outcomes holds exactly one settled outcome per input event, in input
	// order. A short result leaves the remaining events undecided rather than
	// deciding them, so membership stays unknown instead of not-member.
	Outcomes []domain.EvidenceOutcome
	// CacheDegraded is the sanitized cause of a skipped or failed enrichment
	// cache operation. A degraded cache never fails the refresh (RG-005).
	CacheDegraded string
	// RetryAt is the earliest instructed enrichment retry a rate limit named,
	// and stays zero when nothing was rate limited.
	RetryAt time.Time
}

// Enricher settles the changed-file evidence of one refresh's retained events
// outside the update and render path. Implementations bind their I/O to the
// passed context, coalesce shared evidence identities, and report every failure
// as an explicit outcome rather than by deciding membership.
type Enricher interface {
	Evidence(ctx context.Context, events []domain.Event) (Evidence, error)
}

// evidenceResult is the enrichment phase of one refresh attempt: the retained
// events paired with their settled evidence, the pre-enrichment truncation, and
// the secondary conditions the coordination reported.
type evidenceResult struct {
	retained  []domain.EventEvidence
	truncated bool
	degraded  string
	retryAt   time.Time
}

// enrich settles the evidence of the events one successful poll retains. The
// retained set is bounded before enrichment, so an event the FR-006 bound
// discarded causes no evidence work at all (A-028).
//
// A repository-only selection is decided from repository identity, so it
// consults no evidence and dispatches none. A path selection whose evidence
// never settled keeps every affected event undecided: a failed attempt settles
// the failure explicitly, and a missing coordination or a short result settles
// as incomplete. None of them is converted into member or not-member (FR-004).
func (m Model) enrich(ctx context.Context, scopes domain.ScopeSet, activities []domain.RepositoryActivity) evidenceResult {
	events, truncated := domain.Retain(scopes, activities)
	outcomes, degraded, retryAt := m.settle(ctx, scopes, events)

	retained := make([]domain.EventEvidence, 0, len(events))
	for index, event := range events {
		retained = append(retained, domain.EventEvidence{Event: event, Outcome: outcomes[index]})
	}
	return evidenceResult{retained: retained, truncated: truncated, degraded: degraded, retryAt: retryAt}
}

// settle returns one outcome per retained event plus the secondary conditions
// of the attempt. A selection needing no evidence settles every event at the
// zero outcome no Scope of it consults.
func (m Model) settle(ctx context.Context, scopes domain.ScopeSet, events []domain.Event) ([]domain.EvidenceOutcome, string, time.Time) {
	outcomes := make([]domain.EvidenceOutcome, len(events))
	if !scopes.HasPathScopes() {
		return outcomes, "", time.Time{}
	}
	if m.enricher == nil {
		return fill(outcomes, domain.IncompleteOutcome(reasonNoEnricher)), "", time.Time{}
	}

	evidence, err := m.enricher.Evidence(ctx, events)
	if err != nil {
		return fill(outcomes, domain.FailedOutcome(sanitize(err))), "", time.Time{}
	}
	// A short result settles only the events it reported on; the tail stays
	// undecided rather than being decided without evidence (FR-004).
	settled := copy(outcomes, evidence.Outcomes)
	for index := settled; index < len(outcomes); index++ {
		outcomes[index] = domain.IncompleteOutcome(reasonNoOutcome)
	}
	return outcomes, evidence.CacheDegraded, evidence.RetryAt
}

// fill settles every event of the attempt at one outcome.
func fill(outcomes []domain.EvidenceOutcome, outcome domain.EvidenceOutcome) []domain.EvidenceOutcome {
	for index := range outcomes {
		outcomes[index] = outcome
	}
	return outcomes
}
