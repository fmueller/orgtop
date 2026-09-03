package enrichment

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/fmueller/orgtop/internal/cache"
	"github.com/fmueller/orgtop/internal/domain"
)

// ErrNoAdapter reports a coordinator built without an evidence adapter. It is a
// wiring failure, not a refresh outcome, so it is never turned into unknown
// path membership.
var ErrNoAdapter = errors.New("enrichment coordination needs an evidence adapter")

// Sanitized budget reasons. Each one names the exhausted refresh capacity and
// nothing about a credential, a request, or a response.
const (
	reasonQueueFull      = "the refresh enrichment queue is full"
	reasonRequestBudget  = "the refresh spent its enrichment request budget"
	reasonUnusableEntity = "event evidence identity is unusable"
)

// Adapter is the bounded GitHub evidence boundary one refresh drives. It owns
// no queue, ledger, cache, or retry policy: the coordinator does.
type Adapter interface {
	// Changed returns the one normalized outcome of a descriptor's
	// changed-file evidence.
	Changed(ctx context.Context, descriptor domain.EvidenceDescriptor) domain.EvidenceOutcome
	// CurrentPullRequest captures a pull request's current base and head once
	// and returns the immutable compare descriptor they define, or a settled
	// descriptor when that read needs no compare at all.
	CurrentPullRequest(ctx context.Context, descriptor domain.EvidenceDescriptor) domain.EvidenceDescriptor
}

// Cache is the enrichment store surface one refresh uses. *cache.Store
// satisfies it; a nil Cache coordinates GitHub work with no reuse at all.
type Cache interface {
	Lookup(ctx context.Context, key cache.Key) (cache.Entry, bool, error)
	Save(ctx context.Context, entry cache.Entry) error
	Touch(ctx context.Context) error
	MaintenanceDue(ctx context.Context) (bool, error)
	Maintain(ctx context.Context) (cache.Maintenance, error)
}

// Coordinator turns the retained events of one refresh into settled evidence
// outcomes. It is application work: it never renders and never handles input.
type Coordinator struct {
	// Adapter performs one bounded evidence operation at a time.
	Adapter Adapter
	// Cache reuses and maintains stored evidence. A nil Cache is no cache.
	Cache Cache
	// Bounds are this refresh's capacities. Zero fields use DefaultBounds.
	Bounds Bounds
	// Now supplies the refresh's fixed clock. A nil Now uses time.Now.
	Now func() time.Time
}

// Result is the settled evidence of one refresh and the ledger it spent.
type Result struct {
	// Outcomes holds exactly one outcome per input event, in input order.
	Outcomes []domain.EvidenceOutcome
	// Ledger records what the coordination spent.
	Ledger Ledger
}

// Evidence settles the changed-file evidence of every retained event. Equal
// evidence identities coalesce into one unit of work regardless of how many
// events or Scopes need them, and every bound is spent in deterministic event
// then evidence-identity order.
func (c Coordinator) Evidence(ctx context.Context, events []domain.Event) (Result, error) {
	if c.Adapter == nil {
		return Result{}, ErrNoAdapter
	}
	current := &run{
		coordinator: c,
		bounds:      c.Bounds.normalized(),
		identities:  make(map[string]domain.EvidenceOutcome),
		aliases:     make(map[string]string),
	}
	return current.settle(ctx, events), nil
}

func (c Coordinator) now() time.Time {
	if c.Now == nil {
		return time.Now()
	}
	return c.Now()
}

// run is the mutable state of one refresh ledger. It lives no longer than the
// Evidence call that created it, so no counter survives into the next refresh.
type run struct {
	coordinator Coordinator
	bounds      Bounds
	ledger      Ledger
	// identities maps one work key to the outcome its evidence identity
	// settled at, before any requesting descriptor's provenance is applied.
	identities map[string]domain.EvidenceOutcome
	// aliases maps a current-PR metadata work key to the compare identity its
	// metadata derived, so both units settle from one compare outcome.
	aliases map[string]string
	// limited stops new dispatch once GitHub rate-limited this refresh.
	limited bool
	// mu guards the ledger and the dispatch gate while units run.
	mu sync.Mutex
}

// unit is one queued work key and the descriptor that defines it.
type unit struct {
	key        string
	descriptor domain.EvidenceDescriptor
}

// settle runs the two-stage admission of one refresh: direct immutable
// descriptors and coalesced current-PR metadata first, then the compares that
// metadata derived.
func (r *run) settle(ctx context.Context, events []domain.Event) Result {
	keys := make([]string, len(events))
	outcomes := make([]domain.EvidenceOutcome, len(events))
	var initial []unit
	seen := make(map[string]bool)

	for index, event := range events {
		descriptor := event.Evidence
		if settled, isSettled := descriptor.Settled(); isSettled {
			outcomes[index] = settled
			continue
		}
		key := descriptor.Key()
		if key == "" {
			outcomes[index] = domain.IncompleteOutcome(reasonUnusableEntity)
			continue
		}
		keys[index] = key
		if seen[key] {
			continue
		}
		seen[key] = true
		initial = append(initial, r.admit(ctx, unit{key: key, descriptor: descriptor}))
	}

	derived := r.dispatch(ctx, initial)
	r.dispatch(ctx, r.deriveCompares(ctx, derived, seen))
	r.maintainCache(ctx)

	for index, event := range events {
		if keys[index] == "" {
			continue
		}
		outcomes[index] = forDescriptor(r.identities[r.resolve(keys[index])], event.Evidence)
	}
	return Result{Outcomes: outcomes, Ledger: r.ledger}
}

// admit settles one work key from the cache or records it against the queue
// bound. A complete hit avoids its API unit entirely, so it is read before the
// queue is consulted; a key beyond the queue bound is settled as incomplete
// rather than dispatched, so its path membership stays unknown and may retry in
// a later refresh.
func (r *run) admit(ctx context.Context, work unit) unit {
	if outcome, hit := r.lookup(ctx, work.descriptor); hit {
		r.identities[work.key] = outcome
		return unit{}
	}
	if r.ledger.QueuedUnits >= r.bounds.QueuedUnits {
		r.identities[work.key] = domain.IncompleteOutcome(reasonQueueFull)
		return unit{}
	}
	r.ledger.QueuedUnits++
	return work
}

// deriveCompares turns every settled current-PR metadata result into the second
// stage's compare units, in originating event and key order. A compare identity
// another event already settled is reused rather than requested again.
func (r *run) deriveCompares(ctx context.Context, results []unit, seen map[string]bool) []unit {
	var stage []unit
	for _, result := range results {
		if result.key == "" {
			continue
		}
		compare := result.descriptor
		key := compare.Key()
		if key == "" {
			r.identities[result.key] = domain.IncompleteOutcome(reasonUnusableEntity)
			continue
		}
		r.aliases[result.key] = key
		if seen[key] {
			continue
		}
		seen[key] = true
		stage = append(stage, r.admit(ctx, unit{key: key, descriptor: compare}))
	}
	return stage
}

// dispatch runs the admitted units, at most Concurrency at a time, spending one
// request per dispatch in queue order. It returns the pull request metadata
// units whose derived compare still needs work.
func (r *run) dispatch(ctx context.Context, units []unit) []unit {
	pending := make([]unit, len(units))
	slots := make(chan struct{}, r.bounds.Concurrency)
	var group sync.WaitGroup
	var running int

	for index, work := range units {
		if work.key == "" {
			continue
		}
		slots <- struct{}{}
		outcome, dispatched := r.gate(ctx)
		if !dispatched {
			<-slots
			r.settleIdentity(work, outcome)
			continue
		}
		group.Add(1)
		r.mu.Lock()
		running++
		r.ledger.PeakConcurrency = max(r.ledger.PeakConcurrency, running)
		r.mu.Unlock()

		go func() {
			defer group.Done()
			defer func() {
				r.mu.Lock()
				running--
				r.mu.Unlock()
				<-slots
			}()
			pending[index] = r.perform(ctx, work)
		}()
	}
	group.Wait()
	return pending
}

// gate decides whether one queued unit may be dispatched now. Cancellation,
// rate limiting, and a spent request budget each stop dispatch and settle the
// unit instead; none of them retries inside this refresh.
func (r *run) gate(ctx context.Context) (domain.EvidenceOutcome, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ctx.Err() != nil {
		r.ledger.Canceled = true
		return domain.CanceledOutcome(), false
	}
	if r.limited {
		return domain.RateLimitedOutcome(r.ledger.RetryAt), false
	}
	if r.ledger.Requests >= r.bounds.Requests {
		return domain.IncompleteOutcome(reasonRequestBudget), false
	}
	r.ledger.Requests++
	return domain.EvidenceOutcome{}, true
}

// perform runs one dispatched unit and records what it settled. A pull request
// metadata unit that derived a live compare is returned for the second stage;
// every other unit settles its own identity here.
func (r *run) perform(ctx context.Context, work unit) unit {
	if work.descriptor.Operation() == domain.EvidencePullRequest {
		derived := r.coordinator.Adapter.CurrentPullRequest(ctx, work.descriptor)
		if settled, isSettled := derived.Settled(); isSettled {
			r.settleIdentity(work, settled)
			return unit{}
		}
		// The metadata key settles through the compare identity its derived
		// descriptor names, so both units share one outcome.
		return unit{key: work.key, descriptor: derived}
	}
	outcome := r.coordinator.Adapter.Changed(ctx, work.descriptor)
	r.settleIdentity(work, outcome)
	r.save(ctx, work.descriptor, outcome)
	return unit{}
}

// settleIdentity records one work key's outcome and the rate-limit stop it may
// have reported.
func (r *run) settleIdentity(work unit, outcome domain.EvidenceOutcome) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.identities[work.key] = outcome
	switch outcome.Kind() {
	case domain.OutcomeRateLimited:
		// The first instructed retry is the one the refresh reports: a later
		// unit cannot make the earliest allowed retry any earlier.
		if !r.limited {
			r.limited = true
			r.ledger.RetryAt = outcome.RetryAt()
		}
	case domain.OutcomeCanceled:
		r.ledger.Canceled = true
	}
}

// resolve follows a current-PR metadata key to the compare identity that
// settles it. Every other key settles under itself.
func (r *run) resolve(key string) string {
	if compare, aliased := r.aliases[key]; aliased {
		return compare
	}
	return key
}

// forDescriptor applies commit-parent qualification and provenance to one
// requesting descriptor. One coalesced identity can therefore safely serve
// several events without another request.
func forDescriptor(outcome domain.EvidenceOutcome, descriptor domain.EvidenceDescriptor) domain.EvidenceOutcome {
	if descriptor.Operation() == domain.EvidenceCommit {
		outcome = outcome.ForSoleParent(descriptor.Before())
	}
	if !outcome.IsComplete() || outcome.Provenance() == descriptor.Provenance() {
		return outcome
	}
	restamped := domain.CompleteOutcome(descriptor.Provenance(), outcome.Paths())
	if parent := outcome.SoleParent(); parent != "" {
		return restamped.WithSoleParent(parent)
	}
	return restamped
}

// lookup reads one evidence identity from the cache. A spent read budget, an
// expired or invalid record, and a contended store are all misses: the refresh
// falls back to GitHub and never turns incomplete state into complete evidence.
func (r *run) lookup(ctx context.Context, descriptor domain.EvidenceDescriptor) (domain.EvidenceOutcome, bool) {
	// A canceled refresh publishes nothing, so it opens no cache transaction.
	// Reporting the cancellation the store would echo back as cache
	// degradation would blame the cache for the caller's own shutdown.
	if ctx.Err() != nil {
		return domain.EvidenceOutcome{}, false
	}
	if r.coordinator.Cache == nil || r.ledger.CacheReads >= r.bounds.CacheReads {
		return domain.EvidenceOutcome{}, false
	}
	key, usable := cache.KeyFor(descriptor)
	if !usable {
		return domain.EvidenceOutcome{}, false
	}
	r.ledger.CacheReads++
	entry, found, err := r.coordinator.Cache.Lookup(ctx, key)
	if err != nil {
		// Expiry and a failed row invariant are ordinary misses the store
		// resolves through its own bounded cleanup, not cache degradation.
		if !errors.Is(err, cache.ErrExpired) && !errors.Is(err, cache.ErrInvalidRecord) {
			r.ledger.degrade(err)
		}
		return domain.EvidenceOutcome{}, false
	}
	if !found {
		return domain.EvidenceOutcome{}, false
	}
	r.ledger.CacheHits++
	return entry.Outcome(descriptor.Provenance()), true
}

// save persists one complete acquired result under its exact key. A skipped or
// failed replacement is cache degradation only: the acquired evidence stays
// valid for this refresh.
func (r *run) save(ctx context.Context, descriptor domain.EvidenceDescriptor, outcome domain.EvidenceOutcome) {
	if ctx.Err() != nil {
		return
	}
	if r.coordinator.Cache == nil || !outcome.IsComplete() {
		return
	}
	key, usable := cache.KeyFor(descriptor)
	if !usable {
		return
	}
	// A commit record is usable only with the sole parent the adapter proved,
	// so an unproven one is not persisted rather than stored unusable.
	if descriptor.Operation() == domain.EvidenceCommit && outcome.SoleParent() == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ledger.CacheWrites >= r.bounds.CacheWrites {
		return
	}
	now := r.coordinator.now()
	err := r.coordinator.Cache.Save(ctx, cache.Entry{
		Key:            key,
		VerifiedParent: outcome.SoleParent(),
		Paths:          outcome.Paths(),
		AcquiredAt:     now,
		LastUsedAt:     now,
	})
	if err != nil {
		r.ledger.degrade(err)
		return
	}
	r.ledger.CacheWrites++
}

// maintainCache drives the store's freshness and maintenance surface once per
// refresh, after every functional read and write: one batched hit touch, one
// maintenance check, and at most one bounded cleanup batch. Every failure is
// degradation, never a failed refresh, and a canceled refresh performs none of
// it.
func (r *run) maintainCache(ctx context.Context) {
	if r.coordinator.Cache == nil || ctx.Err() != nil {
		return
	}
	if err := r.coordinator.Cache.Touch(ctx); err != nil {
		r.ledger.degrade(err)
	} else {
		r.ledger.Touched = true
	}
	due, err := r.coordinator.Cache.MaintenanceDue(ctx)
	if err != nil {
		r.ledger.degrade(err)
		return
	}
	if !due {
		return
	}
	if _, err := r.coordinator.Cache.Maintain(ctx); err != nil {
		r.ledger.degrade(err)
		return
	}
	r.ledger.Cleaned = true
}
