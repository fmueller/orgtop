package domain

import (
	"slices"
	"strings"
	"time"
)

// Category classifies a normalized event for aggregation and display.
type Category string

// v0.1.0 event categories.
const (
	CategoryPush        Category = "push"
	CategoryPullRequest Category = "pull_request"
	CategoryReview      Category = "review"
	CategoryComment     Category = "comment"
	CategoryOther       Category = "other"
)

// EntityKind names the kind of thing an event refers to.
type EntityKind string

// v0.1.0 entity kinds.
const (
	EntityRepository  EntityKind = "repository"
	EntityCommit      EntityKind = "commit"
	EntityPullRequest EntityKind = "pull_request"
	EntityOther       EntityKind = "other"
)

// Event is a source-independent OrgTop occurrence. Actor, EntityRef, and
// Description are optional detail; ID, OccurredAt, Repository, Category, and
// EntityKind are always populated by a successful normalization.
type Event struct {
	// ID is the stable source event ID used for deduplication.
	ID string
	// OccurredAt is the instant the event happened at the source.
	OccurredAt time.Time
	// Repository is the canonical repository the event belongs to.
	Repository Repository
	// Actor is the optional human-readable originator.
	Actor string
	// Category classifies the event.
	Category Category
	// EntityKind names the kind of entity the event refers to.
	EntityKind EntityKind
	// EntityRef is the optional reference to that entity.
	EntityRef string
	// Description is concise human-readable detail.
	Description string
}

// Deduplicate returns the events with repeated source event IDs removed, keeping
// the first occurrence of each ID and the input order. The input is not modified.
func Deduplicate(events []Event) []Event {
	if len(events) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(events))
	unique := make([]Event, 0, len(events))
	for _, event := range events {
		if _, duplicate := seen[event.ID]; duplicate {
			continue
		}
		seen[event.ID] = struct{}{}
		unique = append(unique, event)
	}
	return unique
}

// SortByRecency returns the events ordered by timestamp descending and then by
// source event ID ascending. The input is not modified.
func SortByRecency(events []Event) []Event {
	if len(events) == 0 {
		return nil
	}
	sorted := slices.Clone(events)
	slices.SortStableFunc(sorted, func(a, b Event) int {
		if byRecency := b.OccurredAt.Compare(a.OccurredAt); byRecency != 0 {
			return byRecency
		}
		return strings.Compare(a.ID, b.ID)
	})
	return sorted
}
