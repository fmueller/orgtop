package tui

import (
	"time"

	"charm.land/lipgloss/v2"
)

// recency is how old an event is on RG-008's closed discrete scale. It is
// prepared outside rendering from an explicit reference time, so every view
// applies the same thresholds to the reference RG-006/RG-007 gives it and no
// renderer reads the host clock (RG-008).
type recency int

// The discrete states, newest first.
const (
	recencyNew recency = iota
	recencyRecent
	recencyAging
	recencyExpired
)

// The half-open thresholds the states start at: `new` for `0 <= age < 5m`,
// `recent` for `5m <= age < 15m`, `aging` for `15m <= age < 60m`, and `expired`
// for `age >= 60m` (RG-008).
const (
	recentAge  = 5 * time.Minute
	agingAge   = 15 * time.Minute
	expiredAge = time.Hour
)

// recencyNames spells the states as RG-008 names them, so a legend, a
// diagnostic, and a test state the one shared word.
var recencyNames = map[recency]string{
	recencyNew:     "new",
	recencyRecent:  "recent",
	recencyAging:   "aging",
	recencyExpired: "expired",
}

// String returns the shared name of the state.
func (r recency) String() string { return recencyNames[r] }

// removesRainItem reports whether the state removes a Rain item rather than
// merely hiding it. Expiry removes; it never deletes a normalized snapshot
// event or independently removes a Stream row (RG-008).
func (r recency) removesRainItem() bool { return r == recencyExpired }

// recencyAt returns the state of a prepared event age. A negative age is an
// event stamped after its applicable reference and clamps to zero, because an
// event cannot be older than the reference carrying it.
func recencyAt(age time.Duration) recency {
	switch {
	case age < recentAge:
		return recencyNew
	case age < agingAge:
		return recencyRecent
	case age < expiredAge:
		return recencyAging
	default:
		return recencyExpired
	}
}

// recencyOf returns the state of an event against the explicit reference its
// view was prepared with, never against the current clock.
func recencyOf(occurred, reference time.Time) recency {
	return recencyAt(reference.Sub(occurred))
}

// colorCapability is how much color the effective terminal profile renders. It
// is an injected capability rather than a probe of the live terminal, so the
// same inputs render the same emphasis in a test and in a session (RG-008).
type colorCapability int

// The supported profiles. No-color is the zero value, because it is the answer
// a terminal that says nothing about its color support gets, and because losing
// color never loses meaning.
const (
	capabilityNoColor colorCapability = iota
	capabilityANSI
	capabilityTruecolor
)

// capabilityNames spells the profiles as RG-008 names them.
var capabilityNames = map[colorCapability]string{
	capabilityNoColor:   "no-color",
	capabilityANSI:      "ansi",
	capabilityTruecolor: "truecolor",
}

// String returns the shared name of the profile.
func (c colorCapability) String() string { return capabilityNames[c] }

// recencyEmphasis is RG-008's closed presentation of one state: the accent of
// each color profile and the attributes that reinforce it. Bold and faint are
// optional reinforcement, so their absence never removes text or glyph meaning,
// and color reinforces recency alone: category stays encoded by glyph or text,
// and no accent implies an event outcome.
type recencyEmphasis struct {
	truecolor string
	ansi      string
	bold      bool
	faint     bool
}

// The accents. They are their own semantic role, distinct from the chrome
// colors that mean transport, freshness, and errors, and deliberately outside
// conventional success/error coloring: an event carries no outcome. The two
// oldest states share one neutral, reduced-intensity presentation, because they
// differ in Rain removal rather than in emphasis.
const (
	strongestAccent = "#e6a8ff"
	normalAccent    = "#a855c7"
	neutralAccent   = "#8a8a8a"

	strongestANSI = "13"
	normalANSI    = "5"
	neutralANSI   = "8"
)

// recencyStyles is the closed shared style table every view emphasizes a state
// through, so one prepared vocabulary answers for all of them (RG-008).
var recencyStyles = map[recency]recencyEmphasis{
	recencyNew:     {truecolor: strongestAccent, ansi: strongestANSI, bold: true},
	recencyRecent:  {truecolor: normalAccent, ansi: normalANSI},
	recencyAging:   {truecolor: neutralAccent, ansi: neutralANSI, faint: true},
	recencyExpired: {truecolor: neutralAccent, ansi: neutralANSI, faint: true},
}

// style returns the prepared style of the state at the given color capability:
// the full palette at truecolor, its nearest reduced palette at ansi, and no
// color at all otherwise. The bold and faint reinforcement is the same at every
// capability, so a monochrome terminal still separates the states.
func (r recency) style(capability colorCapability) lipgloss.Style {
	emphasis := recencyStyles[r]
	style := lipgloss.NewStyle().Bold(emphasis.bold).Faint(emphasis.faint)
	switch capability {
	case capabilityTruecolor:
		return style.Foreground(lipgloss.Color(emphasis.truecolor))
	case capabilityANSI:
		return style.Foreground(lipgloss.Color(emphasis.ansi))
	default:
		return style
	}
}
