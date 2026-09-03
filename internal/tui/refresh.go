package tui

import (
	"context"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// defaultDelay is the v0.1.0 polling floor (FR-004). The lifecycle re-asserts it
// so a source that advertises no usable scheduling metadata can never produce a
// tight retry loop.
const defaultDelay = 60 * time.Second

// Result is the scheduling-aware outcome of one refresh attempt. Delay is
// reported for both outcomes: a failing source returns the retry delay in an
// otherwise empty Result, so the lifecycle schedules from source metadata
// instead of re-deriving GitHub's rules.
type Result struct {
	// Repositories holds one entry per Scope entry after a completely
	// successful refresh, and nothing at all after a failed one.
	Repositories []domain.RepositoryActivity
	// Delay is the wait before the next attempt is eligible.
	Delay time.Duration
}

// Source performs one atomic refresh of the selected Scope outside the update
// and render path. Implementations bind their I/O to the passed context and
// return no partial data with an error.
type Source interface {
	Refresh(ctx context.Context, scopes domain.ScopeSet) (Result, error)
}

// refreshedMsg reports one completed refresh attempt back into the update loop.
type refreshedMsg struct {
	result Result
	err    error
	// expansion reports the organization expansion phase of the attempt.
	expansion expansionResult
	// polled reports whether the fixed selection was polled at all. A refresh
	// whose expansion left no selection to poll, and one whose selection holds
	// no repository, both publish without a source request.
	polled bool
}

// failure returns the cause that degrades the attempt, or nil for a successful
// one. A refresh that never reached its poll is degraded by the expansion that
// stopped it.
func (m refreshedMsg) failure() error {
	if !m.polled {
		return m.expansion.err
	}
	return m.err
}

// expanded reports whether this attempt produced the selection it published, so
// publication clears the selection degradation of an earlier failed expansion
// only when a new one actually succeeded.
func (m refreshedMsg) expanded() bool {
	return m.expansion.attempted && m.expansion.err == nil
}

// refreshDueMsg reports that the scheduled delay elapsed.
type refreshDueMsg struct{}

// refresh returns the command that performs one refresh. The command owns all
// source I/O, so keyboard, resize, and quit handling never wait for it. One
// refresh runs the due expansion and then polls one immutable selection
// snapshot, so a mid-refresh organization change cannot alter its repository
// set (RG-010).
func (m Model) refresh() tea.Cmd {
	selection, hasSelection := m.selection, m.hasSelection
	due := m.expansionDue()
	return func() tea.Msg {
		message := refreshedMsg{}
		if due {
			message.expansion = m.expand(m.ctx)
			switch {
			case message.expansion.err == nil:
				selection = message.expansion.outcome.Selection
			case !hasSelection || message.expansion.outcome.RateLimited:
				// A first expansion failure has no selection to fall back to,
				// and a rate-limited one must not dispatch further work. Both
				// publish no partial or empty selection.
				return message
			}
		}

		// A model without a selection always has its expansion due, so every
		// attempt that reaches here polls one: the freshly expanded selection,
		// the last successful one, or the exact selection of an invocation
		// that never expands.
		message.polled = true
		if selection.Scopes.Len() == 0 {
			// A successful expansion with no eligible repository is a vacuously
			// successful zero-repository poll phase, so it performs no request.
			return message
		}
		message.result, message.err = m.source.Refresh(m.ctx, selection.Scopes)
		return message
	}
}

// startRefresh begins the next attempt. At most one refresh is in flight, so a
// timer that fires while one is pending starts nothing (FR-004).
func (m Model) startRefresh() (tea.Model, tea.Cmd) {
	if m.pending {
		return m, nil
	}
	m.pending = true
	return m, m.refresh()
}

// applyRefresh publishes the completed attempt and starts the next timer from
// the metadata that attempt reported.
//
// Cancellation before publication discards the whole attempt: neither its newly
// expanded selection nor its would-be timer survives, so the prior selection,
// snapshot, and expansion schedule stay intact and expansion remains due.
func (m Model) applyRefresh(message refreshedMsg) (tea.Model, tea.Cmd) {
	m.pending = false
	if m.ctx.Err() != nil {
		return m, nil
	}

	m = m.applyExpansion(message.expansion)
	if failure := message.failure(); failure != nil {
		m.state = degraded(m.state, failure)
	} else if message.polled {
		m.state = published(m.state, m.selection, message.result, m.now(), message.expanded())
	}
	return m, m.tick(m.delay(message))
}

// delay returns the wait before the next attempt is eligible. An attempt that
// polled schedules from the source metadata against the FR-004 polling floor;
// one that a failed expansion stopped schedules against RG-010's own retry
// bound instead, so neither floor moves the other if it ever changes.
func (m Model) delay(message refreshedMsg) time.Duration {
	if !message.polled {
		return max(message.expansion.outcome.RetryDelay, expansionRetry)
	}
	return max(message.result.Delay, defaultDelay)
}

// published replaces the selection, its snapshot, and its disclosure atomically
// and clears the failure state. An empty but complete success still records a
// last-success instant (FR-008). The selection degradation of an earlier failed
// expansion is cleared only by a refresh that expanded successfully itself; a
// success polled from a retained selection keeps it (RG-010).
func published(state State, selection Selection, result Result, at time.Time, expanded bool) State {
	state.Scopes = selection.Scopes
	state.Selection = selection
	state.Snapshot = domain.NewSnapshot(selection.Scopes, result.Repositories)
	state.Freshness = FreshnessCurrent
	state.LastSuccess = at
	state.Cause = ""
	if expanded {
		state.SelectionFreshness = SelectionCurrent
		state.SelectionCause = ""
	}
	return state
}

// degraded keeps the last successful snapshot untouched and marks the header. A
// failure before any success is an error state; a later failure is stale
// (FR-008).
func degraded(state State, err error) State {
	state.Freshness = FreshnessError
	if !state.LastSuccess.IsZero() {
		state.Freshness = FreshnessStale
	}
	state.Cause = sanitize(err)
	return state
}

// sanitize collapses the reported cause to one header-safe line. Sources report
// causes that already carry no credential value (NFR-003); dropping the
// non-printable runes keeps a relayed upstream string from reaching the
// terminal as an escape sequence.
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

// tickAfter is the production timer seam: it schedules refreshDueMsg without
// blocking the update loop.
func tickAfter(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg { return refreshDueMsg{} })
}
