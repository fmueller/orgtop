package enrichment_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/fmueller/orgtop/internal/cache"
	"github.com/fmueller/orgtop/internal/domain"
	"github.com/fmueller/orgtop/internal/enrichment"
)

// assertPaths reports the paths an outcome settled with, so a test can prove the
// event took the evidence acquired for it rather than an unwritten result slot.
// The label names which outcome is under test.
func assertPaths(t *testing.T, label string, outcome domain.EvidenceOutcome, want ...string) {
	t.Helper()
	got := make([]string, 0, len(outcome.Paths()))
	for _, path := range outcome.Paths() {
		got = append(got, path.String())
	}
	if !slices.Equal(got, want) {
		t.Errorf("%s paths = %v, want %v", label, got, want)
	}
}

// completeWith scripts one acquired changed-file result whose paths identify it.
func completeWith(t *testing.T, provenance domain.EvidenceProvenance, value string) domain.EvidenceOutcome {
	t.Helper()
	return domain.CompleteOutcome(provenance, []domain.ChangedPath{changedPath(t, value)})
}

// derivedCompare builds the compare that a current-PR metadata read resolves to.
func derivedCompare(t *testing.T, base, head string) domain.EvidenceDescriptor {
	t.Helper()
	descriptor, err := domain.NewCompareEvidence(repository(t), base, head, domain.ProvenanceCurrentPR)
	if err != nil {
		t.Fatalf("building the derived compare failed: %v", err)
	}
	return descriptor
}

// TestStoredEvidenceCarriesTheRefreshClock guards the RG-009 cache reuse rule
// that a stored record is validated against the refresh's own fixed clock: a
// replacement is acquired and last used at the coordinator's supplied time, so
// its later expiry is deterministic rather than dependent on the wall clock at
// the moment of the write.
func TestStoredEvidenceCarriesTheRefreshClock(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()

	result, err := coordinator(adapter, store).Evidence(t.Context(),
		[]domain.Event{compareEvent(t, "a", baseA, headA)})
	if err != nil {
		t.Fatalf("coordinating a stored replacement failed: %v", err)
	}
	if result.Ledger.CacheWrites != 1 {
		t.Fatalf("cache writes = %d, want 1", result.Ledger.CacheWrites)
	}
	stored := store.entries[headA]
	if !stored.AcquiredAt.Equal(fixedNow) {
		t.Errorf("stored acquired at = %v, want the refresh clock %v", stored.AcquiredAt, fixedNow)
	}
	if !stored.LastUsedAt.Equal(fixedNow) {
		t.Errorf("stored last used at = %v, want the refresh clock %v", stored.LastUsedAt, fixedNow)
	}
}

// TestDuplicateEvidenceStillEnrichesTheLaterDistinctIdentity guards the NFR-002
// coalescing rule that duplicate work is dropped and nothing else is: an event
// repeating an identity already admitted costs no second request, and a later
// event naming a different identity is still enriched.
func TestDuplicateEvidenceStillEnrichesTheLaterDistinctIdentity(t *testing.T) {
	adapter := &fakeAdapter{changed: map[string]domain.EvidenceOutcome{}}
	store := newFakeCache()
	first := compareEvent(t, "a", baseA, headA)
	last := compareEvent(t, "c", baseA, headB)
	adapter.changed[last.Evidence.Key()] = completeWith(t, domain.ProvenanceEventTime, "internal/last.go")

	result, err := coordinator(adapter, store).Evidence(t.Context(),
		[]domain.Event{first, compareEvent(t, "b", baseA, headA), last})
	if err != nil {
		t.Fatalf("coordinating a repeated identity failed: %v", err)
	}
	if result.Ledger.Requests != 2 {
		t.Errorf("requests = %d, want 2 for the two distinct identities", result.Ledger.Requests)
	}
	if got := len(adapter.dispatched()); got != 2 {
		t.Errorf("dispatched = %v, want both distinct identities", adapter.dispatched())
	}
	assertPaths(t, "the last outcome", result.Outcomes[2], "internal/last.go")
}

// TestSettledEvidenceDoesNotStopLaterEventsFromSettling guards the FR contract
// that every retained event receives exactly one outcome in input order: an
// event whose evidence needs no coordination at all does not deprive a later
// event of the evidence acquired for it.
func TestSettledEvidenceDoesNotStopLaterEventsFromSettling(t *testing.T) {
	adapter := &fakeAdapter{changed: map[string]domain.EvidenceOutcome{}}
	store := newFakeCache()
	enriched := compareEvent(t, "b", baseA, headA)
	adapter.changed[enriched.Evidence.Key()] = completeWith(t, domain.ProvenanceEventTime, "internal/app.go")
	events := []domain.Event{
		{ID: "a", Evidence: domain.NewDirectEvidence(changedPath(t, "cmd/main.go"))},
		enriched,
	}

	result, err := coordinator(adapter, store).Evidence(t.Context(), events)
	if err != nil {
		t.Fatalf("coordinating settled evidence before enriched evidence failed: %v", err)
	}
	assertPaths(t, "the settled outcome", result.Outcomes[0], "cmd/main.go")
	assertPaths(t, "the enriched outcome", result.Outcomes[1], "internal/app.go")
}

// TestCachedIdentityDoesNotStopLaterDispatch guards the NFR-002 rule that cache
// reuse only removes the work it satisfies: an identity served from the store
// costs no request, and the next identity is still dispatched to GitHub.
func TestCachedIdentityDoesNotStopLaterDispatch(t *testing.T) {
	adapter := &fakeAdapter{changed: map[string]domain.EvidenceOutcome{}}
	store := newFakeCache()
	hit := compareEvent(t, "a", baseA, headA)
	key, ok := cache.KeyFor(hit.Evidence)
	if !ok {
		t.Fatal("compare evidence has no cache key")
	}
	store.entries[headA] = cache.Entry{
		Key:        key,
		Paths:      []domain.ChangedPath{changedPath(t, "internal/cached.go")},
		AcquiredAt: fixedNow,
		LastUsedAt: fixedNow,
	}
	miss := compareEvent(t, "b", baseA, headB)
	adapter.changed[miss.Evidence.Key()] = completeWith(t, domain.ProvenanceEventTime, "internal/fetched.go")

	result, err := coordinator(adapter, store).Evidence(t.Context(), []domain.Event{hit, miss})
	if err != nil {
		t.Fatalf("coordinating a hit before a miss failed: %v", err)
	}
	if result.Ledger.CacheHits != 1 || result.Ledger.Requests != 1 {
		t.Errorf("ledger = %+v, want one reused identity and one requested identity", result.Ledger)
	}
	assertPaths(t, "the reused outcome", result.Outcomes[0], "internal/cached.go")
	assertPaths(t, "the requested outcome", result.Outcomes[1], "internal/fetched.go")
}

// TestEveryUnitBeyondTheRequestBudgetSettlesIncomplete guards the RG-009 rule
// that a spent request budget makes path membership unknown rather than empty:
// every unit the budget could not pay for settles incomplete, not only the
// first one to find the budget spent.
func TestEveryUnitBeyondTheRequestBudgetSettlesIncomplete(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()
	service := bounded(adapter, store, func(bounds *enrichment.Bounds) { bounds.Requests = 1 })

	result, err := service.Evidence(t.Context(), []domain.Event{
		compareEvent(t, "a", baseA, headA),
		compareEvent(t, "b", baseA, headB),
		compareEvent(t, "c", parentA, headB),
	})
	if err != nil {
		t.Fatalf("coordinating a spent budget failed: %v", err)
	}
	if result.Ledger.Requests != 1 {
		t.Errorf("requests = %d, want 1", result.Ledger.Requests)
	}
	for _, index := range []int{1, 2} {
		if got := result.Outcomes[index].Kind(); got != domain.OutcomeIncomplete {
			t.Errorf("outcome %d = %v, want incomplete so its path membership stays unknown", index, got)
		}
	}
}

// TestPullRequestCompareIsDerivedAfterASettledIdentity guards the two-stage
// admission order of RG-009: a first-stage identity that settles on its own does
// not prevent the current-PR metadata behind it from deriving and enriching its
// compare.
func TestPullRequestCompareIsDerivedAfterASettledIdentity(t *testing.T) {
	adapter := &fakeAdapter{
		changed:  map[string]domain.EvidenceOutcome{},
		metadata: map[string]domain.EvidenceDescriptor{},
	}
	store := newFakeCache()
	metadata := pullRequestEvent(t, "b", 42)
	derived := derivedCompare(t, baseA, headB)
	adapter.metadata[metadata.Evidence.Key()] = derived
	adapter.changed[derived.Key()] = completeWith(t, domain.ProvenanceCurrentPR, "internal/derived.go")

	result, err := coordinator(adapter, store).Evidence(t.Context(),
		[]domain.Event{compareEvent(t, "a", baseA, headA), metadata})
	if err != nil {
		t.Fatalf("coordinating a derived compare failed: %v", err)
	}
	if result.Ledger.Requests != 3 {
		t.Errorf("requests = %d, want the direct compare, the metadata, and its derived compare", result.Ledger.Requests)
	}
	assertPaths(t, "the pull request outcome", result.Outcomes[1], "internal/derived.go")
}

// TestRepeatedDerivedCompareStillEnrichesTheLaterOne guards the NFR-002
// coalescing rule inside the second admission stage: two pull requests whose
// metadata derives one shared compare cost a single compare request, and a third
// pull request naming a different compare is still enriched.
func TestRepeatedDerivedCompareStillEnrichesTheLaterOne(t *testing.T) {
	adapter := &fakeAdapter{
		changed:  map[string]domain.EvidenceOutcome{},
		metadata: map[string]domain.EvidenceDescriptor{},
	}
	store := newFakeCache()
	shared := derivedCompare(t, baseA, headA)
	distinct := derivedCompare(t, baseA, headB)
	events := []domain.Event{
		pullRequestEvent(t, "a", 1),
		pullRequestEvent(t, "b", 2),
		pullRequestEvent(t, "c", 3),
	}
	adapter.metadata[events[0].Evidence.Key()] = shared
	adapter.metadata[events[1].Evidence.Key()] = shared
	adapter.metadata[events[2].Evidence.Key()] = distinct
	adapter.changed[shared.Key()] = completeWith(t, domain.ProvenanceCurrentPR, "internal/shared.go")
	adapter.changed[distinct.Key()] = completeWith(t, domain.ProvenanceCurrentPR, "internal/distinct.go")

	result, err := coordinator(adapter, store).Evidence(t.Context(), events)
	if err != nil {
		t.Fatalf("coordinating repeated derived compares failed: %v", err)
	}
	if result.Ledger.Requests != 5 {
		t.Errorf("requests = %d, want three metadata reads and two distinct compares", result.Ledger.Requests)
	}
	for _, index := range []int{0, 1} {
		assertPaths(t, fmt.Sprintf("outcome %d", index), result.Outcomes[index], "internal/shared.go")
	}
	assertPaths(t, "the last outcome", result.Outcomes[2], "internal/distinct.go")
}

// TestOrdinaryCacheMissesAreNotReportedAsDegradation guards the FR-011
// disclosure rule that the refresh reports cache degradation only when the cache
// actually failed it: an expired record and a record that failed its own row
// invariant are misses the store resolves itself, so the refresh refetches from
// GitHub and discloses no degraded cache.
func TestOrdinaryCacheMissesAreNotReportedAsDegradation(t *testing.T) {
	for name, cause := range map[string]error{
		"expired": cache.ErrExpired,
		"invalid": cache.ErrInvalidRecord,
	} {
		t.Run(name, func(t *testing.T) {
			adapter := &fakeAdapter{}
			store := newFakeCache()
			store.lookupErr = cause

			result, err := coordinator(adapter, store).Evidence(t.Context(),
				[]domain.Event{compareEvent(t, "a", baseA, headA)})
			if err != nil {
				t.Fatalf("coordinating an ordinary miss failed: %v", err)
			}
			if result.Ledger.CacheDegraded != "" {
				t.Errorf("cache degradation = %q, want an ordinary miss reported as no degradation",
					result.Ledger.CacheDegraded)
			}
			if result.Ledger.Requests != 1 {
				t.Errorf("requests = %d, want the refresh to refetch from github", result.Ledger.Requests)
			}
			if !result.Outcomes[0].IsComplete() {
				t.Errorf("outcome = %v, want complete from github", result.Outcomes[0].Kind())
			}
		})
	}
}

// TestFirstCacheDegradationCauseIsTheOneReported guards the FR-011 disclosure
// rule that one refresh discloses one cache cause: when several cache operations
// fail, the earliest one is reported, because it is the failure that explains
// the ones after it.
func TestFirstCacheDegradationCauseIsTheOneReported(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()
	store.lookupErr = fmt.Errorf("%w: the read came first", cache.ErrContended)
	store.touchErr = fmt.Errorf("%w: the touch came later", cache.ErrContended)

	result, err := coordinator(adapter, store).Evidence(t.Context(),
		[]domain.Event{compareEvent(t, "a", baseA, headA)})
	if err != nil {
		t.Fatalf("coordinating a repeatedly degraded cache failed: %v", err)
	}
	cause := result.Ledger.CacheDegraded
	if !strings.Contains(cause, "the read came first") {
		t.Errorf("cache degradation = %q, want the earliest cause of the refresh", cause)
	}
	if strings.Contains(cause, "the touch came later") {
		t.Errorf("cache degradation = %q, want a later cause not to replace the first one", cause)
	}
	if !result.Outcomes[0].IsComplete() {
		t.Errorf("outcome = %v, want a degraded cache never to fail the refresh", result.Outcomes[0].Kind())
	}
}
