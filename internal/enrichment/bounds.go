// Package enrichment coordinates cache reuse and bounded GitHub changed-file
// work for one refresh. It is the application service between the source
// adapter and the enrichment cache: it owns the RG-009 work ledger, the
// deterministic two-stage admission order, coalescing, concurrency, and the
// cache's freshness and maintenance surface. It performs no rendering and no
// keyboard handling, so none of that work can run in a renderer or key handler.
package enrichment

import (
	"strings"
	"time"
	"unicode"
)

// Bounds are RG-009's per-refresh enrichment capacities. Production always uses
// DefaultBounds; tests narrow them so every boundary is provable with a small
// deterministic fixture instead of a thousand-event refresh.
type Bounds struct {
	// Requests is the enrichment share of the refresh's HTTP budget. Every
	// attempted request spends one before dispatch and cancellation refunds
	// nothing.
	Requests int
	// QueuedUnits is the number of distinct work keys one refresh admits.
	QueuedUnits int
	// Concurrency is the number of queued units that may run at once.
	Concurrency int
	// CacheReads is the number of distinct evidence identities one refresh
	// reads from the cache.
	CacheReads int
	// CacheWrites is the number of exact-key replacement transactions one
	// refresh performs.
	CacheWrites int
}

// DefaultBounds returns the closed RG-009 enrichment capacities of one refresh
// that performed no organization expansion.
func DefaultBounds() Bounds {
	return Bounds{
		Requests:    20,
		QueuedUnits: 100,
		Concurrency: 2,
		CacheReads:  500,
		CacheWrites: 20,
	}
}

// normalized returns the bounds with every unset capacity replaced by its
// default, so a zero-valued Coordinator still coordinates bounded work instead
// of admitting nothing at all. Reuse is disabled by wiring no cache, never by
// leaving a capacity unset: a wired store that silently served nothing would
// report a healthy refresh that spent GitHub budget it did not need to.
func (b Bounds) normalized() Bounds {
	defaults := DefaultBounds()
	if b.Requests <= 0 {
		b.Requests = defaults.Requests
	}
	if b.QueuedUnits <= 0 {
		b.QueuedUnits = defaults.QueuedUnits
	}
	if b.Concurrency <= 0 {
		b.Concurrency = defaults.Concurrency
	}
	if b.CacheReads <= 0 {
		b.CacheReads = defaults.CacheReads
	}
	if b.CacheWrites <= 0 {
		b.CacheWrites = defaults.CacheWrites
	}
	return b
}

// Ledger is the one work ledger of a refresh. It records what the coordination
// actually spent, so a verification run can report peak concurrency, queue
// admission, request counts, cache work, and cancellation without inspecting
// the adapter or the store.
type Ledger struct {
	// Requests is the number of enrichment HTTP requests dispatched.
	Requests int
	// QueuedUnits is the number of distinct work keys admitted to the queue.
	QueuedUnits int
	// PeakConcurrency is the highest number of units running at once.
	PeakConcurrency int
	// CacheReads and CacheHits count attempted lookups and proven hits.
	CacheReads int
	CacheHits  int
	// CacheWrites counts committed exact-key replacements.
	CacheWrites int
	// Touched and Cleaned report the single batched hit-touch transaction and
	// the at most one bounded cleanup batch of this refresh.
	Touched bool
	Cleaned bool
	// Canceled reports that the refresh ended before every unit settled.
	Canceled bool
	// RetryAt is the earliest instructed enrichment retry a rate limit named.
	RetryAt time.Time
	// CacheDegraded is the sanitized cause of a skipped or failed cache
	// operation. A degraded cache never fails the refresh.
	CacheDegraded string
}

// degrade records the first sanitized cache degradation cause of the refresh.
// Later causes do not overwrite it: the first skipped operation is the one that
// explains the rest.
func (l *Ledger) degrade(err error) {
	if err == nil || l.CacheDegraded != "" {
		return
	}
	l.CacheDegraded = sanitize(err)
}

// sanitize collapses a cause to one header-safe line. Cache and adapter causes
// already carry no credential value; dropping the non-printable runes keeps a
// relayed string from reaching the terminal as an escape sequence.
func sanitize(err error) string {
	return strings.Join(strings.Fields(strings.Map(printable, err.Error())), " ")
}

// printable drops the runes a terminal must never receive verbatim, keeping the
// blanks the caller collapses.
func printable(character rune) rune {
	if unicode.IsPrint(character) || unicode.IsSpace(character) {
		return character
	}
	return -1
}
