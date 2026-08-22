package tui

// stream is the Stream view's state slot and rendering seam. Reverse-
// chronological event rows replace the placeholder body without changing this
// seam.
type stream struct {
	// offset is the first rendered row. The shell preserves it across mode
	// switches so a view keeps its scroll position (FR-007).
	offset int
}

// render returns the Stream body for the shared content area.
func (s stream) render(state State, width, height int) string {
	return renderBody(streamLines(state), s.offset, width, height)
}

// streamLines returns the explicit state or placeholder content lines.
func streamLines(state State) []string {
	switch state.Freshness {
	case FreshnessLoading:
		return []string{"Loading recent events…"}
	case FreshnessError:
		return []string{"Recent events are unavailable"}
	default:
		return []string{"Recent events are not rendered yet"}
	}
}
