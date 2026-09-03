package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
	"github.com/fmueller/orgtop/internal/enrichment"
)

// stubAdapter settles every descriptor at one scripted outcome.
type stubAdapter struct {
	outcome domain.EvidenceOutcome
	calls   int
}

func (a *stubAdapter) Changed(context.Context, domain.EvidenceDescriptor) domain.EvidenceOutcome {
	a.calls++
	return a.outcome
}

func (a *stubAdapter) CurrentPullRequest(_ context.Context, descriptor domain.EvidenceDescriptor) domain.EvidenceDescriptor {
	return descriptor
}

// commitEvent builds one push event whose evidence needs a commit lookup.
func commitEvent(t *testing.T, id, head string) domain.Event {
	t.Helper()
	repository, err := domain.ParseRepository("acme/backend")
	if err != nil {
		t.Fatalf("ParseRepository returned error: %v", err)
	}
	descriptor, err := domain.NewCommitEvidence(repository, head)
	if err != nil {
		t.Fatalf("NewCommitEvidence(%q) returned error: %v", head, err)
	}
	return domain.Event{
		ID:         id,
		OccurredAt: time.Date(2026, time.August, 22, 10, 0, 0, 0, time.UTC),
		Repository: repository,
		Category:   domain.CategoryPush,
		EntityKind: domain.EntityCommit,
		Evidence:   descriptor,
	}
}

// TestEnrichmentAdapterMapsSettledEvidenceOntoTheLifecycleSeam guards the
// boundary between the coordinator's ledger and the prepared application state:
// one outcome per input event plus the secondary conditions the refresh reports.
func TestEnrichmentAdapterMapsSettledEvidenceOntoTheLifecycleSeam(t *testing.T) {
	retryAt := time.Date(2026, time.August, 22, 10, 5, 0, 0, time.UTC)
	adapter := enrichmentAdapter{coordinator: enrichment.Coordinator{
		Adapter: &stubAdapter{outcome: domain.RateLimitedOutcome(retryAt)},
	}}

	evidence, err := adapter.Evidence(context.Background(), []domain.Event{commitEvent(t, "one", "0123456789abcdef0123456789abcdef01234567")})
	if err != nil {
		t.Fatalf("Evidence failed: %v", err)
	}
	if len(evidence.Outcomes) != 1 {
		t.Fatalf("Evidence returned %d outcomes, want 1", len(evidence.Outcomes))
	}
	if kind := evidence.Outcomes[0].Kind(); kind != domain.OutcomeRateLimited {
		t.Errorf("Evidence settled the event at %v, want rate-limited", kind)
	}
	if !evidence.RetryAt.Equal(retryAt) {
		t.Errorf("Evidence retry = %v, want %v", evidence.RetryAt, retryAt)
	}
}

// TestEnrichmentAdapterReportsAWiringFailure guards that a coordinator without
// an evidence adapter is reported rather than silently settling membership.
func TestEnrichmentAdapterReportsAWiringFailure(t *testing.T) {
	adapter := enrichmentAdapter{}

	if _, err := adapter.Evidence(context.Background(), nil); !errors.Is(err, enrichment.ErrNoAdapter) {
		t.Fatalf("Evidence error = %v, want %v", err, enrichment.ErrNoAdapter)
	}
}
