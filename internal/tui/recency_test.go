package tui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// recencyCase is one RG-008 recency boundary: a prepared event age and the
// discrete state it falls in.
type recencyCase struct {
	age   time.Duration
	state recency
}

// recencyContract is RG-008's half-open recency model, transcribed from the
// spec rather than from the implementation so a drifting threshold fails here:
// `new` for `0 <= age < 5m`, `recent` for `5m <= age < 15m`, `aging` for
// `15m <= age < 60m`, and `expired` for `age >= 60m`.
var recencyContract = []recencyCase{
	{age: -time.Hour, state: recencyNew},
	{age: -time.Nanosecond, state: recencyNew},
	{age: 0, state: recencyNew},
	{age: time.Nanosecond, state: recencyNew},
	{age: 5*time.Minute - time.Nanosecond, state: recencyNew},
	{age: 5 * time.Minute, state: recencyRecent},
	{age: 15*time.Minute - time.Nanosecond, state: recencyRecent},
	{age: 15 * time.Minute, state: recencyAging},
	{age: time.Hour - time.Nanosecond, state: recencyAging},
	{age: time.Hour, state: recencyExpired},
	{age: 100 * time.Hour, state: recencyExpired},
}

// recencyStates lists the discrete states in progression order.
var recencyStates = []recency{recencyNew, recencyRecent, recencyAging, recencyExpired}

// colorCapabilities lists RG-008's injectable rendering capabilities.
var colorCapabilities = []colorCapability{capabilityTruecolor, capabilityANSI, capabilityNoColor}

// accentedStates are the states a color capability accents distinctly, and
// neutralStates the older ones that deliberately share one reduced-intensity
// neutral, because they differ in Rain removal rather than in emphasis.
var (
	accentedStates = []recency{recencyNew, recencyRecent, recencyAging}
	neutralStates  = []recency{recencyAging, recencyExpired}
)

// sample is the text a style is rendered over when two styles are compared.
const sample = "x"

// TestRecencyThresholdsMatchTheSharedContract guards RG-008: every boundary is
// half-open at the exact tabled minute, and a negative age clamps to zero.
func TestRecencyThresholdsMatchTheSharedContract(t *testing.T) {
	for _, want := range recencyContract {
		if got := recencyAt(want.age); got != want.state {
			t.Errorf("recency at age %s is %q, want %q", want.age, got, want.state)
		}
	}
}

// TestRecencyClampsAnEventAheadOfItsReference guards RG-008: an event stamped
// after its applicable reference is age zero, not a negative age that would
// order before the newest one or read as a rendering fault.
func TestRecencyClampsAnEventAheadOfItsReference(t *testing.T) {
	reference := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	ahead := reference.Add(time.Hour)
	if got := recencyOf(ahead, reference); got != recencyNew {
		t.Errorf("an event %s ahead of its reference is %q, want %q", time.Hour, got, recencyNew)
	}
}

// TestRecencyOfUsesTheExplicitReference guards RG-008: recency is calculated
// from the prepared reference the view was given, never from the host clock, so
// an old snapshot keeps the states it published.
func TestRecencyOfUsesTheExplicitReference(t *testing.T) {
	reference := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, want := range recencyContract {
		age := max(want.age, 0)
		if got := recencyOf(reference.Add(-age), reference); got != want.state {
			t.Errorf("an event %s before the reference is %q, want %q", age, got, want.state)
		}
	}
}

// TestRecencyNamesMatchTheSharedContract guards RG-008: the states are spelled
// as the spec names them, so a legend or a diagnostic states the shared word.
func TestRecencyNamesMatchTheSharedContract(t *testing.T) {
	names := map[recency]string{
		recencyNew:     "new",
		recencyRecent:  "recent",
		recencyAging:   "aging",
		recencyExpired: "expired",
	}
	for state, want := range names {
		if got := state.String(); got != want {
			t.Errorf("recency %d is named %q, want %q", int(state), got, want)
		}
	}
}

// TestOnlyExpiredRemovesARainItem guards RG-008: expiry removes a Rain item
// rather than merely hiding it, and every earlier state stays visible.
func TestOnlyExpiredRemovesARainItem(t *testing.T) {
	for _, state := range recencyStates {
		want := state == recencyExpired
		if got := state.removesRainItem(); got != want {
			t.Errorf("removesRainItem of %q is %t, want %t", state, got, want)
		}
	}
}

// TestRecencyStylesReinforceEmphasisOnly guards RG-008's style table: `new` is
// bold, `recent` is plain, and both older states are the same reduced-intensity
// neutral, at every capability.
func TestRecencyStylesReinforceEmphasisOnly(t *testing.T) {
	for _, capability := range colorCapabilities {
		t.Run(capability.String(), func(t *testing.T) {
			if !recencyNew.style(capability).GetBold() {
				t.Error("new is not bold, want the strongest emphasis")
			}
			recent := recencyRecent.style(capability)
			if recent.GetBold() {
				t.Error("recent is bold, want a plainer weight than new")
			}
			if recent.GetFaint() {
				t.Error("recent is faint, want normal intensity")
			}
			for _, state := range neutralStates {
				style := state.style(capability)
				if !style.GetFaint() {
					t.Errorf("%q is not faint, want reduced intensity", state)
				}
				if style.GetBold() {
					t.Errorf("%q is bold, want reduced intensity", state)
				}
			}
			aging, expired := recencyAging.style(capability), recencyExpired.style(capability)
			if aging.Render(sample) != expired.Render(sample) {
				t.Errorf("aging renders %q and expired %q, want the same reduced intensity", aging.Render(sample), expired.Render(sample))
			}
		})
	}
}

// reinforcementAttributes are the SGR parameters RG-008 allows a no-color
// rendering to carry: bold and faint reinforcement, and the reset that ends
// them. Any other parameter would be color the profile must not spend.
var reinforcementAttributes = map[string]bool{"": true, "0": true, "1": true, "2": true}

// TestNoColorCarriesNoColor guards RG-008: the no-color capability spends no
// color at all, while its bold and faint reinforcement stays.
func TestNoColorCarriesNoColor(t *testing.T) {
	for _, state := range recencyStates {
		rendered := state.style(capabilityNoColor).Render(sample)
		for _, parameter := range sgrParameters(rendered) {
			if !reinforcementAttributes[parameter] {
				t.Errorf("%q renders %q at no-color, want no color beyond bold and faint", state, rendered)
			}
		}
		if plain := ansi.Strip(rendered); plain != sample {
			t.Errorf("%q renders the text as %q at no-color, want %q", state, plain, sample)
		}
	}
}

// sgrPattern matches one select-graphic-rendition escape and captures its
// semicolon-separated parameters.
var sgrPattern = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// sgrParameters returns every SGR parameter the rendered text sets.
func sgrParameters(rendered string) []string {
	var parameters []string
	for _, match := range sgrPattern.FindAllStringSubmatch(rendered, -1) {
		parameters = append(parameters, strings.Split(match[1], ";")...)
	}
	return parameters
}

// TestColorCapabilitiesAccentTheNewestStates guards RG-008: a capability that
// carries color gives `new` the strongest accent and `recent` a distinct normal
// one, while the older states share the neutral.
func TestColorCapabilitiesAccentTheNewestStates(t *testing.T) {
	for _, capability := range []colorCapability{capabilityTruecolor, capabilityANSI} {
		t.Run(capability.String(), func(t *testing.T) {
			accents := make(map[string]recency, len(accentedStates))
			for _, state := range accentedStates {
				foreground := state.style(capability).GetForeground()
				if foreground == nil {
					t.Fatalf("%q has no foreground at %s, want an accent", state, capability)
				}
				rendered := lipgloss.NewStyle().Foreground(foreground).Render(sample)
				if previous, taken := accents[rendered]; taken {
					t.Errorf("%q reuses the accent of %q, want a distinct one", state, previous)
				}
				accents[rendered] = state
			}
		})
	}
}

// TestRecencyRenderingIsDeterministic guards RG-008: identical timestamp,
// reference, and capability inputs produce identical output, without a live
// terminal or a wall-clock read.
func TestRecencyRenderingIsDeterministic(t *testing.T) {
	reference := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	for _, want := range recencyContract {
		occurred := reference.Add(-max(want.age, 0))
		for _, capability := range colorCapabilities {
			first := recencyOf(occurred, reference).style(capability).Render(sample)
			second := recencyOf(occurred, reference).style(capability).Render(sample)
			if first != second {
				t.Errorf("age %s at %s rendered %q then %q, want one value", want.age, capability, first, second)
			}
		}
	}
}
