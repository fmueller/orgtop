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
	Refresh(ctx context.Context, scope domain.Scope) (Result, error)
}

// refreshedMsg reports one completed refresh attempt back into the update loop.
type refreshedMsg struct {
	result Result
	err    error
}

// refreshDueMsg reports that the scheduled delay elapsed.
type refreshDueMsg struct{}

// refresh returns the command that performs one refresh. The command owns all
// source I/O, so keyboard, resize, and quit handling never wait for it.
func (m Model) refresh() tea.Cmd {
	ctx, source, scope := m.ctx, m.source, m.state.Scope
	return func() tea.Msg {
		result, err := source.Refresh(ctx, scope)
		return refreshedMsg{result: result, err: err}
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
func (m Model) applyRefresh(message refreshedMsg) (tea.Model, tea.Cmd) {
	m.pending = false
	if message.err != nil {
		m.state = degraded(m.state, message.err)
	} else {
		m.state = published(m.state, message.result, m.now())
	}
	return m, m.tick(max(message.result.Delay, defaultDelay))
}

// published replaces the snapshot atomically and clears the failure state. An
// empty but complete success still records a last-success instant (FR-008).
func published(state State, result Result, at time.Time) State {
	state.Snapshot = domain.NewSnapshot(state.Scope, result.Repositories)
	state.Freshness = FreshnessCurrent
	state.LastSuccess = at
	state.Cause = ""
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
