package tui

// viewport is the scrolling mechanism both root views share: the first rendered
// row plus the single clamp that keeps a window inside its content. Each view
// owns its own viewport, and the shell preserves it across mode switches so a
// view keeps its scroll position (FR-007).
type viewport struct {
	// offset is the first rendered row.
	offset int
}

// scrolled returns the viewport moved by one scrolling keystroke over count
// lines in a body of height rows. The offset is clamped before and after the
// move, so a refresh that shrank the content or a resize that grew the body
// never leaves the window past its last position.
func (v viewport) scrolled(keystroke string, count, height int) viewport {
	moved := viewport{offset: v.clamped(count, height)}
	switch keystroke {
	case "up":
		moved.offset--
	case "down":
		moved.offset++
	case "pgup":
		moved.offset -= height
	case "pgdown":
		moved.offset += height
	}
	return viewport{offset: moved.clamped(count, height)}
}

// clamped bounds the stored first rendered row so the window always ends at the
// last line. A non-positive height renders unbounded, where nothing scrolls.
func (v viewport) clamped(count, height int) int {
	if height <= 0 {
		return 0
	}
	return min(max(v.offset, 0), max(count-height, 0))
}
