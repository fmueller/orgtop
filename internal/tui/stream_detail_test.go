package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// detailScopes is the selection every detail fixture renders against: the
// repository Scope every event is a member of, and one path Scope whose outcome
// the fixture chooses.
func detailScopes(t *testing.T, repository, segment string) domain.ScopeSet {
	t.Helper()
	return scopeSet(t,
		domain.NewRepositoryScope(testRepository(t, repository)),
		pathScope(t, repository, segment),
	)
}

// detailEvidence returns one retained event of the repository carrying the
// optional actor and description detail bounded detail must report.
func detailEvidence(t *testing.T, id, repository, actor, description string, since time.Duration, outcome domain.EvidenceOutcome) domain.EventEvidence {
	t.Helper()
	event := testEvent(t, id, repository, domain.CategoryPullRequest, domain.EntityPullRequest)
	event.OccurredAt = streamBase.Add(-since)
	event.Actor = actor
	event.EntityRef = "#7"
	event.Description = description
	return domain.EventEvidence{Event: event, Outcome: outcome}
}

// opened returns the model with bounded detail opened on the focused event.
func opened(t *testing.T, model Model, width, height int) Model {
	t.Helper()
	sized, _ := apply(t, model, tea.WindowSizeMsg{Width: width, Height: height})
	entered, _ := apply(t, sized, press("enter"))
	return entered
}

// detailRows returns the rendered detail body lines of a Stream render.
func detailRows(t *testing.T, content string) []string {
	t.Helper()
	return bodyLines(t, content)
}

// rowWithPrefix returns the single rendered line that starts with the field
// name, so a detail assertion names the fact rather than a body index.
func rowWithPrefix(t *testing.T, rows []string, prefix string) string {
	t.Helper()
	for _, row := range rows {
		if strings.HasPrefix(strings.TrimSpace(row), prefix) {
			return strings.TrimSpace(row)
		}
	}
	t.Fatalf("no rendered detail line starts with %q:\n%v", prefix, rows)
	return ""
}

// TestStreamDetailOpensOnEnterAndClosesOnEsc guards A-077: `enter` on the
// focused event opens bounded detail carrying the prepared facts, and `esc`
// returns to the unchanged focused event and Stream viewport.
func TestStreamDetailOpensOnEnterAndClosesOnEsc(t *testing.T) {
	api := "acme/api"
	scopes := detailScopes(t, api, "src")
	retained := []domain.EventEvidence{
		detailEvidence(t, "opened", api, "alice", "opened #7", time.Minute, completeEvidence(t, "src/main.go")),
	}
	model := scopedStreamModel(t, scopes, retained)

	detail := opened(t, model, wideWidth, wideHeight)
	rows := detailRows(t, detail.View().Content)

	for prefix, want := range map[string]string{
		"Event:":       "Event: opened",
		"Time:":        "1m",
		"Repository:":  "Repository: acme/api",
		"Category:":    "Category: pull request",
		"Actor:":       "Actor: alice",
		"Entity:":      "#7",
		"Description:": "Description: opened #7",
		"Member:":      "acme/api",
	} {
		if got := rowWithPrefix(t, rows, prefix); !strings.Contains(got, want) {
			t.Errorf("the %q detail line is %q, want it to carry %q", prefix, got, want)
		}
	}
	if got := rowWithPrefix(t, rows, "Time:"); !strings.Contains(got, streamBase.Add(-time.Minute).Format(time.RFC3339)) {
		t.Errorf("the timestamp line %q drops the prepared timestamp", got)
	}

	closed, _ := apply(t, detail, press("esc"))
	if got, want := closed.stream.focus, detail.stream.focus; got != want {
		t.Errorf("esc moved the focused event from %d to %d", want, got)
	}
	if got, want := closed.stream.offset, detail.stream.offset; got != want {
		t.Errorf("esc moved the Stream viewport from %d to %d", want, got)
	}
	if rows := eventRows(t, closed.View().Content); len(rows) != 1 {
		t.Fatalf("esc returned %d event rows, want the unchanged Stream list:\n%v", len(rows), rows)
	}
}

// TestStreamDetailReportsEveryUnknownScopeReason guards RG-004 and A-077: an
// undecided Scope is reported as unresolved with the prepared reason group,
// never folded into the confirmed members.
func TestStreamDetailReportsEveryUnknownScopeReason(t *testing.T) {
	api := "acme/api"
	scopes := detailScopes(t, api, "src")
	retained := []domain.EventEvidence{
		detailEvidence(t, "undecided", api, "bob", "opened #7", time.Minute, domain.FailedOutcome("upstream failed")),
	}

	rows := detailRows(t, opened(t, scopedStreamModel(t, scopes, retained), wideWidth, wideHeight).View().Content)

	unknown := rowWithPrefix(t, rows, "Unknown:")
	if want := "P2 acme/api:src/**" + separator + "failed"; unknown != "Unknown: "+want {
		t.Errorf("the unknown Scope line is %q, want %q", unknown, "Unknown: "+want)
	}
	if member := rowWithPrefix(t, rows, "Member:"); strings.Contains(member, "src/**") {
		t.Errorf("the undecided path Scope %q is reported as a member", member)
	}
}

// TestStreamDetailQualifiesCurrentPRMembership guards A-039 and A-077: a member
// decided from current-PR evidence stays qualified in the full detail label,
// and a member decided from event-time evidence carries no qualification.
func TestStreamDetailQualifiesCurrentPRMembership(t *testing.T) {
	api := "acme/api"
	scopes := detailScopes(t, api, "src")
	current := domain.CompleteOutcome(domain.ProvenanceCurrentPR, mustChangedPaths(t, "src/main.go"))
	retained := []domain.EventEvidence{
		detailEvidence(t, "qualified", api, "carol", "opened #7", time.Minute, current),
	}

	rows := detailRows(t, opened(t, scopedStreamModel(t, scopes, retained), wideWidth, wideHeight).View().Content)

	members := make([]string, 0, 2)
	for _, row := range rows {
		if strings.HasPrefix(strings.TrimSpace(row), "Member:") {
			members = append(members, strings.TrimSpace(row))
		}
	}
	if len(members) != 2 {
		t.Fatalf("detail reported %d member Scopes, want the repository and the path Scope:\n%v", len(members), rows)
	}
	if want := "Member: R1 acme/api"; members[0] != want {
		t.Errorf("the repository member line is %q, want the unqualified %q", members[0], want)
	}
	if want := "Member: P2 acme/api:src/**" + separator + "current PR"; members[1] != want {
		t.Errorf("the path member line is %q, want %q", members[1], want)
	}
}

// TestStreamDetailWrapsEveryLogicalLineWithoutDroppingText guards A-077: each
// logical line greedily wraps into whole-grapheme chunks of at most the width,
// and every prepared cell stays reachable rather than becoming an ellipsis.
func TestStreamDetailWrapsEveryLogicalLineWithoutDroppingText(t *testing.T) {
	api := "acme/api"
	scopes := detailScopes(t, api, "src")
	description := strings.Repeat("wrapped detail text ", 6)
	retained := []domain.EventEvidence{
		detailEvidence(t, "wrapped", api, "dave", description, time.Minute, completeEvidence(t, "src/main.go")),
	}

	const width = 28
	content := opened(t, scopedStreamModel(t, scopes, retained), width, wideHeight).View().Content
	rows := detailRows(t, content)

	for index, row := range rows {
		if rendered := lipgloss.Width(row); rendered > width {
			t.Errorf("detail line %d is %d cells wide, want at most %d: %q", index, rendered, width, row)
		}
		if strings.Contains(row, shortenedMark) {
			t.Errorf("detail line %d was shortened rather than wrapped: %q", index, row)
		}
	}
	if joined := strings.Join(rows, ""); !strings.Contains(joined, "Description: "+strings.TrimSpace(description)) {
		t.Errorf("the wrapped detail lost description text:\n%v", rows)
	}
}

// TestStreamDetailReachesEveryWrappedLine guards A-077: vertical scrolling, not
// hidden text, bounds the presentation, so a detail taller than the body is
// reachable line by line.
func TestStreamDetailReachesEveryWrappedLine(t *testing.T) {
	api := "acme/api"
	scopes := detailScopes(t, api, "src")
	description := strings.Repeat("scrollable detail text ", 8)
	retained := []domain.EventEvidence{
		detailEvidence(t, "scrolled", api, "erin", description, time.Minute, completeEvidence(t, "src/main.go")),
	}

	const width, height = 30, 8
	model := opened(t, scopedStreamModel(t, scopes, retained), width, height)
	seen := map[string]bool{}
	for range 12 {
		for _, row := range detailRows(t, model.View().Content) {
			seen[row] = true
		}
		model = scrolled(t, model, "down")
	}

	for index, chunk := range wrapDetailLine("Description: "+description, width) {
		if !seen[chunk] {
			t.Errorf("scrolling never revealed wrapped description line %d: %q", index, chunk)
		}
	}
	if !seen["Member: P2 acme/api:src/**"] {
		t.Errorf("scrolling to the bottom never reached the Scope lines, saw %d distinct lines", len(seen))
	}
}

// TestStreamDetailFollowsARetainedEventThroughRefresh guards A-077: a refresh
// that retains the source event replaces its prepared facts atomically and moves
// Stream focus to the event's new index.
func TestStreamDetailFollowsARetainedEventThroughRefresh(t *testing.T) {
	api := "acme/api"
	scopes := detailScopes(t, api, "src")
	followed := detailEvidence(t, "followed", api, "alice", "opened #7", time.Minute, completeEvidence(t, "src/main.go"))
	model := opened(t, scopedStreamModel(t, scopes, []domain.EventEvidence{followed}), wideWidth, wideHeight)

	// The refresh publishes a newer event ahead of the followed one and a fresh
	// description for it, so both the index and the prepared facts change.
	newer := detailEvidence(t, "newer", api, "bob", "opened #9", 0, completeEvidence(t, "src/other.go"))
	followed.Event.Description = "merged #7"
	refreshed, _ := apply(t, model, refreshedMsg{
		polled:   true,
		evidence: evidenceResult{retained: []domain.EventEvidence{newer, followed}},
	})

	if !refreshed.stream.detail.open {
		t.Fatal("a refresh that retained the event closed its detail")
	}
	rows := detailRows(t, refreshed.View().Content)
	if got := rowWithPrefix(t, rows, "Description:"); got != "Description: merged #7" {
		t.Errorf("the refreshed detail description is %q, want the republished %q", got, "Description: merged #7")
	}
	if got := rowWithPrefix(t, rows, "Event:"); got != "Event: followed" {
		t.Errorf("the refreshed detail follows %q, want the stable source event ID", got)
	}
	if got, want := refreshed.stream.focus, 1; got != want {
		t.Errorf("Stream focus after the refresh is %d, want the event's new index %d", got, want)
	}
}

// TestStreamDetailClosesWhenARefreshRemovesTheEvent guards A-077: a refresh that
// drops the followed source event closes detail and leaves Stream on its
// preserved, clamped numeric focus.
func TestStreamDetailClosesWhenARefreshRemovesTheEvent(t *testing.T) {
	api := "acme/api"
	scopes := detailScopes(t, api, "src")
	retained := []domain.EventEvidence{
		detailEvidence(t, "kept", api, "alice", "opened #7", time.Minute, completeEvidence(t, "src/main.go")),
		detailEvidence(t, "removed", api, "bob", "opened #9", 2*time.Minute, completeEvidence(t, "src/other.go")),
	}
	model := opened(t, scopedStreamModel(t, scopes, retained), wideWidth, wideHeight)
	model = scrolled(t, model, "esc")
	model = scrolled(t, model, "down")
	model, _ = apply(t, model, press("enter"))
	if got := model.stream.detail.eventID; got != "removed" {
		t.Fatalf("the fixture opened detail on %q, want the second event", got)
	}

	refreshed, _ := apply(t, model, refreshedMsg{
		polled:   true,
		evidence: evidenceResult{retained: retained[:1]},
	})

	if refreshed.stream.detail.open {
		t.Error("a refresh that removed the followed event left its detail open")
	}
	if got, want := refreshed.stream.focus, 0; got != want {
		t.Errorf("Stream focus after the removal is %d, want the clamped %d", got, want)
	}
	if rows := eventRows(t, refreshed.View().Content); len(rows) != 1 {
		t.Fatalf("Stream rendered %d rows after the removal, want 1:\n%v", len(rows), rows)
	}
}

// TestStreamDetailKeepsTheMinimumTerminalReadable guards A-081: at 40x10 detail
// keeps the active view, the transport and freshness chrome, at least one detail
// line, and the mandatory quit hint.
func TestStreamDetailKeepsTheMinimumTerminalReadable(t *testing.T) {
	api := "acme/api"
	scopes := detailScopes(t, api, "src")
	retained := []domain.EventEvidence{
		detailEvidence(t, "narrow", api, "alice", strings.Repeat("narrow detail ", 8), time.Minute, completeEvidence(t, "src/main.go")),
	}

	content := opened(t, scopedStreamModel(t, scopes, retained), narrowWidth, narrowHeight).View().Content

	assertFits(t, content, narrowWidth, narrowHeight)
	lines := strings.Split(content, "\n")
	header, footer := lines[0], lines[len(lines)-1]
	for _, want := range []string{ModeStream.Label(), transportLabel} {
		if !strings.Contains(header, want) {
			t.Errorf("the 40x10 detail header %q drops %q", header, want)
		}
	}
	if !strings.Contains(footer, quitHint) {
		t.Errorf("the 40x10 detail footer %q drops the quit hint", footer)
	}
	if rows := detailRows(t, content); len(rows) == 0 {
		t.Fatalf("the 40x10 detail rendered no detail line:\n%s", content)
	}
}

// TestStreamDetailReportsItsHiddenLines guards A-078: the active-view header
// reports the visible detail-line range whenever the body cannot hold them all.
func TestStreamDetailReportsItsHiddenLines(t *testing.T) {
	api := "acme/api"
	scopes := detailScopes(t, api, "src")
	retained := []domain.EventEvidence{
		detailEvidence(t, "counted", api, "alice", strings.Repeat("counted detail ", 8), time.Minute, completeEvidence(t, "src/main.go")),
	}

	header := strings.Split(opened(t, scopedStreamModel(t, scopes, retained), wideWidth, 8).View().Content, "\n")[0]

	if !strings.Contains(header, "lines 1-") {
		t.Errorf("the detail header %q reports no visible line range", header)
	}
}

// TestWrapDetailLineKeepsWholeGraphemes guards A-077's wrapping contract
// directly: chunks never exceed the width, never drop text, and the one-cell
// exception replaces a wide grapheme rather than splitting it.
func TestWrapDetailLineKeepsWholeGraphemes(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		width int
		want  []string
	}{
		{name: "unbounded", line: "abcdef", width: unbounded, want: []string{"abcdef"}},
		{name: "zero width preserves the line", line: "abcdef", width: 0, want: []string{""}},
		{name: "exact multiple", line: "abcdef", width: 3, want: []string{"abc", "def"}},
		{name: "remainder", line: "abcde", width: 2, want: []string{"ab", "cd", "e"}},
		{name: "wide grapheme is never split", line: "a世b", width: 2, want: []string{"a", "世", "b"}},
		{name: "one cell replaces a wide grapheme", line: "a世b", width: 1, want: []string{"a", "?", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapDetailLine(tt.line, tt.width)
			if len(got) != len(tt.want) {
				t.Fatalf("wrapDetailLine(%q, %d) = %v, want %v", tt.line, tt.width, got, tt.want)
			}
			for index := range got {
				if got[index] != tt.want[index] {
					t.Errorf("wrapDetailLine(%q, %d)[%d] = %q, want %q", tt.line, tt.width, index, got[index], tt.want[index])
				}
			}
		})
	}
}

// TestStreamDetailPerformsNoSourceWork guards A-077: opening, scrolling, and
// closing detail run entirely off the prepared snapshot, so no refresh is
// dispatched and the published state is untouched.
func TestStreamDetailPerformsNoSourceWork(t *testing.T) {
	api := "acme/api"
	scopes := detailScopes(t, api, "src")
	retained := []domain.EventEvidence{
		detailEvidence(t, "quiet", api, "alice", "opened #7", time.Minute, completeEvidence(t, "src/main.go")),
	}
	model := opened(t, scopedStreamModel(t, scopes, retained), wideWidth, wideHeight)

	for _, keystroke := range []string{"down", "pgdown", "up", "esc"} {
		next, cmd := apply(t, model, press(keystroke))
		if cmd != nil {
			t.Errorf("the detail keystroke %q dispatched a command", keystroke)
		}
		model = next
	}
	if got, want := len(model.state.Scoped.StreamEvents()), len(retained); got != want {
		t.Errorf("detail navigation republished %d events, want the untouched %d", got, want)
	}
}

// TestStreamAdvertisesItsDetailControls guards FR-011: the footer advertises
// only the controls the active view implements, so Stream names the `enter`
// that opens bounded detail and the open detail names the `esc` that returns
// from it, rather than leaving either as an undocumented keystroke.
func TestStreamAdvertisesItsDetailControls(t *testing.T) {
	api := "acme/api"
	scopes := detailScopes(t, api, "src")
	retained := []domain.EventEvidence{
		detailEvidence(t, "advertised", api, "alice", "opened #7", time.Minute, completeEvidence(t, "src/main.go")),
	}
	model := scopedStreamModel(t, scopes, retained)

	list := footerOf(t, renderAt(t, model, wideWidth, wideHeight))
	if !strings.Contains(list, "enter") {
		t.Errorf("the Stream footer %q advertises no detail control", list)
	}
	if strings.Contains(list, "esc") {
		t.Errorf("the Stream footer %q advertises a control the closed list has no use for", list)
	}

	detail := footerOf(t, opened(t, model, wideWidth, wideHeight).View().Content)
	if !strings.Contains(detail, "esc") {
		t.Errorf("the detail footer %q advertises no way back to the event list", detail)
	}
	for _, footer := range []string{list, detail} {
		if !strings.Contains(footer, quitHint) {
			t.Errorf("the footer %q drops the mandatory quit hint", footer)
		}
	}
}

// footerOf returns the shared footer line of a render.
func footerOf(t *testing.T, content string) string {
	t.Helper()
	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		t.Fatalf("render has no footer:\n%s", content)
	}
	return lines[len(lines)-1]
}

// TestStreamDetailIgnoresEscFromAnotherView guards FR-007: a keystroke never
// acts on a view the operator is not looking at, which is the same rule Rain's
// own controls follow. `esc` in Overview or Rain must therefore leave Stream's
// open detail alone, so switching away and back returns to the detail the
// operator left rather than to the event list.
func TestStreamDetailIgnoresEscFromAnotherView(t *testing.T) {
	api := "acme/api"
	scopes := detailScopes(t, api, "src")
	retained := []domain.EventEvidence{
		detailEvidence(t, "elsewhere", api, "alice", "opened #7", time.Minute, completeEvidence(t, "src/main.go")),
	}
	model := opened(t, scopedStreamModel(t, scopes, retained), wideWidth, wideHeight)

	for _, view := range []string{"1", "3"} {
		switched, _ := apply(t, model, press(view), press("esc"), press("2"))
		if !switched.stream.detail.open {
			t.Errorf("esc pressed in view %q closed Stream's detail", view)
		}
		if got := rowWithPrefix(t, detailRows(t, switched.View().Content), "Event:"); got != "Event: elsewhere" {
			t.Errorf("returning to Stream rendered %q, want the detail the operator left", got)
		}
	}
}
