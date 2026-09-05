package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// The closed RG-007 Interesting Now bounds: an event is a candidate while its
// effective strip age stays inside the fixed 15-minute window, a selection
// stores at most 20 entries, and at most the first five stored entries are
// visible when height permits.
const (
	stripWindow  = 15 * time.Minute
	stripStored  = 20
	stripVisible = 5
)

// interestingScope is one confirmed matching Scope of an entry together with
// that Scope's current-PR qualification. Membership and qualification are
// per Scope, so an entry matching three Scopes carries three of these rather
// than one collapsed verdict.
type interestingScope struct {
	scope     domain.Scope
	qualified bool
}

// interestingEntry is one stored strip entry. Every field is a direct
// normalized or application fact of RG-007's entry explanation; nothing here is
// derived, compared against a baseline, or ordered by a judgement of its own.
type interestingEntry struct {
	// id is the normalized source event ID, which is the strip identity.
	id string
	// occurredAt is the source instant the entry's age is measured from.
	occurredAt time.Time
	// repository is the canonical repository the event belongs to.
	repository domain.Repository
	// actor is the optional human-readable originator.
	actor string
	// category is the normalized shared category the glyph vocabulary draws.
	category domain.Category
	// entityKind and entityRef name the optional entity context.
	entityKind domain.EntityKind
	entityRef  string
	// description is the concise normalized detail.
	description string
	// age is the effective strip age against the monotonic strip reference.
	age time.Duration
	// sponsor is the Scope whose fair turn selected the entry. It explains
	// selection and order alone and never narrows membership.
	sponsor domain.Scope
	// scopes are every confirmed matching Scope in stable Scope order.
	scopes []interestingScope
}

// ageText spells the entry's age in one unit, truncating toward zero: whole
// seconds below a minute, whole minutes below an hour, whole hours below a day,
// then whole days. An age of 14m59.999s therefore renders `14m` and never the
// ineligible boundary `15m` early (RG-007).
func (e interestingEntry) ageText() string {
	switch age := max(e.age, 0); {
	case age < time.Minute:
		return fmt.Sprintf("%ds", age/time.Second)
	case age < time.Hour:
		return fmt.Sprintf("%dm", age/time.Minute)
	case age < day:
		return fmt.Sprintf("%dh", age/time.Hour)
	default:
		return fmt.Sprintf("%dd", age/day)
	}
}

// additionalScopes reports the confirmed matching Scopes beyond the sponsoring
// one, which renders as `+N scopes`.
func (e interestingEntry) additionalScopes() int { return max(len(e.scopes)-1, 0) }

// qualifiedScopes reports how many confirmed matching Scopes qualified through
// current-PR evidence, which renders as `Q current PR`.
func (e interestingEntry) qualifiedScopes() int {
	qualified := 0
	for _, scope := range e.scopes {
		if scope.qualified {
			qualified++
		}
	}
	return qualified
}

// interestingCounts are the prepared strip totals rendering reports without
// recomputing them. They are disjoint in the sense RG-007 requires: omitted is
// capacity rejection alone, and hidden entries are stored minus rendered, which
// only the renderer knows.
type interestingCounts struct {
	// eligible is the number of unique eligible source events of the snapshot.
	eligible int
	// stored is the number of selected entries, at most 20.
	stored int
	// visible is the number of stored entries shown when height permits.
	visible int
	// omitted is eligible minus stored: the capacity-omitted remainder.
	omitted int
}

// interesting is the deterministic bounded Interesting Now selection state.
// Every transition is a pure function of explicit refresh, tick, and Scope
// inputs, so identical inputs always yield identical entries and order. It is
// prepared outside rendering and is not a second Stream.
type interesting struct {
	// scopes is the active selection in the stable Scope identity order.
	scopes []domain.Scope
	// snapshot is the retained snapshot every recomputation reads.
	snapshot domain.ScopedSnapshot
	// entries are the stored entries in exact selection order.
	entries []interestingEntry
	// counts are the prepared totals.
	counts interestingCounts
	// reference is the monotonic strip reference every age is measured against.
	reference time.Time
	// started reports whether a first successful snapshot established it.
	started bool
	// start is the zero-based Scope index the current selection began at, kept
	// so tick recomputation of the same snapshot repeats the same rotation.
	start int
	// nextStart is the zero-based index the next successful refresh starts at.
	nextStart int
	// nextAnchor is the canonical identity of that Scope, retained so a Scope
	// set change remaps the rotation instead of silently restarting it.
	nextAnchor domain.ScopeIdentity
	// nextAnchored reports whether an anchor exists, which an empty set clears.
	nextAnchored bool
}

// newInteresting returns the strip before its first successful snapshot: no
// reference, no entries, and a rotation owed to the first Scope.
func newInteresting() interesting { return interesting{} }

// visible returns the stored entries a full-height strip renders, which is the
// first five in stored order. Collapse may render fewer; it never reorders or
// reselects (RG-007).
func (s interesting) visible() []interestingEntry {
	return s.entries[:min(len(s.entries), stripVisible)]
}

// reconciled applies one successful atomic refresh. It replaces the retained
// snapshot and Scope set, advances the monotonic strip reference to the newest
// of its previous value and the refresh reference, starts the rotation at the
// saved next Scope, and recomputes selection from the complete snapshot. A
// publication whose reference is older still replaces the snapshot and advances
// the rotation; only the reference and the ages it drives stay monotonic.
func (s interesting) reconciled(scopes domain.ScopeSet, snapshot domain.ScopedSnapshot, reference time.Time) interesting {
	if !s.started || reference.After(s.reference) {
		s.reference = reference
	}
	s.started = true
	s.scopes = scopes.Ordered()
	s.snapshot = snapshot
	s = s.remapped()
	s.start = s.nextStart
	return s.selected(true)
}

// ticked applies one explicit application tick. The strip reference advances
// even while Rain is paused or hidden, so entries keep ageing and expiring; a
// tick before the first success, a repeated one, or one older than the current
// reference changes nothing at all.
func (s interesting) ticked(at time.Time) interesting {
	if !s.started || !at.After(s.reference) {
		return s
	}
	s.reference = at
	return s.selected(false)
}

// remapped moves the saved rotation anchor onto the current Scope set. A
// removed anchor keeps its old zero-based index, which now names its successor,
// or the final predecessor when no successor remains. An empty selection clears
// the anchor, and the next nonempty selection starts at its first Scope.
func (s interesting) remapped() interesting {
	if len(s.scopes) == 0 {
		s.nextAnchor, s.nextAnchored, s.nextStart = domain.ScopeIdentity{}, false, 0
		return s
	}
	index := 0
	if s.nextAnchored {
		index = slices.IndexFunc(s.scopes, func(scope domain.Scope) bool {
			return scope.Identity() == s.nextAnchor
		})
		if index < 0 {
			index = min(s.nextStart, len(s.scopes)-1)
		}
	}
	s.nextStart = index
	s.nextAnchor, s.nextAnchored = s.scopes[index].Identity(), true
	return s
}

// anchoredNext saves the zero-based index the next successful refresh starts at.
func (s interesting) anchoredNext(index int) interesting {
	if len(s.scopes) == 0 {
		return s
	}
	s.nextStart = index % len(s.scopes)
	s.nextAnchor, s.nextAnchored = s.scopes[s.nextStart].Identity(), true
	return s
}

// selected recomputes eligibility, Scope-fair selection, and the prepared totals
// from the retained snapshot at the current strip reference. Only a successful
// refresh rotates the saved next start; a tick repeats the start its refresh
// used, so ageing never reorders the strip underneath the operator.
func (s interesting) selected(rotating bool) interesting {
	queues, eligible := s.candidates()
	entries, filled := s.selectedFrom(queues, eligible)

	s.entries = entries
	s.counts = interestingCounts{
		eligible: eligible,
		stored:   len(entries),
		visible:  len(s.visible()),
		omitted:  eligible - len(entries),
	}
	if rotating && filled >= 0 {
		s = s.anchoredNext(filled + 1)
	}
	return s
}

// candidates prepares each active Scope's ordered candidate queue and reports
// how many unique eligible events the retained snapshot carries. A candidate is
// one normalized event with at least one confirmed member outcome whose
// effective strip age is inside the fixed 15-minute window; unknown-only and
// all-not-member events make none, and unknown Scope context is never converted
// into membership. An event matching several Scopes stands in each of their
// queues, where the fair rotation stores it exactly once.
func (s interesting) candidates() (map[domain.ScopeIdentity][]interestingEntry, int) {
	queues := make(map[domain.ScopeIdentity][]interestingEntry, len(s.scopes))
	seen := make(map[string]struct{})
	eligible := 0
	for _, scoped := range s.snapshot.ScopedEvents() {
		if _, repeated := seen[scoped.Event.ID]; repeated {
			// Snapshot normalization keeps the first canonical occurrence of a
			// repeated source ID, so the strip makes no second payload choice
			// here. The first occurrence is therefore what an ID is judged by,
			// whether or not it turned out eligible.
			continue
		}
		seen[scoped.Event.ID] = struct{}{}
		entry, admissible := s.candidate(scoped)
		if !admissible {
			continue
		}
		eligible++
		for _, matching := range entry.scopes {
			identity := matching.scope.Identity()
			queues[identity] = append(queues[identity], entry)
		}
	}
	for _, queue := range queues {
		slices.SortStableFunc(queue, compareInterestingCandidates)
	}
	return queues, eligible
}

// candidate prepares one event's entry facts and reports whether the event is
// eligible. The entry retains every confirmed matching Scope in stable Scope
// order with its own current-PR qualification.
func (s interesting) candidate(scoped domain.ScopedEvent) (interestingEntry, bool) {
	entry := interestingEntry{
		id:          scoped.Event.ID,
		occurredAt:  scoped.Event.OccurredAt,
		repository:  scoped.Event.Repository,
		actor:       scoped.Event.Actor,
		category:    normalizeCategory(scoped.Event.Category),
		entityKind:  scoped.Event.EntityKind,
		entityRef:   scoped.Event.EntityRef,
		description: scoped.Event.Description,
		age:         max(s.reference.Sub(scoped.Event.OccurredAt), 0),
	}
	for _, membership := range scoped.Memberships {
		if !membership.Membership.IsMember() {
			continue
		}
		entry.scopes = append(entry.scopes, interestingScope{
			scope:     membership.Scope,
			qualified: membership.Membership.QualifiedCurrentPR(),
		})
	}
	return entry, len(entry.scopes) > 0 && entry.age < stripWindow
}

// compareInterestingCandidates orders one Scope's queue by event timestamp
// newest first and then by source event ID in UTF-8 byte order.
func compareInterestingCandidates(a, b interestingEntry) int {
	if byRecency := b.occurredAt.Compare(a.occurredAt); byRecency != 0 {
		return byRecency
	}
	return strings.Compare(a.id, b.id)
}

// selectedFrom applies RG-007's Scope-fair rotation: starting at the retained
// start, each Scope turn examines and advances past exactly one queue event,
// selecting it unless another Scope's turn already did, so an overlapping event
// spends each matching Scope's turn instead of earning extra entries. Rounds
// repeat until 20 unique events are stored or every queue cursor is exhausted.
//
// The second result is the zero-based index of the turn that reached the
// 20-entry bound, or -1 when the queues exhausted first and the saved rotation
// therefore stays where it is.
func (s interesting) selectedFrom(queues map[domain.ScopeIdentity][]interestingEntry, eligible int) ([]interestingEntry, int) {
	scopes := len(s.scopes)
	if scopes == 0 || eligible == 0 {
		return nil, -1
	}

	capacity := min(eligible, stripStored)
	cursors := make([]int, scopes)
	selected := make(map[string]struct{}, capacity)
	entries := make([]interestingEntry, 0, capacity)
	turn, idle := s.start%scopes, 0
	for len(entries) < stripStored && idle < scopes {
		queue := queues[s.scopes[turn].Identity()]
		if cursors[turn] >= len(queue) {
			idle++
			turn = (turn + 1) % scopes
			continue
		}
		entry := queue[cursors[turn]]
		cursors[turn]++
		idle = 0
		if _, taken := selected[entry.id]; !taken {
			selected[entry.id] = struct{}{}
			entry.sponsor = s.scopes[turn]
			entries = append(entries, entry)
			if len(entries) == stripStored {
				return entries, turn
			}
		}
		turn = (turn + 1) % scopes
	}
	return entries, -1
}
