package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fmueller/orgtop/internal/domain"
)

// plain drops the styling of a rendered line, so an assertion measures the text
// a reader sees rather than the escape sequences carrying its emphasis.
func plain(rendered string) string { return ansi.Strip(rendered) }

// stripModel builds a Rain shell whose field and strip were both published by
// one successful refresh at the strip fixture base, sized to the terminal.
func stripModel(t *testing.T, scopes domain.ScopeSet, retained []domain.EventEvidence, width, height int) Model {
	t.Helper()
	model, err := New(context.Background(), scopes, &fakeSource{})
	if err != nil {
		t.Fatalf("building the strip test model failed: %v", err)
	}
	snapshot := stripSnapshot(scopes, retained...)
	model.state.Scopes = scopes
	model.state.Scoped = snapshot
	model.state.Freshness = FreshnessCurrent
	model.state.LastSuccess = stripBase
	model.mode = ModeRain
	model.charset = charsetUTF8
	model.capability = capabilityTruecolor
	model.rain = newRain().reconciled(scopes, snapshot, stripBase)
	model.interesting = newInteresting().reconciled(scopes, snapshot, stripBase)
	model, _ = apply(t, model, tea.WindowSizeMsg{Width: width, Height: height})
	return model
}

// stripRenderScopes is the single-repository selection the render vectors that
// do not exercise overlap share.
func stripRenderScopes(t *testing.T, repository string) domain.ScopeSet {
	t.Helper()
	return scopeSet(t, repositoryScope(t, repository))
}

// renderedStrip returns the strip lines the prepared state renders for the
// available body rows, which is what a Rain body appends below its field.
func renderedStrip(strip interesting, tokens map[domain.ScopeIdentity]string, width, available int) []string {
	rows, shown := strip.rows(available)
	return strip.render(tokens, charsetUTF8, capabilityTruecolor, width, rows, shown)
}

// stripLinesOf renders the strip of the started state at the width and the
// available body rows.
func stripLinesOf(t *testing.T, scopes domain.ScopeSet, retained []domain.EventEvidence, width, available int) []string {
	t.Helper()
	strip := startedStrip(scopes, stripSnapshot(scopes, retained...))
	return renderedStrip(strip, scopes.Tokens(), width, available)
}

// TestInterestingStripRendersTitleAndVisibleEntries guards RG-007's full-height
// strip: one title stating the disjoint accounting and the first five stored
// entries in stored order beneath it.
func TestInterestingStripRendersTitleAndVisibleEntries(t *testing.T) {
	repository := "acme/api"
	scopes := stripRenderScopes(t, repository)
	var retained []domain.EventEvidence
	for index := range 8 {
		retained = append(retained, stripEvidence(t, fmt.Sprintf("e%02d", index), repository, time.Duration(index+1)*time.Minute))
	}

	lines := stripLinesOf(t, scopes, retained, wideWidth, 12)
	if got, want := len(lines), 6; got != want {
		t.Fatalf("the strip rendered %d lines, want %d:\n%v", got, want, lines)
	}
	if got, want := plain(lines[0]), "Interesting Now: 5 shown · 3 hidden · 0 omitted"; got != want {
		t.Fatalf("the strip title is %q, want %q", got, want)
	}
	for index := range 5 {
		if got, want := plain(lines[index+1]), fmt.Sprintf("%dm", index+1); !strings.Contains(got, want) {
			t.Errorf("strip entry %d is %q, want the age %q", index, got, want)
		}
	}
	if strings.Contains(strings.Join(lines, "\n"), "e05") {
		t.Errorf("the strip rendered a stored entry beyond the five visible ones:\n%v", lines)
	}
}

// TestInterestingStripEntryStatesItsDirectFacts guards RG-007's entry
// explanation: a full-width entry names its category, actor, entity,
// repository, sponsoring Scope, the additional confirmed Scopes, the qualified
// current-PR count, and its textual age.
func TestInterestingStripEntryStatesItsDirectFacts(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t,
		repositoryScope(t, repository),
		pathScope(t, repository, "src"),
	)
	event := testEvent(t, "e0", repository, domain.CategoryReview, domain.EntityPullRequest)
	event.OccurredAt = stripBase.Add(-2 * time.Minute)
	event.Actor = "octocat"
	event.EntityRef = "#42"
	retained := []domain.EventEvidence{{
		Event:   event,
		Outcome: domain.CompleteOutcome(domain.ProvenanceCurrentPR, mustChangedPaths(t, "src/main.go")),
	}}

	lines := stripLinesOf(t, scopes, retained, wideWidth, 12)
	if len(lines) < 2 {
		t.Fatalf("the strip rendered %d lines, want a title and one entry:\n%v", len(lines), lines)
	}
	entry := plain(lines[1])
	for _, want := range []string{"review", "octocat", "#42", repository, "+1 scopes", "current PR", "2m"} {
		if !strings.Contains(entry, want) {
			t.Errorf("the entry %q states no %q", entry, want)
		}
	}
}

// TestInterestingStripEntryKeepsItsMinimalFormWhenNarrow guards RG-007's width
// reduction: the last logical form keeps the glyph, the repository, the
// sponsoring Scope, and the age, and drops the actor and entity detail first.
func TestInterestingStripEntryKeepsItsMinimalFormWhenNarrow(t *testing.T) {
	repository := "acme/api"
	scopes := stripRenderScopes(t, repository)
	event := testEvent(t, "e0", repository, domain.CategoryPush, domain.EntityCommit)
	event.OccurredAt = stripBase.Add(-3 * time.Minute)
	event.Actor = "octocat"
	event.EntityRef = "abcdef1"
	retained := []domain.EventEvidence{{Event: event}}

	lines := stripLinesOf(t, scopes, retained, 26, 12)
	if len(lines) < 2 {
		t.Fatalf("the strip rendered %d lines, want a title and one entry:\n%v", len(lines), lines)
	}
	entry := plain(lines[1])
	if lipgloss.Width(entry) > 26 {
		t.Fatalf("the narrow entry %q is %d cells wide, want at most 26", entry, lipgloss.Width(entry))
	}
	for _, want := range []string{repository, "3m"} {
		if !strings.Contains(entry, want) {
			t.Errorf("the narrow entry %q dropped %q, which every form retains", entry, want)
		}
	}
	if strings.Contains(entry, "octocat") {
		t.Errorf("the narrow entry %q kept the actor detail, which yields first", entry)
	}
}

// TestInterestingStripRendersWithoutColor guards RG-008: a no-color profile
// still spells the category and the age, so nothing about an entry depends on
// color.
func TestInterestingStripRendersWithoutColor(t *testing.T) {
	repository := "acme/api"
	scopes := stripRenderScopes(t, repository)
	retained := []domain.EventEvidence{stripEvidence(t, "e0", repository, time.Minute)}
	strip := startedStrip(scopes, stripSnapshot(scopes, retained...))

	rows, shown := strip.rows(12)
	lines := strip.render(scopes.Tokens(), charsetASCII, capabilityNoColor, wideWidth, rows, shown)
	rendered := strings.Join(lines, "\n")
	// The title is shared chrome, whose colors are their own semantic role; the
	// entry lines are the event presentation RG-008 keeps free of color.
	for _, entry := range lines[1:] {
		if colorEscape.MatchString(entry) {
			t.Fatalf("the no-color strip entry emitted a color escape:\n%q", entry)
		}
	}
	for _, want := range []string{"push", "1m"} {
		if !strings.Contains(plain(rendered), want) {
			t.Errorf("the no-color strip states no %q:\n%s", want, plain(rendered))
		}
	}
}

// TestInterestingStripHeightLadder guards RG-007's height collapse: the rows
// the strip takes of the available body rows, and how many entries it renders.
func TestInterestingStripHeightLadder(t *testing.T) {
	repository := "acme/api"
	scopes := stripRenderScopes(t, repository)
	var retained []domain.EventEvidence
	for index := range 8 {
		retained = append(retained, stripEvidence(t, fmt.Sprintf("e%02d", index), repository, time.Duration(index)*time.Minute))
	}
	strip := startedStrip(scopes, stripSnapshot(scopes, retained...))

	for _, vector := range []struct {
		available int
		rows      int
		shown     int
	}{
		{available: 0, rows: 0, shown: 0},
		{available: 3, rows: 0, shown: 0},
		{available: 4, rows: 1, shown: 0},
		{available: 5, rows: 2, shown: 1},
		{available: 8, rows: 5, shown: 4},
		{available: 9, rows: 6, shown: 5},
		{available: 20, rows: 6, shown: 5},
	} {
		rows, shown := strip.rows(vector.available)
		if rows != vector.rows || shown != vector.shown {
			t.Errorf("at %d available rows the strip took %d rows showing %d entries, want %d and %d",
				vector.available, rows, shown, vector.rows, vector.shown)
		}
		if lines := strip.render(scopes.Tokens(), charsetUTF8, capabilityTruecolor, wideWidth, rows, shown); len(lines) != rows {
			t.Errorf("at %d available rows the strip rendered %d lines, want %d", vector.available, len(lines), rows)
		}
	}
}

// TestInterestingStripCountOnlyLineAtFourRows guards RG-007: at four available
// rows the body keeps three field rows and the count-only strip line.
func TestInterestingStripCountOnlyLineAtFourRows(t *testing.T) {
	repository := "acme/api"
	scopes := stripRenderScopes(t, repository)
	var retained []domain.EventEvidence
	for index := range 8 {
		retained = append(retained, stripEvidence(t, fmt.Sprintf("e%02d", index), repository, time.Duration(index)*time.Minute))
	}

	lines := stripLinesOf(t, scopes, retained, wideWidth, 4)
	if got, want := len(lines), 1; got != want {
		t.Fatalf("the collapsed strip rendered %d lines, want %d:\n%v", got, want, lines)
	}
	if got, want := plain(lines[0]), "interesting: 0 shown/8 hidden/0 omitted"; got != want {
		t.Fatalf("the count-only line is %q, want %q", got, want)
	}
}

// TestInterestingStripEmptyState guards RG-007's empty state: with no eligible
// event and at least four rows the strip is exactly one line, and below that it
// takes no row at all.
func TestInterestingStripEmptyState(t *testing.T) {
	repository := "acme/api"
	scopes := stripRenderScopes(t, repository)
	retained := []domain.EventEvidence{stripEvidence(t, "old", repository, time.Hour)}

	lines := stripLinesOf(t, scopes, retained, wideWidth, 8)
	if got, want := len(lines), 1; got != want {
		t.Fatalf("the empty strip rendered %d lines, want %d:\n%v", got, want, lines)
	}
	if got, want := plain(lines[0]), "Interesting Now: no recent activity"; got != want {
		t.Fatalf("the empty strip line is %q, want %q", got, want)
	}
	if lines := stripLinesOf(t, scopes, retained, wideWidth, 3); len(lines) != 0 {
		t.Errorf("the empty strip took %d rows below four available, want none:\n%v", len(lines), lines)
	}
}

// TestInterestingStripIsSilentBeforeItsFirstSnapshot guards the strip's
// honesty: a shell that has published nothing yet reports no strip state at all
// rather than an empty one it cannot stand behind.
func TestInterestingStripIsSilentBeforeItsFirstSnapshot(t *testing.T) {
	strip := newInteresting()
	if rows, shown := strip.rows(12); rows != 0 || shown != 0 {
		t.Fatalf("the unstarted strip took %d rows showing %d entries, want none", rows, shown)
	}
}

// TestInterestingStripAccountingShortensWithWidth guards RG-007's constrained
// accounting spellings, which shorten before they drop a count.
func TestInterestingStripAccountingShortensWithWidth(t *testing.T) {
	repository := "acme/api"
	scopes := stripRenderScopes(t, repository)
	var retained []domain.EventEvidence
	for index := range 8 {
		retained = append(retained, stripEvidence(t, fmt.Sprintf("e%02d", index), repository, time.Duration(index)*time.Minute))
	}
	strip := startedStrip(scopes, stripSnapshot(scopes, retained...))

	for _, vector := range []struct {
		width int
		want  string
	}{
		{width: wideWidth, want: "Interesting Now: 5 shown · 3 hidden · 0 omitted"},
		{width: 34, want: "I: 5 shown 3 hidden 0 omitted"},
		{width: 12, want: "I:5/3/0"},
	} {
		lines := renderedStrip(strip, scopes.Tokens(), vector.width, 12)
		if got := plain(lines[0]); got != vector.want {
			t.Errorf("at width %d the strip title is %q, want %q", vector.width, got, vector.want)
		}
	}
}

// TestRainBodySplitsRowsBetweenFieldAndStrip guards RG-007's collapse inside
// the rendered Rain body: the strip is the last lines of the body and the field
// keeps every row the strip did not take.
func TestRainBodySplitsRowsBetweenFieldAndStrip(t *testing.T) {
	repository := "acme/api"
	scopes := stripRenderScopes(t, repository)
	var retained []domain.EventEvidence
	for index := range 8 {
		retained = append(retained, stripEvidence(t, fmt.Sprintf("e%02d", index), repository, time.Duration(index)*time.Minute))
	}
	model := stripModel(t, scopes, retained, wideWidth, 14)

	body := rainBodyLines(model.render())
	if got, want := len(body), 12; got != want {
		t.Fatalf("the Rain body rendered %d lines, want %d:\n%v", got, want, body)
	}
	title := plain(body[len(body)-6])
	if !strings.HasPrefix(title, "Interesting Now: 5 shown") {
		t.Fatalf("the strip title line is %q, want the Interesting Now accounting", title)
	}
}

// TestRainFooterCarriesStripAccountingWhenCollapsed guards RG-007: with three
// or fewer body rows the strip has no line of its own, and the Rain footer
// states the accounting before the mandatory quit hint.
func TestRainFooterCarriesStripAccountingWhenCollapsed(t *testing.T) {
	repository := "acme/api"
	scopes := stripRenderScopes(t, repository)
	var retained []domain.EventEvidence
	for index := range 8 {
		retained = append(retained, stripEvidence(t, fmt.Sprintf("e%02d", index), repository, time.Duration(index)*time.Minute))
	}
	model := stripModel(t, scopes, retained, wideWidth, 6)

	lines := strings.Split(model.render(), "\n")
	footer := plain(lines[len(lines)-1])
	if !strings.Contains(footer, "interesting: 0 shown/8 hidden/0 omitted") {
		t.Fatalf("the collapsed Rain footer is %q, want the strip accounting", footer)
	}
	if !strings.HasSuffix(footer, "q quit") {
		t.Fatalf("the collapsed Rain footer is %q, want the mandatory quit hint last", footer)
	}
	for _, line := range rainBodyLines(model.render()) {
		if strings.Contains(plain(line), "Interesting Now") {
			t.Fatalf("the collapsed body kept a strip line %q, want every row for the field", plain(line))
		}
	}
}

// TestRainFooterMarksHiddenStripEntriesWhenNarrow guards RG-007's overflow
// indicator: between six and sixteen cells the collapsed footer renders `q I+`
// while any strip entry is hidden or omitted.
func TestRainFooterMarksHiddenStripEntriesWhenNarrow(t *testing.T) {
	repository := "acme/api"
	scopes := stripRenderScopes(t, repository)
	var retained []domain.EventEvidence
	for index := range 8 {
		retained = append(retained, stripEvidence(t, fmt.Sprintf("e%02d", index), repository, time.Duration(index)*time.Minute))
	}
	model := stripModel(t, scopes, retained, 12, 6)

	lines := strings.Split(model.render(), "\n")
	if got, want := plain(lines[len(lines)-1]), "q I+"; got != want {
		t.Fatalf("the narrow collapsed footer is %q, want %q", got, want)
	}
}

// TestStripAgesWhileRainIsPaused guards RG-007: the strip reference advances on
// the shared explicit tick even while the Rain field is frozen, so an entry
// leaving the fixed 15-minute window disappears from the strip.
func TestStripAgesWhileRainIsPaused(t *testing.T) {
	repository := "acme/api"
	scopes := stripRenderScopes(t, repository)
	retained := []domain.EventEvidence{stripEvidence(t, "e0", repository, 14*time.Minute)}
	model := stripModel(t, scopes, retained, wideWidth, 14)

	model, _ = apply(t, model, press("p"))
	model, _ = apply(t, model, rainTickMsg{chain: model.rain.chain, at: stripBase.Add(2 * time.Minute)})

	if got := model.interesting.counts.eligible; got != 0 {
		t.Fatalf("the paused strip kept %d eligible entries past the window, want none", got)
	}
	if !model.rain.paused {
		t.Fatalf("the tick resumed the paused field, want it frozen")
	}
}

// TestSuccessfulRefreshPublishesTheStrip guards the strip's place in the
// lifecycle: the same atomic publication that reconciles the Rain field
// prepares the strip, so a Rain body renders its companion without a second
// refresh (RG-007).
func TestSuccessfulRefreshPublishesTheStrip(t *testing.T) {
	repository := "acme/api"
	scopes := testScope(t, repository)
	event := testEvent(t, "e0", repository, domain.CategoryPush, domain.EntityCommit)
	event.OccurredAt = fixedInstant.Add(-time.Minute)
	activities := []domain.RepositoryActivity{testActivity(t, repository, event)}

	model, err := New(context.Background(), scopes, &fakeSource{}, WithClock(func() time.Time { return fixedInstant }))
	if err != nil {
		t.Fatalf("building the refresh test model failed: %v", err)
	}
	model.mode = ModeRain
	model, _ = apply(t, model, tea.WindowSizeMsg{Width: wideWidth, Height: 14}, refreshedMsg{
		polled:   true,
		result:   Result{Repositories: activities},
		evidence: retainedEvidence(scopes, activities),
	})

	if got, want := model.interesting.counts.stored, 1; got != want {
		t.Fatalf("the published refresh stored %d strip entries, want %d", got, want)
	}
	body := strings.Join(rainBodyLines(model.render()), "\n")
	if !strings.Contains(plain(body), "Interesting Now: 1 shown") {
		t.Fatalf("the Rain body carries no published strip:\n%s", plain(body))
	}
}

// TestInterestingStripEntryKeepsGlyphAndAgeAtTightWidths guards RG-007's final
// logical form: RG-012 may shorten the repository and Scope labels of
// `glyph repository scope age`, but never substitute the facts around them, so
// a tight width still states the category glyph and the textual age.
func TestInterestingStripEntryKeepsGlyphAndAgeAtTightWidths(t *testing.T) {
	repository := "acme/api"
	scopes := stripRenderScopes(t, repository)
	event := testEvent(t, "e0", repository, domain.CategoryPush, domain.EntityCommit)
	event.OccurredAt = stripBase.Add(-3 * time.Minute)
	event.Actor = "octocat"
	retained := []domain.EventEvidence{{Event: event}}
	strip := startedStrip(scopes, stripSnapshot(scopes, retained...))
	glyph := categoryGlyph(domain.CategoryPush, charsetUTF8)

	for _, width := range []int{16, 12, 8} {
		entry := plain(strip.entries[0].render(scopes.Tokens(), charsetUTF8, capabilityTruecolor, width))
		if lipgloss.Width(entry) > width {
			t.Errorf("at width %d the entry %q is %d cells wide", width, entry, lipgloss.Width(entry))
		}
		if !strings.HasPrefix(entry, glyph) {
			t.Errorf("at width %d the entry %q dropped its category glyph", width, entry)
		}
		if !strings.HasSuffix(entry, "3m") {
			t.Errorf("at width %d the entry %q dropped its textual age", width, entry)
		}
	}
}

// TestInterestingStripFillsAnUnboundedBody guards the unbounded body: a size the
// shell has not been told yet bounds neither the field nor the strip, so the
// strip renders its whole visible selection and no more than the five entries
// RG-007 makes visible.
func TestInterestingStripFillsAnUnboundedBody(t *testing.T) {
	repository := "acme/api"
	scopes := stripRenderScopes(t, repository)
	var retained []domain.EventEvidence
	for index := range 8 {
		retained = append(retained, stripEvidence(t, fmt.Sprintf("e%02d", index), repository, time.Duration(index+1)*time.Minute))
	}
	strip := startedStrip(scopes, stripSnapshot(scopes, retained...))

	rows, shown := strip.rows(unbounded)
	if rows != 6 || shown != 5 {
		t.Fatalf("the unbounded strip took %d rows showing %d entries, want 6 and 5", rows, shown)
	}
	lines := strip.render(scopes.Tokens(), charsetUTF8, capabilityTruecolor, unbounded, rows, shown)
	if len(lines) != rows {
		t.Fatalf("the unbounded strip rendered %d lines, want %d", len(lines), rows)
	}
	if got, want := plain(lines[0]), "Interesting Now: 5 shown · 3 hidden · 0 omitted"; got != want {
		t.Errorf("the unbounded strip title is %q, want %q", got, want)
	}

	body := newRain().reconciled(scopes, stripSnapshot(scopes, retained...), stripBase).
		render(State{Scopes: scopes, Freshness: FreshnessCurrent}, strip, charsetUTF8, capabilityTruecolor, wideWidth, unbounded)
	if !strings.Contains(plain(body), "Interesting Now: 5 shown") {
		t.Errorf("an unbounded Rain body renders no strip:\n%s", plain(body))
	}
}
