package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// streamMutScopeColumn returns the Scope context column of a rendered Stream
// row: the field between the category column and the row's detail. The columns
// are joined by the shared row gap, so the context ends where the first gap
// after the category does.
func streamMutScopeColumn(t *testing.T, row string) string {
	t.Helper()
	_, after, found := strings.Cut(row, "push"+rowGap)
	if !found {
		t.Fatalf("row %q carries no category column", row)
	}
	column, _, _ := strings.Cut(after, rowGap)
	return strings.TrimRight(column, " ")
}

// streamMutHeadings returns the column heading line of a Stream render, which
// is the body line naming the age column.
func streamMutHeadings(t *testing.T, content string) string {
	t.Helper()
	for _, line := range bodyLines(t, content) {
		if strings.HasPrefix(strings.TrimSpace(line), "age") {
			return line
		}
	}
	t.Fatalf("stream rendered no column headings:\n%s", content)
	return ""
}

// streamMutOverlappingScopes returns a selection of six path Scopes of one
// repository and one event matching every one of them, which is more Scope
// context than a constrained row can spell out.
func streamMutOverlappingScopes(t *testing.T, repository string) (domain.ScopeSet, []domain.EventEvidence) {
	t.Helper()
	segments := []string{"alpha", "bravo", "delta", "echo", "fox", "golf"}
	scopes := make([]domain.Scope, 0, len(segments))
	paths := make([]string, 0, len(segments))
	for _, segment := range segments {
		scopes = append(scopes, pathScope(t, repository, segment))
		paths = append(paths, segment+"/main.go")
	}
	retained := []domain.EventEvidence{
		scopedEvidence(t, "wide", repository, time.Minute, completeEvidence(t, paths...)),
	}
	return scopeSet(t, scopes...), retained
}

// TestStreamDropsItsCoverageDisclosureBeforeItsHeadings guards FR-011 and
// A-010: a content area too short for Stream's own chrome keeps one event row
// under the headings that name its columns, and gives up the coverage
// disclosure to do it, rather than spending the area on chrome and overflowing
// the terminal with rows it cannot show.
func TestStreamDropsItsCoverageDisclosureBeforeItsHeadings(t *testing.T) {
	const height = 4

	content := renderAt(t, streamModel(t, detailedEvents(t)), wideWidth, height)

	assertFits(t, content, wideWidth, height)
	lines := bodyLines(t, content)
	if len(lines) != 2 {
		t.Fatalf("a two-line content area rendered %d body lines, want the headings and one row:\n%s", len(lines), content)
	}
	if got := strings.TrimSpace(lines[0]); !strings.HasPrefix(got, "age") {
		t.Errorf("the first body line is %q, want the column headings", got)
	}
	if want := "pushed 2 commits to main"; !strings.Contains(lines[1], want) {
		t.Errorf("the second body line is %q, want the newest event row %q", lines[1], want)
	}
	for _, line := range lines {
		if strings.Contains(line, "showing") {
			t.Errorf("body line %q keeps the coverage disclosure instead of an event row", line)
		}
	}
}

// TestStreamKeepsReadableDetailWhenChoosingItsLayout guards FR-011: a layout
// fits a width only when its aligned columns and the start of its free-form
// detail both fit, so a width with room for the rich columns alone renders the
// sparser layout rather than columns with no readable detail behind them.
func TestStreamKeepsReadableDetailWhenChoosingItsLayout(t *testing.T) {
	const (
		sparse = 61
		rich   = 62
	)
	model := streamModel(t, detailedEvents(t))

	sparseHeadings := streamMutHeadings(t, renderAt(t, model, sparse, wideHeight))
	richHeadings := streamMutHeadings(t, renderAt(t, model, rich, wideHeight))

	for _, spelling := range []string{"repository", "category", "description"} {
		if strings.Contains(sparseHeadings, spelling) {
			t.Errorf("the %d-cell headings %q keep the rich spelling %q with no room for the detail behind it", sparse, sparseHeadings, spelling)
		}
	}
	for _, spelling := range []string{"repo", "type", "detail"} {
		if !strings.Contains(sparseHeadings, spelling) {
			t.Errorf("the %d-cell headings %q lose the sparse spelling %q", sparse, sparseHeadings, spelling)
		}
	}
	for _, spelling := range []string{"repository", "category", "description"} {
		if !strings.Contains(richHeadings, spelling) {
			t.Errorf("the %d-cell headings %q lose the rich spelling %q the width has room for", rich, richHeadings, spelling)
		}
	}
}

// TestStreamBoundsScopeContextToWhatTheWidthHasLeft guards FR-011 and RG-012: a
// constrained row spends only the cells its aligned columns and the gaps
// between them leave on Scope context, so the context degrades to a bounded
// marked form that names what it left out instead of spelling out membership
// the row cannot hold.
func TestStreamBoundsScopeContextToWhatTheWidthHasLeft(t *testing.T) {
	const width = 40
	api := "acme/api"
	scopes, retained := streamMutOverlappingScopes(t, api)

	content := renderAt(t, scopedStreamModel(t, scopes, retained), width, wideHeight)

	assertFits(t, content, width, wideHeight)
	rows := eventRows(t, content)
	if len(rows) != 1 {
		t.Fatalf("stream rendered %d rows for one event, want 1:\n%v", len(rows), rows)
	}
	if got, want := streamMutScopeColumn(t, rows[0]), "P1, P2, P3, +3"; got != want {
		t.Errorf("the %d-cell row context is %q, want %q", width, got, want)
	}
}

// TestStreamSpendsNoScopeBudgetWithoutAReportedWidth guards FR-011: the Scope
// context column is bounded only by a width the terminal actually reported. A
// non-positive width bounds nothing, so the complete context is prepared rather
// than a column budgeted from a width that does not exist.
func TestStreamSpendsNoScopeBudgetWithoutAReportedWidth(t *testing.T) {
	for _, width := range []int{unbounded, 0} {
		if got := scopeBudget(width, 30); got != unbounded {
			t.Errorf("a width of %d budgets %d cells of Scope context, want the unbounded %d", width, got, unbounded)
		}
	}
}

// streamMutRepositoryEvidence returns one retained push per repository, each
// occurring the given number of minutes before the strip fixture base, with the
// repository and index in its source event ID.
func streamMutRepositoryEvidence(t *testing.T, repositories []string, events int) []domain.EventEvidence {
	t.Helper()
	retained := make([]domain.EventEvidence, 0, len(repositories)*events)
	for index := range events {
		for _, repository := range repositories {
			retained = append(retained, stripEvidence(t,
				fmt.Sprintf("%s-%02d", repository, index),
				repository,
				time.Duration(index)*time.Minute,
			))
		}
	}
	return retained
}

// streamMutSponsor returns the sponsoring Scope identifier of the strip's
// first stored entry, which is the Scope whose turn the selection began at.
func streamMutSponsor(t *testing.T, strip interesting) string {
	t.Helper()
	if len(strip.entries) == 0 {
		t.Fatal("the strip stored no entries to read a sponsor from")
	}
	return strip.entries[0].sponsor.String()
}

// TestInterestingAdvancesItsReferenceOnEveryRefresh guards RG-007: a refresh
// carrying a newer reference advances the monotonic strip reference, so stored
// entries age against the refresh that published them and one that left the
// fixed 15-minute window expires.
func TestInterestingAdvancesItsReferenceOnEveryRefresh(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository))
	snapshot := stripSnapshot(scopes,
		stripEvidence(t, "fresh", repository, 0),
		stripEvidence(t, "ageing", repository, 14*time.Minute),
	)
	strip := startedStrip(scopes, snapshot)

	if got, want := stripIDs(strip), []string{"fresh", "ageing"}; !slices.Equal(got, want) {
		t.Fatalf("the first refresh stored %v, want %v", got, want)
	}

	strip = strip.reconciled(scopes, snapshot, stripAt(2*time.Minute))

	if got, want := stripIDs(strip), []string{"fresh"}; !slices.Equal(got, want) {
		t.Fatalf("the refreshed strip stored %v, want %v", got, want)
	}
	if got, want := strip.counts.eligible, 1; got != want {
		t.Errorf("the refreshed strip counted %d eligible events, want %d", got, want)
	}
	if got, want := strip.entries[0].ageText(), "2m"; got != want {
		t.Errorf("the refreshed entry renders age %q, want %q", got, want)
	}
}

// TestInterestingKeepsTheSurvivingAnchorsOwnTurn guards RG-007: a Scope set
// change remaps the saved rotation anchor onto its new position, so an anchor
// that survives at the head of the new selection takes its own turn instead of
// being read as removed and skipped.
func TestInterestingKeepsTheSurvivingAnchorsOwnTurn(t *testing.T) {
	scopes, retained := stripRotationFixture(t)
	strip := startedStrip(scopes, stripSnapshot(scopes, retained...))

	web, zeta := repositoryScope(t, "acme/web"), repositoryScope(t, "acme/zeta")
	next := scopeSet(t, web, zeta)
	later := streamMutRepositoryEvidence(t, []string{"acme/web", "acme/zeta"}, 10)
	strip = strip.reconciled(next, stripSnapshot(next, later...), stripBase)

	if got, want := streamMutSponsor(t, strip), web.String(); got != want {
		t.Errorf("the remapped rotation began at %q, want the retained anchor %q", got, want)
	}
}

// TestInterestingRotatesPastEveryTurnThatFillsTheStrip guards RG-007: every
// refresh that reaches the 20-entry bound saves the Scope after the turn that
// reached it as the next refresh's start, including the very first turn of the
// round, so no Scope keeps the head of the rotation.
func TestInterestingRotatesPastEveryTurnThatFillsTheStrip(t *testing.T) {
	scopes, retained := stripRotationFixture(t)
	snapshot := stripSnapshot(scopes, retained...)

	strip := startedStrip(scopes, snapshot)
	sponsors := []string{streamMutSponsor(t, strip)}
	for range 2 {
		strip = strip.reconciled(scopes, snapshot, stripBase)
		sponsors = append(sponsors, streamMutSponsor(t, strip))
	}

	want := []string{"acme/api", "acme/web", "acme/ops"}
	if !slices.Equal(sponsors, want) {
		t.Errorf("three filling refreshes began at %v, want %v", sponsors, want)
	}
}

// TestInterestingKeepsTheEventsAfterARepeatedIdentity guards RG-007: the source
// event ID is the strip identity and a repeated one stores a single entry, but
// the events behind it are still candidates rather than lost with the repeat.
func TestInterestingKeepsTheEventsAfterARepeatedIdentity(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository))
	strip := startedStrip(scopes, stripSnapshot(scopes,
		stripEvidence(t, "repeated", repository, 0),
		stripEvidence(t, "repeated", repository, time.Minute),
		stripEvidence(t, "later", repository, 2*time.Minute),
	))

	if got, want := stripIDs(strip), []string{"repeated", "later"}; !slices.Equal(got, want) {
		t.Fatalf("a repeated identity stored %v, want %v", got, want)
	}
	if got, want := strip.counts.eligible, 2; got != want {
		t.Errorf("the strip counted %d eligible events, want %d", got, want)
	}
}

// TestInterestingKeepsMembershipBehindANotMemberScope guards RG-007: an event
// is eligible through any confirmed membership, so a Scope that decided
// not-member never ends the search for the confirmed one behind it.
func TestInterestingKeepsMembershipBehindANotMemberScope(t *testing.T) {
	repository := "acme/api"
	docs, source := pathScope(t, repository, "docs"), pathScope(t, repository, "src")
	scopes := scopeSet(t, docs, source)
	event := testEvent(t, "matched", repository, domain.CategoryPush, domain.EntityCommit)
	event.OccurredAt = stripBase.Add(-time.Minute)
	evidence := domain.EventEvidence{Event: event, Outcome: completeEvidence(t, "src/main.go")}

	strip := startedStrip(scopes, stripSnapshot(scopes, evidence))

	if got, want := stripIDs(strip), []string{"matched"}; !slices.Equal(got, want) {
		t.Fatalf("the strip stored %v, want %v", got, want)
	}
	entry := strip.entries[0]
	if got, want := len(entry.scopes), 1; got != want {
		t.Fatalf("the entry retained %d matching Scopes, want %d", got, want)
	}
	if got, want := entry.scopes[0].scope.Identity(), source.Identity(); got != want {
		t.Errorf("the entry retained %v as its matching Scope, want %v", got, want)
	}
	if got, want := entry.sponsor.Identity(), source.Identity(); got != want {
		t.Errorf("the entry is sponsored by %v, want %v", got, want)
	}
}

// TestInterestingStoresNothingWithoutAnActiveScope guards RG-007: selection is
// Scope-fair, so a selection with no active Scope has no turn to take and
// stores nothing at all, whatever the retained snapshot still carries.
func TestInterestingStoresNothingWithoutAnActiveScope(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository))
	snapshot := stripSnapshot(scopes, stripEvidence(t, "fresh", repository, time.Minute))

	strip := newInteresting().reconciled(domain.ScopeSet{}, snapshot, stripBase)

	if got := len(strip.entries); got != 0 {
		t.Errorf("an empty selection stored %d entries, want 0", got)
	}
	if got := len(strip.visible()); got != 0 {
		t.Errorf("an empty selection made %d entries visible, want 0", got)
	}
	if got, want := strip.counts.stored, 0; got != want {
		t.Errorf("an empty selection reported %d stored, want %d", got, want)
	}
}

// TestInterestingKeepsItsNextStartWhenTheQueuesExhaust guards RG-007: a refresh
// that exhausts every queue before storing 20 entries leaves the saved next
// start unchanged, so the following refresh begins at the same Scope and the
// stored order repeats.
func TestInterestingKeepsItsNextStartWhenTheQueuesExhaust(t *testing.T) {
	repositories := []string{"acme/api", "acme/ops", "acme/web"}
	scopes := scopeSet(t,
		repositoryScope(t, repositories[0]),
		repositoryScope(t, repositories[1]),
		repositoryScope(t, repositories[2]),
	)
	snapshot := stripSnapshot(scopes, streamMutRepositoryEvidence(t, repositories, 1)...)

	strip := startedStrip(scopes, snapshot)
	want := stripIDs(strip)
	if len(want) != len(repositories) {
		t.Fatalf("the exhausting refresh stored %v, want one entry per Scope", want)
	}

	strip = strip.reconciled(scopes, snapshot, stripBase)

	if got := stripIDs(strip); !slices.Equal(got, want) {
		t.Errorf("the following refresh stored %v, want the unchanged rotation %v", got, want)
	}
	if got, want := streamMutSponsor(t, strip), repositories[0]; got != want {
		t.Errorf("the following refresh began at %q, want the unchanged %q", got, want)
	}
}
