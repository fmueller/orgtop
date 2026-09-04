package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// categoryCase is RG-008's closed category row: the two text registers and the
// two glyph repertoires one normalized category is rendered from.
type categoryCase struct {
	category domain.Category
	rich     string
	compact  string
	utf8     string
	ascii    string
}

// otherContract is the catch-all row every unsupported category normalizes to.
var otherContract = categoryCase{category: domain.CategoryOther, rich: "other", compact: "other", utf8: "·", ascii: "?"}

// categoryContract is RG-008's category table, transcribed from the spec rather
// than from the implementation so a drifting mapping fails here.
var categoryContract = []categoryCase{
	{category: domain.CategoryPush, rich: "push", compact: "push", utf8: "↑", ascii: "P"},
	{category: domain.CategoryPullRequest, rich: "pull request", compact: "pull", utf8: "↗", ascii: "R"},
	{category: domain.CategoryReview, rich: "review", compact: "review", utf8: "◇", ascii: "V"},
	{category: domain.CategoryComment, rich: "comment", compact: "comment", utf8: "○", ascii: "C"},
	otherContract,
}

// categoryEncodings names each encoding RG-008 tables and how a category is
// rendered through it, so a check that holds for every encoding is written once.
var categoryEncodings = map[string]func(domain.Category) string{
	"rich":    func(category domain.Category) string { return categoryText(category, registerRich) },
	"compact": func(category domain.Category) string { return categoryText(category, registerCompact) },
	"utf-8":   func(category domain.Category) string { return categoryGlyph(category, charsetUTF8) },
	"ascii":   func(category domain.Category) string { return categoryGlyph(category, charsetASCII) },
}

// TestCategoryVocabularyMatchesTheSharedContract guards RG-008: every shipped
// category keeps its v0.1.0 text and gains exactly the tabled glyphs.
func TestCategoryVocabularyMatchesTheSharedContract(t *testing.T) {
	for _, want := range categoryContract {
		t.Run(string(want.category), func(t *testing.T) {
			if got := categoryText(want.category, registerRich); got != want.rich {
				t.Errorf("rich text of %q is %q, want %q", want.category, got, want.rich)
			}
			if got := categoryText(want.category, registerCompact); got != want.compact {
				t.Errorf("compact text of %q is %q, want %q", want.category, got, want.compact)
			}
			if got := categoryGlyph(want.category, charsetUTF8); got != want.utf8 {
				t.Errorf("UTF-8 glyph of %q is %q, want %q", want.category, got, want.utf8)
			}
			if got := categoryGlyph(want.category, charsetASCII); got != want.ascii {
				t.Errorf("ASCII glyph of %q is %q, want %q", want.category, got, want.ascii)
			}
		})
	}
}

// TestCategoryGlyphsAreOneCell guards RG-008: a Rain slot spends one terminal
// cell on the glyph, so neither repertoire may widen past it.
func TestCategoryGlyphsAreOneCell(t *testing.T) {
	for _, want := range categoryContract {
		for _, set := range []charset{charsetUTF8, charsetASCII} {
			glyph := categoryGlyph(want.category, set)
			if width := lipgloss.Width(glyph); width != 1 {
				t.Errorf("glyph %q of %q is %d cells wide, want 1", glyph, want.category, width)
			}
		}
	}
}

// TestUnsupportedCategoriesRenderAsOther guards RG-008: an absent, malformed, or
// newly introduced source category normalizes to `other` before rendering rather
// than earning an ad hoc glyph or spelling.
func TestUnsupportedCategoriesRenderAsOther(t *testing.T) {
	unsupported := []domain.Category{"", "  ", "PUSH", "deployment", "release", "workflow_run", "pull-request"}

	for _, category := range unsupported {
		t.Run(string(category), func(t *testing.T) {
			if got := normalizeCategory(category); got != domain.CategoryOther {
				t.Errorf("category %q normalizes to %q, want %q", category, got, domain.CategoryOther)
			}
			if got := categoryText(category, registerRich); got != otherContract.rich {
				t.Errorf("rich text of %q is %q, want the catch-all %q", category, got, otherContract.rich)
			}
			if got := categoryText(category, registerCompact); got != otherContract.compact {
				t.Errorf("compact text of %q is %q, want the catch-all %q", category, got, otherContract.compact)
			}
			if got := categoryGlyph(category, charsetUTF8); got != otherContract.utf8 {
				t.Errorf("UTF-8 glyph of %q is %q, want the catch-all %q", category, got, otherContract.utf8)
			}
			if got := categoryGlyph(category, charsetASCII); got != otherContract.ascii {
				t.Errorf("ASCII glyph of %q is %q, want the catch-all %q", category, got, otherContract.ascii)
			}
		})
	}
}

// TestSupportedCategoriesNormalizeToThemselves guards RG-008: normalization only
// rescues an unsupported input and never rewrites a shipped category.
func TestSupportedCategoriesNormalizeToThemselves(t *testing.T) {
	for _, want := range categoryContract {
		if got := normalizeCategory(want.category); got != want.category {
			t.Errorf("category %q normalizes to %q, want itself", want.category, got)
		}
	}
}

// TestCategoryEncodingsAreDistinct guards RG-008: category stays readable
// without color, so no two categories may share a text or a glyph encoding.
func TestCategoryEncodingsAreDistinct(t *testing.T) {
	for name, encode := range categoryEncodings {
		spelled := map[string]domain.Category{}
		for _, want := range categoryContract {
			value := encode(want.category)
			if value == "" {
				t.Errorf("the %s encoding of %q is empty", name, want.category)
			}
			if previous, duplicate := spelled[value]; duplicate {
				t.Errorf("the %s encoding spells both %q and %q as %q", name, previous, want.category, value)
			}
			spelled[value] = want.category
		}
	}
}

// TestCategoryEncodingsCarryNoStyle guards RG-008: the shared mapping prepares
// plain tokens, so a no-color or reduced-color terminal renders the same
// category encoding a full-color one does.
func TestCategoryEncodingsCarryNoStyle(t *testing.T) {
	for _, want := range categoryContract {
		for name, encode := range categoryEncodings {
			if token := encode(want.category); strings.ContainsRune(token, '\x1b') {
				t.Errorf("the %s encoding of %q carries a terminal escape: %q", name, want.category, token)
			}
		}
	}
}

// TestCharsetResolutionFollowsTheLocaleContract guards RG-008: the glyph
// repertoire is decided by the first non-empty locale variable's codeset and by
// TERM, and NO_COLOR changes color only rather than the character set.
func TestCharsetResolutionFollowsTheLocaleContract(t *testing.T) {
	cases := []struct {
		name        string
		environment map[string]string
		want        charset
	}{
		{name: "no locale at all", environment: map[string]string{}, want: charsetASCII},
		{name: "utf-8 language", environment: map[string]string{"LANG": "en_US.UTF-8"}, want: charsetUTF8},
		{name: "utf8 without the hyphen", environment: map[string]string{"LANG": "en_US.UTF8"}, want: charsetUTF8},
		{name: "lowercase codeset", environment: map[string]string{"LANG": "en_US.utf-8"}, want: charsetUTF8},
		{name: "codeset before a modifier", environment: map[string]string{"LANG": "en_US.UTF8@modifier"}, want: charsetUTF8},
		{name: "a dot inside the modifier", environment: map[string]string{"LANG": "en_US.UTF-8@currency.euro"}, want: charsetUTF8},
		{name: "a dot inside the modifier of a latin-1 locale", environment: map[string]string{"LANG": "en_US.ISO-8859-1@currency.euro"}, want: charsetASCII},
		{name: "a dotted modifier without a codeset", environment: map[string]string{"LANG": "en_US@currency.euro"}, want: charsetASCII},
		{name: "the C.UTF-8 locale", environment: map[string]string{"LANG": "C.UTF-8"}, want: charsetUTF8},
		{name: "a modifier without a codeset", environment: map[string]string{"LANG": "en_US@modifier"}, want: charsetASCII},
		{name: "a latin-1 codeset", environment: map[string]string{"LANG": "en_US.ISO-8859-1"}, want: charsetASCII},
		{name: "the bare C locale", environment: map[string]string{"LANG": "C"}, want: charsetASCII},
		{
			name:        "LC_ALL outranks the rest",
			environment: map[string]string{"LC_ALL": "en_US.UTF-8", "LC_CTYPE": "C", "LANG": "C"},
			want:        charsetUTF8,
		},
		{
			name:        "an empty LC_ALL falls through to LC_CTYPE",
			environment: map[string]string{"LC_ALL": "", "LC_CTYPE": "en_US.UTF-8", "LANG": "C"},
			want:        charsetUTF8,
		},
		{
			name:        "LC_CTYPE outranks LANG",
			environment: map[string]string{"LC_CTYPE": "C", "LANG": "en_US.UTF-8"},
			want:        charsetASCII,
		},
		{
			name:        "a dumb terminal keeps ASCII",
			environment: map[string]string{"TERM": "dumb", "LC_ALL": "en_US.UTF-8"},
			want:        charsetASCII,
		},
		{
			name:        "another terminal keeps the locale's answer",
			environment: map[string]string{"TERM": "xterm-256color", "LC_ALL": "en_US.UTF-8"},
			want:        charsetUTF8,
		},
		{
			name:        "NO_COLOR changes color only",
			environment: map[string]string{"NO_COLOR": "1", "LC_ALL": "en_US.UTF-8"},
			want:        charsetUTF8,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := resolveCharset(func(name string) string { return testCase.environment[name] })
			if got != testCase.want {
				t.Errorf("charset of %v is %v, want %v", testCase.environment, got, testCase.want)
			}
		})
	}
}

// TestStreamLayoutsUseTheSharedVocabulary guards RG-008: Stream owns no category
// spelling of its own, so every layout on its ladder names a category exactly as
// one shared register does.
func TestStreamLayoutsUseTheSharedVocabulary(t *testing.T) {
	registers := []categoryRegister{registerRich, registerCompact}
	if len(streamLayouts) != len(registers) {
		t.Fatalf("stream has %d layouts, want one per shared register", len(streamLayouts))
	}

	for index, layout := range streamLayouts {
		for _, want := range categoryContract {
			got := layout.name(want.category)
			if expected := categoryText(want.category, registers[index]); got != expected {
				t.Errorf("layout %d names %q %q, want the shared %q", index, want.category, got, expected)
			}
		}
		if got := layout.name("workflow_run"); got != categoryText(domain.CategoryOther, registers[index]) {
			t.Errorf("layout %d names an unsupported category %q, want the catch-all", index, got)
		}
	}
}

// TestStreamRendersTheSharedCategoryTextAtEveryWidth guards RG-008: the rendered
// rows carry the shared text rather than a per-view abbreviation, so the same
// category reads the same way in every view and at every terminal size.
func TestStreamRendersTheSharedCategoryTextAtEveryWidth(t *testing.T) {
	cases := []struct {
		name     string
		width    int
		register categoryRegister
	}{
		{name: "wide", width: wideWidth, register: registerRich},
		{name: "narrow", width: narrowWidth, register: registerCompact},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			content := renderAt(t, streamModel(t, detailedEvents(t)), testCase.width, wideHeight)
			rows := eventRows(t, content)
			if len(rows) != len(categoryContract) {
				t.Fatalf("stream rendered %d rows, want one per category:\n%s", len(rows), content)
			}
			for index, want := range categoryContract {
				text := categoryText(want.category, testCase.register)
				if !strings.Contains(rows[index], text) {
					t.Errorf("row %q does not spell %q as the shared %q", rows[index], want.category, text)
				}
			}
		})
	}
}
