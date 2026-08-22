package domain_test

import (
	"slices"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

func eventIDs(events []domain.Event) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, event.ID)
	}
	return out
}

func assertIDs(t *testing.T, got []domain.Event, want []string) {
	t.Helper()
	if ids := eventIDs(got); !slices.Equal(ids, want) {
		t.Fatalf("got IDs %v, want %v", ids, want)
	}
}

func TestEventCarriesRequiredData(t *testing.T) {
	repo := mustParseRepository(t, "Owner/Repo")
	at := time.Date(2026, time.August, 22, 10, 30, 0, 0, time.UTC)

	event := domain.Event{
		ID:          "12345",
		OccurredAt:  at,
		Repository:  repo,
		Actor:       "octocat",
		Category:    domain.CategoryPullRequest,
		EntityKind:  domain.EntityPullRequest,
		EntityRef:   "#42",
		Description: "merged pull request #42",
	}

	if event.ID != "12345" || !event.OccurredAt.Equal(at) || event.Repository.Key() != "owner/repo" {
		t.Errorf("unexpected identity fields: %+v", event)
	}
	if event.Actor != "octocat" || event.EntityRef != "#42" || event.Description != "merged pull request #42" {
		t.Errorf("unexpected descriptive fields: %+v", event)
	}
	if event.Category != domain.CategoryPullRequest || event.EntityKind != domain.EntityPullRequest {
		t.Errorf("unexpected classification fields: %+v", event)
	}
}

func TestCategoryAndEntityKindValues(t *testing.T) {
	categories := map[domain.Category]string{
		domain.CategoryPush:        "push",
		domain.CategoryPullRequest: "pull_request",
		domain.CategoryReview:      "review",
		domain.CategoryComment:     "comment",
		domain.CategoryOther:       "other",
	}
	for category, want := range categories {
		if string(category) != want {
			t.Errorf("category %v = %q, want %q", category, string(category), want)
		}
	}

	kinds := map[domain.EntityKind]string{
		domain.EntityRepository:  "repository",
		domain.EntityCommit:      "commit",
		domain.EntityPullRequest: "pull_request",
		domain.EntityOther:       "other",
	}
	for kind, want := range kinds {
		if string(kind) != want {
			t.Errorf("entity kind %v = %q, want %q", kind, string(kind), want)
		}
	}
}

func TestDeduplicateBySourceEventID(t *testing.T) {
	repo := mustParseRepository(t, "owner/repo")
	base := time.Date(2026, time.August, 22, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		input []domain.Event
		want  []string
	}{
		{name: "empty", input: nil, want: nil},
		{
			name: "distinct ids are kept in input order",
			input: []domain.Event{
				{ID: "2", OccurredAt: base, Repository: repo},
				{ID: "1", OccurredAt: base, Repository: repo},
			},
			want: []string{"2", "1"},
		},
		{
			name: "repeated id keeps the first occurrence",
			input: []domain.Event{
				{ID: "1", OccurredAt: base, Repository: repo, Description: "first"},
				{ID: "2", OccurredAt: base, Repository: repo},
				{ID: "1", OccurredAt: base.Add(time.Hour), Repository: repo, Description: "second"},
			},
			want: []string{"1", "2"},
		},
		{
			name: "ids are matched exactly",
			input: []domain.Event{
				{ID: "1", OccurredAt: base, Repository: repo},
				{ID: "01", OccurredAt: base, Repository: repo},
			},
			want: []string{"1", "01"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.Deduplicate(tt.input)
			assertIDs(t, got, tt.want)
		})
	}
}

func TestDeduplicateKeepsFirstPayloadAndDoesNotMutateInput(t *testing.T) {
	repo := mustParseRepository(t, "owner/repo")
	base := time.Date(2026, time.August, 22, 10, 0, 0, 0, time.UTC)
	input := []domain.Event{
		{ID: "1", OccurredAt: base, Repository: repo, Description: "first"},
		{ID: "1", OccurredAt: base, Repository: repo, Description: "second"},
	}

	got := domain.Deduplicate(input)

	if len(got) != 1 || got[0].Description != "first" {
		t.Fatalf("Deduplicate = %+v, want the first occurrence", got)
	}
	if len(input) != 2 || input[1].Description != "second" {
		t.Errorf("Deduplicate mutated its input: %+v", input)
	}
}

func TestSortByRecency(t *testing.T) {
	repo := mustParseRepository(t, "owner/repo")
	base := time.Date(2026, time.August, 22, 10, 0, 0, 0, time.UTC)
	east := time.FixedZone("UTC+3", 3*60*60)

	tests := []struct {
		name  string
		input []domain.Event
		want  []string
	}{
		{name: "empty", input: nil, want: nil},
		{
			name: "newest first",
			input: []domain.Event{
				{ID: "old", OccurredAt: base, Repository: repo},
				{ID: "new", OccurredAt: base.Add(time.Hour), Repository: repo},
				{ID: "middle", OccurredAt: base.Add(30 * time.Minute), Repository: repo},
			},
			want: []string{"new", "middle", "old"},
		},
		{
			name: "timestamp ties order by id ascending",
			input: []domain.Event{
				{ID: "b", OccurredAt: base, Repository: repo},
				{ID: "c", OccurredAt: base, Repository: repo},
				{ID: "a", OccurredAt: base, Repository: repo},
			},
			want: []string{"a", "b", "c"},
		},
		{
			name: "comparison uses the instant, not the location",
			input: []domain.Event{
				{ID: "utc-noon", OccurredAt: time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC), Repository: repo},
				{ID: "east-noon", OccurredAt: time.Date(2026, time.August, 22, 12, 0, 0, 0, east), Repository: repo},
			},
			want: []string{"utc-noon", "east-noon"},
		},
		{
			name: "identical instants across locations tie on id",
			input: []domain.Event{
				{ID: "z-utc", OccurredAt: time.Date(2026, time.August, 22, 9, 0, 0, 0, time.UTC), Repository: repo},
				{ID: "a-east", OccurredAt: time.Date(2026, time.August, 22, 12, 0, 0, 0, east), Repository: repo},
			},
			want: []string{"a-east", "z-utc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.SortByRecency(tt.input)
			assertIDs(t, got, tt.want)

			again := domain.SortByRecency(tt.input)
			assertIDs(t, again, tt.want)
		})
	}
}

func TestSortByRecencyDoesNotMutateInput(t *testing.T) {
	repo := mustParseRepository(t, "owner/repo")
	base := time.Date(2026, time.August, 22, 10, 0, 0, 0, time.UTC)
	input := []domain.Event{
		{ID: "old", OccurredAt: base, Repository: repo},
		{ID: "new", OccurredAt: base.Add(time.Hour), Repository: repo},
	}

	domain.SortByRecency(input)

	assertIDs(t, input, []string{"old", "new"})
}
