package main

import (
	"context"

	"github.com/fmueller/orgtop/internal/auth"
	"github.com/fmueller/orgtop/internal/domain"
	"github.com/fmueller/orgtop/internal/enrichment"
	"github.com/fmueller/orgtop/internal/github"
	"github.com/fmueller/orgtop/internal/tui"
)

// enrichmentAdapter adapts the bounded enrichment coordination to the shell's
// evidence seam. The shell declares its own Enricher and Evidence because
// internal/tui must not import internal/github or the coordinator's ledger
// (NFR-004); this is the only place they meet.
type enrichmentAdapter struct {
	coordinator enrichment.Coordinator
}

// newEnrichmentAdapter binds the coordination to the public GitHub API. No
// enrichment cache is wired yet, so every refresh acquires the evidence it
// needs from GitHub under the same RG-009 bounds.
func newEnrichmentAdapter(credential auth.Credential) enrichmentAdapter {
	return enrichmentAdapter{coordinator: enrichment.Coordinator{Adapter: github.NewEnricher(credential)}}
}

// Evidence maps one bounded coordination onto the lifecycle's Evidence. The
// ledger's cache degradation and instructed retry are reported as prepared
// conditions rather than as a failed refresh: neither of them invalidates the
// evidence the refresh did settle (RG-004).
func (a enrichmentAdapter) Evidence(ctx context.Context, events []domain.Event) (tui.Evidence, error) {
	result, err := a.coordinator.Evidence(ctx, events)
	if err != nil {
		return tui.Evidence{}, err
	}
	return tui.Evidence{
		Outcomes:      result.Outcomes,
		CacheDegraded: result.Ledger.CacheDegraded,
		RetryAt:       result.Ledger.RetryAt,
	}, nil
}
