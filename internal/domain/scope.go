package domain

import (
	"errors"
	"fmt"
	"slices"
)

// ErrEmptyScope reports that no repository was selected. OrgTop requires at least
// one repository.
var ErrEmptyScope = errors.New("no repository selected")

// Scope is the deduplicated inclusion set of selected repositories. It keeps the
// first requested spelling of each repository and matches case-insensitively.
type Scope struct {
	repositories []Repository
	keys         map[string]struct{}
}

// NewScope validates the requested identifiers and collapses case-insensitive
// duplicates, retaining the first requested spelling and the request order.
func NewScope(values []string) (Scope, error) {
	if len(values) == 0 {
		return Scope{}, ErrEmptyScope
	}
	repositories := make([]Repository, 0, len(values))
	keys := make(map[string]struct{}, len(values))
	for _, value := range values {
		repository, err := ParseRepository(value)
		if err != nil {
			return Scope{}, fmt.Errorf("repository scope: %w", err)
		}
		key := repository.Key()
		if _, duplicate := keys[key]; duplicate {
			continue
		}
		keys[key] = struct{}{}
		repositories = append(repositories, repository)
	}
	return Scope{repositories: repositories, keys: keys}, nil
}

// Repositories returns the selected repositories in request order.
func (s Scope) Repositories() []Repository {
	return slices.Clone(s.repositories)
}

// Len returns the number of selected repositories.
func (s Scope) Len() int { return len(s.repositories) }

// Contains reports whether the repository belongs to the Scope.
func (s Scope) Contains(repository Repository) bool {
	_, selected := s.keys[repository.Key()]
	return selected
}

// Filter returns the events whose repository belongs to the Scope, preserving the
// input order. The input is not modified.
func (s Scope) Filter(events []Event) []Event {
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
