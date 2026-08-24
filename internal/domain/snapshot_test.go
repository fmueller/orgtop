package domain_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

func testEvent(t *testing.T, id string, minute int, repo string, category domain.Category, kind domain.EntityKind) domain.Event {
	t.Helper()
	return domain.Event{
		ID:         id,
		OccurredAt: time.Date(2026, time.August, 22, 10, minute, 0, 0, time.UTC),
		Repository: mustParseRepository(t, repo),
		Category:   category,
		EntityKind: kind,
	}
}

func aggregateNames(aggregates []domain.Aggregate) []string {
	out := make([]string, 0, len(aggregates))
	for _, aggregate := range aggregates {
		out = append(out, aggregate.Repository.String())
	}
	return out
}

func findAggregate(t *testing.T, snapshot domain.Snapshot, key string) domain.Aggregate {
	t.Helper()
	aggregates := snapshot.Aggregates()
	for _, aggregate := range aggregates {
		if aggregate.Repository.Key() == key {
			return aggregate
		}
	}
	t.Fatalf("no aggregate for %q in %v", key, aggregateNames(aggregates))
	return domain.Aggregate{}
}

func TestNewSnapshotFiltersBeforeAggregating(t *testing.T) {
	scope := mustNewScope(t, "owner/selected")
	activities := []domain.RepositoryActivity{{
		Repository: mustParseRepository(t, "owner/selected"),
		Events: []domain.Event{
			testEvent(t, "2", 2, "owner/selected", domain.CategoryPush, domain.EntityCommit),
			testEvent(t, "1", 1, "owner/other", domain.CategoryPush, domain.EntityCommit),
		},
	}}

	snapshot := domain.NewSnapshot(scope, activities)

	assertIDs(t, snapshot.Events(), []string{"2"})
	if got := findAggregate(t, snapshot, "owner/selected").Total; got != 1 {
		t.Errorf("total = %d, want 1", got)
	}
	if len(snapshot.Aggregates()) != 1 {
		t.Errorf("aggregates = %v, want only the selected repository", aggregateNames(snapshot.Aggregates()))
	}
}

func TestNewSnapshotDeduplicatesSortsAndBoundsEvents(t *testing.T) {
	scope := mustNewScope(t, "owner/repo")
	events := []domain.Event{
		testEvent(t, "b", 2, "owner/repo", domain.CategoryPush, domain.EntityCommit),
		testEvent(t, "a", 3, "owner/repo", domain.CategoryPush, domain.EntityCommit),
		testEvent(t, "b", 2, "owner/repo", domain.CategoryPush, domain.EntityCommit),
	}

	snapshot := domain.NewSnapshot(scope, []domain.RepositoryActivity{{
		Repository: mustParseRepository(t, "owner/repo"),
		Events:     events,
	}})

	assertIDs(t, snapshot.Events(), []string{"a", "b"})
	if got := findAggregate(t, snapshot, "owner/repo").Total; got != 2 {
		t.Errorf("total = %d, want 2", got)
	}
}

func TestNewSnapshotKeepsNewest500Events(t *testing.T) {
	scope := mustNewScope(t, "owner/repo")
	repo := mustParseRepository(t, "owner/repo")
	const returned = 600
	events := make([]domain.Event, 0, returned)
	for i := range returned {
		events = append(events, domain.Event{
			ID:         fmt.Sprintf("event-%03d", i),
			OccurredAt: time.Date(2026, time.August, 22, 0, 0, i, 0, time.UTC),
			Repository: repo,
			Category:   domain.CategoryPush,
			EntityKind: domain.EntityCommit,
		})
	}

	snapshot := domain.NewSnapshot(scope, []domain.RepositoryActivity{{Repository: repo, Events: events}})

	kept := snapshot.Events()
	if len(kept) != domain.MaxSnapshotEvents {
		t.Fatalf("events = %d, want %d", len(kept), domain.MaxSnapshotEvents)
	}
	newest := events[len(events)-1]
	if kept[0].ID != newest.ID {
		t.Errorf("first event = %q, want the newest %q", kept[0].ID, newest.ID)
	}
	oldestKept := events[len(events)-domain.MaxSnapshotEvents]
	if last := kept[len(kept)-1]; last.ID != oldestKept.ID {
		t.Errorf("last event = %q, want %q", last.ID, oldestKept.ID)
	}
	if got := findAggregate(t, snapshot, "owner/repo").Total; got != domain.MaxSnapshotEvents {
		t.Errorf("total = %d, want %d", got, domain.MaxSnapshotEvents)
	}
}

func TestNewSnapshotCountsCategoriesUnderFR006(t *testing.T) {
	scope := mustNewScope(t, "owner/repo")
	repo := mustParseRepository(t, "owner/repo")
	events := []domain.Event{
		testEvent(t, "1", 1, "owner/repo", domain.CategoryPush, domain.EntityCommit),
		testEvent(t, "2", 2, "owner/repo", domain.CategoryPush, domain.EntityCommit),
		testEvent(t, "3", 3, "owner/repo", domain.CategoryPullRequest, domain.EntityPullRequest),
		testEvent(t, "4", 4, "owner/repo", domain.CategoryReview, domain.EntityPullRequest),
		testEvent(t, "5", 5, "owner/repo", domain.CategoryComment, domain.EntityPullRequest),
		testEvent(t, "6", 6, "owner/repo", domain.CategoryOther, domain.EntityOther),
		testEvent(t, "7", 7, "owner/repo", domain.CategoryOther, domain.EntityRepository),
	}

	aggregate := findAggregate(t, domain.NewSnapshot(scope, []domain.RepositoryActivity{{Repository: repo, Events: events}}), "owner/repo")

	if aggregate.Total != 7 {
		t.Errorf("total = %d, want 7", aggregate.Total)
	}
	if aggregate.Pushes != 2 {
		t.Errorf("pushes = %d, want 2", aggregate.Pushes)
	}
	if aggregate.PullRequestActivity != 3 {
		t.Errorf("pull request activity = %d, want 3", aggregate.PullRequestActivity)
	}
}

func TestNewSnapshotExcludesIssueCommentsFromPullRequestActivity(t *testing.T) {
	scope := mustNewScope(t, "owner/repo")
	repo := mustParseRepository(t, "owner/repo")
	events := []domain.Event{
		testEvent(t, "1", 1, "owner/repo", domain.CategoryComment, domain.EntityOther),
	}

	aggregate := findAggregate(t, domain.NewSnapshot(scope, []domain.RepositoryActivity{{Repository: repo, Events: events}}), "owner/repo")

	if aggregate.PullRequestActivity != 0 {
		t.Errorf("pull request activity = %d, want 0", aggregate.PullRequestActivity)
	}
	if aggregate.Total != 1 {
		t.Errorf("total = %d, want 1", aggregate.Total)
	}
}

func TestNewSnapshotUsesReturnedDisplayIdentityAndRequestedFallback(t *testing.T) {
	scope := mustNewScope(t, "owner/mixedcase", "owner/empty")
	activities := []domain.RepositoryActivity{
		{
			Repository: mustParseRepository(t, "Owner/MixedCase"),
			Events:     []domain.Event{testEvent(t, "1", 1, "Owner/MixedCase", domain.CategoryPush, domain.EntityCommit)},
		},
		{Repository: mustParseRepository(t, "owner/empty")},
	}

	snapshot := domain.NewSnapshot(scope, activities)

	if got := findAggregate(t, snapshot, "owner/mixedcase").Repository.String(); got != "Owner/MixedCase" {
		t.Errorf("display identity = %q, want %q", got, "Owner/MixedCase")
	}
	empty := findAggregate(t, snapshot, "owner/empty")
	if empty.Repository.String() != "owner/empty" {
		t.Errorf("fallback identity = %q, want %q", empty.Repository.String(), "owner/empty")
	}
	if empty.Total != 0 || empty.Pushes != 0 || empty.PullRequestActivity != 0 {
		t.Errorf("zero-event aggregate = %+v, want zero counts", empty)
	}
}

func TestNewSnapshotOrdersRowsByTotalThenName(t *testing.T) {
	scope := mustNewScope(t, "owner/beta", "owner/Alpha", "owner/busy", "owner/quiet")
	activities := []domain.RepositoryActivity{
		{
			Repository: mustParseRepository(t, "owner/beta"),
			Events:     []domain.Event{testEvent(t, "1", 1, "owner/beta", domain.CategoryPush, domain.EntityCommit)},
		},
		{
			Repository: mustParseRepository(t, "owner/Alpha"),
			Events:     []domain.Event{testEvent(t, "2", 2, "owner/Alpha", domain.CategoryPush, domain.EntityCommit)},
		},
		{
			Repository: mustParseRepository(t, "owner/busy"),
			Events: []domain.Event{
				testEvent(t, "3", 3, "owner/busy", domain.CategoryPush, domain.EntityCommit),
				testEvent(t, "4", 4, "owner/busy", domain.CategoryPush, domain.EntityCommit),
			},
		},
		{Repository: mustParseRepository(t, "owner/quiet")},
	}

	got := aggregateNames(domain.NewSnapshot(scope, activities).Aggregates())

	want := []string{"owner/busy", "owner/Alpha", "owner/beta", "owner/quiet"}
	if !slices.Equal(got, want) {
		t.Errorf("aggregate order = %v, want %v", got, want)
	}
}

func TestNewSnapshotIsRepeatableAndDoesNotMutateInputs(t *testing.T) {
	scope := mustNewScope(t, "owner/repo")
	repo := mustParseRepository(t, "owner/repo")
	events := []domain.Event{
		testEvent(t, "1", 1, "owner/repo", domain.CategoryPush, domain.EntityCommit),
		testEvent(t, "3", 3, "owner/repo", domain.CategoryPullRequest, domain.EntityPullRequest),
		testEvent(t, "2", 2, "owner/repo", domain.CategoryReview, domain.EntityPullRequest),
	}
	original := slices.Clone(events)
	activities := []domain.RepositoryActivity{{Repository: repo, Events: events}}

	first := domain.NewSnapshot(scope, activities)
	second := domain.NewSnapshot(scope, activities)

	assertIDs(t, first.Events(), []string{"3", "2", "1"})
	assertIDs(t, second.Events(), eventIDs(first.Events()))
	if !slices.Equal(eventIDs(events), eventIDs(original)) {
		t.Errorf("input events were reordered: %v", eventIDs(events))
	}
	if !slices.Equal(aggregateNames(first.Aggregates()), aggregateNames(second.Aggregates())) {
		t.Errorf("aggregates are not repeatable")
	}
}

func TestSnapshotAccessorsDoNotExposeInternalState(t *testing.T) {
	scope := mustNewScope(t, "owner/repo")
	repo := mustParseRepository(t, "owner/repo")
	snapshot := domain.NewSnapshot(scope, []domain.RepositoryActivity{{
		Repository: repo,
		Events: []domain.Event{
			testEvent(t, "1", 1, "owner/repo", domain.CategoryPush, domain.EntityCommit),
			testEvent(t, "2", 2, "owner/repo", domain.CategoryPush, domain.EntityCommit),
		},
	}})

	mutated := snapshot.Events()
	mutated[0] = domain.Event{ID: "tampered"}
	aggregates := snapshot.Aggregates()
	aggregates[0] = domain.Aggregate{}

	assertIDs(t, snapshot.Events(), []string{"2", "1"})
	if got := findAggregate(t, snapshot, "owner/repo").Total; got != 2 {
		t.Errorf("total = %d, want 2", got)
	}
}

func TestNewSnapshotWithoutActivitiesStillRendersScopeRows(t *testing.T) {
	scope := mustNewScope(t, "owner/repo")

	snapshot := domain.NewSnapshot(scope, nil)

	if len(snapshot.Events()) != 0 {
		t.Errorf("events = %v, want none", eventIDs(snapshot.Events()))
	}
	if got := aggregateNames(snapshot.Aggregates()); !slices.Equal(got, []string{"owner/repo"}) {
		t.Errorf("aggregates = %v, want the requested spelling row", got)
	}
}

func TestNewSnapshotKeepsFirstReturnedDisplayIdentityPerRepository(t *testing.T) {
	scope := mustNewScope(t, "owner/repo")
	activities := []domain.RepositoryActivity{
		{
			Repository: mustParseRepository(t, "Owner/Repo"),
			Events:     []domain.Event{testEvent(t, "1", 1, "Owner/Repo", domain.CategoryPush, domain.EntityCommit)},
		},
		{
			Repository: mustParseRepository(t, "OWNER/REPO"),
			Events:     []domain.Event{testEvent(t, "2", 2, "OWNER/REPO", domain.CategoryPush, domain.EntityCommit)},
		},
	}

	snapshot := domain.NewSnapshot(scope, activities)

	aggregate := findAggregate(t, snapshot, "owner/repo")
	if aggregate.Repository.String() != "Owner/Repo" {
		t.Errorf("display identity = %q, want the first returned spelling %q", aggregate.Repository.String(), "Owner/Repo")
	}
	if aggregate.Total != 2 {
		t.Errorf("total = %d, want 2", aggregate.Total)
	}
}

// pushEvents returns count push events for the repository, one second apart, so
// a snapshot can be built at an exact distance from the FR-006 bound.
func pushEvents(repo domain.Repository, count int) []domain.Event {
	events := make([]domain.Event, 0, count)
	for i := range count {
		events = append(events, domain.Event{
			ID:         fmt.Sprintf("event-%04d", i),
			OccurredAt: time.Date(2026, time.August, 22, 0, 0, i, 0, time.UTC),
			Repository: repo,
			Category:   domain.CategoryPush,
			EntityKind: domain.EntityCommit,
		})
	}
	return events
}

// TestNewSnapshotRecordsWhetherTheBoundDiscardedEvents guards FR-006: the
// snapshot retains whether the bound cut it, so a view reports a fact rather
// than inferring one from the count reaching the limit.
func TestNewSnapshotRecordsWhetherTheBoundDiscardedEvents(t *testing.T) {
	cases := []struct {
		name     string
		returned int
		want     bool
	}{
		{name: "below the bound", returned: domain.MaxSnapshotEvents - 1, want: false},
		{name: "exactly at the bound", returned: domain.MaxSnapshotEvents, want: false},
		{name: "one past the bound", returned: domain.MaxSnapshotEvents + 1, want: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			scope := mustNewScope(t, "owner/repo")
			repo := mustParseRepository(t, "owner/repo")
			events := pushEvents(repo, testCase.returned)

			snapshot := domain.NewSnapshot(scope, []domain.RepositoryActivity{{Repository: repo, Events: events}})

			if got := snapshot.Truncated(); got != testCase.want {
				t.Errorf("Truncated() = %t for %d returned events, want %t", got, testCase.returned, testCase.want)
			}
			if got, want := len(snapshot.Events()), min(testCase.returned, domain.MaxSnapshotEvents); got != want {
				t.Errorf("events = %d, want %d", got, want)
			}
		})
	}
}

// TestZeroSnapshotIsNotTruncated guards the zero value: a snapshot that never
// held events was never cut by the bound.
func TestZeroSnapshotIsNotTruncated(t *testing.T) {
	if (domain.Snapshot{}).Truncated() {
		t.Error("the zero snapshot reports that the bound discarded events")
	}
}
