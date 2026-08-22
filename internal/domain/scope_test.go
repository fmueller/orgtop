package domain_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

func mustParseRepository(t *testing.T, value string) domain.Repository {
	t.Helper()
	repo, err := domain.ParseRepository(value)
	if err != nil {
		t.Fatalf("ParseRepository(%q) returned error: %v", value, err)
	}
	return repo
}

func mustNewScope(t *testing.T, values ...string) domain.Scope {
	t.Helper()
	scope, err := domain.NewScope(values)
	if err != nil {
		t.Fatalf("NewScope(%v) returned error: %v", values, err)
	}
	return scope
}

func repositoryStrings(repos []domain.Repository) []string {
	out := make([]string, 0, len(repos))
	for _, repo := range repos {
		out = append(out, repo.String())
	}
	return out
}

func TestNewScopeDeduplicatesAndKeepsFirstSpelling(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{name: "single", values: []string{"owner/repo"}, want: []string{"owner/repo"}},
		{name: "request order preserved", values: []string{"b/two", "a/one"}, want: []string{"b/two", "a/one"}},
		{name: "exact duplicate collapses", values: []string{"owner/repo", "owner/repo"}, want: []string{"owner/repo"}},
		{name: "case insensitive duplicate keeps first spelling", values: []string{"Owner/Repo", "owner/repo", "OWNER/REPO"}, want: []string{"Owner/Repo"}},
		{name: "duplicates do not disturb later entries", values: []string{"a/one", "A/ONE", "b/two"}, want: []string{"a/one", "b/two"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope := mustNewScope(t, tt.values...)
			got := repositoryStrings(scope.Repositories())
			if !slices.Equal(got, tt.want) {
				t.Fatalf("Repositories() = %v, want %v", got, tt.want)
			}
			if scope.Len() != len(tt.want) {
				t.Errorf("Len() = %d, want %d", scope.Len(), len(tt.want))
			}
		})
	}
}

func TestNewScopeRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{name: "nil", values: nil},
		{name: "empty slice", values: []string{}},
		{name: "invalid identifier", values: []string{"owner/repo", "not-a-repo"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := domain.NewScope(tt.values); err == nil {
				t.Fatal("NewScope returned no error, want error")
			}
		})
	}
}

func TestNewScopeInvalidIdentifierWrapsErrInvalidRepository(t *testing.T) {
	_, err := domain.NewScope([]string{"owner/repo", "owner/"})
	if !errors.Is(err, domain.ErrInvalidRepository) {
		t.Fatalf("error %v does not match ErrInvalidRepository", err)
	}
}

func TestScopeContainsIsCaseInsensitive(t *testing.T) {
	scope := mustNewScope(t, "Owner/Repo")

	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "same spelling", value: "Owner/Repo", want: true},
		{name: "lowercase", value: "owner/repo", want: true},
		{name: "uppercase", value: "OWNER/REPO", want: true},
		{name: "other repository", value: "owner/other", want: false},
		{name: "other owner", value: "other/repo", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scope.Contains(mustParseRepository(t, tt.value)); got != tt.want {
				t.Errorf("Contains(%q) = %t, want %t", tt.value, got, tt.want)
			}
		})
	}
}

func TestScopeRepositoriesReturnsCopy(t *testing.T) {
	scope := mustNewScope(t, "owner/one", "owner/two")

	repos := scope.Repositories()
	repos[0] = mustParseRepository(t, "other/replaced")

	if got := scope.Repositories()[0].String(); got != "owner/one" {
		t.Errorf("Repositories()[0] = %q after mutating the returned slice, want %q", got, "owner/one")
	}
}

func TestScopeFilterKeepsOnlyInScopeEvents(t *testing.T) {
	scope := mustNewScope(t, "Owner/Repo", "owner/second")
	at := time.Date(2026, time.August, 22, 10, 0, 0, 0, time.UTC)

	input := []domain.Event{
		{ID: "1", OccurredAt: at, Repository: mustParseRepository(t, "owner/repo")},
		{ID: "2", OccurredAt: at, Repository: mustParseRepository(t, "other/repo")},
		{ID: "3", OccurredAt: at, Repository: mustParseRepository(t, "OWNER/SECOND")},
		{ID: "4", OccurredAt: at, Repository: mustParseRepository(t, "owner/repository")},
	}

	got := scope.Filter(input)

	assertIDs(t, got, []string{"1", "3"})
	if ids := eventIDs(input); !slices.Equal(ids, []string{"1", "2", "3", "4"}) {
		t.Errorf("Filter mutated its input: %v", ids)
	}
}

func TestScopeFilterEmptyInput(t *testing.T) {
	scope := mustNewScope(t, "owner/repo")

	if got := scope.Filter(nil); len(got) != 0 {
		t.Errorf("Filter(nil) = %v, want no events", eventIDs(got))
	}
	if got := scope.Filter([]domain.Event{}); len(got) != 0 {
		t.Errorf("Filter(empty) = %v, want no events", eventIDs(got))
	}
}
