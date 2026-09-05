package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// omittingSelection returns the published selection of an invocation whose two
// organization selectors each omitted eligible repositories, so the disclosed
// omission has to be the sum of both rather than either one of them.
func omittingSelection(t *testing.T, first, second int) Selection {
	t.Helper()
	scopes := testScope(t, "acme/backend", "beta/frontend")
	return Selection{
		Scopes:               scopes,
		ExactScopes:          0,
		ExpandedScopes:       2,
		TotalScopes:          2,
		DistinctRepositories: 2,
		Selectors: []SelectorSelection{
			{Organization: "acme", Omitted: first},
			{Organization: "beta", Omitted: second},
		},
	}
}

// TestSelectionDisclosesTheOmissionsOfEverySelectorTogether guards RG-010: a
// bounded result is never presented as the complete organization, so the
// disclosed omission counts every selector's omitted repositories rather than
// only the last selector's.
func TestSelectionDisclosesTheOmissionsOfEverySelectorTogether(t *testing.T) {
	forms := selectionForms(omittingSelection(t, 2, 3))

	if len(forms) != 3 {
		t.Fatalf("the disclosed selection has %d forms, want the full, compact, and minimum ones: %q", len(forms), forms)
	}
	for index, want := range []string{"5 eligible omitted", "5 omitted", "SEL 5"} {
		if !strings.Contains(forms[index], want) {
			t.Errorf("selection form %d is %q, want it to disclose %q", index, forms[index], want)
		}
	}
}

// TestSelectionDisclosesNoOmissionWhenNoSelectorOmitted guards RG-010's other
// side: a selection that omitted nothing states no omission at all.
func TestSelectionDisclosesNoOmissionWhenNoSelectorOmitted(t *testing.T) {
	for _, form := range selectionForms(omittingSelection(t, 0, 0)) {
		if strings.Contains(form, "omitted") || strings.HasPrefix(form, "SEL") {
			t.Errorf("a selection that omitted nothing discloses %q", form)
		}
	}
}

// TestShortenKeepsTheMarkAtTheNarrowestWidthThatHoldsIt guards FR-010: the
// shortening mark is paid for out of the same limit, so the narrowest limit that
// can hold the mark renders the mark, and only a limit too tight for it yields
// nothing.
func TestShortenKeepsTheMarkAtTheNarrowestWidthThatHoldsIt(t *testing.T) {
	markWidth := lipgloss.Width(shortenedMark)
	text := "abcdef"

	if got := shorten(text, markWidth); got != shortenedMark {
		t.Errorf("shorten(%q, %d) is %q, want the mark %q", text, markWidth, got, shortenedMark)
	}
	if got := shorten(text, markWidth-1); got != "" {
		t.Errorf("shorten(%q, %d) is %q, want nothing at a limit too tight for the mark", text, markWidth-1, got)
	}
}

// TestShortenKeepsALeadingPrefixOfTheLine guards FR-010: a shortened line is the
// leading prefix of the line it replaces plus the mark. A character the limit
// cannot hold ends the cut, so a wider character is never skipped over to keep a
// narrower one that follows it.
func TestShortenKeepsALeadingPrefixOfTheLine(t *testing.T) {
	// The third character is two cells wide, so the limit that stops at it
	// still leaves room for the one-cell character behind it.
	const text = "aa世b"

	got := shorten(text, 4)

	if want := "aa" + shortenedMark; got != want {
		t.Errorf("shorten(%q, 4) is %q, want %q", text, got, want)
	}
	cut := strings.TrimSuffix(got, shortenedMark)
	if !strings.HasPrefix(text, cut) {
		t.Errorf("shorten(%q, 4) kept %q, which is not a leading prefix of the line", text, cut)
	}
}

// shortEnricher settles fewer outcomes than the refresh gave it events, which
// is the short result FR-004 leaves undecided rather than decided.
type shortEnricher struct {
	outcomes []domain.EvidenceOutcome
}

func (e *shortEnricher) Evidence(context.Context, []domain.Event) (Evidence, error) {
	return Evidence{Outcomes: e.outcomes}, nil
}

// TestShortEvidenceResultLeavesTheRemainingEventsUndecided guards FR-004: a
// coordination that reported on fewer events than it was given settles only
// those, and every remaining event stays explicitly incomplete instead of being
// decided without evidence.
func TestShortEvidenceResultLeavesTheRemainingEventsUndecided(t *testing.T) {
	scopes, _ := mixedScope(t, "acme/backend", "src")
	reported := completeEvidence(t, "src/main.go")
	model := Model{enricher: &shortEnricher{outcomes: []domain.EvidenceOutcome{reported}}}

	outcomes, _, _ := model.settle(context.Background(), scopes, make([]domain.Event, 3))

	if len(outcomes) != 3 {
		t.Fatalf("the attempt settled %d outcomes, want one per event", len(outcomes))
	}
	if !outcomes[0].IsComplete() {
		t.Errorf("the reported event settled at %v, want the reported complete evidence", outcomes[0].Kind())
	}
	for index := 1; index < len(outcomes); index++ {
		if outcomes[index].Kind() != domain.OutcomeIncomplete {
			t.Errorf("event %d settled at %v, want it left undecided as incomplete", index, outcomes[index].Kind())
		}
		if outcomes[index].Reason() != reasonNoOutcome {
			t.Errorf("event %d settled with reason %q, want %q", index, outcomes[index].Reason(), reasonNoOutcome)
		}
	}
}

// rainPagingModel returns the root model looking at a Rain field over the
// given number of Scopes at a width that seats several fixed pages.
func rainPagingModel(t *testing.T, scopes int, width int) Model {
	t.Helper()
	model := newModel(t, "acme/r0")
	model.mode = ModeRain
	model.rain = rain{scopes: rainScopes(t, scopes), width: width}
	return model
}

// TestRainPageBackwardMovesToThePrecedingPage guards RG-006: `[` moves one fixed
// page backward, wrapping from the first page to the final one, so it is never
// the forward step `]` already performs.
func TestRainPageBackwardMovesToThePrecedingPage(t *testing.T) {
	const (
		scopes  = 8
		width   = 40
		perPage = 3
	)
	model := rainPagingModel(t, scopes, width)
	if got := rainPerPage(width, scopes); got != perPage {
		t.Fatalf("a width of %d seats %d Scopes per page, want %d", width, got, perPage)
	}

	back, _ := apply(t, model, press("["))
	if got, want := back.rain.start, 6; got != want {
		t.Fatalf("paging backward from the first page starts at Scope index %d, want the final page's %d", got, want)
	}

	forward, _ := apply(t, model, press("]"))
	if got, want := forward.rain.start, 3; got != want {
		t.Fatalf("paging forward from the first page starts at Scope index %d, want %d", got, want)
	}

	returned, _ := apply(t, forward, press("["))
	if got, want := returned.rain.start, 0; got != want {
		t.Errorf("paging backward after paging forward starts at Scope index %d, want the original %d", got, want)
	}
}

// TestBodyIsUnboundedAtANonPositiveHeight guards FR-009 and FR-010: a
// non-positive height is the unbounded content area both views share with the
// scrolling clamp, so it holds every line rather than none of them.
func TestBodyIsUnboundedAtANonPositiveHeight(t *testing.T) {
	lines := []string{"alpha", "beta", "gamma"}

	for _, height := range []int{0, unbounded} {
		got := renderBody(lines, viewport{}, unbounded, height)
		if want := strings.Join(lines, "\n"); got != want {
			t.Errorf("the body at height %d is %q, want the unbounded %q", height, got, want)
		}
	}
}

// TestBodyWindowsAllTheRowsABoundedHeightHolds guards FR-010: a bounded content
// area renders exactly the rows it has, neither dropping the last fitting row
// nor overflowing.
func TestBodyWindowsAllTheRowsABoundedHeightHolds(t *testing.T) {
	lines := []string{"alpha", "beta", "gamma"}

	if got, want := renderBody(lines, viewport{}, unbounded, 3), "alpha\nbeta\ngamma"; got != want {
		t.Errorf("the body at its exact height is %q, want %q", got, want)
	}
	if got, want := renderBody(lines, viewport{}, unbounded, 2), "alpha\nbeta"; got != want {
		t.Errorf("the body at height 2 is %q, want %q", got, want)
	}
}

// TestModeLabelDegradesForAnOutOfRangeMode guards FR-007: a mode outside the
// published views degrades to the first view's label rather than panicking a
// render.
func TestModeLabelDegradesForAnOutOfRangeMode(t *testing.T) {
	for _, mode := range []Mode{-2, -1, Mode(len(modeLabels)), Mode(len(modeLabels) + 1)} {
		if got, want := mode.Label(), modeLabels[ModeOverview]; got != want {
			t.Errorf("Mode(%d).Label() is %q, want the degraded %q", int(mode), got, want)
		}
	}
	for index, want := range modeLabels {
		if got := Mode(index).Label(); got != want {
			t.Errorf("Mode(%d).Label() is %q, want %q", index, got, want)
		}
	}
}

// TestModeToggleDegradesForAnOutOfRangeMode guards FR-007: the tab cycle of a
// mode outside the published views selects the first view, and the in-range
// cycle stays the fixed one every published view is reachable through.
func TestModeToggleDegradesForAnOutOfRangeMode(t *testing.T) {
	for _, mode := range []Mode{-2, -1, Mode(len(modeLabels)), Mode(len(modeLabels) + 1)} {
		if got := mode.toggled(); got != ModeOverview {
			t.Errorf("Mode(%d).toggled() is %d, want ModeOverview", int(mode), int(got))
		}
	}
	if got := ModeOverview.toggled(); got != ModeStream {
		t.Errorf("ModeOverview.toggled() is %d, want ModeStream", int(got))
	}
	if got := ModeStream.toggled(); got != ModeRain {
		t.Errorf("ModeStream.toggled() is %d, want ModeRain", int(got))
	}
	if got := ModeRain.toggled(); got != ModeOverview {
		t.Errorf("ModeRain.toggled() is %d, want ModeOverview", int(got))
	}
}

// TestScopeContextCompleteFormOmitsAGroupWithNoTokens guards RG-012: the
// complete form names only the groups the row has tokens for, so an empty group
// never contributes its naming word to the row.
func TestScopeContextCompleteFormOmitsAGroupWithNoTokens(t *testing.T) {
	membersOnly := contextOf([]string{"R1", "P2~"}, nil).render(unbounded)
	if want := "in R1, P2~"; membersOnly != want {
		t.Errorf("a members-only context is %q, want %q", membersOnly, want)
	}
	if strings.Contains(membersOnly, unknownGroup) {
		t.Errorf("a members-only context names the unknown group: %q", membersOnly)
	}

	unknownsOnly := contextOf(nil, []string{"P3", "P4"}).render(unbounded)
	if want := "unresolved ?P3, ?P4"; unknownsOnly != want {
		t.Errorf("an unknowns-only context is %q, want %q", unknownsOnly, want)
	}
	if strings.Contains(unknownsOnly, memberGroup) {
		t.Errorf("an unknowns-only context names the member group: %q", unknownsOnly)
	}
}

// TestScopeContextIntermediateOmitsAGroupWithNoTokens guards RG-012: the
// intermediate form spells only the groups the row has tokens for, so a row of
// one group alone never spends budget on the empty other one.
func TestScopeContextIntermediateOmitsAGroupWithNoTokens(t *testing.T) {
	// Both budgets sit below the complete form and above the narrowest
	// expansion, so the ladder renders the intermediate rung.
	membersOnly := contextOf([]string{"R1", "R2", "R3"}, nil).render(10)
	if want := "R1, R2, R3"; membersOnly != want {
		t.Errorf("the intermediate members-only context is %q, want %q", membersOnly, want)
	}

	unknownsOnly := contextOf(nil, []string{"P3", "P4", "P5"}).render(12)
	if want := "?P3, ?P4, +1"; unknownsOnly != want {
		t.Errorf("the intermediate unknowns-only context is %q, want %q", unknownsOnly, want)
	}

	for name, got := range map[string]string{"members-only": membersOnly, "unknowns-only": unknownsOnly} {
		if strings.HasPrefix(got, separator) || strings.HasSuffix(got, separator) || strings.Contains(got, separator+separator) {
			t.Errorf("the intermediate %s context %q separates an empty group", name, got)
		}
	}
}

// TestScopeContextIntermediateOmitsAnAbsentQualifiedCount guards RG-012: the
// intermediate form counts the current-PR qualified members it left out, and
// states nothing at all when it left none out.
func TestScopeContextIntermediateOmitsAnAbsentQualifiedCount(t *testing.T) {
	context := contextOf([]string{"R1", "R2"}, []string{"P3"})

	got := context.render(12)

	if want := "R1, R2 · ?P3"; got != want {
		t.Errorf("the intermediate context is %q, want %q", got, want)
	}
	if strings.Contains(got, "current PR") {
		t.Errorf("the intermediate context counts current-PR members it hid none of: %q", got)
	}
}

// membersOnlyContext builds the context of an event matching many members and
// no unknowns, where the spelled-out counts are narrower than the token form.
func membersOnlyContext(t *testing.T, count int) scopeContext {
	t.Helper()
	members := make([]string, 0, count)
	for index := range count {
		members = append(members, fmt.Sprintf("REPO%06d", index))
	}
	return contextOf(members, nil)
}

// TestScopeContextCountedFormOmitsTheCountsTheRowHoldsNoneOf guards RG-012: the
// counted rung states the counts the row actually holds, so a row with no
// unknown and no current-PR qualified member states neither of them.
func TestScopeContextCountedFormOmitsTheCountsTheRowHoldsNoneOf(t *testing.T) {
	context := membersOnlyContext(t, 500)

	got := context.render(10)

	if want := "500 member"; got != want {
		t.Errorf("the counted context is %q, want %q", got, want)
	}
	for _, absent := range []string{"unknown", "current PR"} {
		if strings.Contains(got, absent) {
			t.Errorf("the counted context states %q for a count the row holds none of: %q", absent, got)
		}
	}
}

// TestScopeContextMinimumFormOmitsTheCountsTheRowHoldsNoneOf guards RG-012 at
// the narrowest counted rung: the same omission holds behind the short marks.
func TestScopeContextMinimumFormOmitsTheCountsTheRowHoldsNoneOf(t *testing.T) {
	context := membersOnlyContext(t, 500)

	got := context.render(4)

	if want := "500m"; got != want {
		t.Errorf("the minimum counted context is %q, want %q", got, want)
	}
}

// TestViewportDoesNotScrollAtANonPositiveHeight guards FR-009 and FR-010: a
// non-positive height renders the content unbounded, where nothing scrolls, so
// every keystroke leaves the window at the first row.
func TestViewportDoesNotScrollAtANonPositiveHeight(t *testing.T) {
	scrolled := viewport{offset: 5}

	for _, keystroke := range []string{"up", "down", "pgup", "pgdown"} {
		for _, height := range []int{0, unbounded} {
			if got := scrolled.scrolled(keystroke, 20, height).offset; got != 0 {
				t.Errorf("%q at height %d left the offset at %d, want the unbounded 0", keystroke, height, got)
			}
			if got := (viewport{offset: 5}).clamped(20, height); got != 0 {
				t.Errorf("clamping at height %d yielded %d, want the unbounded 0", height, got)
			}
		}
	}
}

// localeCharset resolves the glyph repertoire of an environment naming only
// the locale variable, leaving TERM unset.
func localeCharset(locale string) charset {
	return resolveCharset(func(name string) string {
		if name == "LANG" {
			return locale
		}
		return ""
	})
}

// TestCharsetIgnoresACodesetSpelledInsideTheModifier guards RG-008: the modifier
// is dropped before the codeset is read, so a `.` inside the modifier never
// passes for the codeset separator.
func TestCharsetIgnoresACodesetSpelledInsideTheModifier(t *testing.T) {
	cases := map[string]charset{
		"@currency.UTF-8":           charsetASCII,
		"en_US@currency.UTF-8":      charsetASCII,
		"en_US.UTF-8@currency.euro": charsetUTF8,
	}

	for locale, want := range cases {
		if got := localeCharset(locale); got != want {
			t.Errorf("the locale %q resolved to charset %d, want %d", locale, got, want)
		}
	}
}

// TestCharsetReadsTheCodesetAfterTheFinalSeparator guards RG-008: the codeset is
// what follows the final `.` of the locale, wherever that separator sits.
func TestCharsetReadsTheCodesetAfterTheFinalSeparator(t *testing.T) {
	cases := map[string]charset{
		".UTF-8":  charsetUTF8,
		"C.UTF-8": charsetUTF8,
		"UTF-8":   charsetUTF8,
		"C":       charsetASCII,
		".8859-1": charsetASCII,
	}

	for locale, want := range cases {
		if got := localeCharset(locale); got != want {
			t.Errorf("the locale %q resolved to charset %d, want %d", locale, got, want)
		}
	}
}
