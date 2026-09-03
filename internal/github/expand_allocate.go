package github

import (
	"slices"
	"strings"

	"github.com/fmueller/orgtop/internal/domain"
)

// allocate turns the fetched listings into canonical repository Scopes and the
// prepared disclosure state. Exact Scopes keep unconditional capacity
// precedence, so expansion only receives the capacity they leave.
func allocate(request ExpansionRequest, listings []*listing) (Expansion, error) {
	selected := newSelection(request.Exact)
	exact := len(selected.scopes)

	queues := make([][]domain.Repository, len(listings))
	reports := make([]SelectorSelection, len(listings))
	for index, entry := range listings {
		queues[index], reports[index] = entry.eligible(request)
	}

	// Allocation visits one next candidate per selector in first-request
	// round-robin order until every fetched candidate is examined. A candidate
	// that no longer fits is not admitted, but the round continues so every
	// selector's provenance and omission counts stay complete.
	for turn := range allocationRounds(queues) {
		for index, queue := range queues {
			if turn >= len(queue) {
				continue
			}
			if selected.admit(queue[turn], listings[index].organization) {
				reports[index].Retained++
			} else {
				reports[index].Omitted++
			}
		}
	}

	scopes, err := selected.scopeSet()
	if err != nil {
		return Expansion{}, err
	}
	return Expansion{
		Scopes: scopes,
		Selection: Selection{
			ExactScopes:          exact,
			ExpandedScopes:       len(selected.scopes) - exact,
			TotalScopes:          len(selected.scopes),
			DistinctRepositories: len(selected.repositories),
			Selectors:            reports,
			Provenance:           selected.provenance,
			PaginationRemains:    paginationRemains(reports),
		},
	}, nil
}

// allocationRounds is the number of round-robin turns a complete allocation
// takes: the longest selector queue, so every fetched candidate is examined.
func allocationRounds(queues [][]domain.Repository) int {
	rounds := 0
	for _, queue := range queues {
		rounds = max(rounds, len(queue))
	}
	return rounds
}

// eligible returns the selector's eligible candidates in canonical order and
// the disclosure counts of its fetched records. The exact listing booleans are
// the complete eligibility filter: no visibility, activity, or permission fact
// removes a listed repository and no extra probe is made.
func (l *listing) eligible(request ExpansionRequest) ([]domain.Repository, SelectorSelection) {
	report := SelectorSelection{
		Organization: l.organization,
		Listed:       len(l.records),
		HasMore:      l.url != "",
	}
	candidates := make([]domain.Repository, 0, len(l.records))
	for _, record := range l.records {
		// The exclusion buckets are disjoint in this precedence, so their sum
		// plus the eligible count is exactly the listed count.
		switch {
		case record.disabled:
			report.Disabled++
		case record.archived && !request.IncludeArchived:
			report.Archived++
		case record.fork && !request.IncludeForks:
			report.Fork++
		default:
			candidates = append(candidates, record.repository)
		}
	}
	report.Eligible = len(candidates)
	slices.SortStableFunc(candidates, func(a, b domain.Repository) int {
		return strings.Compare(a.Key(), b.Key())
	})
	return candidates, report
}

// paginationRemains reports the derived global condition that at least one
// selector left a valid next page link unconsumed.
func paginationRemains(reports []SelectorSelection) bool {
	return slices.ContainsFunc(reports, func(report SelectorSelection) bool {
		return report.HasMore
	})
}

// selection accumulates the published Scopes, their provenance, and the
// capacities they consume.
type selection struct {
	scopes       []domain.Scope
	provenance   []ScopeProvenance
	index        map[domain.ScopeIdentity]int
	repositories map[string]struct{}
}

// newSelection seeds the selection with the exact Scopes in request order.
func newSelection(exact domain.ScopeSet) *selection {
	scopes := exact.Scopes()
	selected := &selection{
		scopes:       make([]domain.Scope, 0, len(scopes)),
		provenance:   make([]ScopeProvenance, 0, len(scopes)),
		index:        make(map[domain.ScopeIdentity]int, len(scopes)),
		repositories: make(map[string]struct{}, len(scopes)),
	}
	for _, scope := range scopes {
		selected.record(scope, ScopeProvenance{Scope: scope, Exact: true})
	}
	return selected
}

// admit adds the expanded repository Scope for one candidate and reports
// whether the candidate is retained. A candidate whose repository Scope already
// exists consumes its turn, gains selector provenance, and creates no second
// Scope; a candidate that would exceed either remaining capacity is omitted and
// evicts nothing.
func (s *selection) admit(repository domain.Repository, organization string) bool {
	scope := domain.NewRepositoryScope(repository)
	if index, exists := s.index[scope.Identity()]; exists {
		if s.provenance[index].Selector == "" {
			s.provenance[index].Selector = organization
		}
		return true
	}
	if len(s.scopes) >= domain.MaxScopes {
		return false
	}
	_, referenced := s.repositories[repository.Key()]
	if !referenced && len(s.repositories) >= domain.MaxSelectedRepositories {
		return false
	}
	s.record(scope, ScopeProvenance{Scope: scope, Selector: organization})
	return true
}

// record publishes one Scope and the capacities it consumes.
func (s *selection) record(scope domain.Scope, provenance ScopeProvenance) {
	s.index[scope.Identity()] = len(s.scopes)
	s.scopes = append(s.scopes, scope)
	s.provenance = append(s.provenance, provenance)
	s.repositories[scope.Repository().Key()] = struct{}{}
}

// scopeSet returns the published selection. A successful expansion that selects
// nothing is an explicit empty selection rather than a rejected one.
func (s *selection) scopeSet() (domain.ScopeSet, error) {
	if len(s.scopes) == 0 {
		return domain.ScopeSet{}, nil
	}
	return domain.NewScopeSet(s.scopes)
}
