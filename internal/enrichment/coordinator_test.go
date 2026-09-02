package enrichment_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode"

	"github.com/fmueller/orgtop/internal/cache"
	"github.com/fmueller/orgtop/internal/domain"
	"github.com/fmueller/orgtop/internal/enrichment"
)

// fixedNow is the deterministic refresh clock every test coordinates against.
var fixedNow = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

const (
	headA   = "1111111111111111111111111111111111111111"
	headB   = "2222222222222222222222222222222222222222"
	baseA   = "3333333333333333333333333333333333333333"
	parentA = "4444444444444444444444444444444444444444"
)

// fakeAdapter answers evidence work from a scripted table and records the
// dispatch order, the request count, and the peak concurrency it observed.
type fakeAdapter struct {
	changed  map[string]domain.EvidenceOutcome
	metadata map[string]domain.EvidenceDescriptor
	// block gates every Changed call so a test can prove the concurrency bound.
	block chan struct{}

	mu       sync.Mutex
	requests []string
	inFlight int64
	peak     int64
}

func (f *fakeAdapter) Changed(ctx context.Context, descriptor domain.EvidenceDescriptor) domain.EvidenceOutcome {
	f.enter(descriptor.Key())
	defer f.leave()
	if f.block != nil {
		<-f.block
	}
	if outcome, found := f.changed[descriptor.Key()]; found {
		return outcome
	}
	return domain.CompleteOutcome(descriptor.Provenance(), nil)
}

func (f *fakeAdapter) CurrentPullRequest(ctx context.Context, descriptor domain.EvidenceDescriptor) domain.EvidenceDescriptor {
	f.enter(descriptor.Key())
	defer f.leave()
	if derived, found := f.metadata[descriptor.Key()]; found {
		return derived
	}
	return domain.NewSettledEvidence(domain.IncompleteOutcome("no scripted metadata"))
}

func (f *fakeAdapter) enter(key string) {
	f.mu.Lock()
	f.requests = append(f.requests, key)
	f.mu.Unlock()
	current := atomic.AddInt64(&f.inFlight, 1)
	for {
		peak := atomic.LoadInt64(&f.peak)
		if current <= peak || atomic.CompareAndSwapInt64(&f.peak, peak, current) {
			break
		}
	}
}

func (f *fakeAdapter) leave() { atomic.AddInt64(&f.inFlight, -1) }

func (f *fakeAdapter) dispatched() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

// fakeCache is a deterministic in-memory stand-in for the version-1 store.
type fakeCache struct {
	entries map[string]cache.Entry
	// failures are injected per operation, so a degraded cache never fails a
	// refresh that GitHub could still serve.
	lookupErr, saveErr, touchErr, dueErr, maintainErr error
	due                                               bool

	lookups   []string
	saved     []cache.Key
	touches   int
	dueChecks int
	maintains int
}

func newFakeCache() *fakeCache { return &fakeCache{entries: map[string]cache.Entry{}} }

func (f *fakeCache) Lookup(ctx context.Context, key cache.Key) (cache.Entry, bool, error) {
	if err := ctx.Err(); err != nil {
		// The version-1 store opens its read transaction on the caller
		// context, so a canceled refresh reaches it as a contended cache.
		return cache.Entry{}, false, fmt.Errorf("%w: %w", cache.ErrContended, err)
	}
	f.lookups = append(f.lookups, key.Head())
	if f.lookupErr != nil {
		return cache.Entry{}, false, f.lookupErr
	}
	entry, found := f.entries[key.Head()]
	return entry, found, nil
}

func (f *fakeCache) Save(ctx context.Context, entry cache.Entry) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %w", cache.ErrContended, err)
	}
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, entry.Key)
	f.entries[entry.Key.Head()] = entry
	return nil
}

func (f *fakeCache) Touch(ctx context.Context) error {
	f.touches++
	return f.touchErr
}

func (f *fakeCache) MaintenanceDue(ctx context.Context) (bool, error) {
	f.dueChecks++
	return f.due, f.dueErr
}

func (f *fakeCache) Maintain(ctx context.Context) (cache.Maintenance, error) {
	f.maintains++
	return cache.Maintenance{}, f.maintainErr
}

func repository(t *testing.T) domain.Repository {
	t.Helper()
	value, err := domain.ParseRepository("octo/demo")
	if err != nil {
		t.Fatalf("building the repository failed: %v", err)
	}
	return value
}

func compareEvent(t *testing.T, id, base, head string) domain.Event {
	t.Helper()
	descriptor, err := domain.NewCompareEvidence(repository(t), base, head, domain.ProvenanceEventTime)
	if err != nil {
		t.Fatalf("building compare evidence failed: %v", err)
	}
	return domain.Event{ID: id, Repository: repository(t), Evidence: descriptor}
}

func commitEvent(t *testing.T, id, head string) domain.Event {
	t.Helper()
	descriptor, err := domain.NewCommitEvidence(repository(t), head)
	if err != nil {
		t.Fatalf("building commit evidence failed: %v", err)
	}
	return domain.Event{ID: id, Repository: repository(t), Evidence: descriptor}
}

func pullRequestEvent(t *testing.T, id string, number int) domain.Event {
	t.Helper()
	descriptor, err := domain.NewPullRequestEvidence(repository(t), number)
	if err != nil {
		t.Fatalf("building pull request evidence failed: %v", err)
	}
	return domain.Event{ID: id, Repository: repository(t), Evidence: descriptor}
}

func changedPath(t *testing.T, value string) domain.ChangedPath {
	t.Helper()
	path, err := domain.NewChangedPath(value)
	if err != nil {
		t.Fatalf("building the changed path failed: %v", err)
	}
	return path
}

// coordinator returns a coordinator on the fixed refresh clock and the default
// RG-009 bounds unless a test narrows them.
func coordinator(adapter enrichment.Adapter, store enrichment.Cache) enrichment.Coordinator {
	return enrichment.Coordinator{
		Adapter: adapter,
		Cache:   store,
		Bounds:  enrichment.DefaultBounds(),
		Now:     func() time.Time { return fixedNow },
	}
}

// bounded returns a coordinator whose bounds are the RG-009 defaults with one
// capacity narrowed, so a boundary is provable with a two-event fixture instead
// of a full-budget refresh.
func bounded(adapter enrichment.Adapter, store enrichment.Cache, narrow func(*enrichment.Bounds)) enrichment.Coordinator {
	service := coordinator(adapter, store)
	bounds := enrichment.DefaultBounds()
	narrow(&bounds)
	service.Bounds = bounds
	return service
}

func TestSettledEvidenceConsumesNoWork(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()
	events := []domain.Event{
		{ID: "direct", Evidence: domain.NewDirectEvidence(changedPath(t, "cmd/main.go"))},
		{ID: "unsupported", Evidence: domain.NewUnsupportedEvidence("event type carries no changed-file evidence")},
	}

	result, err := coordinator(adapter, store).Evidence(t.Context(), events)
	if err != nil {
		t.Fatalf("coordinating settled evidence failed: %v", err)
	}
	if got := len(result.Outcomes); got != 2 {
		t.Fatalf("outcomes = %d, want 2", got)
	}
	if !result.Outcomes[0].IsComplete() {
		t.Errorf("direct evidence outcome = %v, want complete", result.Outcomes[0].Kind())
	}
	if result.Outcomes[1].Kind() != domain.OutcomeUnsupported {
		t.Errorf("unsupported evidence outcome = %v, want unsupported", result.Outcomes[1].Kind())
	}
	if result.Ledger.Requests != 0 || result.Ledger.CacheReads != 0 {
		t.Errorf("ledger = %+v, want no request and no cache read", result.Ledger)
	}
}

func TestIdenticalEvidenceAcrossEventsCoalescesIntoOneRequest(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()
	events := []domain.Event{
		compareEvent(t, "a", baseA, headA),
		compareEvent(t, "b", baseA, headA),
		compareEvent(t, "c", baseA, headA),
	}

	result, err := coordinator(adapter, store).Evidence(t.Context(), events)
	if err != nil {
		t.Fatalf("coordinating coalesced evidence failed: %v", err)
	}
	if result.Ledger.Requests != 1 {
		t.Errorf("requests = %d, want 1", result.Ledger.Requests)
	}
	if result.Ledger.CacheReads != 1 {
		t.Errorf("cache reads = %d, want 1", result.Ledger.CacheReads)
	}
	for index, outcome := range result.Outcomes {
		if !outcome.IsComplete() {
			t.Errorf("outcome %d = %v, want complete", index, outcome.Kind())
		}
	}
}

func TestFreshCacheHitAvoidsTheRequest(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()
	key, ok := cache.KeyFor(compareEvent(t, "a", baseA, headA).Evidence)
	if !ok {
		t.Fatal("compare evidence has no cache key")
	}
	store.entries[headA] = cache.Entry{
		Key:        key,
		Paths:      []domain.ChangedPath{changedPath(t, "internal/app.go")},
		AcquiredAt: fixedNow,
		LastUsedAt: fixedNow,
	}

	result, err := coordinator(adapter, store).Evidence(t.Context(), []domain.Event{compareEvent(t, "a", baseA, headA)})
	if err != nil {
		t.Fatalf("coordinating a cache hit failed: %v", err)
	}
	if result.Ledger.Requests != 0 {
		t.Errorf("requests = %d, want 0", result.Ledger.Requests)
	}
	if result.Ledger.CacheHits != 1 {
		t.Errorf("cache hits = %d, want 1", result.Ledger.CacheHits)
	}
	if paths := result.Outcomes[0].Paths(); len(paths) != 1 || paths[0].String() != "internal/app.go" {
		t.Errorf("hit paths = %v, want the stored path", paths)
	}
	if got := result.Outcomes[0].Provenance(); got != domain.ProvenanceEventTime {
		t.Errorf("hit provenance = %v, want the requesting descriptor's provenance", got)
	}
}

func TestAcquiredCompleteEvidenceIsStoredOnce(t *testing.T) {
	adapter := &fakeAdapter{changed: map[string]domain.EvidenceOutcome{}}
	commit := commitEvent(t, "a", headA)
	adapter.changed[commit.Evidence.Key()] =
		domain.CompleteOutcome(domain.ProvenanceEventTime, []domain.ChangedPath{changedPath(t, "go.mod")}).
			WithSoleParent(parentA)
	store := newFakeCache()

	result, err := coordinator(adapter, store).Evidence(t.Context(), []domain.Event{commit, commitEvent(t, "b", headA)})
	if err != nil {
		t.Fatalf("coordinating a cache miss failed: %v", err)
	}
	if len(store.saved) != 1 {
		t.Fatalf("cache writes = %d, want 1", len(store.saved))
	}
	if got := store.saved[0].Head(); got != headA {
		t.Errorf("stored head = %q, want %q", got, headA)
	}
	if got := store.entries[headA].VerifiedParent; got != parentA {
		t.Errorf("stored verified parent = %q, want %q", got, parentA)
	}
	if result.Ledger.CacheWrites != 1 {
		t.Errorf("ledger cache writes = %d, want 1", result.Ledger.CacheWrites)
	}
}

func TestIncompleteEvidenceIsNeverStored(t *testing.T) {
	adapter := &fakeAdapter{changed: map[string]domain.EvidenceOutcome{}}
	event := compareEvent(t, "a", baseA, headA)
	adapter.changed[event.Evidence.Key()] = domain.UnavailableOutcome("github no longer serves this exact entity")
	store := newFakeCache()

	result, err := coordinator(adapter, store).Evidence(t.Context(), []domain.Event{event})
	if err != nil {
		t.Fatalf("coordinating unavailable evidence failed: %v", err)
	}
	if len(store.saved) != 0 {
		t.Errorf("cache writes = %d, want 0", len(store.saved))
	}
	if result.Outcomes[0].Kind() != domain.OutcomeUnavailable {
		t.Errorf("outcome = %v, want unavailable", result.Outcomes[0].Kind())
	}
}

func TestConcurrentEnrichmentStaysWithinTheBound(t *testing.T) {
	adapter := &fakeAdapter{block: make(chan struct{})}
	store := newFakeCache()
	events := []domain.Event{
		compareEvent(t, "a", baseA, headA),
		compareEvent(t, "b", baseA, headB),
		commitEvent(t, "c", headA),
		commitEvent(t, "d", headB),
	}
	go func() {
		for range events {
			adapter.block <- struct{}{}
		}
	}()

	result, err := coordinator(adapter, store).Evidence(t.Context(), events)
	if err != nil {
		t.Fatalf("coordinating concurrent evidence failed: %v", err)
	}
	if result.Ledger.PeakConcurrency > enrichment.DefaultBounds().Concurrency {
		t.Errorf("peak concurrency = %d, want at most %d",
			result.Ledger.PeakConcurrency, enrichment.DefaultBounds().Concurrency)
	}
	if peak := atomic.LoadInt64(&adapter.peak); peak > int64(enrichment.DefaultBounds().Concurrency) {
		t.Errorf("observed adapter concurrency = %d, want at most %d", peak, enrichment.DefaultBounds().Concurrency)
	}
	if result.Ledger.PeakConcurrency < 1 {
		t.Errorf("peak concurrency = %d, want the coordinator to record its dispatch", result.Ledger.PeakConcurrency)
	}
}

func TestSpentRequestBudgetStopsDispatch(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()
	service := bounded(adapter, store, func(bounds *enrichment.Bounds) { bounds.Requests = 1 })

	result, err := service.Evidence(t.Context(), []domain.Event{
		compareEvent(t, "a", baseA, headA),
		compareEvent(t, "b", baseA, headB),
	})
	if err != nil {
		t.Fatalf("coordinating a spent budget failed: %v", err)
	}
	if result.Ledger.Requests != 1 {
		t.Errorf("requests = %d, want 1", result.Ledger.Requests)
	}
	if !result.Outcomes[0].IsComplete() {
		t.Errorf("first outcome = %v, want complete", result.Outcomes[0].Kind())
	}
	if result.Outcomes[1].Kind() != domain.OutcomeIncomplete {
		t.Errorf("second outcome = %v, want incomplete", result.Outcomes[1].Kind())
	}
	if len(adapter.dispatched()) != 1 {
		t.Errorf("dispatched = %v, want exactly one request", adapter.dispatched())
	}
}

func TestFullQueueStopsAdmission(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()
	service := bounded(adapter, store, func(bounds *enrichment.Bounds) { bounds.QueuedUnits = 1 })

	result, err := service.Evidence(t.Context(), []domain.Event{
		compareEvent(t, "a", baseA, headA),
		compareEvent(t, "b", baseA, headB),
	})
	if err != nil {
		t.Fatalf("coordinating a full queue failed: %v", err)
	}
	if len(adapter.dispatched()) != 1 {
		t.Errorf("dispatched = %v, want exactly one admitted unit", adapter.dispatched())
	}
	if result.Outcomes[1].Kind() != domain.OutcomeIncomplete {
		t.Errorf("omitted outcome = %v, want incomplete", result.Outcomes[1].Kind())
	}
	if result.Ledger.QueuedUnits != 1 {
		t.Errorf("queued units = %d, want 1", result.Ledger.QueuedUnits)
	}
}

func TestSpentCacheReadBudgetStillEnrichesFromGitHub(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()
	service := bounded(adapter, store, func(bounds *enrichment.Bounds) { bounds.CacheReads = 1 })

	result, err := service.Evidence(t.Context(), []domain.Event{
		compareEvent(t, "a", baseA, headA),
		compareEvent(t, "b", baseA, headB),
	})
	if err != nil {
		t.Fatalf("coordinating a spent cache budget failed: %v", err)
	}
	if result.Ledger.CacheReads != 1 {
		t.Errorf("cache reads = %d, want 1", result.Ledger.CacheReads)
	}
	if len(store.lookups) != 1 {
		t.Errorf("store lookups = %v, want exactly one", store.lookups)
	}
	for index, outcome := range result.Outcomes {
		if !outcome.IsComplete() {
			t.Errorf("outcome %d = %v, want complete from github", index, outcome.Kind())
		}
	}
}

func TestSpentCacheWriteBudgetKeepsTheAcquiredEvidence(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()
	service := bounded(adapter, store, func(bounds *enrichment.Bounds) { bounds.CacheWrites = 1 })

	result, err := service.Evidence(t.Context(), []domain.Event{
		compareEvent(t, "a", baseA, headA),
		compareEvent(t, "b", baseA, headB),
	})
	if err != nil {
		t.Fatalf("coordinating a spent write budget failed: %v", err)
	}
	if len(store.saved) != 1 {
		t.Errorf("cache writes = %d, want 1", len(store.saved))
	}
	for index, outcome := range result.Outcomes {
		if !outcome.IsComplete() {
			t.Errorf("outcome %d = %v, want complete", index, outcome.Kind())
		}
	}
}

func TestRateLimitStopsFurtherDispatchWithoutRetrying(t *testing.T) {
	retryAt := fixedNow.Add(90 * time.Second)
	adapter := &fakeAdapter{changed: map[string]domain.EvidenceOutcome{}}
	first := compareEvent(t, "a", baseA, headA)
	adapter.changed[first.Evidence.Key()] = domain.RateLimitedOutcome(retryAt)
	store := newFakeCache()
	service := bounded(adapter, store, func(bounds *enrichment.Bounds) { bounds.Concurrency = 1 })

	result, err := service.Evidence(t.Context(), []domain.Event{first, compareEvent(t, "b", baseA, headB)})
	if err != nil {
		t.Fatalf("coordinating a rate limit failed: %v", err)
	}
	if len(adapter.dispatched()) != 1 {
		t.Errorf("dispatched = %v, want no dispatch after the rate limit", adapter.dispatched())
	}
	for index, outcome := range result.Outcomes {
		if outcome.Kind() != domain.OutcomeRateLimited {
			t.Fatalf("outcome %d = %v, want rate limited", index, outcome.Kind())
		}
		if !outcome.RetryAt().Equal(retryAt) {
			t.Errorf("outcome %d retry = %v, want %v", index, outcome.RetryAt(), retryAt)
		}
	}
	if !result.Ledger.RetryAt.Equal(retryAt) {
		t.Errorf("ledger retry = %v, want %v", result.Ledger.RetryAt, retryAt)
	}
}

func TestCancellationSettlesTheRemainingWorkAsCanceled(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	result, err := coordinator(adapter, store).Evidence(ctx, []domain.Event{compareEvent(t, "a", baseA, headA)})
	if err != nil {
		t.Fatalf("coordinating a canceled refresh failed: %v", err)
	}
	if !result.Ledger.Canceled {
		t.Error("ledger canceled = false, want true")
	}
	if result.Outcomes[0].Kind() != domain.OutcomeCanceled {
		t.Errorf("outcome = %v, want canceled", result.Outcomes[0].Kind())
	}
	if len(adapter.dispatched()) != 0 {
		t.Errorf("dispatched = %v, want nothing after cancellation", adapter.dispatched())
	}
	if len(store.saved) != 0 {
		t.Errorf("cache writes = %d, want none after cancellation", len(store.saved))
	}
	if len(store.lookups) != 0 {
		t.Errorf("cache lookups = %v, want none after cancellation", store.lookups)
	}
	if result.Ledger.CacheDegraded != "" {
		t.Errorf("cache degradation = %q, want cancellation reported as cancellation only",
			result.Ledger.CacheDegraded)
	}
}

func TestCurrentPullRequestMetadataAndItsCompareAreTwoUnits(t *testing.T) {
	adapter := &fakeAdapter{metadata: map[string]domain.EvidenceDescriptor{}}
	event := pullRequestEvent(t, "a", 42)
	derived, err := domain.NewCompareEvidence(repository(t), baseA, headA, domain.ProvenanceCurrentPR)
	if err != nil {
		t.Fatalf("building the derived compare failed: %v", err)
	}
	adapter.metadata[event.Evidence.Key()] = derived
	store := newFakeCache()

	result, err := coordinator(adapter, store).Evidence(t.Context(), []domain.Event{event})
	if err != nil {
		t.Fatalf("coordinating pull request metadata failed: %v", err)
	}
	if result.Ledger.Requests != 2 {
		t.Errorf("requests = %d, want 2", result.Ledger.Requests)
	}
	if result.Ledger.QueuedUnits != 2 {
		t.Errorf("queued units = %d, want 2", result.Ledger.QueuedUnits)
	}
	if got := result.Outcomes[0].Provenance(); got != domain.ProvenanceCurrentPR {
		t.Errorf("provenance = %v, want current pr", got)
	}
	if !result.Outcomes[0].IsComplete() {
		t.Errorf("outcome = %v, want complete", result.Outcomes[0].Kind())
	}
}

func TestSettledPullRequestMetadataNeedsNoSecondUnit(t *testing.T) {
	adapter := &fakeAdapter{metadata: map[string]domain.EvidenceDescriptor{}}
	event := pullRequestEvent(t, "a", 42)
	adapter.metadata[event.Evidence.Key()] =
		domain.NewSettledEvidence(domain.CompleteOutcome(domain.ProvenanceCurrentPR, nil))
	store := newFakeCache()

	result, err := coordinator(adapter, store).Evidence(t.Context(), []domain.Event{event})
	if err != nil {
		t.Fatalf("coordinating settled metadata failed: %v", err)
	}
	if result.Ledger.Requests != 1 {
		t.Errorf("requests = %d, want 1", result.Ledger.Requests)
	}
	if !result.Outcomes[0].IsComplete() {
		t.Errorf("outcome = %v, want complete", result.Outcomes[0].Kind())
	}
}

func TestPullRequestCompareReusesEvidenceAlreadyRead(t *testing.T) {
	adapter := &fakeAdapter{metadata: map[string]domain.EvidenceDescriptor{}}
	metadata := pullRequestEvent(t, "a", 42)
	derived, err := domain.NewCompareEvidence(repository(t), baseA, headA, domain.ProvenanceCurrentPR)
	if err != nil {
		t.Fatalf("building the derived compare failed: %v", err)
	}
	adapter.metadata[metadata.Evidence.Key()] = derived
	store := newFakeCache()

	result, err := coordinator(adapter, store).Evidence(t.Context(),
		[]domain.Event{compareEvent(t, "b", baseA, headA), metadata})
	if err != nil {
		t.Fatalf("coordinating a shared compare failed: %v", err)
	}
	// One metadata request plus one shared compare request: the current-PR
	// stage reuses the identity the event-time stage already acquired.
	if result.Ledger.Requests != 2 {
		t.Errorf("requests = %d, want 2", result.Ledger.Requests)
	}
	if got := result.Outcomes[0].Provenance(); got != domain.ProvenanceEventTime {
		t.Errorf("event-time provenance = %v, want event time", got)
	}
	if got := result.Outcomes[1].Provenance(); got != domain.ProvenanceCurrentPR {
		t.Errorf("current-pr provenance = %v, want current pr", got)
	}
}

func TestRefreshPerformsOneTouchAndOneCleanupBatch(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()
	store.due = true

	result, err := coordinator(adapter, store).Evidence(t.Context(), []domain.Event{
		compareEvent(t, "a", baseA, headA),
		compareEvent(t, "b", baseA, headB),
	})
	if err != nil {
		t.Fatalf("coordinating maintenance failed: %v", err)
	}
	if store.touches != 1 {
		t.Errorf("touch transactions = %d, want exactly 1", store.touches)
	}
	if store.dueChecks != 1 {
		t.Errorf("maintenance checks = %d, want exactly 1", store.dueChecks)
	}
	if store.maintains != 1 {
		t.Errorf("cleanup batches = %d, want exactly 1", store.maintains)
	}
	if !result.Ledger.Touched || !result.Ledger.Cleaned {
		t.Errorf("ledger = %+v, want one touch and one cleanup recorded", result.Ledger)
	}
}

func TestUndueMaintenanceRunsNoCleanupBatch(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()

	result, err := coordinator(adapter, store).Evidence(t.Context(),
		[]domain.Event{compareEvent(t, "a", baseA, headA)})
	if err != nil {
		t.Fatalf("coordinating undue maintenance failed: %v", err)
	}
	if store.maintains != 0 {
		t.Errorf("cleanup batches = %d, want 0", store.maintains)
	}
	if result.Ledger.Cleaned {
		t.Error("ledger cleaned = true, want false")
	}
}

func TestContendedCacheOperationsDegradeWithoutFailingTheRefresh(t *testing.T) {
	for name, prepare := range map[string]func(*fakeCache){
		"lookup":   func(f *fakeCache) { f.lookupErr = cache.ErrContended },
		"save":     func(f *fakeCache) { f.saveErr = cache.ErrOverCapacity },
		"touch":    func(f *fakeCache) { f.touchErr = cache.ErrContended },
		"due":      func(f *fakeCache) { f.dueErr = cache.ErrContended },
		"maintain": func(f *fakeCache) { f.due = true; f.maintainErr = cache.ErrContended },
	} {
		t.Run(name, func(t *testing.T) {
			adapter := &fakeAdapter{}
			store := newFakeCache()
			prepare(store)

			result, err := coordinator(adapter, store).Evidence(t.Context(),
				[]domain.Event{compareEvent(t, "a", baseA, headA)})
			if err != nil {
				t.Fatalf("a degraded cache failed the refresh: %v", err)
			}
			if !result.Outcomes[0].IsComplete() {
				t.Errorf("outcome = %v, want complete from github", result.Outcomes[0].Kind())
			}
			if result.Ledger.CacheDegraded == "" {
				t.Error("ledger cache degradation = empty, want a sanitized cause")
			}
		})
	}
}

func TestExpiredCacheRecordIsAMissThatRefetches(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()
	store.lookupErr = cache.ErrExpired

	result, err := coordinator(adapter, store).Evidence(t.Context(),
		[]domain.Event{compareEvent(t, "a", baseA, headA)})
	if err != nil {
		t.Fatalf("coordinating an expired record failed: %v", err)
	}
	if result.Ledger.Requests != 1 {
		t.Errorf("requests = %d, want 1", result.Ledger.Requests)
	}
	if result.Ledger.CacheHits != 0 {
		t.Errorf("cache hits = %d, want 0", result.Ledger.CacheHits)
	}
}

func TestAbsentCacheStillCoordinatesEnrichment(t *testing.T) {
	adapter := &fakeAdapter{}
	service := enrichment.Coordinator{
		Adapter: adapter,
		Bounds:  enrichment.DefaultBounds(),
		Now:     func() time.Time { return fixedNow },
	}

	result, err := service.Evidence(t.Context(), []domain.Event{compareEvent(t, "a", baseA, headA)})
	if err != nil {
		t.Fatalf("coordinating without a cache failed: %v", err)
	}
	if !result.Outcomes[0].IsComplete() {
		t.Errorf("outcome = %v, want complete", result.Outcomes[0].Kind())
	}
	if result.Ledger.CacheReads != 0 || result.Ledger.CacheWrites != 0 {
		t.Errorf("ledger = %+v, want no cache work", result.Ledger)
	}
}

func TestMissingAdapterFailsTheRefresh(t *testing.T) {
	service := enrichment.Coordinator{Bounds: enrichment.DefaultBounds()}
	if _, err := service.Evidence(t.Context(), []domain.Event{compareEvent(t, "a", baseA, headA)}); err == nil {
		t.Fatal("coordinating without an adapter returned no error")
	}
}

func TestCacheHitBeyondTheQueueBoundStillAvoidsItsUnit(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()
	hit := compareEvent(t, "b", baseA, headB)
	key, ok := cache.KeyFor(hit.Evidence)
	if !ok {
		t.Fatal("compare evidence has no cache key")
	}
	store.entries[headB] = cache.Entry{
		Key:        key,
		Paths:      []domain.ChangedPath{changedPath(t, "internal/app.go")},
		AcquiredAt: fixedNow,
		LastUsedAt: fixedNow,
	}
	service := bounded(adapter, store, func(bounds *enrichment.Bounds) { bounds.QueuedUnits = 1 })

	result, err := service.Evidence(t.Context(), []domain.Event{compareEvent(t, "a", baseA, headA), hit})
	if err != nil {
		t.Fatalf("coordinating a hit beyond the queue bound failed: %v", err)
	}
	if result.Ledger.QueuedUnits != 1 {
		t.Errorf("queued units = %d, want 1", result.Ledger.QueuedUnits)
	}
	if result.Ledger.CacheHits != 1 {
		t.Errorf("cache hits = %d, want 1", result.Ledger.CacheHits)
	}
	if !result.Outcomes[1].IsComplete() {
		t.Errorf("hit outcome = %v, want complete despite the full queue", result.Outcomes[1].Kind())
	}
}

func TestUnsetBoundsStillReuseAWiredCache(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()
	service := enrichment.Coordinator{
		Adapter: adapter,
		Cache:   store,
		Now:     func() time.Time { return fixedNow },
	}

	result, err := service.Evidence(t.Context(), []domain.Event{compareEvent(t, "a", baseA, headA)})
	if err != nil {
		t.Fatalf("coordinating with unset bounds failed: %v", err)
	}
	if result.Ledger.CacheReads != 1 {
		t.Errorf("cache reads = %d, want the default budget to reuse the wired cache", result.Ledger.CacheReads)
	}
	if result.Ledger.CacheWrites != 1 {
		t.Errorf("cache writes = %d, want the default budget to store acquired evidence", result.Ledger.CacheWrites)
	}
}

func TestDegradationCauseCarriesNoTerminalControlSequence(t *testing.T) {
	adapter := &fakeAdapter{}
	store := newFakeCache()
	store.lookupErr = fmt.Errorf("%w: \x1b[31mred\x07 disk\nlabel", cache.ErrContended)

	result, err := coordinator(adapter, store).Evidence(t.Context(),
		[]domain.Event{compareEvent(t, "a", baseA, headA)})
	if err != nil {
		t.Fatalf("coordinating a hostile cause failed: %v", err)
	}
	cause := result.Ledger.CacheDegraded
	if cause == "" {
		t.Fatal("cache degradation = empty, want a sanitized cause")
	}
	if strings.ContainsFunc(cause, func(character rune) bool {
		return !unicode.IsPrint(character) && character != ' '
	}) {
		t.Errorf("cache degradation = %q, want no control runes", cause)
	}
	if strings.Contains(cause, "\n") {
		t.Errorf("cache degradation = %q, want one header-safe line", cause)
	}
}

func TestCoalescedCommitEvidenceKeepsItsVerifiedParent(t *testing.T) {
	adapter := &fakeAdapter{metadata: map[string]domain.EvidenceDescriptor{}, changed: map[string]domain.EvidenceOutcome{}}
	commit := commitEvent(t, "a", headA)
	adapter.changed[commit.Evidence.Key()] =
		domain.CompleteOutcome(domain.ProvenanceEventTime, []domain.ChangedPath{changedPath(t, "go.mod")}).
			WithSoleParent(parentA)

	result, err := coordinator(adapter, newFakeCache()).Evidence(t.Context(),
		[]domain.Event{commit, commitEvent(t, "b", headA)})
	if err != nil {
		t.Fatalf("coordinating coalesced commit evidence failed: %v", err)
	}
	for index, outcome := range result.Outcomes {
		if got := outcome.SoleParent(); got != parentA {
			t.Errorf("outcome %d sole parent = %q, want %q so each event can qualify its own before object",
				index, got, parentA)
		}
	}
}

func TestRestampedProvenanceKeepsTheVerifiedParent(t *testing.T) {
	adapter := &fakeAdapter{changed: map[string]domain.EvidenceOutcome{}}
	commit := commitEvent(t, "a", headA)
	// An identity settled under one provenance is re-stamped for the
	// requesting descriptor; the proof the adapter verified must survive that.
	adapter.changed[commit.Evidence.Key()] =
		domain.CompleteOutcome(domain.ProvenanceCurrentPR, []domain.ChangedPath{changedPath(t, "go.mod")}).
			WithSoleParent(parentA)

	result, err := coordinator(adapter, newFakeCache()).Evidence(t.Context(), []domain.Event{commit})
	if err != nil {
		t.Fatalf("coordinating a re-stamped outcome failed: %v", err)
	}
	if got := result.Outcomes[0].Provenance(); got != domain.ProvenanceEventTime {
		t.Errorf("provenance = %v, want the requesting descriptor's provenance", got)
	}
	if got := result.Outcomes[0].SoleParent(); got != parentA {
		t.Errorf("sole parent = %q, want %q preserved across the re-stamp", got, parentA)
	}
}
