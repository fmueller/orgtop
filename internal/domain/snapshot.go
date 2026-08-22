package domain

import (
	"slices"
	"strings"
)

// MaxSnapshotEvents bounds the in-memory snapshot after combination (FR-006).
const MaxSnapshotEvents = 500

// RepositoryActivity is the successful result for one Scope entry. Repository is
// the display identity for this refresh: the returned spelling, or the requested
// spelling when the page is empty (FR-002).
type RepositoryActivity struct {
	Repository Repository
	Events     []Event
}

// Aggregate holds the direct per-repository counts shown in Overview (FR-006).
// Review and pull-request comment events count as pull-request activity;
// unrelated issue comments do not.
type Aggregate struct {
	// Repository is the display identity of the selected repository.
	Repository Repository
	// Total is the number of snapshot events for the repository.
	Total int
	// Pushes is the number of push events for the repository.
	Pushes int
	// PullRequestActivity is the number of pull-request related events.
	PullRequestActivity int
}

// Snapshot is the latest completely successful, filtered, deduplicated,
// reverse-chronological event set and its repository aggregates. It is built
// once and read many times; accessors never expose its backing arrays.
type Snapshot struct {
	events     []Event
	aggregates []Aggregate
}

// NewSnapshot filters the returned activity to the Scope, then deduplicates,
// sorts, and bounds it before aggregating the retained events. Every Scope entry
// yields an aggregate, including repositories with no returned events. Callers
// report a repository at most once; a repeated repository keeps its first
// returned display identity. Activity outside the Scope is ignored, and the
// inputs are not modified.
func NewSnapshot(scope Scope, activities []RepositoryActivity) Snapshot {
	var candidates []Event
	for _, activity := range activities {
		candidates = append(candidates, activity.Events...)
	}

	events := SortByRecency(Deduplicate(scope.Filter(candidates)))
	if len(events) > MaxSnapshotEvents {
		events = events[:MaxSnapshotEvents]
	}
	return Snapshot{events: events, aggregates: aggregate(scope, activities, events)}
}

// Events returns the bounded reverse-chronological events.
func (s Snapshot) Events() []Event { return slices.Clone(s.events) }

// Aggregates returns the per-repository rows ordered by total descending, then
// by display identity case-insensitively.
func (s Snapshot) Aggregates() []Aggregate { return slices.Clone(s.aggregates) }

// aggregate counts the retained events per Scope entry and orders the rows. A
// Scope entry keeps its requested spelling until the refresh returns one, and
// the first returned spelling wins if a caller reports a repository more than
// once.
func aggregate(scope Scope, activities []RepositoryActivity, events []Event) []Aggregate {
	identities := make(map[string]Repository, len(activities))
	for _, activity := range activities {
		key := activity.Repository.Key()
		if _, returned := identities[key]; returned {
			continue
		}
		identities[key] = activity.Repository
	}

	repositories := scope.Repositories()
	aggregates := make([]Aggregate, 0, len(repositories))
	positions := make(map[string]int, len(repositories))
	for _, requested := range repositories {
		key := requested.Key()
		identity, returned := identities[key]
		if !returned {
			identity = requested
		}
		positions[key] = len(aggregates)
		aggregates = append(aggregates, Aggregate{Repository: identity})
	}

	for _, event := range events {
		position, selected := positions[event.Repository.Key()]
		if !selected {
			continue
		}
		aggregates[position].Total++
		if event.Category == CategoryPush {
			aggregates[position].Pushes++
		}
		if isPullRequestActivity(event) {
			aggregates[position].PullRequestActivity++
		}
	}

	slices.SortStableFunc(aggregates, func(a, b Aggregate) int {
		if byTotal := b.Total - a.Total; byTotal != 0 {
			return byTotal
		}
		return strings.Compare(a.Repository.Key(), b.Repository.Key())
	})
	return aggregates
}

// isPullRequestActivity reports whether the event counts as pull-request
// activity under FR-006.
func isPullRequestActivity(event Event) bool {
	switch event.Category {
	case CategoryPullRequest, CategoryReview:
		return true
	case CategoryComment:
		return event.EntityKind == EntityPullRequest
	default:
		return false
	}
}
