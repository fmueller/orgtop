package tui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// chromeLines is the number of shared header and footer lines the body yields.
const chromeLines = 2

// Model is OrgTop's root Bubble Tea model. It owns the active mode, the
// terminal dimensions, the shared state both views read, each view's own state
// slot, and the refresh lifecycle. Rendering only formats that state.
type Model struct {
	state    State
	mode     Mode
	width    int
	height   int
	sized    bool
	overview overview
	stream   stream
	// ctx bounds every refresh. Bubble Tea drives the model by message, so the
	// cancelable root of the source work has to live with the model itself.
	ctx context.Context
	// cancel stops in-flight source work at shutdown.
	cancel context.CancelFunc
	// source performs one atomic refresh of the Scope.
	source Source
	// now supplies the instant a success is recorded at.
	now func() time.Time
	// tick schedules the next attempt after a delay.
	tick func(time.Duration) tea.Cmd
	// pending reports whether a refresh is in flight.
	pending bool
}

// New returns the root model for the selected scope in its initial loading
// state, with its first refresh already owned by Init. The source must not be
// nil, and ctx bounds every refresh the model starts.
func New(ctx context.Context, scope domain.Scope, source Source) Model {
	refreshCtx, cancel := context.WithCancel(ctx)
	return Model{
		state:   State{Scope: scope, Freshness: FreshnessLoading},
		ctx:     refreshCtx,
		cancel:  cancel,
		source:  source,
		now:     time.Now,
		tick:    tickAfter,
		pending: true,
	}
}

// Init implements tea.Model. The first refresh runs as a command, so the
// initial LOADING render is never delayed by source I/O (FR-007).
func (m Model) Init() tea.Cmd { return m.refresh() }

// Update implements tea.Model.
func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height, m.sized = max(message.Width, 0), max(message.Height, 0), true
	case tea.KeyPressMsg:
		return m.handleKey(message)
	case refreshDueMsg:
		return m.startRefresh()
	case refreshedMsg:
		return m.applyRefresh(message)
	}
	return m, nil
}

// handleKey applies navigation and quit keystrokes.
func (m Model) handleKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "q", "ctrl+c":
		m.cancel()
		return m, tea.Quit
	case "1":
		m.mode = ModeOverview
	case "2":
		m.mode = ModeStream
	case "tab":
		m.mode = m.mode.toggled()
	}
	return m, nil
}

// View implements tea.Model. OrgTop owns the whole terminal while it runs, so
// the shell renders on the alternate screen and leaves the caller's screen
// untouched on exit (FR-001).
func (m Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	return view
}

// render composes the shared chrome around the active view's body. The header
// wins the tightest budgets, then the quit hint, then the body.
func (m Model) render() string {
	width, height := m.budget()
	if height == 0 {
		return ""
	}
	header := renderHeader(m.state, m.mode, width)
	if height == 1 {
		return header
	}
	footer := renderFooter(width)
	if height == chromeLines {
		return strings.Join([]string{header, footer}, "\n")
	}
	return strings.Join([]string{header, m.body(width, height-chromeLines), footer}, "\n")
}

// budget returns the render budget. Both dimensions stay unbounded until a
// resize message reports the terminal size; a reported zero is a real bound.
func (m Model) budget() (width, height int) {
	if !m.sized {
		return unbounded, unbounded
	}
	return m.width, m.height
}

// body delegates rendering to the active view within the content area the
// shared chrome leaves.
func (m Model) body(width, height int) string {
	if m.mode == ModeStream {
		return m.stream.render(m.state, width, height)
	}
	return m.overview.render(m.state, width, height)
}

// renderBody windows the view's lines at its offset and fills the content area.
// A negative height leaves the content unbounded.
func renderBody(lines []string, offset, width, height int) string {
	visible := lines[min(max(offset, 0), len(lines)):]
	if height > 0 && len(visible) > height {
		visible = visible[:height]
	}

	rendered := make([]string, 0, max(len(visible), height))
	for _, line := range visible {
		rendered = append(rendered, bodyStyle.Render(truncate(line, width)))
	}
	for len(rendered) < height {
		rendered = append(rendered, "")
	}
	return strings.Join(rendered, "\n")
}
