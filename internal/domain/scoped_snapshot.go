package domain

import "slices"

// EventEvidence pairs one normalized event with the settled changed-file
// evidence outcome of its refresh. Evidence is settled before a snapshot is
// built, so the snapshot never triggers or waits for enrichment.
type EventEvidence struct {
	// Event is the normalized event.
	Event Event
	// Outcome is that event's settled changed-file evidence result.
	Outcome EvidenceOutcome
}

// ScopedActivity is the successful result for one Scope entry together with the
// settled evidence of every returned event. A Scope row's display identity comes
// from the Scope's own retained spelling, so the grouping carries no separate
// repository identity of its own.
type ScopedActivity struct {
	// Events are the returned events and their settled evidence.
	Events []EventEvidence
}

// ScopedEvent is one retained event and its explicit outcome in every selected
// Scope of its repository, in the stable Scope identity order. Rendering reads
// these prepared outcomes; it never matches or infers membership itself.
type ScopedEvent struct {
	// Event is the retained normalized event.
	Event Event
	// Memberships are the event's per-Scope outcomes in stable identity order.
	Memberships []ScopeMembership
}

// ScopeAggregate holds one Scope's direct counts over the bounded snapshot.
// Activity counts only confirmed members and is therefore a lower bound with
// respect to Unknown; NotMember and Unknown never contribute to it.
type ScopeAggregate struct {
	// Scope is the selected Scope this row belongs to.
	Scope Scope
	// Activity is the number of retained events with a member outcome.
	Activity int
	// Pushes is the number of member push events.
	Pushes int
	// PullRequestActivity is the number of member pull-request related events.
	PullRequestActivity int
	// NotMember is the number of retained events decided as not-member.
	NotMember int
	// Unknown is the number of retained events whose outcome stayed undecided.
	Unknown int
	// Evaluated is the number of retained events evaluated against the Scope.
	Evaluated int
	// CurrentPR is the number of member outcomes decided from complete current-PR
	// evidence. It counts a subset of Activity and is never event-time evidence.
	CurrentPR int

	// unknownByReason counts unknowns per primary reason group. It is an array
	// rather than a map so a copied row can never share counts with the snapshot.
	unknownByReason [unknownReasonSlots]int
}

// unknownReasonSlots sizes the per-reason unknown counters, covering ReasonNone
// and every group of the closed RG-004 breakdown.
const unknownReasonSlots = int(ReasonFailed) + 1

// UnknownBy reports how many unknown outcomes the Scope holds for one primary
// reason group. Presentation walks UnknownReasons for the closed breakdown order.
func (a ScopeAggregate) UnknownBy(reason UnknownReason) int {
	if int(reason) >= unknownReasonSlots {
		return 0
	}
	return a.unknownByReason[reason]
}

// UnknownReasonCounts returns the Scope's unknown counts keyed by reason group,
// omitting the groups it holds none of. The result is a fresh map, so a caller
// reading or reshaping it never reaches the snapshot's own counters.
func (a ScopeAggregate) UnknownReasonCounts() map[UnknownReason]int {
	counts := make(map[UnknownReason]int, unknownReasonSlots)
	for _, reason := range UnknownReasons() {
		if count := a.UnknownBy(reason); count > 0 {
			counts[reason] = count
		}
	}
	return counts
}

// ScopedSnapshot is the immutable per-refresh application snapshot: the bounded
// reverse-chronological event set, every event's per-Scope membership, and the
// direct per-Scope aggregates. It is built once and read many times; accessors
// never expose its backing arrays.
type ScopedSnapshot struct {
	events      []ScopedEvent
	aggregates  []ScopeAggregate
	truncated   bool
	overlapping bool
	distinct    int
}

// NewScopedSnapshot filters the returned activity to the Scope set's
// repositories, deduplicates, sorts, and bounds it exactly as the v0.1 snapshot
// does, then builds the snapshot over the retained set. A refresh that enriches
// before it aggregates retains first through Retain and publishes through
// NewRetainedSnapshot instead.
func NewScopedSnapshot(scope ScopeSet, activities []ScopedActivity) ScopedSnapshot {
	outcomes := make(map[string]EvidenceOutcome, len(activities))
	var candidates []Event
	for _, activity := range activities {
		for _, evidence := range activity.Events {
			if _, seen := outcomes[evidence.Event.ID]; seen {
				continue
			}
			outcomes[evidence.Event.ID] = evidence.Outcome
			candidates = append(candidates, evidence.Event)
		}
	}

	// The outcome map already kept the first evidence of a repeated source ID, so
	// the retention below deduplicates nothing new here. It stays because the
	// retained event set is the v0.1 pipeline's, and both paths must keep the
	// same first-occurrence rule.
	bounded, truncated := retain(scope, candidates)
	retained := make([]EventEvidence, 0, len(bounded))
	for _, event := range bounded {
		retained = append(retained, EventEvidence{Event: event, Outcome: outcomes[event.ID]})
	}
	return NewRetainedSnapshot(scope, retained, truncated)
}

// Retain returns the bounded, deduplicated, reverse-chronological event set one
// refresh publishes and whether the FR-006 bound discarded events. A refresh
// enriches exactly this set, so an event the bound discarded causes no cache,
// enrichment, or matcher work (A-028). The inputs are not modified.
func Retain(scope ScopeSet, activities []RepositoryActivity) ([]Event, bool) {
	var candidates []Event
	for _, activity := range activities {
		candidates = append(candidates, activity.Events...)
	}
	return retain(scope, candidates)
}

// retain applies the shared filtering, deduplication, ordering, and bound to
// already collected candidates.
func retain(scope ScopeSet, candidates []Event) ([]Event, bool) {
	bounded := SortByRecency(Deduplicate(scope.Filter(candidates)))
	if len(bounded) > MaxSnapshotEvents {
		return bounded[:MaxSnapshotEvents], true
	}
	return bounded, false
}

// NewRetainedSnapshot evaluates an already retained event set against every
// selected Scope of its repository and aggregates the settled outcomes. It
// carries the truncation Retain decided, so a snapshot built after enrichment
// still reports the events the bound discarded before enrichment ran.
//
// Every retained event keeps one explicit outcome per selected Scope of its
// repository, so not-member and unknown stay countable. Canceled evidence
// publishes no membership at all rather than a synthesized unknown, so such an
// event is retained nowhere. The inputs are not modified.
func NewRetainedSnapshot(scope ScopeSet, retained []EventEvidence, truncated bool) ScopedSnapshot {
	var events []ScopedEvent
	for _, evidence := range retained {
		memberships, published := scope.Evaluate(evidence.Event, evidence.Outcome)
		if !published {
			continue
		}
		events = append(events, ScopedEvent{Event: evidence.Event, Memberships: memberships})
	}

	distinct, overlapping := countDistinctMembership(events)
	return ScopedSnapshot{
		events:      events,
		aggregates:  aggregateScopes(scope, events),
		truncated:   truncated,
		overlapping: overlapping,
		distinct:    distinct,
	}
}

// Events returns the bounded reverse-chronological retained events, including
// the ones no Scope confirmed. Aggregation counts every one of them.
func (s ScopedSnapshot) Events() []Event {
	events := make([]Event, 0, len(s.events))
	for _, scoped := range s.events {
		events = append(events, scoped.Event)
	}
	return events
}

// StreamEvents returns the retained events Stream includes under RG-004: an
// event a Scope confirmed, or one left unknown by a path Scope and therefore
// investigatory context. An event only not-member everywhere is omitted.
func (s ScopedSnapshot) StreamEvents() []Event {
	events := make([]Event, 0, len(s.events))
	for _, scoped := range s.events {
		if included(scoped.Memberships) {
			events = append(events, scoped.Event)
		}
	}
	return events
}

// ScopedEvents returns the retained events with their per-Scope memberships.
func (s ScopedSnapshot) ScopedEvents() []ScopedEvent {
	scoped := slices.Clone(s.events)
	for index := range scoped {
		scoped[index].Memberships = slices.Clone(scoped[index].Memberships)
	}
	return scoped
}

// Aggregates returns the prepared Scope rows in RG-012 order: confirmed activity
// descending, then the stable Scope identity order.
func (s ScopedSnapshot) Aggregates() []ScopeAggregate { return slices.Clone(s.aggregates) }

// Truncated reports whether the FR-006 bound discarded events.
func (s ScopedSnapshot) Truncated() bool { return s.truncated }

// DistinctActivity is the number of retained events with a member outcome in at
// least one Scope. It is the only deduplicated activity total; summing Scope
// rows double counts overlapping membership.
func (s ScopedSnapshot) DistinctActivity() int { return s.distinct }

// Overlapping reports whether any retained event is a member of more than one
// Scope, so product copy never presents a sum of Scope rows as a deduplicated
// organization total.
func (s ScopedSnapshot) Overlapping() bool { return s.overlapping }

// included reports whether Stream shows an evaluated event: it is a member of
// some Scope, or unknown for some Scope and therefore investigatory context. An
// event only not-member everywhere is omitted.
func included(memberships []ScopeMembership) bool {
	for _, membership := range memberships {
		if membership.Membership.Kind() != MembershipNotMember {
			return true
		}
	}
	return false
}

// countDistinctMembership reports the deduplicated member event count and
// whether any event is a member of more than one Scope.
func countDistinctMembership(events []ScopedEvent) (int, bool) {
	distinct, overlapping := 0, false
	for _, event := range events {
		members := 0
		for _, membership := range event.Memberships {
			if membership.Membership.IsMember() {
				members++
			}
		}
		if members > 0 {
			distinct++
		}
		if members > 1 {
			overlapping = true
		}
	}
	return distinct, overlapping
}

// aggregateScopes counts every retained outcome per selected Scope and orders
// the rows. Every configured Scope keeps a row, including zero-activity and
// all-unknown ones, so a view never renders a selection as absent.
func aggregateScopes(scope ScopeSet, events []ScopedEvent) []ScopeAggregate {
	ordered := scope.Ordered()
	aggregates := make([]ScopeAggregate, 0, len(ordered))
	positions := make(map[ScopeIdentity]int, len(ordered))
	for _, selected := range ordered {
		positions[selected.Identity()] = len(aggregates)
		aggregates = append(aggregates, ScopeAggregate{Scope: selected})
	}

	for _, event := range events {
		for _, membership := range event.Memberships {
			position, selected := positions[membership.Scope.Identity()]
			if !selected {
				continue
			}
			aggregates[position].count(event.Event, membership.Membership)
		}
	}

	slices.SortStableFunc(aggregates, func(a, b ScopeAggregate) int {
		if byActivity := b.Activity - a.Activity; byActivity != 0 {
			return byActivity
		}
		return CompareScopes(a.Scope, b.Scope)
	})
	return aggregates
}

// count records one settled outcome in the Scope's direct counts.
func (a *ScopeAggregate) count(event Event, membership Membership) {
	a.Evaluated++
	switch membership.Kind() {
	case MembershipMember:
		a.Activity++
		if event.Category == CategoryPush {
			a.Pushes++
		}
		if isPullRequestActivity(event) {
			a.PullRequestActivity++
		}
		if membership.QualifiedCurrentPR() {
			a.CurrentPR++
		}
	case MembershipNotMember:
		a.NotMember++
	case MembershipUnknown:
		a.Unknown++
		a.unknownByReason[membership.Reason()]++
	}
}
