package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/auth"
	"github.com/fmueller/orgtop/internal/cache"
	"github.com/fmueller/orgtop/internal/cli"
	"github.com/fmueller/orgtop/internal/domain"
	"github.com/fmueller/orgtop/internal/enrichment"
)

// commitBeforeSHA is the before SHA every commit-event fixture carries, so the
// sole parent an acquired outcome proves matches the event that requests it.
const commitBeforeSHA = "1111111111111111111111111111111111111111"

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
	before := commitBeforeSHA
	repository, err := domain.ParseRepository("acme/backend")
	if err != nil {
		t.Fatalf("ParseRepository returned error: %v", err)
	}
	descriptor, err := domain.NewCommitEvidence(repository, before, head)
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

// parseLaunch parses one launch invocation the enricher wiring reads.
func parseLaunch(t *testing.T, args ...string) cli.Config {
	t.Helper()

	config, err := cli.ParseArgs("orgtop", args, io.Discard)
	if err != nil {
		t.Fatalf("parsing %v failed: %v", args, err)
	}
	return config
}

// openStoreAt opens a real version-1 store beneath a temporary user cache
// root, so the launch wiring is exercised against the store it binds.
func openStoreAt(t *testing.T) (*cache.Store, error) {
	t.Helper()

	return cache.Open(cache.LocationIn(t.TempDir()))
}

// acquiredCommitOutcome is one complete commit evidence outcome the GitHub
// adapter could have settled: the changed paths it acquired and the sole
// parent the event's before SHA must match. The paths are in ascending order,
// which is also the storage order a reused record restores them in.
func acquiredCommitOutcome(paths ...string) domain.EvidenceOutcome {
	changed := make([]domain.ChangedPath, 0, len(paths))
	for _, value := range paths {
		path, err := domain.NewChangedPath(value)
		if err != nil {
			panic(err)
		}
		changed = append(changed, path)
	}
	return domain.CompleteOutcome(domain.ProvenanceEventTime, changed).WithSoleParent(commitBeforeSHA)
}

// TestACacheLaunchOpensBindsAndReleasesTheStore guards the default launch: the
// cache is opened, bound to the coordination, and closed when the program ends.
func TestACacheLaunchOpensBindsAndReleasesTheStore(t *testing.T) {
	config := parseLaunch(t, "--repo", "acme/backend")
	var store *cache.Store
	opened := false
	adapter, release := enricherFor(auth.Credential{}, config, func() (*cache.Store, error) {
		opened = true
		var err error
		store, err = openStoreAt(t)
		return store, err
	})

	if !opened {
		t.Fatal("the launch without --no-cache performed no cache open")
	}
	if adapter.coordinator.Cache == nil {
		t.Fatal("the opened store was not bound to the coordinator")
	}
	release()
	if _, err := store.PhysicalBytes(); !errors.Is(err, cache.ErrUnavailable) {
		t.Errorf("the launch ended without closing the store: %v", err)
	}
}

// TestANoCacheLaunchPerformsNoCacheOperationAtAll guards the intentional
// no-cache mode: it opens nothing, binds nothing, and degrades nothing
// (RG-005).
func TestANoCacheLaunchPerformsNoCacheOperationAtAll(t *testing.T) {
	config := parseLaunch(t, "--repo", "acme/backend", "--no-cache")
	adapter, release := enricherFor(auth.Credential{}, config, func() (*cache.Store, error) {
		t.Error("--no-cache performed a cache operation")
		return nil, errors.New("no cache operation is expected")
	})
	defer release()

	if adapter.coordinator.Cache != nil {
		t.Error("--no-cache bound a cache to the coordinator")
	}
	if adapter.degraded != "" {
		t.Errorf("--no-cache reported degradation %q, want the intentional mode", adapter.degraded)
	}
}

// TestAnUnopenableCacheDegradesTheLaunchWithoutFailingIt guards the cache
// failure path: a launch whose cache cannot be opened keeps its otherwise
// complete current evidence valid and reports the sanitized cause through the
// prepared CACHE DEGRADED state rather than failing the launch (RG-005).
func TestAnUnopenableCacheDegradesTheLaunchWithoutFailingIt(t *testing.T) {
	config := parseLaunch(t, "--repo", "acme/backend")
	cause := fmt.Errorf("enrichment cache unavailable:\n\t no user cache directory \x1b")
	adapter, release := enricherFor(auth.Credential{}, config, func() (*cache.Store, error) {
		return nil, cause
	})
	defer release()

	if adapter.coordinator.Cache != nil {
		t.Error("an unopenable cache still bound a store")
	}
	adapter.coordinator.Adapter = &stubAdapter{outcome: acquiredCommitOutcome("go.mod", "internal/api.go")}

	evidence, err := adapter.Evidence(context.Background(), []domain.Event{
		commitEvent(t, "one", "0123456789abcdef0123456789abcdef01234567"),
	})
	if err != nil {
		t.Fatalf("an unopenable cache failed the launch: %v", err)
	}
	if len(evidence.Outcomes) != 1 || !evidence.Outcomes[0].IsComplete() {
		t.Fatalf("the degraded launch settled %v, want its complete acquired evidence", evidence.Outcomes)
	}
	if evidence.CacheDegraded != "enrichment cache unavailable: no user cache directory" {
		t.Errorf("cache degradation = %q, want the sanitized open cause", evidence.CacheDegraded)
	}
}

// TestCacheReuseServesIndistinguishableMembership guards that evidence a later
// refresh serves from the cache decides membership exactly like the evidence
// the launch acquired from GitHub.
func TestCacheReuseServesIndistinguishableMembership(t *testing.T) {
	config := parseLaunch(t, "--repo", "acme/backend")
	adapter, release := enricherFor(auth.Credential{}, config, func() (*cache.Store, error) {
		return openStoreAt(t)
	})
	defer release()
	github := &stubAdapter{outcome: acquiredCommitOutcome("go.mod", "internal/api.go")}
	adapter.coordinator.Adapter = github
	events := []domain.Event{commitEvent(t, "one", "0123456789abcdef0123456789abcdef01234567")}

	acquired, err := adapter.Evidence(context.Background(), events)
	if err != nil {
		t.Fatalf("the acquiring refresh failed: %v", err)
	}
	reused, err := adapter.Evidence(context.Background(), events)
	if err != nil {
		t.Fatalf("the reusing refresh failed: %v", err)
	}

	if github.calls != 1 {
		t.Errorf("the refreshes performed %d GitHub acquisitions, want the second served from the cache", github.calls)
	}
	first, second := acquired.Outcomes[0], reused.Outcomes[0]
	if !second.IsComplete() {
		t.Fatalf("the reused outcome %v is not the complete acquired evidence", second)
	}
	if second.SoleParent() != first.SoleParent() {
		t.Errorf("the reused sole parent %q does not match the acquired %q", second.SoleParent(), first.SoleParent())
	}
	if !slices.Equal(first.Paths(), second.Paths()) {
		t.Errorf("the reused paths %v do not match the acquired %v", second.Paths(), first.Paths())
	}
	if reused.CacheDegraded != "" {
		t.Errorf("a healthy cache refresh reported degradation %q", reused.CacheDegraded)
	}
}
