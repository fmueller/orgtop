package cli

import (
	"fmt"

	"github.com/fmueller/orgtop/internal/domain"
)

// selectionKind names the two selection flags whose values expand into Scopes.
type selectionKind int

const (
	repositorySelection selectionKind = iota
	pathSelectionKind
)

// selectionValue is one requested selection flag value in argument order, so
// left-to-right scanning reports the first malformed value whichever flag
// carries it (RG-001).
type selectionValue struct {
	kind  selectionKind
	value string
}

// selections collects every selection flag value in argument order.
type selections []selectionValue

// value adapts one selection flag to flag.Value while keeping the shared
// argument order.
type selectionFlag struct {
	kind      selectionKind
	collected *selections
}

func (f selectionFlag) String() string { return "" }

func (f selectionFlag) Set(value string) error {
	*f.collected = append(*f.collected, selectionValue{kind: f.kind, value: value})
	return nil
}

// requested is the validated, order-preserving expansion input: the distinct
// repositories of exact --repo values, the distinct bare patterns that filter
// them, and the qualified path Scopes that stand alone.
type requested struct {
	repositories []domain.Repository
	bare         []domain.PathMatcher
	qualified    []domain.Scope
}

// expandSelections validates every selection value in argument order and expands
// the closed RG-001 composition into a deduplicated ScopeSet.
func expandSelections(values selections) (domain.ScopeSet, error) {
	requested, err := validateSelections(values)
	if err != nil {
		return domain.ScopeSet{}, err
	}
	if len(requested.bare) > 0 && len(requested.repositories) == 0 {
		return domain.ScopeSet{}, ErrBarePathWithoutRepository
	}
	if len(requested.repositories) == 0 && len(requested.qualified) == 0 {
		return domain.ScopeSet{}, ErrMissingRepository
	}
	// Bounding the expansion inputs before the product keeps a capacity
	// diagnostic deterministic and cheap: every count here is already a lower
	// bound on the canonical Scope count, which is never truncated.
	if err := requested.checkCapacity(); err != nil {
		return domain.ScopeSet{}, err
	}
	scopes, err := requested.expand()
	if err != nil {
		return domain.ScopeSet{}, err
	}
	return domain.NewScopeSet(scopes)
}

// validateSelections parses each value where it occurs, so the first malformed
// argument is the reported one, and deduplicates the expansion inputs while
// retaining the first requested spelling.
func validateSelections(values selections) (requested, error) {
	var out requested
	repositoryKeys := make(map[string]struct{})
	patterns := make(map[string]struct{})
	qualified := make(map[domain.ScopeIdentity]struct{})

	for _, selection := range values {
		if selection.kind == repositorySelection {
			repository, err := domain.ParseRepository(selection.value)
			if err != nil {
				return requested{}, fmt.Errorf("--%s: %w", repoFlag, err)
			}
			if firstSeen(repositoryKeys, repository.Key()) {
				out.repositories = append(out.repositories, repository)
			}
			continue
		}

		path, err := parsePathValue(selection.value)
		if err != nil {
			return requested{}, fmt.Errorf("--%s: %w", pathFlag, err)
		}
		if !path.qualified {
			if firstSeen(patterns, path.matcher.Identity()) {
				out.bare = append(out.bare, path.matcher)
			}
			continue
		}
		scope, err := domain.NewPathScope(path.repository, path.matcher)
		if err != nil {
			return requested{}, fmt.Errorf("--%s: %w", pathFlag, err)
		}
		if firstSeen(qualified, scope.Identity()) {
			out.qualified = append(out.qualified, scope)
		}
	}
	return out, nil
}

// firstSeen reports whether key is new to seen and records it when it is, so a
// repeated selection keeps its first requested spelling and position.
func firstSeen[K comparable](seen map[K]struct{}, key K) bool {
	if _, duplicate := seen[key]; duplicate {
		return false
	}
	seen[key] = struct{}{}
	return true
}

// checkCapacity rejects an expansion that exceeds a closed RG-009 capacity
// before the product is materialized. It counts the Scopes the invocation
// actually requests, so the reported count is the one the contract names.
func (r requested) checkCapacity() error {
	return domain.CheckSelectionCapacity(len(r.repositories), r.scopeCount())
}

// scopeCount is the number of distinct Scopes the expansion produces. Both
// expansion inputs are already deduplicated, so the product is distinct by
// construction and only a qualified Scope the product already covers can repeat.
func (r requested) scopeCount() int {
	if len(r.bare) == 0 {
		// Repository Scopes never share an identity with a path Scope.
		return len(r.repositories) + len(r.qualified)
	}
	repositories := make(map[string]struct{}, len(r.repositories))
	for _, repository := range r.repositories {
		repositories[repository.Key()] = struct{}{}
	}
	patterns := make(map[string]struct{}, len(r.bare))
	for _, matcher := range r.bare {
		patterns[matcher.Identity()] = struct{}{}
	}

	count := len(r.repositories) * len(r.bare)
	for _, scope := range r.qualified {
		_, filtered := repositories[scope.Repository().Key()]
		_, patterned := patterns[scope.Matcher().Identity()]
		if !filtered || !patterned {
			count++
		}
	}
	return count
}

// expand produces the requested Scopes in expansion order: bare patterns filter
// every exact repository as the Cartesian product with repositories as the outer
// sequence, and in that form a repository supplies no repository Scope. Qualified
// path Scopes follow in request order.
func (r requested) expand() ([]domain.Scope, error) {
	scopes := make([]domain.Scope, 0, len(r.repositories)*max(len(r.bare), 1)+len(r.qualified))
	for _, repository := range r.repositories {
		if len(r.bare) == 0 {
			scopes = append(scopes, domain.NewRepositoryScope(repository))
			continue
		}
		for _, matcher := range r.bare {
			scope, err := domain.NewPathScope(repository, matcher)
			if err != nil {
				return nil, fmt.Errorf("--%s: %w", pathFlag, err)
			}
			scopes = append(scopes, scope)
		}
	}
	return append(scopes, r.qualified...), nil
}
