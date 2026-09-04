package tui

import "time"

// rainWindowDurations are RG-006's closed session-local Rain window presets,
// shortest first. Rain defaults to the longest, and `-`/`+` step one preset
// without wrapping.
var rainWindowDurations = []time.Duration{15 * time.Minute, 30 * time.Minute, time.Hour}

// rainWindowNames spell the presets exactly as the visible `window 15m|30m|60m`
// context names them, so a legend, a diagnostic, and a test share one word.
var rainWindowNames = []string{"15m", "30m", "60m"}

// rainWindow selects one preset by index. The zero value is the shortest
// preset, so defaultRainWindow rather than the zero value starts a session.
type rainWindow int

// defaultRainWindow is the 60-minute preset Rain starts at (RG-006).
const defaultRainWindow rainWindow = 2

// duration returns the selected window length.
func (w rainWindow) duration() time.Duration { return rainWindowDurations[w.clamped()] }

// String returns the shared name of the selected preset.
func (w rainWindow) String() string { return rainWindowNames[w.clamped()] }

// stepped returns the preset one step shorter (-1) or longer (+1). Either key
// at its endpoint is a no-op, so stepping never wraps.
func (w rainWindow) stepped(step int) rainWindow {
	return rainWindow(min(max(int(w.clamped())+step, 0), len(rainWindowDurations)-1))
}

// clamped keeps an out-of-range window inside the closed preset list, so a
// corrupted index degrades to a real preset rather than panicking a view.
func (w rainWindow) clamped() rainWindow {
	return rainWindow(min(max(int(w), 0), len(rainWindowDurations)-1))
}
