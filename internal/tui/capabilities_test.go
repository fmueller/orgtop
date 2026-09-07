package tui

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// capabilityFixture is one launch environment and the rendering it must reach:
// the resolved capability pair, the category glyph the shared vocabulary draws
// at that repertoire, and whether the recency styles emit colour at all.
type capabilityFixture struct {
	name    string
	environ []string
	want    string
	glyph   string
	colored bool
}

// capabilityFixtures cover the environments RG-008 names. CLICOLOR_FORCE keeps
// the colour resolution deterministic without a live terminal: the fixtures
// resolve against a discarded, non-terminal output, exactly as a test must.
var capabilityFixtures = []capabilityFixture{
	{
		name:    "utf-8 truecolor",
		environ: []string{"LANG=en_US.UTF-8", "TERM=xterm", "COLORTERM=truecolor", "CLICOLOR_FORCE=1"},
		want:    "utf-8/truecolor",
		glyph:   "↑",
		colored: true,
	},
	{
		name:    "utf-8 ansi",
		environ: []string{"LC_ALL=C.UTF-8", "TERM=xterm", "CLICOLOR_FORCE=1"},
		want:    "utf-8/ansi",
		glyph:   "↑",
		colored: true,
	},
	{
		name:    "non-utf-8 locale keeps colour",
		environ: []string{"LANG=en_US.ISO-8859-1", "TERM=xterm", "COLORTERM=truecolor", "CLICOLOR_FORCE=1"},
		want:    "ascii/truecolor",
		glyph:   "P",
		colored: true,
	},
	{
		name:    "dumb terminal",
		environ: []string{"LANG=en_US.UTF-8", "TERM=dumb", "COLORTERM=truecolor", "CLICOLOR_FORCE=1"},
		want:    "ascii/no-color",
		glyph:   "P",
	},
	{
		name:    "NO_COLOR changes colour only",
		environ: []string{"LANG=en_US.UTF-8", "TERM=xterm", "COLORTERM=truecolor", "CLICOLOR_FORCE=1", "NO_COLOR=x"},
		want:    "utf-8/no-color",
		glyph:   "↑",
	},
	{
		name:  "an environment that says nothing",
		want:  "ascii/no-color",
		glyph: "P",
	},
}

// TestResolveCapabilitiesReadsTheLaunchEnvironment guards RG-008's resolution:
// the glyph repertoire follows the effective locale and TERM, the colour
// capability follows the pinned Charm profile stack, and TERM=dumb and a
// non-empty NO_COLOR both force no-color.
func TestResolveCapabilitiesReadsTheLaunchEnvironment(t *testing.T) {
	for _, fixture := range capabilityFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			if got := ResolveCapabilities(fixture.environ, io.Discard).String(); got != fixture.want {
				t.Errorf("ResolveCapabilities(%v) resolved %q, want %q", fixture.environ, got, fixture.want)
			}
		})
	}
}

// TestResolvedCapabilitiesReachTheRenderedGlyphsAndStyles guards the wiring
// rather than the helpers: an injected environment reaches the drawn category
// glyph and the recency emphasis of the rendered field.
func TestResolvedCapabilitiesReachTheRenderedGlyphsAndStyles(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	retained := []domain.EventEvidence{rainEvidence(t, "one", repository, time.Minute)}
	rendered := map[string]string{}

	for _, fixture := range capabilityFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			model := rainModel(t, scopes, retained, 120, 20, WithCapabilities(ResolveCapabilities(fixture.environ, io.Discard)))

			content := model.View().Content
			rendered[fixture.name] = content
			drawn, colored := 0, false
			for _, line := range rainBodyLines(content) {
				// The legend spells the same glyph beside its text; only the
				// field rows draw it alone.
				if !strings.Contains(line, fixture.glyph) || strings.Contains(line, categoryText(domain.CategoryPush, registerRich)) {
					continue
				}
				drawn++
				colored = colored || colorEscape.MatchString(line)
			}
			if drawn == 0 {
				t.Fatalf("the field drew no %q glyph for %v:\n%s", fixture.glyph, fixture.environ, content)
			}
			if colored != fixture.colored {
				t.Errorf("the field rows of %v emitted colour %v, want %v:\n%s", fixture.environ, colored, fixture.colored, content)
			}
		})
	}

	if rendered["utf-8 truecolor"] == rendered["utf-8 ansi"] {
		t.Error("the truecolor and ansi profiles rendered the same recency emphasis, want the reduced palette at ansi")
	}
}
