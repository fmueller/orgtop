package tui

import (
	"io"
	"strings"

	"github.com/charmbracelet/colorprofile"
)

// noColorVariable is the environment variable that disables colour. Any
// non-empty value disables it, per https://no-color.org/, which is wider than
// the boolean the profile stack parses, so the resolution below applies it
// itself rather than leaving `NO_COLOR=x` coloured.
const noColorVariable = "NO_COLOR"

// Capabilities is the rendering repertoire of one launch: the glyph charset the
// shared category vocabulary is drawn from and how much colour the recency
// styles may spend. It is resolved once, outside rendering, and injected as
// prepared state, so no view or renderer inspects the environment or the
// terminal for itself and an identical input always renders identically
// (RG-008).
type Capabilities struct {
	charset    charset
	capability colorCapability
}

// String names the resolved pair as RG-008 spells its parts, so a diagnostic
// and a test record one capability rather than two anonymous numbers.
func (c Capabilities) String() string {
	return c.charset.String() + "/" + c.capability.String()
}

// ResolveCapabilities resolves the rendering capabilities of a launch from its
// environment and the stream it renders to. environ carries the process
// environment in os.Environ form and output the program's own writer, because
// the colour profile depends on whether that stream is a terminal. Both are
// parameters rather than process reads, so a test resolves any environment
// without a live terminal.
func ResolveCapabilities(environ []string, output io.Writer) Capabilities {
	lookup := environLookup(environ)
	return Capabilities{
		charset:    resolveCharset(lookup),
		capability: resolveColorCapability(lookup, colorprofile.Detect(output, environ)),
	}
}

// WithCapabilities injects the resolved rendering capabilities of the launch.
// Supplying none keeps the zero value, which is the ASCII, no-colour answer a
// terminal that says nothing about itself gets.
func WithCapabilities(capabilities Capabilities) Option {
	return func(model *Model) {
		model.charset, model.capability = capabilities.charset, capabilities.capability
	}
}

// environLookup turns an os.Environ slice into the lookup the shared charset
// resolution reads, resolving an absent variable to the empty string. A later
// assignment of the same name wins, matching both POSIX and the profile stack's
// own reading of the same slice.
func environLookup(environ []string) func(string) string {
	values := make(map[string]string, len(environ))
	for _, entry := range environ {
		name, value, _ := strings.Cut(entry, "=")
		values[name] = value
	}
	return func(name string) string { return values[name] }
}

// resolveColorCapability maps the detected profile onto RG-008's closed colour
// capability. A dumb terminal and a non-empty NO_COLOR both force no-color, and
// every profile below sixteen colours is no-color too, because a profile that
// cannot separate the recency states by intensity is the one Rain answers with
// the textual recency counts instead.
func resolveColorCapability(lookup func(string) string, profile colorprofile.Profile) colorCapability {
	if lookup("TERM") == dumbTerminal || lookup(noColorVariable) != "" {
		return capabilityNoColor
	}
	switch {
	case profile >= colorprofile.TrueColor:
		return capabilityTruecolor
	case profile >= colorprofile.ANSI:
		return capabilityANSI
	default:
		return capabilityNoColor
	}
}
