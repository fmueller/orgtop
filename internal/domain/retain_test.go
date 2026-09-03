package domain_test

import (
	"testing"

	"github.com/fmueller/orgtop/internal/domain"
)

// TestRetainBoundsTheEventSetBeforeEnrichment guards A-028: only the newest
// MaxSnapshotEvents events are retained, so a discarded event never becomes
// enrichment, cache, or matcher work.
func TestRetainBoundsTheEventSetBeforeEnrichment(t *testing.T) {
	repository := mustParseRepository(t, "owner/repo")
	scope := mustScopeSet(t, domain.NewRepositoryScope(repository))

	events := make([]domain.Event, 0, domain.MaxSnapshotEvents+1)
	for index := range domain.MaxSnapshotEvents + 1 {
		events = append(events, testEvent(t, "event-"+string(rune('a'+index%26))+string(rune('a'+index/26)), index, "owner/repo", domain.CategoryPush, domain.EntityCommit))
	}

	retained, truncated := domain.Retain(scope, []domain.RepositoryActivity{{Repository: repository, Events: events}})
	if !truncated {
		t.Fatal("Retain reported no truncation for one event beyond the bound")
	}
	if len(retained) != domain.MaxSnapshotEvents {
		t.Fatalf("Retain kept %d events, want %d", len(retained), domain.MaxSnapshotEvents)
	}
	// The newest event is retained first and the oldest one is discarded.
	if retained[0].ID != events[len(events)-1].ID {
		t.Fatalf("Retain kept %q first, want the newest event %q", retained[0].ID, events[len(events)-1].ID)
	}
	for _, event := range retained {
		if event.ID == events[0].ID {
			t.Fatalf("Retain kept the oldest event %q beyond the bound", events[0].ID)
		}
	}
}

// TestRetainDropsUnselectedRepositoriesAndDuplicates guards that retention is
// the same filtering, deduplication, and ordering the published snapshot uses,
// so enrichment never runs for an event the snapshot discards.
func TestRetainDropsUnselectedRepositoriesAndDuplicates(t *testing.T) {
	selected := mustParseRepository(t, "owner/repo")
	other := mustParseRepository(t, "owner/other")
	scope := mustScopeSet(t, domain.NewRepositoryScope(selected))

	first := testEvent(t, "shared", 1, "owner/repo", domain.CategoryPush, domain.EntityCommit)
	newer := testEvent(t, "newer", 5, "owner/repo", domain.CategoryPush, domain.EntityCommit)
	foreign := testEvent(t, "foreign", 9, "owner/other", domain.CategoryPush, domain.EntityCommit)

	retained, truncated := domain.Retain(scope, []domain.RepositoryActivity{
		{Repository: selected, Events: []domain.Event{first, newer, first}},
		{Repository: other, Events: []domain.Event{foreign}},
	})
	if truncated {
		t.Fatal("Retain reported truncation for a set inside the bound")
	}
	ids := make([]string, 0, len(retained))
	for _, event := range retained {
		ids = append(ids, event.ID)
	}
	if len(ids) != 2 || ids[0] != "newer" || ids[1] != "shared" {
		t.Fatalf("Retain returned %v, want the deduplicated reverse-chronological selection", ids)
	}
}

// TestNewRetainedSnapshotKeepsTheTruncationRetentionDecided guards that a
// snapshot built from an already retained set still reports the bound that
// retention applied, so the pre-enrichment truncation survives publication.
func TestNewRetainedSnapshotKeepsTheTruncationRetentionDecided(t *testing.T) {
	api := pathScope(t, "owner/repo", literal("services"), separator(), recursive())
	scope := mustScopeSet(t, api)
	evidence := pushEvidence(t, "one", 1, "owner/repo", complete(t, "services/api/main.go"))

	snapshot := domain.NewRetainedSnapshot(scope, []domain.EventEvidence{evidence}, true)
	if !snapshot.Truncated() {
		t.Fatal("NewRetainedSnapshot dropped the truncation retention decided")
	}
	if got := findScopeAggregate(t, snapshot, api).Activity; got != 1 {
		t.Fatalf("NewRetainedSnapshot counted %d activity, want 1", got)
	}
}

// TestHasPathScopesReportsWhetherEvidenceIsNeeded guards the repository-only
// fast path: a selection without a path Scope needs no changed-file evidence.
func TestHasPathScopesReportsWhetherEvidenceIsNeeded(t *testing.T) {
	repository := domain.NewRepositoryScope(mustParseRepository(t, "owner/repo"))
	if mustScopeSet(t, repository).HasPathScopes() {
		t.Fatal("a repository-only selection reported path Scopes")
	}
	if !mustScopeSet(t, repository, pathScope(t, "owner/repo", literal("docs"))).HasPathScopes() {
		t.Fatal("a mixed selection reported no path Scopes")
	}
}
