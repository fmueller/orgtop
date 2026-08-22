package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// chromeLines is the number of shared header and footer lines the body yields.
const chromeLines = 2

// Model is OrgTop's root Bubble Tea model. It owns the active mode, the
// terminal dimensions, the shared state both views read, and each view's own
// state slot. Rendering only formats that state.
type Model struct {
	state    State
	mode     Mode
	width    int
	height   int
	sized    bool
	overview overview
	stream   stream
}

// New returns the root model for the selected scope in its initial loading
// state. Issuing the first refresh belongs to the refresh lifecycle.
func New(scope domain.Scope) Model {
	return Model{state: State{Scope: scope, Freshness: FreshnessLoading}}
}

// Init implements tea.Model. The shell itself starts no work.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height, m.sized = max(message.Width, 0), max(message.Height, 0), true
	case tea.KeyPressMsg:
		return m.handleKey(message)
	}
	return m, nil
}

// handleKey applies navigation and quit keystrokes.
func (m Model) handleKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "q", "ctrl+c":
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

// View implements tea.Model.
func (m Model) View() tea.View { return tea.NewView(m.render()) }

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
