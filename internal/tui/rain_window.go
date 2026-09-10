package tui

import "time"

// rainWindow selects one of RG-006's closed session-local Rain presets. The
// zero value is the shortest preset, so defaultRainWindow rather than the zero
// value starts a session.
type rainWindow int

// The presets, shortest first, with the honest current-snapshot choice last.
// `available` is not a longer lifetime but the absence of one: it admits every
// confirmed membership of the bounded snapshot and never claims complete
// repository history.
const (
	rainWindow15m rainWindow = iota
	rainWindow30m
	rainWindow60m
	rainWindow6h
	rainWindow24h
	rainWindow7d
	rainWindowAvailable
)

// defaultRainWindow is the 24-hour preset Rain starts at, which keeps a useful
// ambient field in a quiet repository (RG-006).
const defaultRainWindow = rainWindow24h

// rainWindowDurations are the lifetimes of the finite presets, indexed by
// preset. `available` has no entry, because it removes an item only when the
// bounded snapshot stops returning it.
var rainWindowDurations = []time.Duration{
	rainWindow15m: 15 * time.Minute,
	rainWindow30m: 30 * time.Minute,
	rainWindow60m: time.Hour,
	rainWindow6h:  6 * time.Hour,
	rainWindow24h: 24 * time.Hour,
	rainWindow7d:  7 * 24 * time.Hour,
}

// rainWindowNames spell the presets exactly as the visible
// `window 15m|30m|60m|6h|24h|7d|available` context names them, so a legend, a
// diagnostic, and a test share one word.
var rainWindowNames = []string{"15m", "30m", "60m", "6h", "24h", "7d", "available"}

// admits reports whether an effective Rain age is still inside the selected
// window. Every finite preset removes an item at its exact boundary, while
// `available` admits every age: its eligibility is membership of the bounded
// current or retained stale snapshot alone.
func (w rainWindow) admits(age time.Duration) bool {
	preset := w.clamped()
	return preset == rainWindowAvailable || age < rainWindowDurations[preset]
}

// String returns the shared name of the selected preset.
func (w rainWindow) String() string { return rainWindowNames[w.clamped()] }

// stepped returns the preset one step shorter (-1) or longer (+1). Either key
// at its endpoint is a no-op, so stepping never wraps: clamping the stepped
// index is what turns a step off either end back into the endpoint itself.
func (w rainWindow) stepped(step int) rainWindow {
	return rainWindow(int(w.clamped()) + step).clamped()
}

// clamped keeps an out-of-range window inside the closed preset list, so a
// corrupted index degrades to a real preset rather than panicking a view.
func (w rainWindow) clamped() rainWindow {
	return rainWindow(min(max(int(w), 0), int(rainWindowAvailable)))
}
