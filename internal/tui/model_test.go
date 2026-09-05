// The shell is tested from inside the package because it exposes only the
// Bubble Tea entry point; mode, shared state, and the per-view seams are
// package-internal by design.
package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

const (
	narrowWidth  = 40
	narrowHeight = 10
)

// fixedInstant is the instant every shell test pins its clock to. Naming one
// instant rather than reading the host clock keeps a rendered age, and every
// assertion derived from it, identical on every run (NFR-006). It sits after
// the events the scripted results carry, so a pinned success is never older
// than the activity it publishes.
var fixedInstant = time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)

func testScope(t *testing.T, values ...string) domain.ScopeSet {
	t.Helper()
	scope, err := domain.NewRepositoryScopeSet(values)
	if err != nil {
		t.Fatalf("building the test scope %v failed: %v", values, err)
	}
	return scope
}

// newModel builds the root model for the scope with a source that is never
// driven, so the shell renders its pending first refresh.
func newModel(t *testing.T, values ...string) Model {
	t.Helper()
	model, err := New(context.Background(), testScope(t, values...), &fakeSource{})
	if err != nil {
		t.Fatalf("building the test model failed: %v", err)
	}
	return model
}

// press builds the key press message for a keystroke the shell reacts to.
func press(keystroke string) tea.KeyPressMsg {
	switch keystroke {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	default:
		return tea.KeyPressMsg{Code: []rune(keystroke)[0], Text: keystroke}
	}
}

// apply drives the messages through the root model and returns the last command.
func apply(t *testing.T, model Model, messages ...tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	var last tea.Cmd
	for _, message := range messages {
		next, cmd := model.Update(message)
		updated, isModel := next.(Model)
		if !isModel {
			t.Fatalf("Update returned %T, want tui.Model", next)
		}
		model, last = updated, cmd
	}
	return model, last
}

func TestNewRejectsAMissingSource(t *testing.T) {
	_, err := New(context.Background(), testScope(t, "acme/backend"), nil)

	if !errors.Is(err, ErrNoSource) {
		t.Fatalf("New without a source returned %v, want ErrNoSource", err)
	}
}

// TestNewDefaultsTheClockToTheRealOne guards the production wiring: a caller
// that supplies no clock, and one that supplies a nil clock, both keep the
// binary reading the host clock exactly as before (NFR-006).
func TestNewDefaultsTheClockToTheRealOne(t *testing.T) {
	tests := []struct {
		name    string
		options []Option
	}{
		{name: "no option"},
		{name: "nil clock", options: []Option{WithClock(nil)}},
	}

	realClock := reflect.ValueOf(time.Now).Pointer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, err := New(context.Background(), testScope(t, "acme/backend"), &fakeSource{}, tt.options...)
			if err != nil {
				t.Fatalf("building the model failed: %v", err)
			}
			if model.now == nil {
				t.Fatal("the model was built without a clock")
			}
			if got := reflect.ValueOf(model.now).Pointer(); got != realClock {
				t.Error("the model's clock is not time.Now, so the binary no longer reads the real clock")
			}
		})
	}
}

// TestWithClockPinsTheInstantASuccessIsRecordedAt proves the seam an external
// caller needs: the supplied clock, not the host one, stamps State.LastSuccess.
func TestWithClockPinsTheInstantASuccessIsRecordedAt(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	model, err := New(context.Background(), testScope(t, "acme/backend"), source, WithClock(func() time.Time { return fixedInstant }))
	if err != nil {
		t.Fatalf("building the model failed: %v", err)
	}

	model, _ = run(t, model, initRefresh(t, model))

	if !model.state.LastSuccess.Equal(fixedInstant) {
		t.Errorf("LastSuccess = %v, want the pinned instant %v", model.state.LastSuccess, fixedInstant)
	}
}

func TestOverviewIsTheInitialViewAndRendersLoadingImmediately(t *testing.T) {
	model := newModel(t, "acme/backend")

	if model.mode != ModeOverview {
		t.Errorf("initial mode is %v, want ModeOverview", model.mode)
	}
	if model.state.Freshness != FreshnessLoading {
		t.Errorf("initial freshness is %v, want FreshnessLoading", model.state.Freshness)
	}

	content := model.View().Content
	for _, want := range []string{"OrgTop", "OVERVIEW", "POLLING", "LOADING", "Loading repository activity"} {
		if !strings.Contains(content, want) {
			t.Errorf("initial render does not contain %q:\n%s", want, content)
		}
	}
}

func TestModeKeysAndTabSwitchViews(t *testing.T) {
	cases := []struct {
		name       string
		keystrokes []string
		wantMode   Mode
		wantBody   string
	}{
		{name: "stream key", keystrokes: []string{"2"}, wantMode: ModeStream, wantBody: "recent events"},
		{name: "overview key", keystrokes: []string{"2", "1"}, wantMode: ModeOverview, wantBody: "repository activity"},
		{name: "tab to stream", keystrokes: []string{"tab"}, wantMode: ModeStream, wantBody: "recent events"},
		{name: "tab to rain", keystrokes: []string{"tab", "tab"}, wantMode: ModeRain, wantBody: "loading recent activity"},
		{name: "rain key", keystrokes: []string{"3"}, wantMode: ModeRain, wantBody: "loading recent activity"},
		{name: "tab back to overview", keystrokes: []string{"tab", "tab", "tab"}, wantMode: ModeOverview, wantBody: "repository activity"},
		{name: "repeated stream key stays", keystrokes: []string{"2", "2"}, wantMode: ModeStream, wantBody: "recent events"},
		{name: "unrelated key keeps the mode", keystrokes: []string{"2", "x"}, wantMode: ModeStream, wantBody: "recent events"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := newModel(t, "acme/backend")
			for _, keystroke := range testCase.keystrokes {
				model, _ = apply(t, model, press(keystroke))
			}

			if model.mode != testCase.wantMode {
				t.Errorf("mode is %v, want %v", model.mode, testCase.wantMode)
			}
			content := model.View().Content
			if !strings.Contains(content, testCase.wantMode.Label()) {
				t.Errorf("header does not contain %q:\n%s", testCase.wantMode.Label(), content)
			}
			if !strings.Contains(strings.ToLower(content), testCase.wantBody) {
				t.Errorf("body does not contain %q:\n%s", testCase.wantBody, content)
			}
		})
	}
}

func TestModeSwitchPreservesPerViewStateAndSnapshot(t *testing.T) {
	snapshot := domain.NewSnapshot(testScope(t, "acme/backend"), nil)

	model := newModel(t, "acme/backend")
	model.state.Snapshot = snapshot
	model.state.Freshness = FreshnessCurrent
	model.overview.offset = 2
	model.stream.offset = 7

	model, _ = apply(t, model, press("2"), press("tab"), press("1"), press("tab"))

	if model.overview.offset != 2 {
		t.Errorf("overview offset is %d, want 2", model.overview.offset)
	}
	if model.stream.offset != 7 {
		t.Errorf("stream offset is %d, want 7", model.stream.offset)
	}
	if got := len(model.state.Snapshot.Aggregates()); got != 1 {
		t.Errorf("snapshot aggregates after switching is %d, want 1", got)
	}
}

func TestQuitKeysRequestQuit(t *testing.T) {
	for _, keystroke := range []string{"q", "ctrl+c"} {
		t.Run(keystroke, func(t *testing.T) {
			_, cmd := apply(t, newModel(t, "acme/backend"), press(keystroke))
			if cmd == nil {
				t.Fatalf("%q returned no command, want quit", keystroke)
			}
			if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
				t.Errorf("%q produced %T, want tea.QuitMsg", keystroke, cmd())
			}
		})
	}
}

func TestNonQuitKeysRequestNoCommand(t *testing.T) {
	for _, keystroke := range []string{"1", "2", "tab", "x"} {
		if _, cmd := apply(t, newModel(t, "acme/backend"), press(keystroke)); cmd != nil {
			t.Errorf("%q produced command %T, want none", keystroke, cmd())
		}
	}
}

func TestResizeUpdatesDimensionsAndBoundsTheRender(t *testing.T) {
	model := newModel(t, "acme/backend", "acme/frontend")
	model, _ = apply(t, model, tea.WindowSizeMsg{Width: 100, Height: 24}, tea.WindowSizeMsg{Width: narrowWidth, Height: narrowHeight})

	if model.width != narrowWidth || model.height != narrowHeight {
		t.Errorf("dimensions are %dx%d, want %dx%d", model.width, model.height, narrowWidth, narrowHeight)
	}
	assertFits(t, model.View().Content, narrowWidth, narrowHeight)
}

func TestNarrowTerminalRetainsRequiredContext(t *testing.T) {
	cases := []struct {
		name      string
		freshness Freshness
		cause     string
		// wantBody is the content line each view retains, because the views
		// state the same freshness in their own words.
		wantBody map[Mode]string
	}{
		{name: "loading", freshness: FreshnessLoading, wantBody: map[Mode]string{ModeOverview: "Loading", ModeStream: "Loading"}},
		{name: "current", freshness: FreshnessCurrent, wantBody: map[Mode]string{ModeOverview: noRecentActivity, ModeStream: noRecentActivity}},
		{name: "error", freshness: FreshnessError, cause: "refreshing acme/backend: request failed", wantBody: map[Mode]string{ModeOverview: "unavailable", ModeStream: "unavailable"}},
		{name: "stale", freshness: FreshnessStale, cause: "refreshing acme/backend: request failed", wantBody: map[Mode]string{ModeOverview: noRecentActivity, ModeStream: noRecentActivity}},
	}

	for _, testCase := range cases {
		for _, mode := range []Mode{ModeOverview, ModeStream} {
			t.Run(testCase.name+"/"+mode.Label(), func(t *testing.T) {
				model := newModel(t, "acme/backend", "acme/frontend")
				model.mode = mode
				model.state.Freshness = testCase.freshness
				model.state.Cause = testCase.cause
				model, _ = apply(t, model, tea.WindowSizeMsg{Width: narrowWidth, Height: narrowHeight})

				content := model.View().Content
				assertFits(t, content, narrowWidth, narrowHeight)
				for _, want := range []string{mode.Label(), transportLabel, "q quit", testCase.wantBody[mode]} {
					if !strings.Contains(content, want) {
						t.Errorf("narrow render does not contain %q:\n%s", want, content)
					}
				}
				if marker := testCase.freshness.Marker(); marker != "" && !strings.Contains(content, marker) {
					t.Errorf("narrow render does not contain the %q marker:\n%s", marker, content)
				}
			})
		}
	}
}

func TestHeaderSeparatesTransportFromFreshness(t *testing.T) {
	markers := []string{"LOADING", "ERROR", "STALE"}
	cases := []struct {
		name       string
		freshness  Freshness
		wantMarker string
	}{
		{name: "loading", freshness: FreshnessLoading, wantMarker: "LOADING"},
		{name: "current", freshness: FreshnessCurrent, wantMarker: ""},
		{name: "error", freshness: FreshnessError, wantMarker: "ERROR"},
		{name: "stale", freshness: FreshnessStale, wantMarker: "STALE"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := newModel(t, "acme/backend")
			model.state.Freshness = testCase.freshness
			content := model.View().Content

			if !strings.Contains(content, transportLabel) {
				t.Errorf("header does not contain the %q transport label:\n%s", transportLabel, content)
			}
			if strings.Contains(content, "LIVE") {
				t.Errorf("header contains a LIVE label:\n%s", content)
			}
			for _, marker := range markers {
				contains := strings.Contains(content, marker)
				if marker == testCase.wantMarker && !contains {
					t.Errorf("header does not contain the %q marker:\n%s", marker, content)
				}
				if marker != testCase.wantMarker && contains {
					t.Errorf("header unexpectedly contains the %q marker:\n%s", marker, content)
				}
			}
		})
	}
}

func TestHeaderKeepsScopeContextAndLastSuccess(t *testing.T) {
	lastSuccess := time.Date(2026, time.August, 22, 12, 34, 56, 0, time.UTC)
	model := newModel(t, "acme/backend", "acme/frontend")
	model.state.Freshness = FreshnessCurrent
	model.state.LastSuccess = lastSuccess

	wide, _ := apply(t, model, tea.WindowSizeMsg{Width: 120, Height: 24})
	content := wide.View().Content
	for _, want := range []string{"acme/backend", "acme/frontend", "updated 12:34:56"} {
		if !strings.Contains(content, want) {
			t.Errorf("wide header does not contain %q:\n%s", want, content)
		}
	}

	narrow, _ := apply(t, model, tea.WindowSizeMsg{Width: narrowWidth, Height: narrowHeight})
	content = narrow.View().Content
	assertFits(t, content, narrowWidth, narrowHeight)
	if !strings.Contains(content, "2 repositories") {
		t.Errorf("narrow header does not summarize the scope as a count:\n%s", content)
	}
	if strings.Contains(content, "acme/frontend") {
		t.Errorf("narrow header still lists the full scope:\n%s", content)
	}
}

func TestFooterAdvertisesOnlyImplementedControls(t *testing.T) {
	model, _ := apply(t, newModel(t, "acme/backend"), tea.WindowSizeMsg{Width: 120, Height: 24})
	content := model.View().Content

	for _, want := range []string{"1 overview", "2 stream", "tab switch", "up/down scroll", "pgup/pgdn page", "q quit"} {
		if !strings.Contains(content, want) {
			t.Errorf("footer does not advertise %q:\n%s", want, content)
		}
	}
	for _, unwanted := range []string{"refresh", "inspect", "filter"} {
		if strings.Contains(strings.ToLower(content), unwanted) {
			t.Errorf("footer advertises the unimplemented control %q:\n%s", unwanted, content)
		}
	}
}

// TestFooterNeverDisplacesTheQuitHint keeps the scroll controls subordinate to
// the quit hint: a width that cannot hold them drops them, never the way out.
func TestFooterNeverDisplacesTheQuitHint(t *testing.T) {
	for _, width := range []int{120, 60, narrowWidth, 30, 20, 10, 6} {
		model, _ := apply(t, newModel(t, "acme/backend"), tea.WindowSizeMsg{Width: width, Height: narrowHeight})
		content := model.View().Content
		assertFits(t, content, width, narrowHeight)

		lines := strings.Split(content, "\n")
		footer := lines[len(lines)-1]
		if !strings.Contains(footer, "q quit") {
			t.Errorf("footer at width %d is %q, want it to keep the quit hint", width, footer)
		}
	}
}

// TestScrollKeysMoveOnlyTheActiveView guards the per-view offsets: both views
// scroll with the same keys, and a keystroke never moves the inactive one.
func TestScrollKeysMoveOnlyTheActiveView(t *testing.T) {
	streamActive, _ := apply(t, streamModel(t, numberedEvents(t, scrollEvents)),
		tea.WindowSizeMsg{Width: wideWidth, Height: scrollTerminalHeight})
	moved := scrolled(t, streamActive, "down", "pgdown")
	if moved.overview.offset != 0 {
		t.Errorf("overview offset is %d after Stream scrolling, want it unmoved", moved.overview.offset)
	}
	if moved.stream.offset == 0 {
		t.Error("stream offset is 0 after Stream scrolling, want the active view moved")
	}

	overviewActive, _ := apply(t, scrollOverviewModel(t, scrollRepositories),
		tea.WindowSizeMsg{Width: wideWidth, Height: scrollTerminalHeight})
	moved = scrolled(t, overviewActive, "down", "pgdown")
	if moved.stream.offset != 0 {
		t.Errorf("stream offset is %d after Overview scrolling, want it unmoved", moved.stream.offset)
	}
	if moved.overview.offset == 0 {
		t.Error("overview offset is 0 after Overview scrolling, want the active view moved")
	}
}

func TestScrollableViewsAccountForHiddenRowsInTheHeader(t *testing.T) {
	tests := []struct {
		name       string
		model      Model
		wantFirst  string
		keystrokes []string
		wantMoved  string
	}{
		{
			name:       "overview",
			model:      scrollOverviewModel(t, scrollRepositories),
			wantFirst:  "scopes 1-8 of 20",
			keystrokes: []string{"pgdown"},
			wantMoved:  "scopes 9-16 of 20",
		},
		{
			name:       "stream",
			model:      streamModel(t, numberedEvents(t, scrollEvents)),
			wantFirst:  "events 1-6 of 20",
			keystrokes: []string{"pgdown"},
			wantMoved:  "events 2-7 of 20",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model, _ := apply(t, test.model, tea.WindowSizeMsg{Width: wideWidth, Height: scrollTerminalHeight})
			if content := model.View().Content; !strings.Contains(content, test.wantFirst) {
				t.Fatalf("initial render does not account for hidden rows with %q:\n%s", test.wantFirst, content)
			}
			moved := scrolled(t, model, test.keystrokes...)
			if content := moved.View().Content; !strings.Contains(content, test.wantMoved) {
				t.Errorf("moved render does not account for hidden rows with %q:\n%s", test.wantMoved, content)
			}
		})
	}
}

func TestOverviewAccountsForHiddenQuietScopesAfterItsEmptyState(t *testing.T) {
	names := repositoryNames(scrollRepositories)
	activities := make([]domain.RepositoryActivity, 0, len(names))
	for _, name := range names {
		activities = append(activities, testActivity(t, name))
	}
	model := publishSnapshots(newModel(t, names...), testScope(t, names...), activities)
	model, _ = apply(t, model, tea.WindowSizeMsg{Width: wideWidth, Height: scrollTerminalHeight})

	if content := model.View().Content; !strings.Contains(content, "scopes 1-7 of 20") {
		t.Fatalf("quiet Overview does not account for the Scope rows below its empty-state line:\n%s", content)
	}
	moved := scrolled(t, model, "pgdown")
	if content := moved.View().Content; !strings.Contains(content, "scopes 8-15 of 20") {
		t.Errorf("scrolled quiet Overview reports the wrong Scope range:\n%s", content)
	}
}

func TestStreamAccountsForZeroVisibleRowsAtChromeOnlyHeight(t *testing.T) {
	model, _ := apply(t, streamModel(t, numberedEvents(t, scrollEvents)),
		tea.WindowSizeMsg{Width: wideWidth, Height: chromeLines})
	content := model.View().Content

	assertFits(t, content, wideWidth, chromeLines)
	for _, want := range []string{"STREAM", "POLLING", "events 0 shown of 20", "q quit"} {
		if !strings.Contains(content, want) {
			t.Errorf("chrome-only Stream does not contain %q:\n%s", want, content)
		}
	}
}

func TestScrollableViewsPreservePositionThroughZeroBodyRows(t *testing.T) {
	t.Run("overview", func(t *testing.T) {
		model, _ := apply(t, scrollOverviewModel(t, scrollRepositories),
			tea.WindowSizeMsg{Width: wideWidth, Height: scrollTerminalHeight})
		model = scrolled(t, model, "pgdown")
		wantOffset, wantRange := model.overview.offset, "scopes 9-16 of 20"

		collapsed, _ := apply(t, model,
			tea.WindowSizeMsg{Width: narrowWidth, Height: chromeLines},
			tea.WindowSizeMsg{Width: narrowWidth, Height: chromeLines})
		if collapsed.overview.offset != wantOffset {
			t.Fatalf("chrome-only resize moved Overview offset from %d to %d", wantOffset, collapsed.overview.offset)
		}

		restored, _ := apply(t, collapsed, tea.WindowSizeMsg{Width: wideWidth, Height: scrollTerminalHeight})
		if restored.overview.offset != wantOffset || !strings.Contains(restored.View().Content, wantRange) {
			t.Errorf("restored Overview did not return to offset %d and %q:\n%s", wantOffset, wantRange, restored.View().Content)
		}
	})

	t.Run("stream", func(t *testing.T) {
		model, _ := apply(t, streamModel(t, numberedEvents(t, scrollEvents)),
			tea.WindowSizeMsg{Width: wideWidth, Height: scrollTerminalHeight})
		model = scrolled(t, model, "pgdown", "pgdown")
		wantFocus, wantOffset, wantRange := model.stream.focus, model.stream.offset, "events 8-13 of 20"

		collapsed, _ := apply(t, model,
			tea.WindowSizeMsg{Width: narrowWidth, Height: chromeLines},
			tea.WindowSizeMsg{Width: narrowWidth, Height: chromeLines})
		if collapsed.stream.focus != wantFocus || collapsed.stream.offset != wantOffset {
			t.Fatalf("chrome-only resize moved Stream focus/offset from %d/%d to %d/%d",
				wantFocus, wantOffset, collapsed.stream.focus, collapsed.stream.offset)
		}

		restored, _ := apply(t, collapsed, tea.WindowSizeMsg{Width: wideWidth, Height: scrollTerminalHeight})
		if restored.stream.focus != wantFocus || restored.stream.offset != wantOffset || !strings.Contains(restored.View().Content, wantRange) {
			t.Errorf("restored Stream did not return to focus/offset %d/%d and %q:\n%s",
				wantFocus, wantOffset, wantRange, restored.View().Content)
		}
	})
}

func TestOverflowRangeUsesClosedFullCompactAndMinimumForms(t *testing.T) {
	overflow := overflowRange{kind: "events", first: 8, last: 13, total: 20}
	want := []string{"events 8-13 of 20", "8-13/20", "+14"}
	if got := overflow.forms(); !reflect.DeepEqual(got, want) {
		t.Errorf("overflow forms are %q, want %q", got, want)
	}

	zero := overflowRange{kind: "events", total: 20}
	want = []string{"events 0 shown of 20", "0/20", "+20"}
	if got := zero.forms(); !reflect.DeepEqual(got, want) {
		t.Errorf("zero-row overflow forms are %q, want %q", got, want)
	}
}

func TestSmallPositiveSizesRenderWithinBounds(t *testing.T) {
	sizes := []tea.WindowSizeMsg{
		{Width: 1, Height: 1},
		{Width: 2, Height: 1},
		{Width: 1, Height: 2},
		{Width: 3, Height: 3},
		{Width: narrowWidth, Height: 2},
		{Width: 8, Height: narrowHeight},
		{Width: narrowWidth, Height: narrowHeight},
	}

	for _, size := range sizes {
		for _, mode := range []Mode{ModeOverview, ModeStream} {
			model := newModel(t, "acme/backend")
			model.mode = mode
			model, _ = apply(t, model, size)
			assertFits(t, model.View().Content, size.Width, size.Height)
		}
	}
}

func TestReportedZeroDimensionsBoundTheRender(t *testing.T) {
	sizes := []tea.WindowSizeMsg{
		{Width: 0, Height: 0},
		{Width: narrowWidth, Height: 0},
		{Width: 0, Height: narrowHeight},
	}

	for _, size := range sizes {
		for _, mode := range []Mode{ModeOverview, ModeStream} {
			model := newModel(t, "acme/backend")
			model.mode = mode
			model, _ = apply(t, model, size)
			assertFits(t, model.View().Content, size.Width, size.Height)
		}
	}
}

func TestUnknownTerminalSizeRendersUnbounded(t *testing.T) {
	content := newModel(t, "acme/backend").View().Content

	if lines := strings.Split(content, "\n"); len(lines) != 3 {
		t.Fatalf("unsized render used %d lines, want header, body, and footer:\n%s", len(lines), content)
	}
	for _, want := range []string{"OVERVIEW", transportLabel, "acme/backend", "q quit"} {
		if !strings.Contains(content, want) {
			t.Errorf("unsized render does not contain %q:\n%s", want, content)
		}
	}
}

func TestRenderIsRepeatable(t *testing.T) {
	model, _ := apply(t, newModel(t, "acme/backend"), tea.WindowSizeMsg{Width: 80, Height: 20})

	first := model.View().Content
	if second := model.View().Content; first != second {
		t.Errorf("repeated render differs:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// assertFits checks the rendered content against the terminal budget.
func assertFits(t *testing.T, content string, width, height int) {
	t.Helper()
	var lines []string
	if content != "" {
		lines = strings.Split(content, "\n")
	}
	if len(lines) > height {
		t.Errorf("render used %d lines, want at most %d:\n%s", len(lines), height, content)
	}
	for index, line := range lines {
		if rendered := lipgloss.Width(line); rendered > width {
			t.Errorf("line %d is %d cells wide, want at most %d: %q", index, rendered, width, line)
		}
	}
}

// TestTheShellRendersOnTheAlternateScreen guards FR-001: a valid launch owns
// the whole terminal and restores the caller's screen on exit.
func TestTheShellRendersOnTheAlternateScreen(t *testing.T) {
	if !newModel(t, "acme/backend").View().AltScreen {
		t.Error("the shell does not render on the alternate screen")
	}
}

// TestHeaderMeasuresWideRunesByTheirRenderedWidth guards the shared header
// candidates against byte-wise measurement. The sanitized failure cause is the
// only free-form text the header carries: repository identities are ASCII by
// construction, so a relayed upstream cause is the one field whose byte length
// and rendered width differ.
// TestHeaderOmitsScopeContextForAnEmptySelection pins that a State carrying no
// Scope renders no scope context at all rather than an empty or zero-count
// summary. Only a selected Scope earns header space (FR-002).
func TestHeaderOmitsScopeContextForAnEmptySelection(t *testing.T) {
	state := State{Freshness: FreshnessCurrent}

	for _, candidate := range headerCandidates(state, ModeOverview) {
		rendered := plainFields(candidate)
		for _, unwanted := range []string{"repository", "repositories"} {
			if strings.Contains(rendered, unwanted) {
				t.Errorf("header candidate %q contains scope context %q for an empty selection", rendered, unwanted)
			}
		}
	}
}

func TestHeaderMeasuresWideRunesByTheirRenderedWidth(t *testing.T) {
	state := State{
		Scopes:    testScope(t, "acme/backend"),
		Freshness: FreshnessStale,
		Cause:     "仓库不可访问",
	}

	// The richest candidate is 65 cells but 71 bytes wide, so a header measured
	// by bytes drops to a sparser layout at exactly the width that fits it.
	const width = 65
	header := renderHeader(state, ModeOverview, width)

	assertFits(t, header, width, 1)
	for _, want := range []string{appName, state.Cause, "acme/backend"} {
		if !strings.Contains(header, want) {
			t.Errorf("header %q drops %q although the richest layout fits the width", header, want)
		}
	}
}

// TestStaleHeaderKeepsTheLastSuccessTimeOverScopeContext guards FR-008: a stale
// header must show the STALE marker, the last-success time, and a concise cause
// together. Once the cause leaves room for a single context field, the scope
// summary gives way — the operator selected the Scope on the command line,
// while the last-success time is live state nothing else reports.
func TestStaleHeaderKeepsTheLastSuccessTimeOverScopeContext(t *testing.T) {
	state := State{
		Scopes:      testScope(t, "acme/backend", "acme/frontend"),
		Freshness:   FreshnessStale,
		LastSuccess: time.Date(2026, time.August, 22, 12, 34, 56, 0, time.UTC),
		Cause:       "refreshing acme/frontend: unexpected github response: status 500",
	}

	const width = 120
	header := renderHeader(state, ModeOverview, width)

	assertFits(t, header, width, 1)
	for _, want := range []string{"STALE", "updated 12:34:56", "status 500"} {
		if !strings.Contains(header, want) {
			t.Errorf("stale header %q drops %q", header, want)
		}
	}
}
