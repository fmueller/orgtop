package domain

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrEmptyScope reports that nothing was selected. OrgTop requires at least one
// repository or path Scope.
var ErrEmptyScope = errors.New("no repository selected")

// ErrScopeCapacity reports a selection that exceeds a closed selection capacity.
// Explicit selections are never truncated.
var ErrScopeCapacity = errors.New("selection capacity exceeded")

const (
	// MaxSelectedRepositories bounds the distinct repositories of one selection.
	MaxSelectedRepositories = 20
	// MaxScopes bounds the repository and path Scopes of one selection.
	MaxScopes = 100
	// MaxPathMatchers bounds the path matchers of one selection. Every path Scope
	// carries exactly one matcher, so MaxScopes enforces this bound structurally
	// while the two closed capacities stay equal.
	MaxPathMatchers = 100
)

// ScopeKind distinguishes the two Scope kinds of the unified selection model.
type ScopeKind int

const (
	// ScopeRepository selects all eligible activity for one repository.
	ScopeRepository ScopeKind = iota
	// ScopePath selects activity in one repository whose known changed-file set
	// matches the Scope's path matcher.
	ScopePath
)

// String names the Scope kind.
func (k ScopeKind) String() string {
	switch k {
	case ScopeRepository:
		return "repository"
	case ScopePath:
		return "path"
	default:
		return fmt.Sprintf("scope kind %d", int(k))
	}
}

// ScopeIdentity is the stable typed identity used to deduplicate, order,
// aggregate, label, and retain view state for a Scope. It is deliberately
// comparable so it can key domain state. It is never an enrichment cache key.
type ScopeIdentity struct {
	Kind          ScopeKind
	RepositoryKey string
	Matcher       string
}

// Scope is the source-independent selection unit: a repository identity and,
// for a path Scope, one path matcher. It retains the first requested spelling for
// presentation while identity stays canonical.
type Scope struct {
	kind       ScopeKind
	repository Repository
	matcher    PathMatcher
}

// NewRepositoryScope returns the Scope selecting all activity of one repository.
func NewRepositoryScope(repository Repository) Scope {
	return Scope{kind: ScopeRepository, repository: repository}
}

// NewPathScope returns the Scope selecting activity of one repository whose known
// changed-file set matches the matcher.
func NewPathScope(repository Repository, matcher PathMatcher) (Scope, error) {
	if matcher.IsZero() {
		return Scope{}, fmt.Errorf("%w: a path Scope requires a matcher", ErrInvalidMatcher)
	}
	return Scope{kind: ScopePath, repository: repository, matcher: matcher}, nil
}

// Kind reports whether the Scope is a repository or a path Scope.
func (s Scope) Kind() ScopeKind { return s.kind }

// Repository returns the repository the Scope belongs to.
func (s Scope) Repository() Repository { return s.repository }

// Matcher returns the path matcher of a path Scope and the zero matcher of a
// repository Scope.
func (s Scope) Matcher() PathMatcher { return s.matcher }

// Identity returns the canonical Scope identity.
func (s Scope) Identity() ScopeIdentity {
	return ScopeIdentity{
		Kind:          s.kind,
		RepositoryKey: s.repository.Key(),
		Matcher:       s.matcher.Identity(),
	}
}

// Equal reports whether both Scopes share one canonical identity.
func (s Scope) Equal(other Scope) bool { return s.Identity() == other.Identity() }

// String renders the requested repository and pattern spelling.
func (s Scope) String() string {
	if s.kind == ScopePath {
		return s.repository.String() + ":" + s.matcher.String()
	}
	return s.repository.String()
}

// CompareScopes orders Scopes by the stable Scope identity order: lowercase
// repository identity in byte order, the repository Scope before path Scopes of
// the same repository, then canonical matcher-token encoding in byte order.
func CompareScopes(a, b Scope) int {
	if order := strings.Compare(a.repository.Key(), b.repository.Key()); order != 0 {
		return order
	}
	if order := cmp.Compare(int(a.kind), int(b.kind)); order != 0 {
		return order
	}
	return strings.Compare(a.matcher.Identity(), b.matcher.Identity())
}

// ScopeSet is the validated, deduplicated selection. It keeps the first
// requested spelling of every Scope and the request order, and exposes the stable
// presentation order separately.
type ScopeSet struct {
	scopes       []Scope
	identities   map[ScopeIdentity]struct{}
	repositories []Repository
	keys         map[string]struct{}
}

// NewScopeSet deduplicates the requested Scopes by canonical identity, retaining
// the first occurrence and its requested spelling, and enforces the closed
// selection capacities before any network work.
func NewScopeSet(scopes []Scope) (ScopeSet, error) {
	if len(scopes) == 0 {
		return ScopeSet{}, ErrEmptyScope
	}
	set := ScopeSet{
		scopes:     make([]Scope, 0, len(scopes)),
		identities: make(map[ScopeIdentity]struct{}, len(scopes)),
		keys:       make(map[string]struct{}, len(scopes)),
	}
	for _, scope := range scopes {
		if scope.repository == (Repository{}) {
			return ScopeSet{}, fmt.Errorf("%w: a Scope requires a repository", ErrInvalidRepository)
		}
		identity := scope.Identity()
		if _, duplicate := set.identities[identity]; duplicate {
			continue
		}
		set.identities[identity] = struct{}{}
		set.scopes = append(set.scopes, scope)
		key := scope.repository.Key()
		if _, seen := set.keys[key]; !seen {
			set.keys[key] = struct{}{}
			set.repositories = append(set.repositories, scope.repository)
		}
	}
	if err := CheckSelectionCapacity(len(set.repositories), len(set.scopes)); err != nil {
		return ScopeSet{}, err
	}
	return set, nil
}

// NewRepositoryScopeSet validates repository identifiers and returns the
// repository-only selection they describe.
func NewRepositoryScopeSet(values []string) (ScopeSet, error) {
	if len(values) == 0 {
		return ScopeSet{}, ErrEmptyScope
	}
	scopes := make([]Scope, 0, len(values))
	for _, value := range values {
		repository, err := ParseRepository(value)
		if err != nil {
			return ScopeSet{}, err
		}
		scopes = append(scopes, NewRepositoryScope(repository))
	}
	return NewScopeSet(scopes)
}

// Scopes returns the selected Scopes in request order.
func (s ScopeSet) Scopes() []Scope { return slices.Clone(s.scopes) }

// Ordered returns the selected Scopes in the stable Scope identity order.
func (s ScopeSet) Ordered() []Scope {
	ordered := slices.Clone(s.scopes)
	slices.SortStableFunc(ordered, CompareScopes)
	return ordered
}

// Repositories returns the distinct selected repositories in request order,
// keeping the first requested spelling.
func (s ScopeSet) Repositories() []Repository { return slices.Clone(s.repositories) }

// Len returns the number of selected Scopes.
func (s ScopeSet) Len() int { return len(s.scopes) }

// Contains reports whether the repository is selected by any Scope.
func (s ScopeSet) Contains(repository Repository) bool {
	_, selected := s.keys[repository.Key()]
	return selected
}

// Filter returns the events whose repository is selected, preserving the input
// order. The input is not modified. Path membership is evaluated separately once
// changed-file evidence is known.
func (s ScopeSet) Filter(events []Event) []Event {
	if len(events) == 0 {
		return nil
	}
	filtered := make([]Event, 0, len(events))
	for _, event := range events {
		if s.Contains(event.Repository) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// CheckSelectionCapacity reports whether a selection of the given distinct
// repository and Scope counts fits the closed selection capacities. Callers
// expanding a selection use it to reject an oversized request before the
// expansion is materialized; explicit selections are never truncated.
func CheckSelectionCapacity(repositories, scopes int) error {
	if repositories > MaxSelectedRepositories {
		return fmt.Errorf("%w: %d distinct repositories requested, at most %d are allowed", ErrScopeCapacity, repositories, MaxSelectedRepositories)
	}
	if scopes > MaxScopes {
		return fmt.Errorf("%w: %d scopes requested, at most %d are allowed", ErrScopeCapacity, scopes, MaxScopes)
	}
	return nil
}

// HasPathScopes reports whether the selection holds a path Scope and therefore
// needs changed-file evidence. A repository-only selection is decided from
// repository identity alone, so it performs no enrichment (FR-003).
func (s ScopeSet) HasPathScopes() bool {
	for _, scope := range s.scopes {
		if scope.kind == ScopePath {
			return true
		}
	}
	return false
}
