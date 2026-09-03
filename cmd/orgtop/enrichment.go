package main

import (
	"context"
	"strings"
	"unicode"

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
	// degraded reports the sanitized cause of a launch that coordinates with
	// no reusable cache at all. Every refresh of that launch reacquires its
	// evidence from GitHub and reports the cause beside it (RG-005).
	degraded string
}

// newEnrichmentAdapter binds the coordination to the public GitHub API. The
// launch's cache binding is attached separately by enricherFor.
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
	// A cache the launch could not open degrades every refresh the same way a
	// failed store operation degrades one, so the first known cause wins.
	degraded := result.Ledger.CacheDegraded
	if degraded == "" {
		degraded = a.degraded
	}
	return tui.Evidence{
		Outcomes:      result.Outcomes,
		CacheDegraded: degraded,
		RetryAt:       result.Ledger.RetryAt,
	}, nil
}

// cacheDegradation collapses a cache open cause to one header-safe line, the
// same policy the coordinator's ledger applies to its own causes: cache errors
// carry no credential value, and dropping the non-printable runes keeps a
// relayed path from reaching the terminal as an escape sequence.
func cacheDegradation(err error) string {
	return strings.Join(strings.Fields(strings.Map(printable, err.Error())), " ")
}

// printable drops the runes a terminal must never receive verbatim, keeping
// the blanks the caller collapses.
func printable(character rune) rune {
	if unicode.IsPrint(character) || unicode.IsSpace(character) {
		return character
	}
	return -1
}
