package domain_test

import (
	"errors"
	"slices"
	"strconv"
	"strings"
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

func mustNewScopeSet(t *testing.T, values ...string) domain.ScopeSet {
	t.Helper()
	scopes, err := domain.NewRepositoryScopeSet(values)
	if err != nil {
		t.Fatalf("NewRepositoryScopeSet(%v) returned error: %v", values, err)
	}
	return scopes
}

func mustPathScope(t *testing.T, repository string, tokens ...domain.MatcherToken) domain.Scope {
	t.Helper()
	scope, err := domain.NewPathScope(mustParseRepository(t, repository), mustMatcher(t, tokens...))
	if err != nil {
		t.Fatalf("NewPathScope(%q, %v) returned error: %v", repository, tokens, err)
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

func scopeStrings(scopes []domain.Scope) []string {
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		out = append(out, scope.String())
	}
	return out
}

// TestRepositoryScopeIdentity pins the closed repository Scope identity tuple and
// its v0.1 case-insensitive repository key.
func TestRepositoryScopeIdentity(t *testing.T) {
	scope := domain.NewRepositoryScope(mustParseRepository(t, "Acme/API"))

	if got, want := scope.Kind(), domain.ScopeRepository; got != want {
		t.Errorf("Kind() = %v, want %v", got, want)
	}
	if got, want := scope.String(), "Acme/API"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if !scope.Matcher().IsZero() {
		t.Error("Matcher() is set on a repository Scope, want the zero matcher")
	}

	identity := scope.Identity()
	if got, want := identity.Kind, domain.ScopeRepository; got != want {
		t.Errorf("Identity().Kind = %v, want %v", got, want)
	}
	if got, want := identity.RepositoryKey, "acme/api"; got != want {
		t.Errorf("Identity().RepositoryKey = %q, want %q", got, want)
	}
	if got := identity.Matcher; got != "" {
		t.Errorf("Identity().Matcher = %q, want an empty matcher encoding", got)
	}

	other := domain.NewRepositoryScope(mustParseRepository(t, "acme/api"))
	if !scope.Equal(other) {
		t.Error("Equal() = false for the same repository in another spelling, want true")
	}
}

// TestPathScopeIdentity pins the closed path Scope identity tuple: the leading
// kind keeps a `**` path Scope distinct from its repository Scope.
func TestPathScopeIdentity(t *testing.T) {
	scope := mustPathScope(t, "Acme/API", recursive())

	if got, want := scope.Kind(), domain.ScopePath; got != want {
		t.Errorf("Kind() = %v, want %v", got, want)
	}
	if got, want := scope.String(), "Acme/API:**"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}

	identity := scope.Identity()
	if got, want := identity.RepositoryKey, "acme/api"; got != want {
		t.Errorf("Identity().RepositoryKey = %q, want %q", got, want)
	}
	if identity.Matcher == "" {
		t.Error("Identity().Matcher is empty, want the canonical matcher encoding")
	}

	repository := domain.NewRepositoryScope(mustParseRepository(t, "acme/api"))
	if scope.Equal(repository) {
		t.Error("Equal() = true for a `**` path Scope and its repository Scope, want false")
	}
	if scope.Identity() == repository.Identity() {
		t.Error("a `**` path Scope shares its repository Scope identity, want distinct identities")
	}
}

func TestNewPathScopeRejectsAnUnconstructedMatcher(t *testing.T) {
	_, err := domain.NewPathScope(mustParseRepository(t, "acme/api"), domain.PathMatcher{})
	if !errors.Is(err, domain.ErrInvalidMatcher) {
		t.Fatalf("error %v does not match ErrInvalidMatcher", err)
	}
}

// TestNewScopeSetDeduplicatesByCanonicalIdentity guards A-023: canonically equal
// path Scopes collapse into one and the first requested spelling survives, while
// overlapping but distinct Scopes remain independent.
func TestNewScopeSetDeduplicatesByCanonicalIdentity(t *testing.T) {
	repository := mustParseRepository(t, "acme/api")
	scopes, err := domain.NewScopeSet([]domain.Scope{
		mustPathScope(t, "acme/api", literal("services"), separator(), literal("api")),
		mustPathScope(t, "Acme/API", literal("services"), separator(), literal("api"), separator(), recursive()),
		mustPathScope(t, "acme/api", literal("services"), separator(), literal("api"), separator(), recursive(), separator(), recursive()),
		mustPathScope(t, "acme/api", literal("services"), separator(), wildcard()),
		domain.NewRepositoryScope(repository),
		domain.NewRepositoryScope(mustParseRepository(t, "ACME/API")),
	})
	if err != nil {
		t.Fatalf("NewScopeSet returned error: %v", err)
	}

	want := []string{"acme/api:services/api", "acme/api:services/*", "acme/api"}
	if got := scopeStrings(scopes.Scopes()); !slices.Equal(got, want) {
		t.Fatalf("Scopes() = %v, want %v", got, want)
	}
	if got, want := scopes.Len(), 3; got != want {
		t.Errorf("Len() = %d, want %d", got, want)
	}
	if got, want := repositoryStrings(scopes.Repositories()), []string{"acme/api"}; !slices.Equal(got, want) {
		t.Errorf("Repositories() = %v, want %v", got, want)
	}
}

// TestNewScopeSetOrdersScopesDeterministically guards the closed stable Scope
// identity order: lowercase repository key byte order, the repository Scope
// before path Scopes of the same repository, then canonical matcher encoding.
func TestNewScopeSetOrdersScopesDeterministically(t *testing.T) {
	scopes, err := domain.NewScopeSet([]domain.Scope{
		mustPathScope(t, "acme/api", literal("services"), separator(), wildcard()),
		domain.NewRepositoryScope(mustParseRepository(t, "acme/web")),
		mustPathScope(t, "Acme/API", recursive()),
		domain.NewRepositoryScope(mustParseRepository(t, "acme/api")),
		mustPathScope(t, "acme/api", literal("docs")),
	})
	if err != nil {
		t.Fatalf("NewScopeSet returned error: %v", err)
	}

	want := []string{"acme/api", "Acme/API:**", "acme/api:docs", "acme/api:services/*", "acme/web"}
	if got := scopeStrings(scopes.Ordered()); !slices.Equal(got, want) {
		t.Fatalf("Ordered() = %v, want %v", got, want)
	}

	requested := []string{"acme/api:services/*", "acme/web", "Acme/API:**", "acme/api", "acme/api:docs"}
	if got := scopeStrings(scopes.Scopes()); !slices.Equal(got, requested) {
		t.Errorf("Scopes() = %v, want the request order %v", got, requested)
	}
}

func TestScopeSetReturnsCopies(t *testing.T) {
	scopes := mustNewScopeSet(t, "owner/one", "owner/two")

	scopes.Repositories()[0] = mustParseRepository(t, "other/replaced")
	scopes.Scopes()[0] = domain.NewRepositoryScope(mustParseRepository(t, "other/replaced"))
	scopes.Ordered()[0] = domain.NewRepositoryScope(mustParseRepository(t, "other/replaced"))

	if got := scopes.Repositories()[0].String(); got != "owner/one" {
		t.Errorf("Repositories()[0] = %q after mutating the returned slice, want %q", got, "owner/one")
	}
	if got := scopes.Scopes()[0].String(); got != "owner/one" {
		t.Errorf("Scopes()[0] = %q after mutating the returned slice, want %q", got, "owner/one")
	}
}

// TestNewScopeSetEnforcesClosedCapacities guards the closed RG-009 selection
// capacities before any network work: the diagnostic names the requested and the
// allowed count.
func TestNewScopeSetEnforcesClosedCapacities(t *testing.T) {
	repositoryScopes := func(count int) []domain.Scope {
		scopes := make([]domain.Scope, 0, count)
		for i := range count {
			scopes = append(scopes, domain.NewRepositoryScope(mustParseRepository(t, "acme/repo"+strconv.Itoa(i))))
		}
		return scopes
	}
	pathScopes := func(count int) []domain.Scope {
		scopes := make([]domain.Scope, 0, count)
		for i := range count {
			scopes = append(scopes, mustPathScope(t, "acme/api", literal("services"), separator(), literal("component"+strconv.Itoa(i))))
		}
		return scopes
	}

	tests := []struct {
		name      string
		scopes    []domain.Scope
		wantError bool
		wantParts []string
	}{
		{name: "repositories at the limit", scopes: repositoryScopes(domain.MaxSelectedRepositories)},
		{name: "scopes at the limit", scopes: pathScopes(domain.MaxScopes)},
		{
			name:      "one repository too many",
			scopes:    repositoryScopes(domain.MaxSelectedRepositories + 1),
			wantError: true,
			wantParts: []string{"21", "20", "repositor"},
		},
		{
			name:      "one Scope too many",
			scopes:    pathScopes(domain.MaxScopes + 1),
			wantError: true,
			wantParts: []string{"101", "100", "scope"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewScopeSet(tt.scopes)
			if !tt.wantError {
				if err != nil {
					t.Fatalf("NewScopeSet returned error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("NewScopeSet returned no error, want a capacity error")
			}
			if !errors.Is(err, domain.ErrScopeCapacity) {
				t.Fatalf("error %v does not match ErrScopeCapacity", err)
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("error %q does not contain %q", err.Error(), part)
				}
			}
		})
	}
}

// TestNewScopeSetCountsCapacitiesAfterDeduplication guards that repeated
// selections are not an error at the capacity boundary.
func TestNewScopeSetCountsCapacitiesAfterDeduplication(t *testing.T) {
	repository := domain.NewRepositoryScope(mustParseRepository(t, "acme/api"))
	scopes := make([]domain.Scope, 0, domain.MaxScopes+10)
	for range domain.MaxScopes + 10 {
		scopes = append(scopes, repository)
	}

	set, err := domain.NewScopeSet(scopes)
	if err != nil {
		t.Fatalf("NewScopeSet returned error: %v", err)
	}
	if got, want := set.Len(), 1; got != want {
		t.Errorf("Len() = %d, want %d", got, want)
	}
}

func TestNewScopeSetRejectsAnEmptySelection(t *testing.T) {
	tests := []struct {
		name   string
		scopes []domain.Scope
	}{
		{name: "nil"},
		{name: "empty slice", scopes: []domain.Scope{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := domain.NewScopeSet(tt.scopes); !errors.Is(err, domain.ErrEmptyScope) {
				t.Fatalf("error %v does not match ErrEmptyScope", err)
			}
		})
	}
}

func TestNewScopeSetRejectsAnUnconstructedScope(t *testing.T) {
	if _, err := domain.NewScopeSet([]domain.Scope{{}}); err == nil {
		t.Fatal("NewScopeSet returned no error for the zero Scope, want error")
	}
}

// TestNewRepositoryScopeSetKeepsV01Behavior guards FR-001: repository-only
// selections keep their v0.1 validation, deduplication, spelling retention, and
// request order.
func TestNewRepositoryScopeSetKeepsV01Behavior(t *testing.T) {
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
			scopes := mustNewScopeSet(t, tt.values...)
			got := repositoryStrings(scopes.Repositories())
			if !slices.Equal(got, tt.want) {
				t.Fatalf("Repositories() = %v, want %v", got, tt.want)
			}
			if scopes.Len() != len(tt.want) {
				t.Errorf("Len() = %d, want %d", scopes.Len(), len(tt.want))
			}
			for _, scope := range scopes.Scopes() {
				if scope.Kind() != domain.ScopeRepository {
					t.Errorf("Scope %q is %v, want a repository Scope", scope, scope.Kind())
				}
			}
		})
	}
}

func TestNewRepositoryScopeSetRejectsInvalidInput(t *testing.T) {
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
			if _, err := domain.NewRepositoryScopeSet(tt.values); err == nil {
				t.Fatal("NewRepositoryScopeSet returned no error, want error")
			}
		})
	}
}

func TestNewRepositoryScopeSetInvalidIdentifierWrapsErrInvalidRepository(t *testing.T) {
	_, err := domain.NewRepositoryScopeSet([]string{"owner/repo", "owner/"})
	if !errors.Is(err, domain.ErrInvalidRepository) {
		t.Fatalf("error %v does not match ErrInvalidRepository", err)
	}
}

// TestNewRepositoryScopeSetInvalidIdentifierKeepsTheParseReason pins the domain
// error as the standalone parse reason: it names the rejected value and why it
// failed, and adds no scope prefix for callers to stack their own context on.
func TestNewRepositoryScopeSetInvalidIdentifierKeepsTheParseReason(t *testing.T) {
	_, want := domain.ParseRepository("acme/*")
	_, err := domain.NewRepositoryScopeSet([]string{"owner/repo", "acme/*"})
	if err == nil {
		t.Fatal("NewRepositoryScopeSet returned no error, want error")
	}
	if got := err.Error(); got != want.Error() {
		t.Errorf("NewRepositoryScopeSet error = %q, want %q", got, want.Error())
	}
}

func TestScopeSetContainsIsCaseInsensitive(t *testing.T) {
	scopes := mustNewScopeSet(t, "Owner/Repo")

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
			if got := scopes.Contains(mustParseRepository(t, tt.value)); got != tt.want {
				t.Errorf("Contains(%q) = %t, want %t", tt.value, got, tt.want)
			}
		})
	}
}

// TestScopeSetContainsCoversPathScopeRepositories guards that a path Scope still
// selects its repository for polling and normalization.
func TestScopeSetContainsCoversPathScopeRepositories(t *testing.T) {
	scopes, err := domain.NewScopeSet([]domain.Scope{mustPathScope(t, "Acme/API", literal("services"))})
	if err != nil {
		t.Fatalf("NewScopeSet returned error: %v", err)
	}

	if !scopes.Contains(mustParseRepository(t, "acme/api")) {
		t.Error("Contains(acme/api) = false for a path Scope on that repository, want true")
	}
	if got, want := repositoryStrings(scopes.Repositories()), []string{"Acme/API"}; !slices.Equal(got, want) {
		t.Errorf("Repositories() = %v, want %v", got, want)
	}
}

func TestScopeSetFilterKeepsOnlyInScopeEvents(t *testing.T) {
	scopes := mustNewScopeSet(t, "Owner/Repo", "owner/second")
	at := time.Date(2026, time.August, 22, 10, 0, 0, 0, time.UTC)

	input := []domain.Event{
		{ID: "1", OccurredAt: at, Repository: mustParseRepository(t, "owner/repo")},
		{ID: "2", OccurredAt: at, Repository: mustParseRepository(t, "other/repo")},
		{ID: "3", OccurredAt: at, Repository: mustParseRepository(t, "OWNER/SECOND")},
		{ID: "4", OccurredAt: at, Repository: mustParseRepository(t, "owner/repository")},
	}

	got := scopes.Filter(input)

	assertIDs(t, got, []string{"1", "3"})
	if ids := eventIDs(input); !slices.Equal(ids, []string{"1", "2", "3", "4"}) {
		t.Errorf("Filter mutated its input: %v", ids)
	}
}

func TestScopeSetFilterEmptyInput(t *testing.T) {
	scopes := mustNewScopeSet(t, "owner/repo")

	if got := scopes.Filter(nil); len(got) != 0 {
		t.Errorf("Filter(nil) = %v, want no events", eventIDs(got))
	}
	if got := scopes.Filter([]domain.Event{}); len(got) != 0 {
		t.Errorf("Filter(empty) = %v, want no events", eventIDs(got))
	}
}

// TestPathMatcherCapacityIsEnforcedByTheScopeCapacity pins the closed RG-009
// invariant this package relies on: every path Scope carries exactly one
// matcher, so the Scope capacity also bounds matchers. Decoupling the two closed
// limits requires a separate matcher check.
func TestPathMatcherCapacityIsEnforcedByTheScopeCapacity(t *testing.T) {
	if domain.MaxPathMatchers < domain.MaxScopes {
		t.Fatalf("MaxPathMatchers = %d is below MaxScopes = %d, want a separate matcher capacity check", domain.MaxPathMatchers, domain.MaxScopes)
	}
}
