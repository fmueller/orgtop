package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"regexp"

	"github.com/fmueller/orgtop/internal/domain"
)

// colorEscape matches the SGR sequences that set a foreground or background
// colour. Bold and faint are intensity rather than colour, so a no-color render
// may still emit them (RG-008).
var colorEscape = regexp.MustCompile(`\x1b\[[0-9;]*(3[0-9]|4[0-9]|9[0-9]|10[0-7])[;m]`)

// rainModel builds a shell already in Rain over a started field, published
// exactly as one successful refresh publishes it and sized to the terminal.
func rainModel(t *testing.T, scopes domain.ScopeSet, retained []domain.EventEvidence, width, height int) Model {
	t.Helper()
	model, err := New(context.Background(), scopes, &fakeSource{})
	if err != nil {
		t.Fatalf("building the Rain test model failed: %v", err)
	}
	snapshot := domain.NewRetainedSnapshot(scopes, retained, false)
	model.state.Scopes = scopes
	model.state.Scoped = snapshot
	model.state.Freshness = FreshnessCurrent
	model.state.LastSuccess = rainBase
	model.mode = ModeRain
	model.rain = newRain().reconciled(scopes, snapshot, rainBase)
	model, _ = apply(t, model, tea.WindowSizeMsg{Width: width, Height: height})
	return model
}

// rainBodyLines returns every rendered body line between the shared header and
// footer, including the blank field rows bodyLines drops.
func rainBodyLines(content string) []string {
	lines := strings.Split(content, "\n")
	if len(lines) <= chromeLines {
		return nil
	}
	return lines[1 : len(lines)-1]
}

// TestRainIsReachableByDirectAndTabNavigation guards the task's navigation
// contract: `3` selects Rain directly and `tab` cycles all three views.
func TestRainIsReachableByDirectAndTabNavigation(t *testing.T) {
	model := newModel(t, "acme/backend")

	model, _ = apply(t, model, press("3"))
	if model.mode != ModeRain {
		t.Fatalf("`3` selected mode %d, want Rain", model.mode)
	}
	if got := model.mode.Label(); got != "RAIN" {
		t.Errorf("the Rain header label is %q, want RAIN", got)
	}

	model, _ = apply(t, model, press("1"))
	for _, want := range []Mode{ModeStream, ModeRain, ModeOverview} {
		model, _ = apply(t, model, press("tab"))
		if model.mode != want {
			t.Fatalf("tab selected mode %d, want %d", model.mode, want)
		}
	}
}

// TestRainRetainsFieldStateAndDataAcrossViewSwitches guards per-view state
// retention: the window, the pause, and the selected page survive a round trip
// through the other two views, and the loaded snapshot is untouched.
func TestRainRetainsFieldStateAndDataAcrossViewSwitches(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	model := rainModel(t, scopes, []domain.EventEvidence{rainEvidence(t, "one", repository, time.Minute)}, 80, 20)

	model, _ = apply(t, model, press("-"), press("p"))
	window, paused := model.rain.window, model.rain.paused
	if window != rainWindow(1) || !paused {
		t.Fatalf("Rain is at window %s paused=%v, want 30m paused", window, paused)
	}

	model, _ = apply(t, model, press("1"), press("2"), press("3"))
	if model.rain.window != window || model.rain.paused != paused {
		t.Errorf("Rain returned at window %s paused=%v, want %s paused=%v",
			model.rain.window, model.rain.paused, window, paused)
	}
	if got := len(model.state.Scoped.Aggregates()); got != 1 {
		t.Errorf("the snapshot holds %d aggregates after switching views, want 1", got)
	}
}

// TestRainWindowAndPauseKeysActOnlyInRain guards that Rain's own controls stay
// Rain's: the other views ignore them entirely.
func TestRainWindowAndPauseKeysActOnlyInRain(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	model := rainModel(t, scopes, nil, 80, 20)

	for _, mode := range []Mode{ModeOverview, ModeStream} {
		model.mode = mode
		switched, _ := apply(t, model, press("p"), press("-"), press("]"))
		if switched.rain.paused || switched.rain.window != defaultRainWindow || switched.rain.start != 0 {
			t.Errorf("%s reacted to Rain controls: paused=%v window=%s start=%d",
				mode.Label(), switched.rain.paused, switched.rain.window, switched.rain.start)
		}
	}
}

// TestRainTickAdvancesTheFieldAndContinuesOneChain guards RG-006's single timer
// chain: the expected generation advances the field and schedules the next
// generation, while a duplicate message starts no second chain.
func TestRainTickAdvancesTheFieldAndContinuesOneChain(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	model := rainModel(t, scopes, []domain.EventEvidence{rainEvidence(t, "one", repository, 0)}, 80, 20)

	before := model.rain.chain
	model, cmd := apply(t, model, rainTickMsg{chain: before, at: rainAt(rainStep)})
	if model.rain.chain != before+1 {
		t.Fatalf("an accepted tick left the chain at %d, want %d", model.rain.chain, before+1)
	}
	if model.rain.cursor != rainAt(rainStep) {
		t.Errorf("the cursor is %v, want %v", model.rain.cursor, rainAt(rainStep))
	}
	if cmd == nil {
		t.Fatal("an accepted tick scheduled no next tick, so the one Rain chain stopped")
	}

	stale, staleCmd := apply(t, model, rainTickMsg{chain: before, at: rainAt(2 * rainStep)})
	if stale.rain.chain != before+1 {
		t.Errorf("a duplicate tick advanced the chain to %d, want %d", stale.rain.chain, before+1)
	}
	if staleCmd != nil {
		t.Error("a duplicate tick scheduled another tick, so a second Rain chain started")
	}
}

// TestInitStartsTheRainChainBesideTheFirstRefresh guards that the one Rain
// timer chain exists from launch rather than only once Rain is selected.
func TestInitStartsTheRainChainBesideTheFirstRefresh(t *testing.T) {
	model := newModel(t, "acme/backend")
	cmd := model.Init()
	if cmd == nil {
		t.Fatal("Init returned no command")
	}
	batch, batched := cmd().(tea.BatchMsg)
	if !batched {
		t.Fatalf("Init produced %T, want a batch holding the refresh and the Rain tick", cmd())
	}
	if len(batch) != 2 {
		t.Fatalf("Init batched %d commands, want the refresh and the Rain tick", len(batch))
	}
}

// TestRainRendersMixedScopeColumnsAndGlyphs guards the Scope columns and the
// shared category vocabulary: every visible Scope gets its RG-012 token label
// and admitted items draw the shared glyph.
func TestRainRendersMixedScopeColumnsAndGlyphs(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t,
		domain.NewRepositoryScope(testRepository(t, repository)),
		pathScope(t, repository, "services"),
	)
	model := rainModel(t, scopes, []domain.EventEvidence{rainEvidence(t, "one", repository, time.Minute)}, 100, 20)
	model.charset = charsetUTF8

	content := model.View().Content
	for _, want := range []string{"R1 " + repository, "P2 " + repository + ":services/**"} {
		if !strings.Contains(content, want) {
			t.Errorf("the Rain columns do not label %q:\n%s", want, content)
		}
	}
	if !strings.Contains(content, categoryGlyph(domain.CategoryPush, charsetUTF8)) {
		t.Errorf("the Rain field draws no shared push glyph:\n%s", content)
	}
	if !strings.Contains(content, "window 60m") {
		t.Errorf("the Rain context does not report its window:\n%s", content)
	}
}

// TestRainLegendRendersWhenDimensionsPermit guards RG-008: a Rain wide enough
// exposes the complete glyph-to-text legend.
func TestRainLegendRendersWhenDimensionsPermit(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	model := rainModel(t, scopes, nil, 120, 20)
	model.charset = charsetUTF8

	content := model.View().Content
	for _, category := range categoryOrder {
		semantics := semanticsOf(category)
		if !strings.Contains(content, semantics.utf8+" "+semantics.rich) {
			t.Errorf("the Rain legend omits %q:\n%s", semantics.rich, content)
		}
	}
	if strings.Contains(content, legendHiddenContext) {
		t.Errorf("a Rain that renders its legend still reports it hidden:\n%s", content)
	}
}

// TestConstrainedRainReportsItsHiddenLegend guards RG-008 and RG-012: a Rain
// too narrow for the legend keeps the hidden-legend indicator.
func TestConstrainedRainReportsItsHiddenLegend(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	cases := []struct {
		name  string
		width int
		want  string
	}{
		{name: "full spelling", width: 47, want: legendHiddenContext},
		{name: "compact spelling", width: narrowWidth, want: "no legend"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := rainModel(t, scopes, nil, testCase.width, narrowHeight)
			model.capability = capabilityTruecolor

			content := model.View().Content
			if !strings.Contains(content, testCase.want) {
				t.Errorf("a constrained Rain hides its legend without saying so:\n%s", content)
			}
		})
	}
}

// TestRainWithoutIntensityReportsRecencyCounts guards RG-008's no-color
// fallback: a profile that renders no distinct intensity attributes gets the
// textual recency counts of the visible page instead.
func TestRainWithoutIntensityReportsRecencyCounts(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	retained := []domain.EventEvidence{
		rainEvidence(t, "fresh", repository, time.Minute),
		rainEvidence(t, "older", repository, 10*time.Minute),
		rainEvidence(t, "oldest", repository, 20*time.Minute),
	}

	plain := rainModel(t, scopes, retained, 120, 20)
	plain.capability = capabilityNoColor
	content := plain.View().Content
	if !strings.Contains(content, "recency: 1 new · 1 recent · 1 aging") {
		t.Errorf("a no-color Rain does not report its recency counts:\n%s", content)
	}
	glyph := categoryGlyph(domain.CategoryPush, plain.charset)
	drawn := 0
	for _, line := range rainBodyLines(content) {
		// The legend spells the same glyph beside its text; only the field
		// rows draw it alone.
		if !strings.Contains(line, glyph) || strings.Contains(line, categoryText(domain.CategoryPush, registerRich)) {
			continue
		}
		drawn++
		if colorEscape.MatchString(line) {
			t.Errorf("a no-color Rain field row emitted a color escape: %q", line)
		}
	}
	if drawn == 0 {
		t.Errorf("the no-color Rain field drew no category glyph at all:\n%s", content)
	}

	colored := rainModel(t, scopes, retained, 120, 20)
	colored.capability = capabilityTruecolor
	if colored := colored.View().Content; strings.Contains(colored, "recency: ") {
		t.Errorf("a Rain that renders intensity still spends a segment on recency counts:\n%s", colored)
	}
}

// TestNarrowRainShortensTheRecencyCounts guards RG-008's shortened form: the
// counts survive a width that cannot hold their full spelling.
func TestNarrowRainShortensTheRecencyCounts(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	model := rainModel(t, scopes, []domain.EventEvidence{rainEvidence(t, "one", repository, time.Minute)}, narrowWidth, narrowHeight)
	model.capability = capabilityNoColor

	content := model.View().Content
	if !strings.Contains(content, "age 1/0/0") {
		t.Errorf("a narrow no-color Rain drops its recency counts entirely:\n%s", content)
	}
}

// TestRainResizeRepagesAndStaysBounded guards RG-006 resize: the column count
// follows the width, the anchor keeps its page, and every render stays inside
// the terminal.
func TestRainResizeRepagesAndStaysBounded(t *testing.T) {
	repositories := []string{"acme/a", "acme/b", "acme/c", "acme/d"}
	built := make([]domain.Scope, 0, len(repositories))
	for _, repository := range repositories {
		built = append(built, domain.NewRepositoryScope(testRepository(t, repository)))
	}
	scopes := scopeSet(t, built...)
	model := rainModel(t, scopes, nil, 120, 20)

	if got := len(model.rain.field().columns); got != len(repositories) {
		t.Fatalf("a 120-cell Rain shows %d columns, want %d", got, len(repositories))
	}
	for _, size := range []tea.WindowSizeMsg{{Width: narrowWidth, Height: narrowHeight}, {Width: 26, Height: 6}, {Width: 120, Height: 20}} {
		model, _ = apply(t, model, size)
		assertFits(t, model.View().Content, size.Width, size.Height)
	}
	if model.rain.start != 0 {
		t.Errorf("the restored page starts at Scope %d, want the anchored first page", model.rain.start)
	}
}

// TestRainAtTheNarrowFloorKeepsRequiredContext guards the closed `40x10`
// dimensions: Rain keeps the active view, the transport label, one field line,
// and the quit hint.
func TestRainAtTheNarrowFloorKeepsRequiredContext(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	model := rainModel(t, scopes, []domain.EventEvidence{rainEvidence(t, "one", repository, time.Minute)}, narrowWidth, narrowHeight)
	model.charset = charsetUTF8

	content := model.View().Content
	assertFits(t, content, narrowWidth, narrowHeight)
	for _, want := range []string{ModeRain.Label(), transportLabel, "q quit", "window 60m"} {
		if !strings.Contains(content, want) {
			t.Errorf("the 40x10 Rain render does not contain %q:\n%s", want, content)
		}
	}
	if body := rainBodyLines(content); len(body) != narrowHeight-chromeLines {
		t.Errorf("the Rain body used %d lines, want %d", len(body), narrowHeight-chromeLines)
	}
}

// TestRainExplicitStatesReplaceTheField guards the shared freshness contract:
// before a snapshot exists Rain says so in its own words rather than drawing an
// empty field that reads as quiet.
func TestRainExplicitStatesReplaceTheField(t *testing.T) {
	cases := []struct {
		freshness Freshness
		want      string
	}{
		{freshness: FreshnessLoading, want: "Loading"},
		{freshness: FreshnessError, want: "unavailable"},
	}
	for _, testCase := range cases {
		t.Run(testCase.freshness.Marker(), func(t *testing.T) {
			model := newModel(t, "acme/backend")
			model.mode = ModeRain
			model.state.Freshness = testCase.freshness
			model, _ = apply(t, model, tea.WindowSizeMsg{Width: narrowWidth, Height: narrowHeight})

			content := model.View().Content
			assertFits(t, content, narrowWidth, narrowHeight)
			if !strings.Contains(content, testCase.want) {
				t.Errorf("the %s Rain render does not contain %q:\n%s", testCase.freshness.Marker(), testCase.want, content)
			}
		})
	}
}

// TestRainReportsAnEmptySnapshotExplicitly guards FR-009's honesty: a complete
// refresh that admitted nothing says so rather than rendering blank rows.
func TestRainReportsAnEmptySnapshotExplicitly(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	model := rainModel(t, scopes, nil, 80, 20)

	if content := model.View().Content; !strings.Contains(content, noRecentActivity) {
		t.Errorf("an empty Rain field does not state it is empty:\n%s", content)
	}
}

// TestRainPageKeysWrapAcrossFixedPages guards RG-006 paging: `]` and `[` move
// one fixed page and wrap at both ends.
func TestRainPageKeysWrapAcrossFixedPages(t *testing.T) {
	built := make([]domain.Scope, 0, 4)
	for _, repository := range []string{"acme/a", "acme/b", "acme/c", "acme/d"} {
		built = append(built, domain.NewRepositoryScope(testRepository(t, repository)))
	}
	scopes := scopeSet(t, built...)
	model := rainModel(t, scopes, nil, 26, 12)

	if perPage := rainPerPage(model.rain.width, len(model.rain.scopes)); perPage != 2 {
		t.Fatalf("a 26-cell Rain holds %d Scopes per page, want 2", perPage)
	}
	model, _ = apply(t, model, press("]"))
	if model.rain.start != 2 {
		t.Fatalf("`]` selected the page at %d, want 2", model.rain.start)
	}
	model, _ = apply(t, model, press("]"))
	if model.rain.start != 0 {
		t.Fatalf("`]` past the last page selected %d, want the wrap to 0", model.rain.start)
	}
	model, _ = apply(t, model, press("["))
	if model.rain.start != 2 {
		t.Errorf("`[` before the first page selected %d, want the wrap to 2", model.rain.start)
	}
}

// TestRainRefreshReconcilesTheFieldAndKeepsTheOtherViews guards that a
// successful refresh publishes into Rain through the same atomic transition the
// other two views read.
func TestRainRefreshReconcilesTheFieldAndKeepsTheOtherViews(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{result: recentActivity(t, "acme/backend")}}}
	model := lifecycle(t, source, fixedInstant, &recorder{}, "acme/backend")

	model, _ = run(t, model, model.refresh())
	if !model.rain.started {
		t.Fatal("a successful refresh left Rain without a cursor")
	}
	if model.rain.cursor != fixedInstant {
		t.Errorf("the Rain cursor is %v, want the publication instant %v", model.rain.cursor, fixedInstant)
	}
	if got := len(model.rain.items); got != 1 {
		t.Errorf("the reconciled field holds %d items, want 1", got)
	}
}

// recentActivity returns a scripted successful result holding one event inside
// the default Rain window, so a reconciled field admits it.
func recentActivity(t *testing.T, value string) Result {
	t.Helper()
	repository := testRepository(t, value)
	return Result{Repositories: []domain.RepositoryActivity{{
		Repository: repository,
		Events: []domain.Event{{
			ID:          repository.Key() + "-recent",
			OccurredAt:  fixedInstant.Add(-time.Minute),
			Repository:  repository,
			Category:    domain.CategoryPush,
			EntityKind:  domain.EntityRepository,
			Description: "pushed",
		}},
	}}}
}

// TestRainFieldRowsStayWithinTheColumnGeometry guards RG-006 geometry: every
// rendered field row is exactly the field width and never wider than a column
// grant.
func TestRainFieldRowsStayWithinTheColumnGeometry(t *testing.T) {
	built := make([]domain.Scope, 0, 3)
	retained := make([]domain.EventEvidence, 0, 3)
	for index, repository := range []string{"acme/a", "acme/b", "acme/c"} {
		built = append(built, domain.NewRepositoryScope(testRepository(t, repository)))
		retained = append(retained, rainEvidence(t, "event-"+repository, repository, time.Duration(index)*time.Minute))
	}
	scopes := scopeSet(t, built...)
	model := rainModel(t, scopes, retained, 41, 12)

	for index, line := range rainBodyLines(model.View().Content) {
		if got := lipgloss.Width(line); got > 41 {
			t.Errorf("body line %d is %d cells wide, want at most 41: %q", index, got, line)
		}
	}
}

// TestRainResizeBelowTheSharedChromeClearsTheField guards RG-006 resize: a
// terminal too short to hold the shared chrome leaves Rain no usable field row,
// so its retained geometry must collapse rather than keep the rows the previous
// size granted. A stale positive height would make the disjoint hidden-item
// accounting believe rows exist that the terminal cannot show.
func TestRainResizeBelowTheSharedChromeClearsTheField(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	model := rainModel(t, scopes, []domain.EventEvidence{rainEvidence(t, "one", repository, time.Minute)}, 80, 20)
	if model.rain.height == 0 {
		t.Fatal("the sized Rain field has no rows to lose")
	}

	for _, height := range []int{chromeLines, chromeLines - 1, 0} {
		resized, _ := apply(t, model, tea.WindowSizeMsg{Width: 80, Height: height})
		if resized.rain.height != 0 {
			t.Errorf("a %d-row terminal left the Rain field %d rows, want 0", height, resized.rain.height)
		}
		if hidden := resized.rain.field().hiddenItems; hidden != len(resized.rain.items) {
			t.Errorf("a %d-row terminal reports %d hidden items, want all %d", height, hidden, len(resized.rain.items))
		}
	}
}
