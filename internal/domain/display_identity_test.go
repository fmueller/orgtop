package domain_test

import (
	"reflect"
	"testing"

	"github.com/fmueller/orgtop/internal/domain"
)

// TestScopeRowLabelKeepsTheRequestedSpellingOverReturnedSpellings guards the
// v0.2 supersession of the v0.1 FR-002 display-name rule: a published Scope row
// is labelled from the Scope's own retained requested spelling under RG-012, so
// a differently spelled returned repository identity never relabels the row.
func TestScopeRowLabelKeepsTheRequestedSpellingOverReturnedSpellings(t *testing.T) {
	requested := mustParseRepository(t, "Acme/Backend")
	scope := mustScopeSet(t, domain.NewRepositoryScope(requested))

	// Both returned spellings differ from the request and from each other; the
	// first one is what the v0.1 rule would have promoted to the display name.
	first := testEvent(t, "first", 5, "acme/backend", domain.CategoryPush, domain.EntityCommit)
	second := testEvent(t, "second", 1, "ACME/BACKEND", domain.CategoryPush, domain.EntityCommit)

	snapshot := domain.NewScopedSnapshot(scope, []domain.ScopedActivity{
		{Events: []domain.EventEvidence{{Event: first}}},
		{Events: []domain.EventEvidence{{Event: second}}},
	})

	aggregates := snapshot.Aggregates()
	if len(aggregates) != 1 {
		t.Fatalf("Aggregates() returned %d rows, want the single selected Scope", len(aggregates))
	}
	if got, want := aggregates[0].Scope.String(), "Acme/Backend"; got != want {
		t.Errorf("Scope row label = %q, want the retained requested spelling %q", got, want)
	}
	if got := aggregates[0].Activity; got != 2 {
		t.Errorf("Scope row activity = %d, want both case-insensitively matching events counted", got)
	}

	// No returned spelling wins over another either: nothing selects one identity
	// for the repository, so each retained event keeps the spelling it returned.
	spellings := make([]string, 0, 2)
	for _, event := range snapshot.Events() {
		spellings = append(spellings, event.Repository.String())
	}
	if len(spellings) != 2 || spellings[0] != "acme/backend" || spellings[1] != "ACME/BACKEND" {
		t.Errorf("retained event spellings = %v, want each event's own returned spelling in recency order", spellings)
	}
}

// TestScopeRowLabelKeepsTheRequestedSpellingForAnEmptyPage guards that the
// empty-page fallback the v0.1 rule described is the unconditional v0.2 rule:
// a Scope with no returned activity still renders its requested spelling.
func TestScopeRowLabelKeepsTheRequestedSpellingForAnEmptyPage(t *testing.T) {
	requested := mustParseRepository(t, "Acme/Frontend")
	scope := mustScopeSet(t, domain.NewRepositoryScope(requested))

	snapshot := domain.NewScopedSnapshot(scope, nil)

	aggregates := snapshot.Aggregates()
	if len(aggregates) != 1 {
		t.Fatalf("Aggregates() returned %d rows, want the single selected Scope", len(aggregates))
	}
	if got, want := aggregates[0].Scope.String(), "Acme/Frontend"; got != want {
		t.Errorf("Scope row label = %q, want the requested spelling %q", got, want)
	}
}

// TestRepositoryActivityCarriesNoUnreadDisplayIdentity guards that the
// aggregation input carries no repository display fact, now that no view reads
// one: the v0.1 returned-spelling display identity is superseded, so retaining
// it on the type would leave a display fact nothing renders.
//
// It pins the complete field set rather than the removed name, because any
// added field would be the same unread display fact under a different spelling.
// Retain and NewScopedSnapshot read Events alone, so a second field could only
// be reintroduced together with a consumer for it.
func TestRepositoryActivityCarriesNoUnreadDisplayIdentity(t *testing.T) {
	activity := reflect.TypeOf(domain.RepositoryActivity{})
	fields := make([]string, 0, activity.NumField())
	for index := range activity.NumField() {
		fields = append(fields, activity.Field(index).Name)
	}
	if len(fields) != 1 || fields[0] != "Events" {
		t.Errorf("RepositoryActivity fields = %v, want only the retained Events a refresh reads", fields)
	}
}
