package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// publishedRain returns a started Rain over one repository Scope holding one
// admitted push, sized to a field the given prepared rows deep, together with
// the published State one successful refresh leaves behind.
func publishedRain(t *testing.T, width, height int) (rain, State) {
	t.Helper()
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository))
	snapshot := rainSnapshot(scopes, rainEvidence(t, "one", repository, time.Minute))
	return startedRain(scopes, snapshot, width, height), State{
		Scopes:      scopes,
		Scoped:      snapshot,
		Freshness:   FreshnessCurrent,
		LastSuccess: rainBase,
	}
}

// firstLegendEntry is the first legend entry: the shared glyph beside its
// full text, which only the complete legend line spells.
func firstLegendEntry() string {
	return categoryGlyph(domain.CategoryPush, charsetUTF8) + " " + categoryText(domain.CategoryPush, registerRich)
}

// onePageField is the prepared geometry of a one-Scope selection on its only
// page: the smallest page whose context line still states every position.
func onePageField() rainField {
	return rainField{scopes: 1, page: 1, pages: 1, first: 1, last: 1, window: defaultRainWindow}
}

// TestRainVisibleLegendNeedsAHeightBeyondTheOtherChrome guards RG-008 and
// RG-012: the complete legend is exposed only when the content area holds the
// headings, one field row, and the context line beside it, so no height ever
// reserves a legend row that stays empty. An unbounded height holds everything.
func TestRainVisibleLegendNeedsAHeightBeyondTheOtherChrome(t *testing.T) {
	cases := []struct {
		name    string
		width   int
		height  int
		visible bool
	}{
		{name: "unbounded", width: 120, height: -1, visible: true},
		{name: "no rows at all", width: 120, height: 0},
		{name: "one row", width: 120, height: 1},
		{name: "field and context", width: 120, height: 2},
		{name: "field, context and headings", width: 120, height: 3},
		{name: "one row beyond the chrome", width: 120, height: 4, visible: true},
		{name: "two rows beyond the chrome", width: 120, height: 5, visible: true},
		{name: "too narrow to spell", width: narrowWidth, height: 20},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			legend := rainVisibleLegend(charsetUTF8, testCase.width, testCase.height)
			if visible := legend != ""; visible != testCase.visible {
				t.Errorf("a %dx%d Rain exposes its legend=%v, want %v: %q",
					testCase.width, testCase.height, visible, testCase.visible, legend)
			}
			if testCase.visible && !strings.Contains(legend, firstLegendEntry()) {
				t.Errorf("the exposed legend omits %q: %q", firstLegendEntry(), legend)
			}
		})
	}
}

// TestRainRenderYieldsItsChromeInPriorityOrder guards RG-012's constrained
// ladder: the field keeps a row at every positive height and Rain's own chrome
// yields around it, the legend first, then the headings, then the context line.
func TestRainRenderYieldsItsChromeInPriorityOrder(t *testing.T) {
	state, published := publishedRain(t, 120, 1)
	cases := []struct {
		name                      string
		height, lines             int
		headings, context, legend bool
	}{
		{name: "unbounded", height: -1, lines: 4, headings: true, context: true, legend: true},
		{name: "one row", height: 1, lines: 1},
		{name: "field and context", height: 2, lines: 2, context: true},
		{name: "field, context and headings", height: 3, lines: 3, headings: true, context: true},
		{name: "every line", height: 4, lines: 4, headings: true, context: true, legend: true},
		{name: "a second field row", height: 5, lines: 5, headings: true, context: true, legend: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			body := state.render(published, newInteresting(), charsetUTF8, capabilityTruecolor, 120, testCase.height)
			lines := strings.Split(body, "\n")
			if len(lines) != testCase.lines {
				t.Fatalf("height %d rendered %d lines, want %d:\n%s", testCase.height, len(lines), testCase.lines, body)
			}
			for _, want := range []struct {
				what    string
				marker  string
				present bool
			}{
				{what: "the Scope headings", marker: "R1 acme/api", present: testCase.headings},
				{what: "the context line", marker: "window 60m", present: testCase.context},
				{what: "the legend", marker: firstLegendEntry(), present: testCase.legend},
			} {
				if got := strings.Contains(body, want.marker); got != want.present {
					t.Errorf("height %d renders %s=%v, want %v:\n%s", testCase.height, want.what, got, want.present, body)
				}
			}
			if !strings.Contains(body, categoryGlyph(domain.CategoryPush, charsetUTF8)) {
				t.Errorf("height %d drew no field row at all:\n%s", testCase.height, body)
			}
			if headed := strings.Contains(lines[0], "R1 acme/api"); headed != testCase.headings {
				t.Errorf("height %d starts with the headings=%v, want %v:\n%s", testCase.height, headed, testCase.headings, body)
			}
		})
	}
}

// TestRainColumnRowDrawsOnlyItsPreparedRows guards RG-006: a column draws its
// tokens on the rows it prepared and keeps the reserved height as blank
// interior cells everywhere else, so a row the geometry never prepared never
// borrows another row's tokens.
func TestRainColumnRowDrawsOnlyItsPreparedRows(t *testing.T) {
	const interior = 4
	column := rainColumn{
		interior: interior,
		slots:    2,
		rows: [][]rainCell{{
			{occupied: true, category: domain.CategoryPush, recency: recencyNew},
			{},
		}},
	}

	drawn := rainColumnRow(column, charsetUTF8, capabilityNoColor, 0)
	if !strings.Contains(drawn, categoryGlyph(domain.CategoryPush, charsetUTF8)) {
		t.Errorf("the prepared row draws no category glyph: %q", drawn)
	}
	if got := lipgloss.Width(drawn); got != interior {
		t.Errorf("the prepared row is %d cells wide, want the granted %d: %q", got, interior, drawn)
	}
	for _, row := range []int{-2, -1, len(column.rows), len(column.rows) + 3} {
		blank := rainColumnRow(column, charsetUTF8, capabilityNoColor, row)
		if strings.TrimSpace(blank) != "" {
			t.Errorf("row %d drew %q, want no token at all", row, blank)
		}
		if got := lipgloss.Width(blank); got != interior {
			t.Errorf("row %d is %d cells wide, want the granted %d", row, got, interior)
		}
	}
}

// TestRainCellTokenClipsTheQualifierInOneCellColumns guards RG-006 and RG-008:
// the current-PR `~` renders beside the glyph wherever the slot is two cells
// wide, and the exact one-cell column drops it rather than widening the slot or
// letting colour carry the qualification.
func TestRainCellTokenClipsTheQualifierInOneCellColumns(t *testing.T) {
	cases := []struct {
		name       string
		qualified  bool
		cellWidth  int
		wantMarked bool
	}{
		{name: "qualified in a two-cell slot", qualified: true, cellWidth: 2, wantMarked: true},
		{name: "unqualified in a two-cell slot", cellWidth: 2},
		{name: "qualified in a one-cell slot", qualified: true, cellWidth: 1},
		{name: "unqualified in a one-cell slot", cellWidth: 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			cell := rainCell{occupied: true, category: domain.CategoryPush, recency: recencyNew, qualified: testCase.qualified}
			token := rainCellToken(cell, charsetUTF8, capabilityNoColor, testCase.cellWidth)
			if got := strings.Contains(token, currentPRQualifier); got != testCase.wantMarked {
				t.Errorf("the token spells the qualifier=%v, want %v: %q", got, testCase.wantMarked, token)
			}
			if got := lipgloss.Width(token); got != testCase.cellWidth {
				t.Errorf("the token is %d cells wide, want the slot's %d: %q", got, testCase.cellWidth, token)
			}
		})
	}
}

// TestPadColumnFillsExactlyTheGrantedInterior guards RG-006's column geometry:
// a drawn column is padded to exactly its granted interior so the next column
// starts where the arithmetic places it, and a column already at or beyond its
// grant is returned untouched.
func TestPadColumnFillsExactlyTheGrantedInterior(t *testing.T) {
	cases := []struct {
		name     string
		drawn    string
		interior int
		want     string
	}{
		{name: "shorter than the grant", drawn: "ab", interior: 5, want: "ab   "},
		{name: "one cell short", drawn: "abcd", interior: 5, want: "abcd "},
		{name: "exactly the grant", drawn: "abc", interior: 3, want: "abc"},
		{name: "beyond the grant", drawn: "abcde", interior: 3, want: "abcde"},
		{name: "empty column", drawn: "", interior: 2, want: "  "},
		{name: "no interior at all", drawn: "", interior: 0, want: ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := padColumn(testCase.drawn, testCase.interior); got != testCase.want {
				t.Errorf("padding %q to %d cells gave %q, want %q", testCase.drawn, testCase.interior, got, testCase.want)
			}
		})
	}
}

// TestRainContextKeepsTheWindowAtEveryWidth guards RG-006 and RG-012: the
// selected window names the lifetime every other count is measured under, so
// the ladder never drops it and never renders an empty context line, however
// narrow the terminal is.
func TestRainContextKeepsTheWindowAtEveryWidth(t *testing.T) {
	field := onePageField()
	for _, width := range []int{80, 20, 10, 5, 2, 1} {
		line := rainContextLine(field, capabilityTruecolor, false, width)
		if lipgloss.Width(line) == 0 {
			t.Errorf("a %d-cell Rain rendered an empty context line, want the selected window", width)
		}
		if width >= 5 && !strings.Contains(line, "wind") {
			t.Errorf("a %d-cell Rain context does not name its window: %q", width, line)
		}
	}
}

// TestRainContextOmitsTheScopeRangeWithoutScopes guards RG-006's empty-state
// page: with no active Scope there is no page position to state, so the context
// never claims a range over a selection that does not exist.
func TestRainContextOmitsTheScopeRangeWithoutScopes(t *testing.T) {
	field := rainField{page: 1, pages: 1, window: defaultRainWindow}
	for _, shortened := range []bool{false, true} {
		for _, segment := range rainContextSegments(field, capabilityTruecolor, false, shortened) {
			if strings.Contains(segment, "scope") || strings.Contains(segment, "/0") {
				t.Errorf("an empty selection states the Scope range %q", segment)
			}
		}
	}
	if line := rainContextLine(field, capabilityTruecolor, false, 80); strings.Contains(line, "scope") {
		t.Errorf("an empty selection renders a Scope range: %q", line)
	}
}

// TestRainScopeRangeStatesSingleAndSpannedPages guards RG-006's one-based
// inclusive page positions: a page holding one Scope names that Scope and a
// page spanning several names its inclusive range, in both spellings.
func TestRainScopeRangeStatesSingleAndSpannedPages(t *testing.T) {
	cases := []struct {
		name                string
		first, last, scopes int
		wantFull, wantShort string
	}{
		{name: "one Scope", first: 2, last: 2, scopes: 5, wantFull: "scope 2 of 5", wantShort: "2/5"},
		{name: "a spanned page", first: 1, last: 3, scopes: 5, wantFull: "scopes 1-3 of 5", wantShort: "1-3/5"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			field := rainField{first: testCase.first, last: testCase.last, scopes: testCase.scopes}
			if got := rainScopeRange(field, false); got != testCase.wantFull {
				t.Errorf("the full spelling is %q, want %q", got, testCase.wantFull)
			}
			if got := rainScopeRange(field, true); got != testCase.wantShort {
				t.Errorf("the compact spelling is %q, want %q", got, testCase.wantShort)
			}
		})
	}
}

// TestRainOmittedTotalsEveryCapacityRejection guards RG-006's disjoint
// counters: the omission counter is the sum of the column, global, and paused
// capacity rejections, so no rejected candidate goes unreported.
func TestRainOmittedTotalsEveryCapacityRejection(t *testing.T) {
	counts := rainCounts{candidates: 12, admitted: 5, columnOmitted: 1, globalOmitted: 2, pausedOmitted: 4}
	if got, want := rainOmitted(counts), 7; got != want {
		t.Errorf("the capacity rejections total %d, want %d", got, want)
	}
	field := onePageField()
	field.counts = counts
	line := rainContextLine(field, capabilityTruecolor, false, 80)
	if want := hiddenMark + "7 omitted"; !strings.Contains(line, want) {
		t.Errorf("the context reports %q, want it to contain %q", line, want)
	}
}
