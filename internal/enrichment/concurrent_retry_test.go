package enrichment

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

type controlledRetryAdapter struct {
	started chan string
	results map[string]chan domain.EvidenceOutcome
}

func (a controlledRetryAdapter) Changed(ctx context.Context, d domain.EvidenceDescriptor) domain.EvidenceOutcome {
	a.started <- d.Key()
	select {
	case result := <-a.results[d.Key()]:
		return result
	case <-ctx.Done():
		return domain.CanceledOutcome()
	}
}

func (a controlledRetryAdapter) CurrentPullRequest(context.Context, domain.EvidenceDescriptor) domain.EvidenceDescriptor {
	panic("unexpected metadata request")
}

func TestConcurrentRetryKeepsLatestDeadline(t *testing.T) {
	for _, extending := range []bool{true, false} {
		name := "later completion extends"
		if !extending {
			name = "later completion cannot shorten"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			repo, err := domain.ParseRepository("acme/api")
			if err != nil {
				t.Fatal(err)
			}
			adapter := controlledRetryAdapter{started: make(chan string, 3), results: make(map[string]chan domain.EvidenceOutcome)}
			var events []domain.Event
			for _, digit := range []string{"2", "3", "4"} {
				d, err := domain.NewCompareEvidence(repo, strings.Repeat("1", 40), strings.Repeat(digit, 40), domain.ProvenanceEventTime)
				if err != nil {
					t.Fatal(err)
				}
				adapter.results[d.Key()] = make(chan domain.EvidenceOutcome)
				events = append(events, domain.Event{ID: digit, Repository: repo, Evidence: d})
			}
			r := &run{coordinator: Coordinator{Adapter: adapter}, bounds: DefaultBounds(), identities: make(map[string]domain.EvidenceOutcome), aliases: make(map[string]string)}
			r.bounds.Concurrency = 2
			done := make(chan Result, 1)
			go func() { done <- r.settle(ctx, events) }()
			for range 2 {
				select {
				case <-adapter.started:
				case <-ctx.Done():
					t.Fatal("two requests did not enter flight")
				}
			}
			first := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC).Add(65 * time.Second)
			latest := first.Add(30400 * time.Millisecond)
			second := latest
			if !extending {
				first, second = latest, first
			}
			adapter.results[events[0].Evidence.Key()] <- domain.RateLimitedOutcome(first)
			// The queued identity settling proves the first result reached the
			// dispatch stop, before the second in-flight result may complete.
			for {
				r.mu.Lock()
				_, settled := r.identities[events[2].Evidence.Key()]
				r.mu.Unlock()
				if settled {
					break
				}
				if ctx.Err() != nil {
					t.Fatal("queued work did not settle after first rate limit")
				}
				runtime.Gosched()
			}
			adapter.results[events[1].Evidence.Key()] <- domain.RateLimitedOutcome(second)
			result := <-done
			if !result.Ledger.RetryAt.Equal(latest) {
				t.Errorf("retry floor = %v, want latest %v", result.Ledger.RetryAt, latest)
			}
			if result.Ledger.Requests != 2 || result.Ledger.PeakConcurrency != 2 || len(adapter.started) != 0 {
				t.Fatalf("queued dispatch resumed: %+v", result.Ledger)
			}
			for i, outcome := range result.Outcomes {
				if outcome.Kind() != domain.OutcomeRateLimited {
					t.Errorf("outcome %d = %v, want terminal rate limit", i, outcome.Kind())
				}
				if !outcome.RetryAt().Equal(latest) {
					t.Errorf("outcome %d retry = %v, want final floor %v", i, outcome.RetryAt(), latest)
				}
			}
		})
	}
}
