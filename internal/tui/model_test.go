// The shell is tested from inside the package because it exposes only the
// Bubble Tea entry point; mode, shared state, and the per-view seams are
// package-internal by design.
package tui

import (
	"context"
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

func testScope(t *testing.T, values ...string) domain.Scope {
	t.Helper()
	scope, err := domain.NewScope(values)
	if err != nil {
		t.Fatalf("building the test scope %v failed: %v", values, err)
	}
	return scope
}

// newModel builds the root model for the scope with a source that is never
// driven, so the shell renders its pending first refresh.
func newModel(t *testing.T, values ...string) Model {
	t.Helper()
	return New(context.Background(), testScope(t, values...), &fakeSource{})
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
		{name: "tab back to overview", keystrokes: []string{"tab", "tab"}, wantMode: ModeOverview, wantBody: "repository activity"},
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
		{name: "current", freshness: FreshnessCurrent, wantBody: map[Mode]string{ModeOverview: "No recent activity", ModeStream: "not rendered yet"}},
		{name: "error", freshness: FreshnessError, cause: "refreshing acme/backend: request failed", wantBody: map[Mode]string{ModeOverview: "unavailable", ModeStream: "unavailable"}},
		{name: "stale", freshness: FreshnessStale, cause: "refreshing acme/backend: request failed", wantBody: map[Mode]string{ModeOverview: "No recent activity", ModeStream: "not rendered yet"}},
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

	for _, want := range []string{"1 overview", "2 stream", "tab switch", "q quit"} {
		if !strings.Contains(content, want) {
			t.Errorf("footer does not advertise %q:\n%s", want, content)
		}
	}
	for _, unwanted := range []string{"scroll", "refresh", "inspect", "filter"} {
		if strings.Contains(strings.ToLower(content), unwanted) {
			t.Errorf("footer advertises the unimplemented control %q:\n%s", unwanted, content)
		}
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
