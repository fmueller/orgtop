package tui

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// streamBase is the instant the newest test event occurred at, and the
// last-success instant the rendered ages are anchored to.
var streamBase = time.Date(2026, time.August, 22, 12, 0, 5, 0, time.Local)

// forbiddenStreamVocabulary lists the deferred capabilities Stream must not
// advertise in v0.1.0: filtering, search, clustering, derived signals, Inspect,
// and Rain.
var forbiddenStreamVocabulary = []string{
	"filter", "search", "cluster", "signal", "inspect", "rain", "drilldown",
}

// streamEvent builds a normalized event with explicit display detail.
func streamEvent(t *testing.T, id, repository string, at time.Time, category domain.Category, kind domain.EntityKind, actor, description string) domain.Event {
	t.Helper()
	parsed, err := domain.ParseRepository(repository)
	if err != nil {
		t.Fatalf("parsing the test repository %q failed: %v", repository, err)
	}
	return domain.Event{
		ID:          id,
		OccurredAt:  at,
		Repository:  parsed,
		Actor:       actor,
		Category:    category,
		EntityKind:  kind,
		Description: description,
	}
}

// detailedEvents returns one event per category, newest first, with the review
// event deliberately missing its optional actor.
func detailedEvents(t *testing.T) []domain.Event {
	t.Helper()
	return []domain.Event{
		streamEvent(t, "1", "acme/backend", streamBase, domain.CategoryPush, domain.EntityCommit, "alice", "pushed 2 commits to main"),
		streamEvent(t, "2", "acme/frontend", streamBase.Add(-time.Minute), domain.CategoryPullRequest, domain.EntityPullRequest, "bob", "opened pull request #7"),
		streamEvent(t, "3", "acme/backend", streamBase.Add(-2*time.Minute), domain.CategoryReview, domain.EntityPullRequest, "", "approved pull request #7"),
		streamEvent(t, "4", "acme/backend", streamBase.Add(-3*time.Minute), domain.CategoryComment, domain.EntityPullRequest, "carol", "commented on pull request #7"),
		streamEvent(t, "5", "acme/frontend", streamBase.Add(-4*time.Minute), domain.CategoryOther, domain.EntityOther, "dave", "created the tag v1.2.3"),
	}
}

// streamModel returns a current model whose snapshot holds the events.
func streamModel(t *testing.T, events []domain.Event) Model {
	t.Helper()
	scope := testScope(t, "acme/backend", "acme/frontend")
	var backend, frontend []domain.Event
	for _, event := range events {
		if event.Repository.String() == "acme/backend" {
			backend = append(backend, event)
			continue
		}
		frontend = append(frontend, event)
	}

	model := newModel(t, "acme/backend", "acme/frontend")
	model.mode = ModeStream
	model.state.Snapshot = domain.NewSnapshot(scope, []domain.RepositoryActivity{
		testActivity(t, "acme/backend", backend...),
		testActivity(t, "acme/frontend", frontend...),
	})
	model.state.Freshness = FreshnessCurrent
	model.state.LastSuccess = streamBase
	return model
}

// agedEvents returns one push event per elapsed duration, in the order given,
// each occurring that long before streamBase.
func agedEvents(t *testing.T, elapsed ...time.Duration) []domain.Event {
	t.Helper()
	events := make([]domain.Event, 0, len(elapsed))
	for index, since := range elapsed {
		events = append(events, streamEvent(t,
			fmt.Sprintf("%03d", index),
			"acme/backend",
			streamBase.Add(-since),
			domain.CategoryPush,
			domain.EntityCommit,
			"alice",
			fmt.Sprintf("pushed commit %02d", index+1),
		))
	}
	return events
}

// numberedEvents returns count push events for acme/backend, newest first, each
// carrying its own one-based position in its description, spaced one minute
// apart.
func numberedEvents(t *testing.T, count int) []domain.Event {
	t.Helper()
	elapsed := make([]time.Duration, 0, count)
	for index := range count {
		elapsed = append(elapsed, time.Duration(index)*time.Minute)
	}
	return agedEvents(t, elapsed...)
}

// The scrolling tests window 20 events through a body that shows 8 of them, so
// every keystroke has both a window above and one below it.
const (
	scrollTerminalHeight = 10
	scrollBodyHeight     = scrollTerminalHeight - chromeLines
	scrollEvents         = 20
)

// scrolled drives the keystrokes through an already sized model.
func scrolled(t *testing.T, model Model, keystrokes ...string) Model {
	t.Helper()
	for _, keystroke := range keystrokes {
		model, _ = apply(t, model, press(keystroke))
	}
	return model
}

// topRow returns the one-based numbered event the first body line shows.
func topRow(t *testing.T, model Model) int {
	t.Helper()
	rows := bodyLines(t, model.View().Content)
	if len(rows) == 0 {
		t.Fatalf("stream rendered no body rows:\n%s", model.View().Content)
	}
	for position := 1; position <= scrollEvents; position++ {
		if strings.Contains(rows[0], fmt.Sprintf("commit %02d", position)) {
			return position
		}
	}
	t.Fatalf("first body row %q names no numbered event", rows[0])
	return 0
}

func TestStreamRendersEveryEventInSnapshotOrder(t *testing.T) {
	events := detailedEvents(t)
	rows := bodyLines(t, renderAt(t, streamModel(t, events), wideWidth, wideHeight))

	if len(rows) != len(events) {
		t.Fatalf("stream rendered %d rows, want one per snapshot event:\n%v", len(rows), rows)
	}
	for index, event := range events {
		for _, want := range []string{
			eventAge(event.OccurredAt, streamBase),
			event.Repository.String(),
			event.Description,
		} {
			if !strings.Contains(rows[index], want) {
				t.Errorf("row %d is %q, want it to contain %q", index, rows[index], want)
			}
		}
		if event.Actor != "" && !strings.Contains(rows[index], event.Actor) {
			t.Errorf("row %d is %q, want the actor %q", index, rows[index], event.Actor)
		}
	}
}

func TestStreamOmitsTheActorWhenTheEventHasNone(t *testing.T) {
	rows := bodyLines(t, renderAt(t, streamModel(t, detailedEvents(t)), wideWidth, wideHeight))

	if len(rows) != 5 {
		t.Fatalf("stream rendered %d rows, want one per snapshot event:\n%v", len(rows), rows)
	}
	review := rows[2]
	if !strings.Contains(review, "approved pull request #7") {
		t.Fatalf("row 2 is %q, want the review description", review)
	}
	for _, actor := range []string{"alice", "bob", "carol", "dave"} {
		if strings.Contains(review, actor) {
			t.Errorf("actorless row %q contains the actor %q", review, actor)
		}
	}
	if strings.Contains(review, separator+separator) {
		t.Errorf("actorless row %q keeps an empty actor field", review)
	}
}

func TestStreamEncodesEachCategoryWithoutColor(t *testing.T) {
	rows := bodyLines(t, renderAt(t, streamModel(t, detailedEvents(t)), wideWidth, wideHeight))

	wantCategories := []string{"push", "pull request", "review", "comment", "other"}
	if len(rows) != len(wantCategories) {
		t.Fatalf("stream rendered %d rows, want one per snapshot event:\n%v", len(rows), rows)
	}
	for index, want := range wantCategories {
		if !strings.Contains(rows[index], want) {
			t.Errorf("row %d is %q, want the category text %q", index, rows[index], want)
		}
		if strings.Contains(rows[index], "\x1b[") {
			t.Errorf("row %d carries color escapes, so its category is not text-encoded: %q", index, rows[index])
		}
	}
}

func TestStreamScrollKeysWindowTheEventsWithinBounds(t *testing.T) {
	model, _ := apply(t, streamModel(t, numberedEvents(t, scrollEvents)),
		tea.WindowSizeMsg{Width: wideWidth, Height: scrollTerminalHeight})
	lastTop := scrollEvents - scrollBodyHeight + 1

	cases := []struct {
		name       string
		keystrokes []string
		wantTop    int
	}{
		{name: "initial", wantTop: 1},
		{name: "down", keystrokes: []string{"down"}, wantTop: 2},
		{name: "up clamps at the newest event", keystrokes: []string{"down", "up", "up"}, wantTop: 1},
		{name: "page down", keystrokes: []string{"pgdown"}, wantTop: 1 + scrollBodyHeight},
		{name: "page up returns", keystrokes: []string{"pgdown", "pgup"}, wantTop: 1},
		{name: "page down clamps at the oldest window", keystrokes: []string{"pgdown", "pgdown", "pgdown", "pgdown"}, wantTop: lastTop},
		{name: "down clamps at the oldest window", keystrokes: []string{"pgdown", "pgdown", "pgdown", "down", "down"}, wantTop: lastTop},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			scrolledModel := scrolled(t, model, testCase.keystrokes...)
			if got := topRow(t, scrolledModel); got != testCase.wantTop {
				t.Errorf("top row is event %d, want %d", got, testCase.wantTop)
			}
			rows := bodyLines(t, scrolledModel.View().Content)
			if len(rows) != scrollBodyHeight {
				t.Errorf("stream rendered %d rows, want the full body of %d", len(rows), scrollBodyHeight)
			}
		})
	}
}

func TestStreamClampsTheWindowAfterRefreshShrinkage(t *testing.T) {
	model, _ := apply(t, streamModel(t, numberedEvents(t, scrollEvents)),
		tea.WindowSizeMsg{Width: wideWidth, Height: scrollTerminalHeight})
	model = scrolled(t, model, "pgdown", "pgdown", "pgdown")

	shrunk := numberedEvents(t, 3)
	model, _ = apply(t, model, refreshedMsg{result: Result{
		Repositories: []domain.RepositoryActivity{
			testActivity(t, "acme/backend", shrunk...),
			testActivity(t, "acme/frontend"),
		},
	}})

	rows := bodyLines(t, model.View().Content)
	if len(rows) != len(shrunk) {
		t.Fatalf("stream rendered %d rows after shrinkage, want %d:\n%v", len(rows), len(shrunk), rows)
	}
	if got := topRow(t, model); got != 1 {
		t.Errorf("top row after shrinkage is event %d, want the newest event", got)
	}
	if got := topRow(t, scrolled(t, model, "down")); got != 1 {
		t.Errorf("top row after scrolling a shrunken snapshot is event %d, want the newest event", got)
	}
}

func TestStreamClampsTheWindowAfterResize(t *testing.T) {
	model, _ := apply(t, streamModel(t, numberedEvents(t, scrollEvents)),
		tea.WindowSizeMsg{Width: wideWidth, Height: scrollTerminalHeight})
	model = scrolled(t, model, "pgdown", "pgdown", "pgdown")

	grown, _ := apply(t, model, tea.WindowSizeMsg{Width: wideWidth, Height: scrollEvents + chromeLines})
	if got := topRow(t, grown); got != 1 {
		t.Errorf("top row after growing the terminal is event %d, want the newest event", got)
	}
	if rows := bodyLines(t, grown.View().Content); len(rows) != scrollEvents {
		t.Errorf("grown stream rendered %d rows, want all %d events", len(rows), scrollEvents)
	}
}

func TestStreamKeepsItsPositionAcrossViewSwitches(t *testing.T) {
	model, _ := apply(t, streamModel(t, numberedEvents(t, scrollEvents)),
		tea.WindowSizeMsg{Width: wideWidth, Height: scrollTerminalHeight})
	model = scrolled(t, model, "down", "down", "down")
	want := topRow(t, model)

	switched := scrolled(t, model, "1", "tab")
	if switched.mode != ModeStream {
		t.Fatalf("mode after switching back is %v, want ModeStream", switched.mode)
	}
	if got := topRow(t, switched); got != want {
		t.Errorf("top row after a view switch is event %d, want %d", got, want)
	}
	if got := len(switched.state.Snapshot.Events()); got != scrollEvents {
		t.Errorf("snapshot after a view switch holds %d events, want %d", got, scrollEvents)
	}
}

func TestStreamRendersEachFreshnessState(t *testing.T) {
	cases := []struct {
		name       string
		freshness  Freshness
		cause      string
		populated  bool
		wantBody   string
		wantNoRows bool
	}{
		{name: "loading", freshness: FreshnessLoading, wantBody: "Loading recent events", wantNoRows: true},
		{name: "first error", freshness: FreshnessError, cause: "refreshing acme/backend: request failed", wantBody: "Recent events are unavailable", wantNoRows: true},
		{name: "empty success", freshness: FreshnessCurrent, wantBody: noRecentActivity, wantNoRows: true},
		{name: "stale with data", freshness: FreshnessStale, cause: "refreshing acme/backend: request failed", populated: true, wantBody: "pushed 2 commits to main"},
		{name: "current", freshness: FreshnessCurrent, populated: true, wantBody: "pushed 2 commits to main"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := streamModel(t, nil)
			if testCase.populated {
				model = streamModel(t, detailedEvents(t))
			}
			model.state.Freshness = testCase.freshness
			model.state.Cause = testCase.cause

			content := renderAt(t, model, wideWidth, wideHeight)
			if !strings.Contains(content, testCase.wantBody) {
				t.Errorf("%s stream does not contain %q:\n%s", testCase.name, testCase.wantBody, content)
			}
			if testCase.wantNoRows && strings.Contains(content, "pushed 2 commits") {
				t.Errorf("%s stream renders event rows without a populated snapshot:\n%s", testCase.name, content)
			}
			if marker := testCase.freshness.Marker(); marker != "" && !strings.Contains(content, marker) {
				t.Errorf("%s stream loses the %q marker:\n%s", testCase.name, marker, content)
			}
			if !strings.Contains(content, "q quit") {
				t.Errorf("%s stream loses the shared footer:\n%s", testCase.name, content)
			}
		})
	}
}

func TestStreamRetainsEventContextAtNarrowSizes(t *testing.T) {
	content := renderAt(t, streamModel(t, detailedEvents(t)), narrowWidth, narrowHeight)

	assertFits(t, content, narrowWidth, narrowHeight)
	rows := bodyLines(t, content)
	if len(rows) == 0 {
		t.Fatalf("narrow stream rendered no event row:\n%s", content)
	}
	for _, want := range []string{youngestAge, "acme/backend", "push"} {
		if !strings.Contains(rows[0], want) {
			t.Errorf("narrow row %q does not retain %q", rows[0], want)
		}
	}
	for _, want := range []string{ModeStream.Label(), transportLabel, "q quit"} {
		if !strings.Contains(content, want) {
			t.Errorf("narrow stream loses the shared chrome %q:\n%s", want, content)
		}
	}
}

func TestStreamNarrowerPositiveSizesDoNotPanic(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{
		{Width: 1, Height: 1}, {Width: 4, Height: 3}, {Width: 12, Height: 5},
		{Width: 20, Height: narrowHeight}, {Width: 30, Height: 4}, {Width: narrowWidth, Height: 3},
	} {
		model := streamModel(t, detailedEvents(t))
		sized, _ := apply(t, model, size)
		assertFits(t, sized.View().Content, size.Width, size.Height)
		assertFits(t, scrolled(t, sized, "down", "pgdown", "pgup", "up").View().Content, size.Width, size.Height)
	}
}

func TestStreamAdvertisesNoDeferredCapability(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 120, Height: 30}, {Width: narrowWidth, Height: narrowHeight}} {
		content := strings.ToLower(renderAt(t, streamModel(t, detailedEvents(t)), size.Width, size.Height))
		for _, forbidden := range forbiddenStreamVocabulary {
			if strings.Contains(content, forbidden) {
				t.Errorf("stream at %dx%d contains the deferred token %q:\n%s", size.Width, size.Height, forbidden, content)
			}
		}
	}
}

func TestStreamHasNoPlaceholderBody(t *testing.T) {
	content := renderAt(t, streamModel(t, detailedEvents(t)), wideWidth, wideHeight)
	if strings.Contains(content, "not rendered yet") {
		t.Errorf("stream still renders the placeholder body:\n%s", content)
	}
}

func TestStreamRendersAnEventWithoutActorOrDescription(t *testing.T) {
	bare := streamEvent(t, "1", "acme/backend", streamBase, domain.CategoryPush, domain.EntityCommit, "", "")
	rows := bodyLines(t, renderAt(t, streamModel(t, []domain.Event{bare}), wideWidth, wideHeight))

	if len(rows) != 1 {
		t.Fatalf("stream rendered %d rows, want the detail-free event to keep its row:\n%v", len(rows), rows)
	}
	for _, want := range []string{youngestAge, "acme/backend", "push"} {
		if !strings.Contains(rows[0], want) {
			t.Errorf("detail-free row %q does not retain %q", rows[0], want)
		}
	}
	if trimmed := strings.TrimRight(rows[0], " "); rows[0] != trimmed {
		t.Errorf("detail-free row %q keeps the padding of an empty detail field, want %q", rows[0], trimmed)
	}
}

func TestStreamRendersPopulatedRowsWithinReportedZeroDimensions(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{
		{Width: 0, Height: narrowHeight}, {Width: 0, Height: 0}, {Width: narrowWidth, Height: 0},
	} {
		sized, _ := apply(t, streamModel(t, detailedEvents(t)), size)
		assertFits(t, sized.View().Content, size.Width, size.Height)
		assertFits(t, scrolled(t, sized, "down", "pgdown", "up", "pgup").View().Content, size.Width, size.Height)
	}
}

func TestStreamMeasuresWideRunesByTheirRenderedWidth(t *testing.T) {
	// Actors and descriptions are free-form GitHub text, so a row must be
	// measured and truncated by rendered cells rather than by bytes.
	events := []domain.Event{
		streamEvent(t, "1", "acme/backend", streamBase, domain.CategoryPush, domain.EntityCommit, "李雷", "推送了三个提交到主分支"),
		streamEvent(t, "2", "acme/frontend", streamBase.Add(-time.Minute), domain.CategoryReview, domain.EntityPullRequest, "韩梅梅", "批准了拉取请求 #7"),
	}

	for _, size := range []tea.WindowSizeMsg{
		{Width: narrowWidth, Height: narrowHeight}, {Width: wideWidth, Height: wideHeight},
	} {
		content := renderAt(t, streamModel(t, events), size.Width, size.Height)
		assertFits(t, content, size.Width, size.Height)
		rows := bodyLines(t, content)
		if len(rows) != len(events) {
			t.Fatalf("stream at width %d rendered %d rows, want %d:\n%v", size.Width, len(rows), len(events), rows)
		}
		for index, row := range rows {
			if !utf8.ValidString(row) {
				t.Errorf("row %d at width %d is not valid UTF-8: %q", index, size.Width, row)
			}
		}
	}
}

// ageUnits orders the age units from youngest to oldest, so two rendered ages
// can be compared without reconstructing the duration behind them.
var ageUnits = []string{"m", "h", "d", "w", "y"}

// renderedAge returns the age column of a rendered row, which is its first
// whitespace-separated field.
func renderedAge(t *testing.T, row string) string {
	t.Helper()
	fields := strings.Fields(row)
	if len(fields) == 0 {
		t.Fatalf("row %q has no age column", row)
	}
	return fields[0]
}

// ageOrder ranks a rendered age so ages can be ordered across units. It fails
// the test on anything that is not one whole unit, which is what a row spelling
// a wall-clock time would be.
func ageOrder(t *testing.T, age string) (int, int) {
	t.Helper()
	if age == youngestAge {
		return 0, 0
	}
	unit := slices.Index(ageUnits, age[len(age)-1:])
	if unit < 0 {
		t.Fatalf("age %q names no age unit of %v", age, ageUnits)
	}
	count, err := strconv.Atoi(age[:len(age)-1])
	if err != nil {
		t.Fatalf("age %q carries no whole unit count: %v", age, err)
	}
	return unit + 1, count
}

// TestStreamRendersAgesThatNeverDecreaseDownTheSnapshot guards FR-010: the
// visible form of the reverse-chronological order is that the age column only
// ever grows downward. A wall-clock stamp satisfies the ordering internally
// while reading as broken, which is the defect this replaces.
func TestStreamRendersAgesThatNeverDecreaseDownTheSnapshot(t *testing.T) {
	elapsed := []time.Duration{30 * time.Second, 25 * time.Minute, 3 * time.Hour, 26 * time.Hour, 4 * day, 23 * day}
	want := []string{youngestAge, "25m", "3h", "1d", "4d", "3w"}

	rows := bodyLines(t, renderAt(t, streamModel(t, agedEvents(t, elapsed...)), wideWidth, wideHeight))
	if len(rows) != len(want) {
		t.Fatalf("stream rendered %d rows, want %d:\n%v", len(rows), len(want), rows)
	}

	previousUnit, previousCount := 0, 0
	for index, row := range rows {
		age := renderedAge(t, row)
		if age != want[index] {
			t.Errorf("row %d spells its age %q, want %q:\n%s", index, age, want[index], row)
		}
		unit, count := ageOrder(t, age)
		if unit < previousUnit || (unit == previousUnit && count < previousCount) {
			t.Errorf("row %d is younger than the row above it: %q after %d unit %d", index, age, previousCount, previousUnit)
		}
		previousUnit, previousCount = unit, count
	}
}

// TestStreamRightAlignsTheAgeColumn guards FR-010: the ages share one fixed
// column so the ordering reads straight down it and the columns behind it stay
// aligned. Left-aligned ages of different widths would stagger every later
// column of the row.
func TestStreamRightAlignsTheAgeColumn(t *testing.T) {
	rows := bodyLines(t, renderAt(t,
		streamModel(t, agedEvents(t, 30*time.Second, 5*time.Minute, 12*time.Hour, 23*day)),
		wideWidth, wideHeight))

	width := lipgloss.Width(youngestAge)
	for index, row := range rows {
		column := string([]rune(row)[:width])
		if strings.TrimLeft(column, " ") != renderedAge(t, row) {
			t.Errorf("row %d starts with %q, want the age right-aligned in %d columns:\n%s", index, column, width, row)
		}
	}
}

// TestStreamRendersNoWallClockTimeOfDay guards FR-010 at every width: no row
// may fall back to a time of day, which a snapshot reaching back weeks cannot
// place in time.
func TestStreamRendersNoWallClockTimeOfDay(t *testing.T) {
	clock := regexp.MustCompile(`\d{1,2}:\d{2}`)
	events := agedEvents(t, 30*time.Second, 25*time.Minute, 3*time.Hour, 26*time.Hour, 23*day)

	for _, width := range []int{narrowWidth, 60, wideWidth} {
		for _, row := range bodyLines(t, renderAt(t, streamModel(t, events), width, wideHeight)) {
			if clock.MatchString(row) {
				t.Errorf("row %q at width %d spells a wall-clock time of day", row, width)
			}
		}
	}
}

// TestStreamAnchorsAgesToTheLastSuccessRatherThanTheCurrentClock guards FR-010:
// the ages must agree with the last-success time the same header reports.
// Measuring against time.Now instead would drift between redraws and disagree
// with the header, since the polling floor is a full minute.
func TestStreamAnchorsAgesToTheLastSuccessRatherThanTheCurrentClock(t *testing.T) {
	model := streamModel(t, agedEvents(t, 5*time.Minute))
	model.state.LastSuccess = streamBase

	rows := bodyLines(t, renderAt(t, model, wideWidth, wideHeight))
	if len(rows) != 1 {
		t.Fatalf("stream rendered %d rows, want 1:\n%v", len(rows), rows)
	}
	if age := renderedAge(t, rows[0]); age != "5m" {
		t.Errorf("row %q is aged %q, want %q measured from the last success rather than the current clock", rows[0], age, "5m")
	}
}

// TestStaleStreamKeepsTheAgesItHadWhileCurrent guards FR-008 and FR-010
// together: a stale snapshot stops being refreshed, so its ages must freeze with
// it rather than aging past data nobody is updating.
func TestStaleStreamKeepsTheAgesItHadWhileCurrent(t *testing.T) {
	model := streamModel(t, agedEvents(t, 30*time.Second, 25*time.Minute, 3*day))
	current := bodyLines(t, renderAt(t, model, wideWidth, wideHeight))

	model.state.Freshness = FreshnessStale
	model.state.Cause = "status 500"
	stale := bodyLines(t, renderAt(t, model, wideWidth, wideHeight))

	if len(stale) != len(current) {
		t.Fatalf("the stale stream rendered %d rows, want the %d it had while current:\n%v", len(stale), len(current), stale)
	}
	for index := range current {
		if want, got := renderedAge(t, current[index]), renderedAge(t, stale[index]); got != want {
			t.Errorf("stale row %d is aged %q, want the frozen %q", index, got, want)
		}
	}
}

// TestStreamClampsAnEventAheadOfTheLastSuccess guards FR-010: a source clock
// running ahead of the anchor must render the youngest age rather than a
// negative or empty one.
func TestStreamClampsAnEventAheadOfTheLastSuccess(t *testing.T) {
	rows := bodyLines(t, renderAt(t, streamModel(t, agedEvents(t, -time.Hour)), wideWidth, wideHeight))
	if len(rows) != 1 {
		t.Fatalf("stream rendered %d rows, want 1:\n%v", len(rows), rows)
	}
	if age := renderedAge(t, rows[0]); age != youngestAge {
		t.Errorf("row %q ahead of the last success is aged %q, want %q", rows[0], age, youngestAge)
	}
}
