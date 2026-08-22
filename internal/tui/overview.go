package tui

// overview is the Overview view's state slot and rendering seam. Repository
// aggregate rows replace the placeholder body without changing this seam.
type overview struct {
	// offset is the first rendered row. The shell preserves it across mode
	// switches so a view keeps its scroll position (FR-007).
	offset int
}

// render returns the Overview body for the shared content area.
func (o overview) render(state State, width, height int) string {
	return renderBody(overviewLines(state), o.offset, width, height)
}

// overviewLines returns the explicit state or placeholder content lines.
func overviewLines(state State) []string {
	switch state.Freshness {
	case FreshnessLoading:
		return []string{"Loading repository activity…"}
	case FreshnessError:
		return []string{"Repository activity is unavailable"}
	default:
		return []string{"Repository activity is not rendered yet"}
	}
}
