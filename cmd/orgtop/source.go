package main

import (
	"context"
	"errors"
	"time"

	"github.com/fmueller/orgtop/internal/auth"
	"github.com/fmueller/orgtop/internal/cli"
	"github.com/fmueller/orgtop/internal/domain"
	"github.com/fmueller/orgtop/internal/github"
	"github.com/fmueller/orgtop/internal/tui"
)

// sourceAdapter adapts the GitHub activity source to the shell's refresh seam.
// The shell declares its own Source and Result because internal/tui must not
// import internal/github (NFR-004); this is the only place the two meet.
type sourceAdapter struct {
	source github.Source
}

// newSourceAdapter binds the adapter to the public GitHub API.
func newSourceAdapter(credential auth.Credential) sourceAdapter {
	return sourceAdapter{source: github.NewSource(credential)}
}

// Refresh maps one atomic GitHub refresh onto the lifecycle's Result. Both
// outcomes carry scheduling metadata: a failure reports the source's retry
// delay so the lifecycle never re-derives GitHub's rules.
func (a sourceAdapter) Refresh(ctx context.Context, scopes domain.ScopeSet) (tui.Result, error) {
	refresh, err := a.source.Refresh(ctx, scopes)
	if err != nil {
		return tui.Result{Delay: retryDelay(err)}, err
	}
	return tui.Result{Repositories: refresh.Repositories, Delay: refresh.PollDelay}, nil
}

// retryDelay reports the delay the source attached to a failed refresh, or zero
// when the failure carries none; the lifecycle applies its own polling floor.
func retryDelay(err error) time.Duration {
	var failure *github.RefreshError
	if errors.As(err, &failure) {
		return failure.RetryDelay
	}
	return 0
}

// expansionAdapter adapts the GitHub organization expansion to the shell's
// selection seam. The validated selectors and eligibility flags are fixed for
// the process, so the request is built once at launch and every attempt lists
// the same organizations (RG-010).
type expansionAdapter struct {
	source  github.Source
	request github.ExpansionRequest
}

// newExpansionAdapter binds the adapter to the public GitHub API for the
// invocation's organization selectors.
func newExpansionAdapter(credential auth.Credential, config cli.Config) expansionAdapter {
	return expansionAdapter{source: github.NewSource(credential), request: expansionRequest(config)}
}

// expansionRequest is the fixed listing request of the invocation: its selectors
// in their first-occurrence order, the exact selection that has capacity
// precedence, and the process-global eligibility flags.
func expansionRequest(config cli.Config) github.ExpansionRequest {
	organizations := make([]string, 0, len(config.Organizations))
	for _, selector := range config.Organizations {
		organizations = append(organizations, selector.Name())
	}
	return github.ExpansionRequest{
		Organizations:   organizations,
		Exact:           config.Scopes,
		IncludeArchived: config.IncludeArchived,
		IncludeForks:    config.IncludeForks,
	}
}

// Expand maps one bounded GitHub expansion onto the lifecycle's Expansion. A
// failure carries the retry bound and whether the rate limit caused it, so the
// lifecycle schedules and stops dispatch from source metadata instead of
// re-deriving GitHub's rules.
func (a expansionAdapter) Expand(ctx context.Context) (tui.Expansion, error) {
	expansion, err := a.source.Expand(ctx, a.request)
	if err != nil {
		var failure *github.ExpansionError
		if errors.As(err, &failure) {
			return tui.Expansion{RetryDelay: failure.RetryDelay, RateLimited: failure.RateLimited}, err
		}
		return tui.Expansion{}, err
	}
	return tui.Expansion{Selection: selection(expansion)}, nil
}

// selection maps the adapter's prepared disclosure onto the application state
// the shell renders. The two types are separate because internal/tui must not
// import internal/github (NFR-004).
func selection(expansion github.Expansion) tui.Selection {
	prepared := expansion.Selection
	selectors := make([]tui.SelectorSelection, 0, len(prepared.Selectors))
	for _, selector := range prepared.Selectors {
		selectors = append(selectors, tui.SelectorSelection{
			Organization: selector.Organization,
			Listed:       selector.Listed,
			Disabled:     selector.Disabled,
			Archived:     selector.Archived,
			Fork:         selector.Fork,
			Eligible:     selector.Eligible,
			Retained:     selector.Retained,
			Omitted:      selector.Omitted,
			HasMore:      selector.HasMore,
		})
	}
	provenance := make([]tui.ScopeProvenance, 0, len(prepared.Provenance))
	for _, entry := range prepared.Provenance {
		provenance = append(provenance, tui.ScopeProvenance{Scope: entry.Scope, Exact: entry.Exact, Selector: entry.Selector})
	}
	return tui.Selection{
		Scopes:               expansion.Scopes,
		ExactScopes:          prepared.ExactScopes,
		ExpandedScopes:       prepared.ExpandedScopes,
		TotalScopes:          prepared.TotalScopes,
		DistinctRepositories: prepared.DistinctRepositories,
		Selectors:            selectors,
		Provenance:           provenance,
		PaginationRemains:    prepared.PaginationRemains,
	}
}
