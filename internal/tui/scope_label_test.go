package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// The A-076 fixture label and its normative rendered forms. The payload carries
// wide clusters, so every budget below the complete width exercises a boundary
// the cell-safe split must not cut through.
const (
	wideToken   = "P3"
	widePayload = "Acme/API:服務/界面/**"
)

// combiningPayload is A-076's 12-cell payload whose accented cluster is the two
// code points `e` and U+0301, so no budget may emit the mark without its base.
const combiningPayload = "acme/caf" + "e\u0301" + "/**"

// TestScopeLabelPinsTheNormativeWidthVectors pins A-076's rendered strings and
// their cell widths at every budget the contract names.
func TestScopeLabelPinsTheNormativeWidthVectors(t *testing.T) {
	vectors := []struct {
		budget int
		want   string
	}{
		{budget: 24, want: "P3 Acme/API:服務/界面/**"},
		{budget: 16, want: "P3 Acme/AP…面/**"},
		{budget: 5, want: "P3 …"},
		{budget: 2, want: "P3"},
	}
	for _, vector := range vectors {
		got := shortenLabel(wideToken, widePayload, vector.budget)
		if got != vector.want {
			t.Errorf("shortenLabel(%q, %q, %d) = %q, want %q",
				wideToken, widePayload, vector.budget, got, vector.want)
		}
		if width := lipgloss.Width(got); width > vector.budget {
			t.Errorf("shortenLabel(%q, %q, %d) is %d cells wide, want at most %d",
				wideToken, widePayload, vector.budget, width, vector.budget)
		}
	}
	if width := lipgloss.Width(wideToken + " " + widePayload); width != 24 {
		t.Errorf("the complete label is %d cells wide, want the normative 24", width)
	}
}

// TestScopeLabelKeepsACombiningSequenceWhole pins A-076's combining-mark vector:
// the two-code-point grapheme is retained together at budget 12.
func TestScopeLabelKeepsACombiningSequenceWhole(t *testing.T) {
	if width := lipgloss.Width(wideToken + " " + combiningPayload); width != 15 {
		t.Fatalf("the combining label is %d cells wide, want the normative 15", width)
	}
	got := shortenLabel(wideToken, combiningPayload, 12)
	want := "P3 acme…" + "e\u0301" + "/**"
	if got != want {
		t.Errorf("shortenLabel(%q, %q, 12) = %q, want %q", wideToken, combiningPayload, got, want)
	}
	if strings.Contains(got, "…\u0301") {
		t.Errorf("shortenLabel(%q, %q, 12) = %q emitted a combining mark without its base",
			wideToken, combiningPayload, got)
	}
}

// TestScopeLabelRendersTheDegradedFormsBelowTheToken pins the forms below the
// complete token width: the longest fitting token prefix, then nothing.
func TestScopeLabelRendersTheDegradedFormsBelowTheToken(t *testing.T) {
	if got := shortenLabel(wideToken, widePayload, 1); got != "P" {
		t.Errorf("shortenLabel at budget 1 = %q, want the longest fitting token prefix %q", got, "P")
	}
	if got := shortenLabel(wideToken, widePayload, 0); got != "" {
		t.Errorf("shortenLabel at budget 0 = %q, want the empty render", got)
	}
}

// TestScopeLabelsWithCollidingPayloadsStayDistinguishable covers two labels the
// same budget shortens to the same payload: their tokens still tell them apart.
func TestScopeLabelsWithCollidingPayloadsStayDistinguishable(t *testing.T) {
	first := shortenLabel("P2", widePayload, 16)
	second := shortenLabel("P3", widePayload, 16)
	if first == second {
		t.Fatalf("shortened labels %q and %q are indistinguishable", first, second)
	}
	firstPayload, secondPayload := strings.TrimPrefix(first, "P2 "), strings.TrimPrefix(second, "P3 ")
	if firstPayload != secondPayload {
		t.Errorf("the fixture no longer collides: %q versus %q", firstPayload, secondPayload)
	}
}

// TestScopeLabelEscapesControlAndBidiCodePoints covers RG-012's sanitization:
// user payloads cannot reach the terminal as control or bidi-reordering output.
func TestScopeLabelEscapesControlAndBidiCodePoints(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "escape", payload: "a\x1bb", want: `P3 a\u{1B}b`},
		{name: "delete", payload: "a\u007Fb", want: `P3 a\u{7F}b`},
		{name: "c1 next line", payload: "a\u0085b", want: `P3 a\u{85}b`},
		{name: "bidi override", payload: "a\u202Eb", want: `P3 a\u{202E}b`},
		{name: "isolates", payload: "a\u2066b\u2069", want: `P3 a\u{2066}b\u{2069}`},
		{name: "arabic letter mark", payload: "a\u061Cb", want: `P3 a\u{61C}b`},
		{name: "left-to-right mark", payload: "a\u200Eb", want: `P3 a\u{200E}b`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := shortenLabel(wideToken, testCase.payload, -1)
			if got != testCase.want {
				t.Errorf("shortenLabel(%q, %q, -1) = %q, want %q", wideToken, testCase.payload, got, testCase.want)
			}
		})
	}
}

// TestScopeLabelMeasuresEscapesBeforeShortening covers RG-012's ordering: an
// escaped code point occupies its rendered cells when the budget is applied.
func TestScopeLabelMeasuresEscapesBeforeShortening(t *testing.T) {
	payload := "ab\u202Ecd"
	got := shortenLabel(wideToken, payload, 12)
	if width := lipgloss.Width(got); width > 12 {
		t.Fatalf("shortenLabel(%q, %q, 12) = %q, %d cells wide, want at most 12",
			wideToken, payload, got, width)
	}
	if strings.ContainsRune(got, '\u202E') {
		t.Errorf("shortenLabel(%q, %q, 12) = %q still carries the raw bidi override",
			wideToken, payload, got)
	}
}

// TestScopeLabelEscapesUnattachedZeroWidthClusters covers RG-012's zero-cell
// rule: a cluster attached to no positive-width grapheme occupies no cell, so
// it renders as one visible escape per code point instead of disappearing.
func TestScopeLabelEscapesUnattachedZeroWidthClusters(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "standalone combining mark", payload: "\u0301ab", want: `P3 \u{301}ab`},
		{name: "standalone variation selector", payload: "\ufe0fab", want: `P3 \u{FE0F}ab`},
		{name: "standalone joiner", payload: "\u200Dab", want: `P3 \u{200D}ab`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := shortenLabel(wideToken, testCase.payload, -1)
			if got != testCase.want {
				t.Errorf("shortenLabel(%q, %q, -1) = %q, want %q", wideToken, testCase.payload, got, testCase.want)
			}
		})
	}
}

// TestScopeLabelKeepsZeroWidthCodePointsInsideAGrapheme guards the other half of
// the same rule: a joiner or variation selector that a positive-width grapheme
// carries stays unescaped, so an emoji sequence is not spelled out as escapes.
func TestScopeLabelKeepsZeroWidthCodePointsInsideAGrapheme(t *testing.T) {
	payloads := []string{
		"a\u200Db",
		"acme/\U0001f469\u200D\U0001f4bb/**",
		"acme/\u2764\uFE0F/**",
	}
	for _, payload := range payloads {
		want := wideToken + " " + payload
		if got := shortenLabel(wideToken, payload, -1); got != want {
			t.Errorf("shortenLabel(%q, %q, -1) = %q, want it unescaped", wideToken, payload, got)
		}
	}
}
