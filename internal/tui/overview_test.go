package tui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// A terminal wide and tall enough for the richest row layout.
const (
	wideWidth  = 100
	wideHeight = 24
)

// forbiddenOverviewVocabulary lists the interpretive language FR-009 excludes.
// Overview shows direct counts and never derives meaning from them.
var forbiddenOverviewVocabulary = []string{
	"%", "percent", "baseline", "anomal", "normal", "productiv",
	"signal", "trend", "score", "velocity", "average", "rate",
}

// testEvent builds a normalized event for the repository in a category.
func testEvent(t *testing.T, id, repository string, category domain.Category, kind domain.EntityKind) domain.Event {
	t.Helper()
	parsed, err := domain.ParseRepository(repository)
	if err != nil {
		t.Fatalf("parsing the test repository %q failed: %v", repository, err)
	}
	return domain.Event{
		ID:         id,
		OccurredAt: time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC),
		Repository: parsed,
		Category:   category,
		EntityKind: kind,
	}
}

// testActivity builds the successful repository result for the events.
func testActivity(t *testing.T, repository string, events ...domain.Event) domain.RepositoryActivity {
	t.Helper()
	parsed, err := domain.ParseRepository(repository)
	if err != nil {
		t.Fatalf("parsing the test repository %q failed: %v", repository, err)
	}
	return domain.RepositoryActivity{Repository: parsed, Events: events}
}

// populatedModel returns a current model whose snapshot gives acme/backend two
// pushes and one pull request, acme/frontend one push, and acme/docs nothing.
func populatedModel(t *testing.T) Model {
	t.Helper()
	scope := testScope(t, "acme/backend", "acme/frontend", "acme/docs")
	snapshot := domain.NewSnapshot(scope, []domain.RepositoryActivity{
		testActivity(t, "acme/backend",
			testEvent(t, "1", "acme/backend", domain.CategoryPush, domain.EntityCommit),
			testEvent(t, "2", "acme/backend", domain.CategoryPush, domain.EntityCommit),
			testEvent(t, "3", "acme/backend", domain.CategoryPullRequest, domain.EntityPullRequest),
		),
		testActivity(t, "acme/frontend",
			testEvent(t, "4", "acme/frontend", domain.CategoryPush, domain.EntityCommit),
		),
		testActivity(t, "acme/docs"),
	})

	model := newModel(t, "acme/backend", "acme/frontend", "acme/docs")
	model.state.Snapshot = snapshot
	model.state.Freshness = FreshnessCurrent
	return model
}

// renderAt sizes the model to the terminal and returns what it renders.
func renderAt(t *testing.T, model Model, width, height int) string {
	t.Helper()
	sized, _ := apply(t, model, tea.WindowSizeMsg{Width: width, Height: height})
	return sized.View().Content
}

// countsOf returns the numbers of a rendered row in their rendered order.
func countsOf(row string) []string {
	numbers := make([]string, 0, 3)
	for _, field := range strings.Fields(strings.ReplaceAll(row, separator, " ")) {
		if _, err := strconv.Atoi(field); err == nil {
			numbers = append(numbers, field)
		}
	}
	return numbers
}

// bodyLines returns the non-empty body lines of a render.
func bodyLines(t *testing.T, content string) []string {
	t.Helper()
	lines := strings.Split(content, "\n")
	if len(lines) < 3 {
		t.Fatalf("render has no body:\n%s", content)
	}
	body := make([]string, 0, len(lines)-2)
	for _, line := range lines[1 : len(lines)-1] {
		if strings.TrimSpace(line) != "" {
			body = append(body, line)
		}
	}
	return body
}

func TestOverviewRendersOneOrderedRowPerRepository(t *testing.T) {
	rows := bodyLines(t, renderAt(t, populatedModel(t), wideWidth, wideHeight))

	if len(rows) != 3 {
		t.Fatalf("overview rendered %d rows, want one per selected repository:\n%v", len(rows), rows)
	}
	wantIdentities := []string{"acme/backend", "acme/frontend", "acme/docs"}
	for index, want := range wantIdentities {
		if !strings.Contains(rows[index], want) {
			t.Errorf("row %d is %q, want the identity %q", index, rows[index], want)
		}
	}
	for index, wantCounts := range [][]string{{"3", "1", "2"}, {"1", "0", "1"}, {"0", "0", "0"}} {
		numbers := countsOf(rows[index])
		if strings.Join(numbers, ",") != strings.Join(wantCounts, ",") {
			t.Errorf("row %d has counts %v, want total, pull-request activity, pushes %v: %q", index, numbers, wantCounts, rows[index])
		}
	}
}

func TestOverviewLabelsEveryCountedDimension(t *testing.T) {
	content := strings.ToLower(renderAt(t, populatedModel(t), wideWidth, wideHeight))

	for _, want := range []string{"event", "pull request", "push"} {
		if !strings.Contains(content, want) {
			t.Errorf("wide overview does not label %q:\n%s", want, content)
		}
	}
}

func TestOverviewSuccessWithoutEventsStatesNoRecentActivity(t *testing.T) {
	scope := testScope(t, "acme/backend", "acme/frontend")
	model := newModel(t, "acme/backend", "acme/frontend")
	model.state.Snapshot = domain.NewSnapshot(scope, []domain.RepositoryActivity{
		testActivity(t, "acme/backend"),
		testActivity(t, "acme/frontend"),
	})
	model.state.Freshness = FreshnessCurrent

	content := renderAt(t, model, wideWidth, wideHeight)
	if !strings.Contains(strings.ToLower(content), "no recent activity") {
		t.Errorf("all-zero overview does not state that there is no recent activity:\n%s", content)
	}
	for _, want := range []string{"acme/backend", "acme/frontend"} {
		if !strings.Contains(content, want) {
			t.Errorf("all-zero overview drops the selected repository %q:\n%s", want, content)
		}
	}
}

func TestOverviewRendersEachFreshnessState(t *testing.T) {
	cases := []struct {
		name       string
		freshness  Freshness
		cause      string
		populated  bool
		wantBody   string
		wantNoRows bool
	}{
		{name: "loading", freshness: FreshnessLoading, wantBody: "Loading repository activity", wantNoRows: true},
		{name: "first error", freshness: FreshnessError, cause: "refreshing acme/backend: request failed", wantBody: "Repository activity is unavailable", wantNoRows: true},
		{name: "stale with data", freshness: FreshnessStale, cause: "refreshing acme/backend: request failed", populated: true, wantBody: "acme/backend"},
		{name: "current", freshness: FreshnessCurrent, populated: true, wantBody: "acme/backend"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := newModel(t, "acme/backend", "acme/frontend", "acme/docs")
			if testCase.populated {
				model = populatedModel(t)
			}
			model.state.Freshness = testCase.freshness
			model.state.Cause = testCase.cause

			content := renderAt(t, model, wideWidth, wideHeight)
			if !strings.Contains(content, testCase.wantBody) {
				t.Errorf("%s overview does not contain %q:\n%s", testCase.name, testCase.wantBody, content)
			}
			if testCase.wantNoRows && strings.Contains(content, "pushes") {
				t.Errorf("%s overview renders aggregate rows without a snapshot:\n%s", testCase.name, content)
			}
			if marker := testCase.freshness.Marker(); marker != "" && !strings.Contains(content, marker) {
				t.Errorf("%s overview loses the %q marker:\n%s", testCase.name, marker, content)
			}
			if !strings.Contains(content, "q quit") {
				t.Errorf("%s overview loses the shared footer:\n%s", testCase.name, content)
			}
		})
	}
}

func TestOverviewRetainsIdentityAndCountsAtNarrowSizes(t *testing.T) {
	content := renderAt(t, populatedModel(t), narrowWidth, narrowHeight)

	assertFits(t, content, narrowWidth, narrowHeight)
	rows := bodyLines(t, content)
	if len(rows) != 3 {
		t.Fatalf("narrow overview rendered %d rows, want 3:\n%v", len(rows), rows)
	}
	for index, want := range []string{"acme/backend", "acme/frontend", "acme/docs"} {
		if !strings.Contains(rows[index], want) {
			t.Errorf("narrow row %d is %q, want the identity %q", index, rows[index], want)
		}
	}
	// The sparsest layout still names all three counted dimensions, so a narrow
	// row stays readable instead of collapsing into a bare run of numbers.
	for _, want := range []string{"3 ev", "1 pr", "2 push"} {
		if !strings.Contains(rows[0], want) {
			t.Errorf("narrow row for acme/backend drops the labeled count %q: %q", want, rows[0])
		}
	}
	for index, row := range rows {
		for _, label := range []string{"ev", "pr", "push"} {
			if !strings.Contains(row, label) {
				t.Errorf("narrow row %d does not label %q: %q", index, label, row)
			}
		}
	}
	if !strings.Contains(content, transportLabel) || !strings.Contains(content, "q quit") {
		t.Errorf("narrow overview loses the shared chrome:\n%s", content)
	}
}

func TestOverviewNarrowerPositiveSizesDoNotPanic(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{
		{Width: 1, Height: 1}, {Width: 4, Height: 3}, {Width: 12, Height: 5},
		{Width: 20, Height: narrowHeight}, {Width: 30, Height: 4}, {Width: narrowWidth, Height: 3},
	} {
		content := renderAt(t, populatedModel(t), size.Width, size.Height)
		assertFits(t, content, size.Width, size.Height)
	}
}

func TestOverviewShowsNoInterpretiveMeasures(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 120, Height: 30}, {Width: narrowWidth, Height: narrowHeight}} {
		content := strings.ToLower(renderAt(t, populatedModel(t), size.Width, size.Height))
		for _, forbidden := range forbiddenOverviewVocabulary {
			if strings.Contains(content, forbidden) {
				t.Errorf("overview at %dx%d contains the interpretive token %q:\n%s", size.Width, size.Height, forbidden, content)
			}
		}
	}
}

func TestOverviewHasNoPlaceholderBody(t *testing.T) {
	if content := renderAt(t, populatedModel(t), wideWidth, wideHeight); strings.Contains(content, "not rendered yet") {
		t.Errorf("overview still renders the placeholder body:\n%s", content)
	}
}

// TestSharedRowHelpersMeasureWideRunesByTheirRenderedWidth guards the padding,
// alignment, and truncation helpers the Overview and Stream rows share. The
// Overview's own row inputs are ASCII by construction — domain.ParseRepository
// rejects anything else and the count labels are fixed — so a wide rune cannot
// reach them through a rendered row. The helpers are covered directly instead,
// which is the only level at which byte-wise measurement is observable.
func TestSharedRowHelpersMeasureWideRunesByTheirRenderedWidth(t *testing.T) {
	// Four wide runes: eight rendered cells, twelve bytes.
	const wide = "推送提交"

	t.Run("padRight pads to the rendered width", func(t *testing.T) {
		const width = 12
		padded := padRight(wide, width)
		if rendered := lipgloss.Width(padded); rendered != width {
			t.Errorf("padRight(%q, %d) is %d cells wide, want %d: %q", wide, width, rendered, width, padded)
		}
	})

	t.Run("widestWidth measures the widest rendered line", func(t *testing.T) {
		lines := []string{wide, "abcdefghij"}
		if got := widestWidth(lines); got != 10 {
			t.Errorf("widestWidth(%q) = %d, want 10", lines, got)
		}
	})

	t.Run("shorten marks a cut on a cell boundary", func(t *testing.T) {
		const limit = 5
		shortened := shorten(wide, limit)
		if shortened != "推送"+shortenedMark {
			t.Errorf("shorten(%q, %d) = %q, want %q", wide, limit, shortened, "推送"+shortenedMark)
		}
		if rendered := lipgloss.Width(shortened); rendered > limit {
			t.Errorf("shorten(%q, %d) is %d cells wide, want at most %d", wide, limit, rendered, limit)
		}
	})

	t.Run("shorten leaves content that fits unmarked", func(t *testing.T) {
		if shortened := shorten(wide, 8); shortened != wide {
			t.Errorf("shorten(%q, 8) = %q, want it unmarked", wide, shortened)
		}
	})

	t.Run("shorten yields the width entirely when the mark does not fit", func(t *testing.T) {
		if shortened := shorten(wide, 0); shortened != "" {
			t.Errorf("shorten(%q, 0) = %q, want the empty string", wide, shortened)
		}
	})

	t.Run("truncate cuts on a cell boundary", func(t *testing.T) {
		const limit = 5
		truncated := truncate(wide, limit)
		if truncated != "推送" {
			t.Errorf("truncate(%q, %d) = %q, want %q", wide, limit, truncated, "推送")
		}
		if rendered := lipgloss.Width(truncated); rendered > limit {
			t.Errorf("truncate(%q, %d) is %d cells wide, want at most %d", wide, limit, rendered, limit)
		}
	})
}

// The Overview scrolling tests window 20 repository rows through the same body
// the Stream scrolling tests use, so every keystroke has a window above it and
// one below it.
const scrollRepositories = 20

// repositoryNames returns count canonical repository identities whose
// case-insensitive name order is their one-based position.
func repositoryNames(count int) []string {
	names := make([]string, 0, count)
	for index := range count {
		names = append(names, fmt.Sprintf("acme/repo-%02d", index+1))
	}
	return names
}

// numberedRepositories returns one activity per repository, each carrying the
// same single push, so every row holds an equal total and the rows keep the
// deterministic name order the aggregates impose.
func numberedRepositories(t *testing.T, names []string) []domain.RepositoryActivity {
	t.Helper()
	activities := make([]domain.RepositoryActivity, 0, len(names))
	for index, name := range names {
		activities = append(activities, testActivity(t, name,
			testEvent(t, strconv.Itoa(index), name, domain.CategoryPush, domain.EntityCommit)))
	}
	return activities
}

// scrollOverviewModel returns a current model whose scope holds count numbered
// repositories, each with one push.
func scrollOverviewModel(t *testing.T, count int) Model {
	t.Helper()
	names := repositoryNames(count)
	model := newModel(t, names...)
	model.state.Snapshot = domain.NewSnapshot(testScope(t, names...), numberedRepositories(t, names))
	model.state.Freshness = FreshnessCurrent
	return model
}

// overviewRowAt returns the one-based numbered repository the body line names.
func overviewRowAt(t *testing.T, model Model, index int) int {
	t.Helper()
	rows := bodyLines(t, model.View().Content)
	if index >= len(rows) {
		t.Fatalf("overview rendered %d body rows, want at least %d:\n%v", len(rows), index+1, rows)
	}
	for position := 1; position <= scrollRepositories; position++ {
		if strings.Contains(rows[index], fmt.Sprintf("repo-%02d", position)) {
			return position
		}
	}
	t.Fatalf("body row %d %q names no numbered repository", index, rows[index])
	return 0
}

// topOverviewRow returns the one-based numbered repository the first body line
// names.
func topOverviewRow(t *testing.T, model Model) int {
	t.Helper()
	return overviewRowAt(t, model, 0)
}

func TestOverviewScrollKeysWindowTheRowsWithinBounds(t *testing.T) {
	model, _ := apply(t, scrollOverviewModel(t, scrollRepositories),
		tea.WindowSizeMsg{Width: wideWidth, Height: scrollTerminalHeight})
	lastTop := scrollRepositories - scrollBodyHeight + 1

	cases := []struct {
		name       string
		keystrokes []string
		wantTop    int
	}{
		{name: "initial", wantTop: 1},
		{name: "down", keystrokes: []string{"down"}, wantTop: 2},
		{name: "up clamps at the first repository", keystrokes: []string{"down", "up", "up"}, wantTop: 1},
		{name: "page down", keystrokes: []string{"pgdown"}, wantTop: 1 + scrollBodyHeight},
		{name: "page up returns", keystrokes: []string{"pgdown", "pgup"}, wantTop: 1},
		{name: "page down clamps at the last window", keystrokes: []string{"pgdown", "pgdown", "pgdown", "pgdown"}, wantTop: lastTop},
		{name: "down clamps at the last window", keystrokes: []string{"pgdown", "pgdown", "pgdown", "down", "down"}, wantTop: lastTop},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			scrolledModel := scrolled(t, model, testCase.keystrokes...)
			if got := topOverviewRow(t, scrolledModel); got != testCase.wantTop {
				t.Errorf("top row is repository %d, want %d", got, testCase.wantTop)
			}
			rows := bodyLines(t, scrolledModel.View().Content)
			if len(rows) != scrollBodyHeight {
				t.Errorf("overview rendered %d rows, want the full body of %d", len(rows), scrollBodyHeight)
			}
		})
	}
}

func TestOverviewClampsTheWindowAfterRefreshShrinkage(t *testing.T) {
	names := repositoryNames(scrollRepositories)
	model := newModel(t, names...)
	// An all-zero success prepends the explicit no-recent-activity line, so the
	// body holds one line more than the rows a later success renders.
	empty := make([]domain.RepositoryActivity, 0, len(names))
	for _, name := range names {
		empty = append(empty, testActivity(t, name))
	}
	model.state.Snapshot = domain.NewSnapshot(testScope(t, names...), empty)
	model.state.Freshness = FreshnessCurrent

	model, _ = apply(t, model, tea.WindowSizeMsg{Width: wideWidth, Height: scrollTerminalHeight})
	model = scrolled(t, model, "pgdown", "pgdown", "pgdown", "pgdown")

	model, _ = apply(t, model, refreshedMsg{result: Result{Repositories: numberedRepositories(t, names)}})

	rows := bodyLines(t, model.View().Content)
	if len(rows) != scrollBodyHeight {
		t.Fatalf("overview rendered %d rows after shrinkage, want the full body of %d:\n%v", len(rows), scrollBodyHeight, rows)
	}
	if got := overviewRowAt(t, model, len(rows)-1); got != scrollRepositories {
		t.Errorf("last row after shrinkage is repository %d, want %d", got, scrollRepositories)
	}
	if got := topOverviewRow(t, scrolled(t, model, "down")); got != scrollRepositories-scrollBodyHeight+1 {
		t.Errorf("top row after scrolling a shrunken snapshot is repository %d, want the last window", got)
	}
}

func TestOverviewClampsTheWindowAfterResize(t *testing.T) {
	model, _ := apply(t, scrollOverviewModel(t, scrollRepositories),
		tea.WindowSizeMsg{Width: wideWidth, Height: scrollTerminalHeight})
	model = scrolled(t, model, "pgdown", "pgdown", "pgdown")

	grown, _ := apply(t, model, tea.WindowSizeMsg{Width: wideWidth, Height: scrollRepositories + chromeLines})
	if got := topOverviewRow(t, grown); got != 1 {
		t.Errorf("top row after growing the terminal is repository %d, want the first repository", got)
	}
	if rows := bodyLines(t, grown.View().Content); len(rows) != scrollRepositories {
		t.Errorf("grown overview rendered %d rows, want all %d repositories", len(rows), scrollRepositories)
	}
}

func TestOverviewKeepsItsPositionAcrossViewSwitches(t *testing.T) {
	model, _ := apply(t, scrollOverviewModel(t, scrollRepositories),
		tea.WindowSizeMsg{Width: wideWidth, Height: scrollTerminalHeight})
	model = scrolled(t, model, "down", "down", "down")
	want := topOverviewRow(t, model)

	switched := scrolled(t, model, "2", "tab")
	if switched.mode != ModeOverview {
		t.Fatalf("mode after switching back is %v, want ModeOverview", switched.mode)
	}
	if got := topOverviewRow(t, switched); got != want {
		t.Errorf("top row after a view switch is repository %d, want %d", got, want)
	}
}

func TestScrolledOverviewRendersWithinNarrowBounds(t *testing.T) {
	model, _ := apply(t, scrollOverviewModel(t, scrollRepositories),
		tea.WindowSizeMsg{Width: narrowWidth, Height: narrowHeight})
	content := scrolled(t, model, "down", "pgdown", "pgup", "up").View().Content

	assertFits(t, content, narrowWidth, narrowHeight)
	for _, want := range []string{ModeOverview.Label(), transportLabel, "acme/repo-01", "q quit"} {
		if !strings.Contains(content, want) {
			t.Errorf("scrolled %dx%d overview does not contain %q:\n%s", narrowWidth, narrowHeight, want, content)
		}
	}

	for _, size := range []tea.WindowSizeMsg{
		{Width: 1, Height: 1}, {Width: 4, Height: 3}, {Width: 12, Height: 5},
		{Width: 20, Height: narrowHeight}, {Width: 30, Height: 4}, {Width: narrowWidth, Height: 3},
	} {
		sized, _ := apply(t, scrollOverviewModel(t, scrollRepositories), size)
		assertFits(t, scrolled(t, sized, "down", "pgdown", "up", "pgup").View().Content, size.Width, size.Height)
	}
}

// TestOverviewMarksRowsTheWidthCannotHold guards FR-009 against the shared
// shortening helper: Overview marks a cut row like Stream does, and its identity
// and counts stay readable ahead of the mark.
func TestOverviewMarksRowsTheWidthCannotHold(t *testing.T) {
	const width = 20

	content := renderAt(t, populatedModel(t), width, narrowHeight)
	assertFits(t, content, width, narrowHeight)

	rows := bodyLines(t, content)
	if len(rows) != 3 {
		t.Fatalf("overview rendered %d rows at width %d, want 3:\n%s", len(rows), width, content)
	}
	for index, row := range rows {
		if !strings.HasSuffix(row, shortenedMark) {
			t.Errorf("shortened row %d is %q, want it marked with %q", index, row, shortenedMark)
		}
		if rendered := lipgloss.Width(row); rendered > width {
			t.Errorf("marked row %d is %d cells wide, want at most %d: %q", index, rendered, width, row)
		}
	}
	if !strings.Contains(rows[0], "acme/backend") {
		t.Errorf("the marked row %q loses the repository identity", rows[0])
	}
}
