package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// fakeEnricher is the injected evidence seam under test. It is deterministic:
// no clock, no network, and no goroutine of its own.
type fakeEnricher struct {
	// outcomes maps an event ID to the evidence the refresh settles it at.
	outcomes map[string]domain.EvidenceOutcome
	// evidence is returned beside the mapped outcomes.
	evidence Evidence
	// err fails the whole enrichment attempt.
	err error
	// calls counts started enrichments.
	calls int
	// events records the exact event set every attempt was given.
	events [][]domain.Event
	// contexts records the context of every attempt.
	contexts []context.Context
}

func (e *fakeEnricher) Evidence(ctx context.Context, events []domain.Event) (Evidence, error) {
	e.calls++
	e.contexts = append(e.contexts, ctx)
	e.events = append(e.events, events)
	if e.err != nil {
		return Evidence{}, e.err
	}
	evidence := e.evidence
	evidence.Outcomes = make([]domain.EvidenceOutcome, 0, len(events))
	for _, event := range events {
		evidence.Outcomes = append(evidence.Outcomes, e.outcomes[event.ID])
	}
	return evidence, nil
}

// mustChangedPaths builds the validated changed paths of a complete outcome.
func mustChangedPaths(t *testing.T, values ...string) []domain.ChangedPath {
	t.Helper()
	paths := make([]domain.ChangedPath, 0, len(values))
	for _, value := range values {
		path, err := domain.NewChangedPath(value)
		if err != nil {
			t.Fatalf("NewChangedPath(%q) returned error: %v", value, err)
		}
		paths = append(paths, path)
	}
	return paths
}

// completeEvidence returns event-time evidence proving the changed paths.
func completeEvidence(t *testing.T, values ...string) domain.EvidenceOutcome {
	t.Helper()
	return domain.CompleteOutcome(domain.ProvenanceEventTime, mustChangedPaths(t, values...))
}

// mixedScope returns a selection holding the repository Scope and one path
// Scope of the same repository.
func mixedScope(t *testing.T, repo string, segment string) (domain.ScopeSet, domain.Scope) {
	t.Helper()
	repository := testRepository(t, repo)
	matcher, err := domain.NewPathMatcher([]domain.MatcherToken{
		domain.LiteralToken(segment), domain.SeparatorToken(), domain.RecursiveToken(),
	})
	if err != nil {
		t.Fatalf("NewPathMatcher(%q) returned error: %v", segment, err)
	}
	path, err := domain.NewPathScope(repository, matcher)
	if err != nil {
		t.Fatalf("NewPathScope(%q) returned error: %v", repo, err)
	}
	scopes, err := domain.NewScopeSet([]domain.Scope{domain.NewRepositoryScope(repository), path})
	if err != nil {
		t.Fatalf("NewScopeSet(%q) returned error: %v", repo, err)
	}
	return scopes, path
}

// enrichedLifecycle builds a deterministic model over a mixed selection with the
// evidence seam bound.
func enrichedLifecycle(t *testing.T, scopes domain.ScopeSet, source Source, enricher Enricher, timer *recorder) Model {
	t.Helper()
	model, err := New(context.Background(), scopes, source, WithEnricher(enricher))
	if err != nil {
		t.Fatalf("building the enriched lifecycle model failed: %v", err)
	}
	model.now = func() time.Time { return fixedInstant }
	model.tick = timer.tick
	return model
}

// aggregateOf returns the published row of one Scope identity.
func aggregateOf(t *testing.T, state State, scope domain.Scope) domain.ScopeAggregate {
	t.Helper()
	for _, aggregate := range state.Scoped.Aggregates() {
		if aggregate.Scope.Identity() == scope.Identity() {
			return aggregate
		}
	}
	t.Fatalf("no published aggregate for %q", scope)
	return domain.ScopeAggregate{}
}

// TestRefreshEvaluatesMembershipFromEnrichedEvidence guards the RG-004
// publication contract: one refresh settles evidence before aggregation and
// publishes prepared per-Scope membership.
func TestRefreshEvaluatesMembershipFromEnrichedEvidence(t *testing.T) {
	scopes, path := mixedScope(t, "acme/backend", "services")
	result := activity(t, "acme/backend")
	id := result.Repositories[0].Events[0].ID
	enricher := &fakeEnricher{outcomes: map[string]domain.EvidenceOutcome{
		id: completeEvidence(t, "services/api/main.go"),
	}}
	source := &fakeSource{outcomes: []outcome{{result: result}}}
	model := enrichedLifecycle(t, scopes, source, enricher, &recorder{})

	updated, _ := run(t, model, model.Init())
	state := updated.state

	if enricher.calls != 1 {
		t.Fatalf("the refresh started %d enrichments, want 1", enricher.calls)
	}
	if len(enricher.events) != 1 || len(enricher.events[0]) != 1 || enricher.events[0][0].ID != id {
		t.Fatalf("the refresh enriched %v, want the retained event %q", enricher.events, id)
	}
	if got := aggregateOf(t, state, path).Activity; got != 1 {
		t.Fatalf("the path Scope published %d activity, want 1", got)
	}
	if state.Freshness != FreshnessCurrent {
		t.Fatalf("the refresh published freshness %v, want current", state.Freshness)
	}
}

// TestRepositoryOnlyRefreshPerformsNoEnrichment guards the v0.1 path: a
// selection without a path Scope needs no changed-file evidence and still
// publishes a coherent prepared snapshot.
func TestRepositoryOnlyRefreshPerformsNoEnrichment(t *testing.T) {
	repositoryScope := testScope(t, "acme/backend")
	enricher := &fakeEnricher{}
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	model := enrichedLifecycle(t, repositoryScope, source, enricher, &recorder{})

	updated, _ := run(t, model, model.Init())
	state := updated.state

	if enricher.calls != 0 {
		t.Fatalf("a repository-only refresh started %d enrichments, want 0", enricher.calls)
	}
	if got := len(state.Scoped.Events()); got != 1 {
		t.Fatalf("a repository-only refresh published %d scoped events, want 1", got)
	}
	if got := state.Scoped.DistinctActivity(); got != 1 {
		t.Fatalf("a repository-only refresh published %d distinct activity, want 1", got)
	}
}

// TestPathScopeWithoutAnEnricherStaysUnknown guards FR-004: absent evidence is
// never silently converted into not-member.
func TestPathScopeWithoutAnEnricherStaysUnknown(t *testing.T) {
	scopes, path := mixedScope(t, "acme/backend", "services")
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	model, err := New(context.Background(), scopes, source)
	if err != nil {
		t.Fatalf("building the model failed: %v", err)
	}
	model.now = func() time.Time { return fixedInstant }
	model.tick = (&recorder{}).tick

	updated, _ := run(t, model, model.Init())
	row := aggregateOf(t, updated.state, path)

	if row.Unknown != 1 || row.NotMember != 0 || row.Activity != 0 {
		t.Fatalf("an unenriched path Scope published %+v, want one unknown", row)
	}
}

// TestFailedEnrichmentKeepsTheSourceSnapshotCurrent guards RG-004: a successful
// source poll with unknown enrichment publishes current repository data rather
// than a stale or empty snapshot.
func TestFailedEnrichmentKeepsTheSourceSnapshotCurrent(t *testing.T) {
	scopes, path := mixedScope(t, "acme/backend", "services")
	repository := domain.NewRepositoryScope(testRepository(t, "acme/backend"))
	enricher := &fakeEnricher{err: errors.New("evidence transport failed")}
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	model := enrichedLifecycle(t, scopes, source, enricher, &recorder{})

	updated, _ := run(t, model, model.Init())
	state := updated.state

	if state.Freshness != FreshnessCurrent {
		t.Fatalf("failed enrichment published freshness %v, want current", state.Freshness)
	}
	if state.Cause != "" {
		t.Fatalf("failed enrichment published the source cause %q, want none", state.Cause)
	}
	if got := aggregateOf(t, state, repository).Activity; got != 1 {
		t.Fatalf("the repository Scope published %d activity, want 1", got)
	}
	row := aggregateOf(t, state, path)
	if row.Unknown != 1 || row.UnknownBy(domain.ReasonFailed) != 1 {
		t.Fatalf("failed enrichment published %+v, want one failed unknown", row)
	}
}

// TestEnrichmentDegradationAndRateLimitReachPreparedState guards RG-004's
// combined state: cache degradation and an enrichment rate limit are published
// beside current source data instead of failing the refresh.
func TestEnrichmentDegradationAndRateLimitReachPreparedState(t *testing.T) {
	scopes, _ := mixedScope(t, "acme/backend", "services")
	retryAt := fixedInstant.Add(90 * time.Second)
	enricher := &fakeEnricher{evidence: Evidence{CacheDegraded: "the cache is read only", RetryAt: retryAt}}
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	model := enrichedLifecycle(t, scopes, source, enricher, &recorder{})

	updated, _ := run(t, model, model.Init())
	state := updated.state

	if state.CacheDegraded != "the cache is read only" {
		t.Fatalf("the refresh published cache degradation %q, want the reported cause", state.CacheDegraded)
	}
	if !state.EnrichmentRetryAt.Equal(retryAt) {
		t.Fatalf("the refresh published retry %v, want %v", state.EnrichmentRetryAt, retryAt)
	}
	if state.Freshness != FreshnessCurrent {
		t.Fatalf("degraded enrichment published freshness %v, want current", state.Freshness)
	}
}

// TestLaterSuccessRecoversUnknownMembershipAndClearsDegradation guards the
// RG-004 recovery contract: a successful retry replaces the affected outcome,
// recomputes every dependent Scope, and clears only the recovered causes.
func TestLaterSuccessRecoversUnknownMembershipAndClearsDegradation(t *testing.T) {
	scopes, path := mixedScope(t, "acme/backend", "services")
	result := activity(t, "acme/backend")
	id := result.Repositories[0].Events[0].ID
	enricher := &fakeEnricher{err: errors.New("evidence transport failed")}
	source := &fakeSource{outcomes: []outcome{{result: result}}}
	timer := &recorder{}
	model := enrichedLifecycle(t, scopes, source, enricher, timer)

	updated, cmd := run(t, model, model.Init())
	if got := aggregateOf(t, updated.state, path).Unknown; got != 1 {
		t.Fatalf("the first refresh published %d unknown, want 1", got)
	}

	enricher.err = nil
	enricher.outcomes = map[string]domain.EvidenceOutcome{id: completeEvidence(t, "services/api/main.go")}
	enricher.evidence = Evidence{}
	pending, next := run(t, updated, cmd)
	recovered, _ := run(t, pending, next)
	state := recovered.state

	row := aggregateOf(t, state, path)
	if row.Activity != 1 || row.Unknown != 0 {
		t.Fatalf("the recovered refresh published %+v, want one member and no unknown", row)
	}
	if state.CacheDegraded != "" || !state.EnrichmentRetryAt.IsZero() {
		t.Fatalf("the recovered refresh kept degradation %q / retry %v", state.CacheDegraded, state.EnrichmentRetryAt)
	}
}

// TestCanceledEvidencePublishesNoMembership guards RG-004: canceled partial work
// is never synthesized into unknown membership.
func TestCanceledEvidencePublishesNoMembership(t *testing.T) {
	scopes, _ := mixedScope(t, "acme/backend", "services")
	result := activity(t, "acme/backend")
	id := result.Repositories[0].Events[0].ID
	enricher := &fakeEnricher{outcomes: map[string]domain.EvidenceOutcome{id: domain.CanceledOutcome()}}
	source := &fakeSource{outcomes: []outcome{{result: result}}}
	model := enrichedLifecycle(t, scopes, source, enricher, &recorder{})

	updated, _ := run(t, model, model.Init())
	state := updated.state

	if got := len(state.Scoped.Events()); got != 0 {
		t.Fatalf("canceled evidence published %d scoped events, want 0", got)
	}
}

// TestHeaderNeverLabelsPollingAsLive guards the FR-007 transport label: the
// Events API is near-current, so no published state may claim a live feed.
func TestHeaderNeverLabelsPollingAsLive(t *testing.T) {
	scopes, _ := mixedScope(t, "acme/backend", "services")
	result := activity(t, "acme/backend")
	id := result.Repositories[0].Events[0].ID
	enricher := &fakeEnricher{outcomes: map[string]domain.EvidenceOutcome{
		id: completeEvidence(t, "services/api/main.go"),
	}}
	source := &fakeSource{outcomes: []outcome{{result: result}}}
	model := enrichedLifecycle(t, scopes, source, enricher, &recorder{})

	updated, _ := run(t, model, model.Init())
	rendered := updated.render()

	if strings.Contains(strings.ToUpper(rendered), "LIVE") {
		t.Fatalf("the published header claims a live feed:\n%s", rendered)
	}
	if !strings.Contains(rendered, transportLabel) {
		t.Fatalf("the published header dropped the polling label:\n%s", rendered)
	}
}
