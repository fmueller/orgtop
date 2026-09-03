package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// scopedStreamModel returns a current Stream model publishing the retained
// evidence against the selection, exactly as a successful refresh does.
func scopedStreamModel(t *testing.T, scopes domain.ScopeSet, retained []domain.EventEvidence) Model {
	t.Helper()
	model := scopedModel(t, scopes, retained)
	model.mode = ModeStream
	model.state.LastSuccess = streamBase
	return model
}

// scopedEvidence builds one retained push of the repository, settled at the
// outcome and occurring the given time before the last success.
func scopedEvidence(t *testing.T, id, repository string, since time.Duration, outcome domain.EvidenceOutcome) domain.EventEvidence {
	t.Helper()
	event := testEvent(t, id, repository, domain.CategoryPush, domain.EntityCommit)
	event.OccurredAt = streamBase.Add(-since)
	event.Description = "pushed " + id
	return domain.EventEvidence{Event: event, Outcome: outcome}
}

// streamContextOf returns the Scope context the row carrying the description
// renders, which is the row text between its category and its detail.
func streamContextOf(t *testing.T, rows []string, description string) string {
	t.Helper()
	for _, row := range rows {
		if !strings.Contains(row, description) {
			continue
		}
		_, after, found := strings.Cut(row, "push"+rowGap)
		if !found {
			t.Fatalf("row %q carries no category column", row)
		}
		context, _, _ := strings.Cut(strings.TrimLeft(after, " "), description)
		return strings.TrimSpace(context)
	}
	t.Fatalf("no rendered row carries %q:\n%v", description, rows)
	return ""
}

// TestStreamKeepsOneRowPerEventMatchingSeveralScopes guards FR-007 and RG-012: a
// multi-Scope event keeps one source-event row that names every matching Scope,
// so it is neither duplicated per Scope nor stripped of its membership context.
func TestStreamKeepsOneRowPerEventMatchingSeveralScopes(t *testing.T) {
	api := "acme/api"
	scopes := scopeSet(t,
		domain.NewRepositoryScope(testRepository(t, api)),
		pathScope(t, api, "docs"),
		pathScope(t, api, "src"),
	)
	retained := []domain.EventEvidence{
		scopedEvidence(t, "both", api, time.Minute, completeEvidence(t, "src/main.go", "docs/readme.md")),
	}

	rows := eventRows(t, renderAt(t, scopedStreamModel(t, scopes, retained), wideWidth, wideHeight))

	if len(rows) != 1 {
		t.Fatalf("stream rendered %d rows for one multi-Scope event, want 1:\n%v", len(rows), rows)
	}
	if got, want := streamContextOf(t, rows, "pushed both"), "in R1, P2, P3"; got != want {
		t.Errorf("the multi-Scope row context is %q, want %q", got, want)
	}
}

// TestStreamSeparatesUnknownScopeContextFromMembership guards FR-004 and RG-012:
// an unknown Scope is investigatory context marked as unresolved, never folded
// into the confirmed members of the same row.
func TestStreamSeparatesUnknownScopeContextFromMembership(t *testing.T) {
	api := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, api)), pathScope(t, api, "src"))
	retained := []domain.EventEvidence{
		scopedEvidence(t, "undecided", api, time.Minute, domain.FailedOutcome("upstream failed")),
	}

	rows := eventRows(t, renderAt(t, scopedStreamModel(t, scopes, retained), wideWidth, wideHeight))

	if got, want := streamContextOf(t, rows, "pushed undecided"), "in R1 · unresolved ?P2"; got != want {
		t.Errorf("the undecided row context is %q, want %q", got, want)
	}
}

// TestStreamQualifiesCurrentPRMembership guards A-039: a member decided from
// current-PR evidence stays visibly qualified in the row context.
func TestStreamQualifiesCurrentPRMembership(t *testing.T) {
	api := "acme/api"
	scopes := scopeSet(t, pathScope(t, api, "src"))
	current := domain.CompleteOutcome(domain.ProvenanceCurrentPR, mustChangedPaths(t, "src/main.go"))
	retained := []domain.EventEvidence{scopedEvidence(t, "qualified", api, time.Minute, current)}

	rows := eventRows(t, renderAt(t, scopedStreamModel(t, scopes, retained), wideWidth, wideHeight))

	if got, want := streamContextOf(t, rows, "pushed qualified"), "in P1~"; got != want {
		t.Errorf("the current-PR row context is %q, want %q", got, want)
	}
}

// TestStreamOmitsEventsEveryScopeDecidedNotMember guards RG-004: an event no
// Scope confirmed and no Scope left undecided carries no investigatory value and
// keeps no row, while the retained order of the remaining events is unchanged.
func TestStreamOmitsEventsEveryScopeDecidedNotMember(t *testing.T) {
	api := "acme/api"
	scopes := scopeSet(t, pathScope(t, api, "src"))
	retained := []domain.EventEvidence{
		scopedEvidence(t, "newest", api, time.Minute, completeEvidence(t, "src/main.go")),
		scopedEvidence(t, "elsewhere", api, 2*time.Minute, completeEvidence(t, "docs/readme.md")),
		scopedEvidence(t, "oldest", api, 3*time.Minute, completeEvidence(t, "src/other.go")),
	}

	content := renderAt(t, scopedStreamModel(t, scopes, retained), wideWidth, wideHeight)
	rows := eventRows(t, content)

	if coverage := coverageLine(t, content); !strings.Contains(coverage, "2 events") {
		t.Errorf("coverage disclosure %q counts events no Scope confirmed or left undecided", coverage)
	}
	if len(rows) != 2 {
		t.Fatalf("stream rendered %d rows, want the two events the Scope confirmed:\n%v", len(rows), rows)
	}
	if !strings.Contains(rows[0], "pushed newest") || !strings.Contains(rows[1], "pushed oldest") {
		t.Errorf("stream lost its reverse-chronological order:\n%v", rows)
	}
	for _, row := range rows {
		if strings.Contains(row, "pushed elsewhere") {
			t.Errorf("row %q shows an event every Scope decided not-member", row)
		}
	}
}

// TestStreamKeepsScopeContextAtTheMinimumTerminal guards A-081: at 40x10 the
// event row survives with a bounded, marked Scope context and no line outruns
// the width.
func TestStreamKeepsScopeContextAtTheMinimumTerminal(t *testing.T) {
	api := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, api)), pathScope(t, api, "src"))
	retained := []domain.EventEvidence{
		scopedEvidence(t, "narrow", api, time.Minute, completeEvidence(t, "src/main.go")),
	}

	content := renderAt(t, scopedStreamModel(t, scopes, retained), narrowWidth, narrowHeight)

	assertFits(t, content, narrowWidth, narrowHeight)
	rows := eventRows(t, content)
	if len(rows) == 0 {
		t.Fatalf("the 40x10 stream rendered no event row:\n%s", content)
	}
	for index, row := range rows {
		if rendered := lipgloss.Width(row); rendered > narrowWidth {
			t.Errorf("row %d is %d cells wide, want at most %d: %q", index, rendered, narrowWidth, row)
		}
	}
	if want := "in R1, P2"; !strings.Contains(rows[0], want) {
		t.Errorf("the 40x10 row %q loses its membership context %q", rows[0], want)
	}
}
