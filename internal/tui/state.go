// Package tui owns OrgTop's Bubble Tea application shell: the root model, the
// shared chrome, navigation between the Overview and Stream views, and the
// state slots those views render. It consumes application and domain state and
// performs no normalization, filtering, aggregation, or source I/O.
package tui

import (
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// Mode is the active root view.
type Mode int

// The views the shell switches between (FR-007).
const (
	ModeOverview Mode = iota
	ModeStream
)

// Label returns the shared header label of the mode.
func (m Mode) Label() string {
	if m == ModeStream {
		return "STREAM"
	}
	return "OVERVIEW"
}

// toggled returns the other mode, which is what tab selects.
func (m Mode) toggled() Mode {
	if m == ModeStream {
		return ModeOverview
	}
	return ModeStream
}

// Freshness is the header marker shown beside, never instead of, the constant
// POLLING transport label (FR-007). Current data needs no marker.
type Freshness int

// The freshness states of the shared header.
const (
	FreshnessLoading Freshness = iota
	FreshnessCurrent
	FreshnessError
	FreshnessStale
)

// Marker returns the header marker, or an empty string when the snapshot is
// current.
func (f Freshness) Marker() string {
	switch f {
	case FreshnessLoading:
		return "LOADING"
	case FreshnessError:
		return "ERROR"
	case FreshnessStale:
		return "STALE"
	default:
		return ""
	}
}

// State is the shared application state the root model owns and both views
// read. The refresh lifecycle produces it; the shell only stores and renders it.
type State struct {
	// Scopes is the validated selection the views render.
	Scopes domain.ScopeSet
	// Snapshot is the latest completely successful activity snapshot.
	Snapshot domain.Snapshot
	// Scoped is the prepared per-Scope snapshot of the same refresh: its
	// retained events, their explicit per-Scope membership, and the direct
	// aggregates the views render.
	Scoped domain.ScopedSnapshot
	// CacheDegraded is the sanitized cause of the latest refresh's skipped or
	// failed enrichment cache work. A degraded cache never invalidates the
	// evidence the refresh did acquire (RG-004).
	CacheDegraded string
	// EnrichmentRetryAt is the earliest instructed enrichment retry the latest
	// refresh was given, and stays zero when nothing was rate limited.
	EnrichmentRetryAt time.Time
	// Freshness is the marker shown next to the transport label.
	Freshness Freshness
	// LastSuccess is the instant of the last complete refresh success. It stays
	// zero until one succeeds.
	LastSuccess time.Time
	// Cause is the sanitized failure cause shown with ERROR or STALE.
	Cause string
	// Selection is the published selection snapshot the Scopes above come from,
	// with the prepared truncation and provenance disclosure of its expansion.
	Selection Selection
	// SelectionFreshness marks whether that selection is the one the latest due
	// expansion produced. It is independent of the source freshness above.
	SelectionFreshness SelectionFreshness
	// SelectionCause is the sanitized cause of the failed re-expansion that made
	// the selection stale.
	SelectionCause string
}
