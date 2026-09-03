package main

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/fmueller/orgtop/internal/auth"
	"github.com/fmueller/orgtop/internal/cache"
	"github.com/fmueller/orgtop/internal/cli"
	"github.com/fmueller/orgtop/internal/domain"
	"github.com/fmueller/orgtop/internal/tui"
)

// launch runs the terminal UI against the public GitHub API.
func launch(ctx context.Context, config cli.Config, credential auth.Credential) error {
	enricher, releaseCache := enricherFor(credential, config, openEnrichmentCache)
	defer releaseCache()
	return launchProgram(ctx, config.Scopes, newSourceAdapter(credential), expanderFor(credential, config), enricher)
}

// openEnrichmentCache opens the store at the fixed RG-005 default location.
func openEnrichmentCache() (*cache.Store, error) {
	location, err := cache.DefaultLocation()
	if err != nil {
		return nil, err
	}
	return cache.Open(location)
}

// enricherFor binds the changed-file evidence coordination the launch settles
// evidence through. A launch without --no-cache opens the cache and binds it to
// the coordinator, and the returned release closes the store when the program
// ends. --no-cache performs no cache operation at all. A cache that cannot be
// opened is degradation, not a failed launch: the coordination reacquires every
// evidence from GitHub and the sanitized cause reaches the prepared CACHE
// DEGRADED state rather than the process exit path (RG-005). open is the
// process seam so the launch wiring is testable without the user's real cache
// directory.
func enricherFor(credential auth.Credential, config cli.Config, open func() (*cache.Store, error)) (enrichmentAdapter, func()) {
	adapter := newEnrichmentAdapter(credential)
	if config.NoCache {
		return adapter, func() {}
	}
	store, err := open()
	if err != nil {
		adapter.degraded = cacheDegradation(err)
		return adapter, func() {}
	}
	adapter.coordinator.Cache = store
	// A close failure at program end is not a launch failure either: the store
	// is disposable and its error has no remediation left to offer (RG-005).
	return adapter, func() { _ = store.Close() }
}

// expanderFor returns the organization expansion the launch polls behind, or
// nil for an invocation that named no selector and therefore polls exactly the
// selection it was given (RG-010).
func expanderFor(credential auth.Credential, config cli.Config) tui.Expander {
	if len(config.Organizations) == 0 {
		return nil
	}
	return newExpansionAdapter(credential, config)
}

// launchProgram runs the Bubble Tea shell for the selection until it exits. A
// non-nil expander makes the selection the result of a bounded organization
// expansion the shell runs before it polls anything, and a non-nil enricher
// settles changed-file evidence between a successful poll and aggregation. The
// program
// and every refresh it starts share one context, so a canceled process context
// ends both, and returning cancels whatever source work is still in flight no
// matter which path ended the program (NFR-001).
func launchProgram(ctx context.Context, scopes domain.ScopeSet, source tui.Source, expander tui.Expander, enricher tui.Enricher, options ...tea.ProgramOption) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	model, err := tui.New(ctx, scopes, source, tui.WithExpander(expander), tui.WithEnricher(enricher))
	if err != nil {
		return fmt.Errorf("building the terminal ui: %w", err)
	}

	programOptions := append([]tea.ProgramOption{tea.WithContext(ctx)}, options...)
	program := tea.NewProgram(model, programOptions...)
	if _, err := program.Run(); err != nil && !isShutdown(err) {
		return fmt.Errorf("running the terminal ui: %w", err)
	}
	return nil
}

// isShutdown reports whether the program ended through a requested shutdown
// rather than a failure: an interrupt and a canceled process context both
// terminate cleanly (FR-001).
func isShutdown(err error) bool {
	return errors.Is(err, tea.ErrInterrupted) || errors.Is(err, context.Canceled)
}
