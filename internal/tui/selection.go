package tui

import (
	"context"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// The re-expansion bounds of the selection lifecycle (RG-010). A successful
// attempt is followed by expansionInterval measured from its explicit
// completion; a failed one is retried no earlier than expansionRetry, or at the
// later instructed time a rate-limited attempt reports.
const (
	expansionInterval = 15 * time.Minute
	expansionRetry    = 60 * time.Second
)

// SelectionFreshness marks whether the displayed selection is the one the latest
// due expansion produced. It is independent of source freshness: a selection can
// be stale while the events polled from it are current (FR-008, RG-010).
type SelectionFreshness int

// The selection states of the shared chrome.
const (
	SelectionCurrent SelectionFreshness = iota
	SelectionStale
)

// Marker returns the header marker, or an empty string when the displayed
// selection is the latest one.
func (f SelectionFreshness) Marker() string {
	if f == SelectionStale {
		return "SELECTION STALE"
	}
	return ""
}

// Selection is one immutable selection snapshot. Every poll, membership, and
// aggregation of one refresh reads the same snapshot, so an organization that
// changes mid-refresh cannot alter the repository set that refresh publishes.
// The disclosure fields never change Scope identity or behavior; they carry the
// prepared truncation and provenance state the views disclose.
type Selection struct {
	// Scopes is the exact selection followed by the expansion-created
	// repository Scopes in their deterministic allocation order.
	Scopes domain.ScopeSet
	// ExactScopes and ExpandedScopes partition TotalScopes.
	ExactScopes    int
	ExpandedScopes int
	TotalScopes    int
	// DistinctRepositories is the number of distinct repositories Scopes
	// references.
	DistinctRepositories int
	// Selectors reports one entry per organization selector in first-request
	// order. It is empty for an invocation without a selector.
	Selectors []SelectorSelection
	// Provenance carries one entry per published Scope, in Scope order.
	Provenance []ScopeProvenance
	// PaginationRemains reports that at least one selector left a valid next
	// page unconsumed, so further eligible repositories may exist beyond the
	// disclosed omissions.
	PaginationRemains bool
}

// SelectorSelection is the per-selector disclosure state. The disjoint Disabled,
// Archived, and Fork exclusion buckets plus Eligible sum to Listed, and Retained
// plus Omitted sum to Eligible.
type SelectorSelection struct {
	Organization string
	Listed       int
	Disabled     int
	Archived     int
	Fork         int
	Eligible     int
	Retained     int
	Omitted      int
	// HasMore reports an unconsumed valid next page.
	HasMore bool
}

// ScopeProvenance records how one published Scope was selected. Exact marks a
// Scope the invocation named directly; Selector names the contributing
// organization selector, including for an exact Scope an expansion also reached.
type ScopeProvenance struct {
	Scope    domain.Scope
	Exact    bool
	Selector string
}

// Expansion is the outcome of one bounded organization expansion. Selection
// holds the new snapshot of a successful attempt; RetryDelay and RateLimited
// describe a failed one, which returns them beside its error the way a failing
// Source reports its retry delay.
type Expansion struct {
	Selection Selection
	// RetryDelay is the delay before the next expansion attempt is eligible.
	RetryDelay time.Duration
	// RateLimited reports a failure that prohibits further GitHub dispatch, so
	// the refresh polls nothing and waits for the instructed retry time.
	RateLimited bool
}

// Expander performs one bounded organization expansion outside the update and
// render path. Implementations bind their I/O to the passed context and return
// no partial selector subset with an error.
type Expander interface {
	Expand(ctx context.Context) (Expansion, error)
}

// exactSelection is the selection of an invocation without an organization
// selector: every Scope is exact and no expansion contributes to it.
func exactSelection(scopes domain.ScopeSet) Selection {
	provenance := make([]ScopeProvenance, 0, scopes.Len())
	for _, scope := range scopes.Scopes() {
		provenance = append(provenance, ScopeProvenance{Scope: scope, Exact: true})
	}
	return Selection{
		Scopes:               scopes,
		ExactScopes:          scopes.Len(),
		TotalScopes:          scopes.Len(),
		DistinctRepositories: len(scopes.Repositories()),
		Provenance:           provenance,
	}
}

// expansionResult reports the expansion phase of one refresh attempt back into
// the update loop.
type expansionResult struct {
	// attempted reports whether the refresh ran an expansion at all. A refresh
	// before the next attempt is due reuses the fixed selection instead.
	attempted bool
	// outcome carries the new snapshot of a success and the retry metadata of a
	// failure.
	outcome Expansion
	// at is the explicit completion instant the next attempt is measured from.
	at time.Time
	// err is the sanitized cause of a failed attempt.
	err error
}

// expansionDue reports whether the next expansion attempt may run. Without a
// fixed selection the first attempt is immediately due; afterwards the closed
// interval or the failure retry bound has to elapse.
func (m Model) expansionDue() bool {
	if m.expander == nil {
		return false
	}
	return !m.hasSelection || !m.now().Before(m.nextExpansion)
}

// expand runs one expansion attempt and reports its completion instant, so the
// next attempt is scheduled from when this one actually finished.
func (m Model) expand(ctx context.Context) expansionResult {
	expansion, err := m.expander.Expand(ctx)
	return expansionResult{attempted: true, outcome: expansion, at: m.now(), err: err}
}

// applyExpansion records the attempt's effect on the fixed selection and its
// schedule. A successful attempt becomes the fixed input of the poll that
// follows it, even when that poll then fails; a failed one keeps the last
// successful selection rather than narrowing or emptying it, and marks the
// displayed selection stale once one exists.
func (m Model) applyExpansion(expansion expansionResult) Model {
	if !expansion.attempted {
		return m
	}
	if expansion.err == nil {
		m.selection, m.hasSelection = expansion.outcome.Selection, true
		m.nextExpansion = expansion.at.Add(expansionInterval)
		return m
	}

	m.nextExpansion = expansion.at.Add(max(expansion.outcome.RetryDelay, expansionRetry))
	if m.hasSelection {
		m.state.SelectionFreshness = SelectionStale
		m.state.SelectionCause = sanitize(expansion.err)
	}
	return m
}
