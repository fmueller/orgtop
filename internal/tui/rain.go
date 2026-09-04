package tui

import (
	"slices"
	"strings"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// The closed RG-009 Rain capacities: one Scope column admits at most 50
// representations, the whole field at most 500, and the paused arrival queue
// retains at most 100 in addition to the field.
const (
	rainColumnCap = 50
	rainGlobalCap = 500
	rainQueueCap  = 100
)

// rainStep is the movement quantum RG-006 closes: one accepted transition
// advances one row per complete 500 ms of elapsed logical time.
const rainStep = 500 * time.Millisecond

// rainKey identifies one Rain representation: the normalized source event ID
// plus one canonical Scope identity. One event confirmed in three Scopes is
// three independently bounded items, never one item with three labels.
type rainKey struct {
	event string
	scope domain.ScopeIdentity
}

// rainKeyOf returns the identity of one event/Scope representation.
func rainKeyOf(event string, scope domain.Scope) rainKey {
	return rainKey{event: event, scope: scope.Identity()}
}

// rainItem is one admitted or queued representation. It stores the bounded
// animation phase rather than cumulative runtime steps, and its effective Rain
// age rather than a wall-clock instant, so no transition reads the host clock.
type rainItem struct {
	key rainKey
	// scope is the retained Scope the representation belongs to, kept so
	// placement and column preparation never re-resolve identity.
	scope domain.Scope
	// occurredAt is the event instant candidate ordering compares.
	occurredAt time.Time
	// category is the normalized category the shared glyph vocabulary draws.
	category domain.Category
	// qualified reports current-PR membership, which renders as the `~`
	// qualifier and is never replaced by color.
	qualified bool
	// age is the effective Rain age, advanced by explicit deltas alone.
	age time.Duration
	// rowPhase is the bounded unsigned-64 animation phase. A distributed item
	// starts at the full row hash and a runtime arrival at zero.
	rowPhase uint64
}

// row reduces the animation phase onto a positive field height. A field with no
// usable row has no position at all, which the view renders as its explicit
// constrained state rather than as row zero.
func (i rainItem) row(height int) int { return reduce(i.rowPhase, height) }

// rainCounts are the prepared capacity totals rendering reports without
// recomputing them. They are disjoint: an omission is capacity rejection alone
// and never responsive hiding.
type rainCounts struct {
	// candidates is every eligible member representation of the snapshot.
	candidates int
	// admitted is the representations that won column and global capacity.
	admitted int
	// columnOmitted is the candidates the 50-item column cap rejected.
	columnOmitted int
	// globalOmitted is the column survivors the 500-item global cap rejected.
	globalOmitted int
	// pausedOmitted is the current paused-queue candidates the 100-entry queue
	// cap rejected. It is not cumulative across refreshes.
	pausedOmitted int
}

// rain is the deterministic bounded Rain field state. Every transition is a
// pure function of explicit event, tick, refresh, pause, window, and dimension
// inputs, so identical inputs always yield identical state.
type rain struct {
	window rainWindow
	// scopes is the active selection in the stable Scope identity order.
	scopes []domain.Scope
	// items is the admitted field in stable Scope then candidate order.
	items []rainItem
	// queue holds the bounded arrivals a paused refresh retained.
	queue []rainItem
	// observed is every representation identity a prior snapshot carried, so a
	// candidate first admitted after a capacity omission is recognized as
	// historical rather than as a publication arrival. It is rebuilt rather
	// than mutated, so a copied state never shares it.
	observed map[rainKey]struct{}
	// cursor is the explicit logical reference the field has advanced to.
	cursor time.Time
	// remainder is the sub-step logical time no transition has spent yet.
	remainder time.Duration
	// chain is the generation the single Rain timer chain expects next.
	chain uint64
	// started reports whether a first successful snapshot established a cursor.
	started bool
	// paused reports whether `p` froze movement, age, expiry, and the cursor.
	paused bool
	// needsRebase reports that resume is waiting for the first temporal input
	// newer than the frozen cursor, which sets the cursor without advancing.
	needsRebase bool
	// width and height are the field dimensions columns are allocated over.
	width, height int
	// anchor is the canonical identity of the first visible Scope, retained so
	// resize and Scope changes reselect the page containing it.
	anchor domain.ScopeIdentity
	// anchored reports whether an anchor exists, which an empty selection clears.
	anchored bool
	// start is the zero-based index of the page's first Scope.
	start int
	// counts are the prepared capacity totals.
	counts rainCounts
}

// newRain returns Rain before its first successful snapshot: the default
// 60-minute window, no cursor, and no field.
func newRain() rain { return rain{window: defaultRainWindow} }

// resized applies new field dimensions. Resize recomputes columns and the fixed
// page holding the anchor; it never changes age, admission, pause state, queue
// order, or omission totals, because the retained phase is only reduced onto
// the new height at display time.
func (r rain) resized(width, height int) rain {
	r.width, r.height = max(width, 0), max(height, 0)
	return r.repaged()
}

// paged moves one fixed page in either direction, wrapping at both ends, and
// makes the new first-visible Scope the anchor.
func (r rain) paged(step int) rain {
	perPage := rainPerPage(r.width, len(r.scopes))
	r.start = rainStepPage(r.start, len(r.scopes), perPage, step)
	return r.anchoredAtStart()
}

// repaged reselects the unique fixed page holding the retained anchor after a
// dimension, Scope-order, or Scope-count change. A removed anchor keeps its old
// zero-based index, which now names its successor, or the final predecessor
// when no successor remains. An empty selection clears the anchor, and the next
// nonempty selection selects its first Scope.
func (r rain) repaged() rain {
	scopes := len(r.scopes)
	if scopes == 0 {
		r.anchor, r.anchored, r.start = domain.ScopeIdentity{}, false, 0
		return r
	}
	index := 0
	if r.anchored {
		index = slices.IndexFunc(r.scopes, func(scope domain.Scope) bool {
			return scope.Identity() == r.anchor
		})
		if index < 0 {
			index = min(r.start, scopes-1)
		}
	}
	r.start = rainPageStart(index, rainPerPage(r.width, scopes))
	return r.anchoredAtStart()
}

// anchoredAtStart makes the page's first visible Scope the retained anchor.
func (r rain) anchoredAtStart() rain {
	if len(r.scopes) == 0 {
		r.anchor, r.anchored = domain.ScopeIdentity{}, false
		return r
	}
	r.start = min(max(r.start, 0), len(r.scopes)-1)
	r.anchor, r.anchored = r.scopes[r.start].Identity(), true
	return r
}

// ticked applies one explicit Rain tick. Only the generation the single chain
// expects and a timestamp strictly after the current logical reference is
// accepted; a duplicate, older-generation, equal-time, or older-time message
// neither advances state nor starts another chain.
//
// A rejected message leaves the expected generation where it was, so the caller
// driving the one Rain timer chain schedules the next tick from the generation
// this state reports afterwards rather than from a counter of its own. Before
// the first successful snapshot no Rain cursor exists at all, so every tick is
// rejected and the chain still expects its first generation.
func (r rain) ticked(chain uint64, at time.Time) rain {
	if !r.started || chain != r.chain || !at.After(r.cursor) {
		return r
	}
	r.chain++
	return r.applied(at)
}

// applied advances the field to an explicit logical reference. A paused field
// advances nothing at all, and the first input after resume only rebases the
// cursor, so paused duration never catches up.
func (r rain) applied(at time.Time) rain {
	if r.paused || !at.After(r.cursor) {
		return r
	}
	if r.needsRebase {
		r.cursor, r.needsRebase = at, false
		return r
	}
	return r.advanced(at)
}

// rebasing reports whether the next explicit reference only establishes the
// cursor after a resume, which advances neither phase nor age.
func (r rain) rebasing(at time.Time) bool {
	return r.needsRebase && !r.paused && at.After(r.cursor)
}

// advanced applies RG-006's one movement transition: it spends the complete
// elapsed delta on age and expiry, converts it plus the retained remainder into
// whole 500 ms steps, and moves each item modulo the field height once rather
// than replaying missed frames.
func (r rain) advanced(at time.Time) rain {
	elapsed := at.Sub(r.cursor)
	total := elapsed + r.remainder
	steps := uint64(total / rainStep)
	r.remainder = total % rainStep
	r.cursor = at

	items := make([]rainItem, 0, len(r.items))
	for _, item := range r.items {
		item.age += elapsed
		if item.age >= r.window.duration() {
			continue
		}
		if r.height > 0 {
			item.rowPhase = (item.rowPhase + steps) % uint64(r.height)
		}
		items = append(items, item)
	}
	r.items = items
	return r
}

// toggledPause applies `p`. Before the first successful snapshot no Rain cursor
// or field exists, so it is a no-op. Pausing freezes the logical cursor;
// resuming merges the retained arrival queue and waits for the first newer
// temporal input to rebase.
func (r rain) toggledPause() rain {
	if !r.started {
		return r
	}
	if !r.paused {
		r.paused = true
		return r
	}
	r.paused, r.needsRebase = false, true
	return r.merged()
}

// merged admits the frozen field and the retained queue together under the same
// fair capacity, places admitted arrivals at row zero, and clears the queue
// atomically. Older field items may be evicted; paused omissions do not
// materialize here and wait for a later successful unpaused refresh.
func (r rain) merged() rain {
	candidates := append(slices.Clone(r.items), r.queue...)
	r.queue = nil
	r.counts.pausedOmitted = 0
	return r.admitted(candidates, r.counts.candidates)
}

// reconciled applies one successful refresh. An unpaused field first advances
// through the same transition a tick uses, so the cursor already sits at the
// refresh reference and no interval is ever counted twice.
func (r rain) reconciled(scopes domain.ScopeSet, snapshot domain.ScopedSnapshot, reference time.Time) rain {
	first, rebasing := !r.started, r.rebasing(reference)
	if first {
		r.started, r.cursor, r.remainder = true, reference, 0
	} else {
		r = r.applied(reference)
	}
	r.scopes = scopes.Ordered()
	r = r.repaged()
	return r.rebuilt(snapshot, first, rebasing, false)
}

// windowed applies `-` or `+` and immediately reconciles the field against the
// current normalized snapshot. Shortening removes newly out-of-window items;
// lengthening restores eligible representations under the normal admission
// rules, with distributed placement for the restored historical ones.
func (r rain) windowed(step int, scopes domain.ScopeSet, snapshot domain.ScopedSnapshot) rain {
	stepped := r.window.stepped(step)
	if stepped == r.window || !r.started {
		r.window = stepped
		return r
	}
	r.window = stepped
	r.scopes = scopes.Ordered()
	r = r.repaged()
	return r.rebuilt(snapshot, false, r.paused, true)
}

// rebuilt reconciles the field against one snapshot's complete candidate set.
// A paused refresh only removes representations whose event or confirmed
// membership disappeared and retains new ones in the bounded arrival queue; a
// paused window change instead restores historical representations to the
// field; an unpaused transition recomputes admission from the complete
// candidate set, so newer or fairer candidates may evict older admitted items.
//
// The snapshot and the Scope set the caller reconciles against must be the two
// halves of one refresh: a membership naming a Scope outside r.scopes would be
// counted as a candidate and then admitted nowhere, which would break the
// disjoint counters RG-006 requires.
func (r rain) rebuilt(snapshot domain.ScopedSnapshot, first, frozenAges, restoring bool) rain {
	candidates, total, observed := r.candidates(snapshot, first, frozenAges)
	r.observed = observed
	switch {
	case r.paused && restoring:
		return r.restored(candidates, total)
	case r.paused:
		return r.frozen(candidates, total)
	default:
		return r.admitted(candidates, total)
	}
}

// candidates returns every eligible member representation of the snapshot in
// the stable Scope then candidate order, carrying the phase and age each one
// keeps, and the candidate total. Unknown and not-member outcomes create none,
// so the field represents direct displayed events alone.
func (r rain) candidates(snapshot domain.ScopedSnapshot, first, frozenAges bool) ([]rainItem, int, map[rainKey]struct{}) {
	retained := make(map[rainKey]rainItem, len(r.items)+len(r.queue))
	for _, item := range r.items {
		retained[item.key] = item
	}
	for _, item := range r.queue {
		retained[item.key] = item
	}

	observed := make(map[rainKey]struct{}, len(r.observed))
	byScope := make(map[domain.ScopeIdentity][]rainItem, len(r.scopes))
	total := 0
	for _, scoped := range snapshot.ScopedEvents() {
		for _, membership := range scoped.Memberships {
			if !membership.Membership.IsMember() {
				continue
			}
			candidate, eligible := r.candidate(scoped.Event, membership, retained, first, frozenAges)
			observed[candidate.key] = struct{}{}
			if !eligible {
				continue
			}
			total++
			byScope[candidate.key.scope] = append(byScope[candidate.key.scope], candidate)
		}
	}

	candidates := make([]rainItem, 0, total)
	for _, scope := range r.scopes {
		ordered := byScope[scope.Identity()]
		slices.SortStableFunc(ordered, compareRainCandidates)
		candidates = append(candidates, ordered...)
	}
	return candidates, total, observed
}

// candidate prepares one representation and reports whether it is eligible. A
// surviving item keeps its phase and never has its age decreased; a membership
// first observed in an unpaused refresh enters at row zero; the first snapshot
// and every historical readmission use the stable distributed placement.
func (r rain) candidate(event domain.Event, membership domain.ScopeMembership, retained map[rainKey]rainItem, first, frozenAges bool) (rainItem, bool) {
	key := rainKeyOf(event.ID, membership.Scope)
	age := max(r.cursor.Sub(event.OccurredAt), 0)
	item := rainItem{
		key:        key,
		scope:      membership.Scope,
		occurredAt: event.OccurredAt,
		category:   normalizeCategory(event.Category),
		qualified:  membership.Membership.QualifiedCurrentPR(),
	}
	switch previous, surviving := retained[key]; {
	case surviving:
		item.rowPhase, item.age = previous.rowPhase, max(previous.age, age)
		if frozenAges {
			item.age = previous.age
		}
	case first:
		item.rowPhase, item.age = rainRowHash(event.ID, membership.Scope), age
	default:
		if _, historical := r.observed[key]; historical {
			item.rowPhase = rainRowHash(event.ID, membership.Scope)
		}
		item.age = age
	}
	return item, item.age < r.window.duration()
}

// compareRainCandidates orders one Scope's candidates by event timestamp newest
// first and then by event ID byte order. Canonical Scope identity is constant
// within a column, so it never breaks a tie here.
func compareRainCandidates(a, b rainItem) int {
	if byRecency := b.occurredAt.Compare(a.occurredAt); byRecency != 0 {
		return byRecency
	}
	return strings.Compare(a.key.event, b.key.event)
}

// admitted applies RG-009's two capacities: each Scope keeps its newest 50
// candidates, then the global round robin takes one remaining candidate per
// Scope in stable Scope order until the 500-item field is full. The bounded
// one-item bias of a partial final round goes to the stable-order prefix.
func (r rain) admitted(candidates []rainItem, total int) rain {
	columns, columnOmitted := r.capped(candidates, rainColumnCap)
	admitted, globalOmitted := roundRobin(r.scopes, columns, rainGlobalCap)

	r.items = admitted
	r.counts.candidates = total
	r.counts.admitted = len(admitted)
	r.counts.columnOmitted = columnOmitted
	r.counts.globalOmitted = globalOmitted
	return r
}

// frozen applies a paused refresh: the field keeps only representations the
// snapshot still confirms, with their frozen phase and age, and every newly
// confirmed representation competes for the bounded arrival queue instead.
func (r rain) frozen(candidates []rainItem, total int) rain {
	eligible := make(map[rainKey]struct{}, len(candidates))
	for _, candidate := range candidates {
		eligible[candidate.key] = struct{}{}
	}
	standing := make(map[rainKey]struct{}, len(r.items))
	field := make([]rainItem, 0, len(r.items))
	for _, item := range r.items {
		standing[item.key] = struct{}{}
		if _, confirmed := eligible[item.key]; confirmed {
			field = append(field, item)
		}
	}

	// A candidate is eligible by construction, so one already standing in the
	// field was just kept above and only the rest are arrivals.
	arrivals := make([]rainItem, 0, len(candidates))
	for _, candidate := range candidates {
		if _, inField := standing[candidate.key]; inField {
			continue
		}
		candidate.rowPhase = 0
		arrivals = append(arrivals, candidate)
	}
	retained, _ := roundRobin(r.scopes, arrivals, rainQueueCap)

	r.items = field
	r.queue = retained
	r.counts.candidates = total
	r.counts.admitted = len(field)
	r.counts.pausedOmitted = len(arrivals) - len(retained)
	return r
}

// restored applies a window change while paused. It reconciles both halves of
// the paused state against the new window: the frozen field readmits every
// still-eligible representation that is not waiting in the queue, under the
// normal column and global admission rules and with distributed placement for
// the restored historical ones, while the queue keeps only its own still
// eligible entries. A restoration is not a paused arrival, so it never consumes
// paused-arrival capacity (RG-006).
func (r rain) restored(candidates []rainItem, total int) rain {
	waiting := make(map[rainKey]struct{}, len(r.queue))
	for _, item := range r.queue {
		waiting[item.key] = struct{}{}
	}

	field := make([]rainItem, 0, len(candidates))
	arrivals := make([]rainItem, 0, len(r.queue))
	for _, candidate := range candidates {
		if _, queued := waiting[candidate.key]; !queued {
			field = append(field, candidate)
			continue
		}
		candidate.rowPhase = 0
		arrivals = append(arrivals, candidate)
	}
	retained, _ := roundRobin(r.scopes, arrivals, rainQueueCap)

	r = r.admitted(field, total)
	r.queue = retained
	r.counts.pausedOmitted = len(arrivals) - len(retained)
	return r
}

// capped truncates every Scope's ordered candidates to the per-column capacity
// and reports how many representations that rejected.
func (r rain) capped(candidates []rainItem, capacity int) ([]rainItem, int) {
	kept := make([]rainItem, 0, len(candidates))
	seen := make(map[domain.ScopeIdentity]int, len(r.scopes))
	omitted := 0
	for _, candidate := range candidates {
		if seen[candidate.key.scope] >= capacity {
			omitted++
			continue
		}
		seen[candidate.key.scope]++
		kept = append(kept, candidate)
	}
	return kept, omitted
}

// roundRobin takes one remaining candidate per Scope in the stable Scope order
// repeatedly until no candidate remains or the capacity is reached, so a busy
// Scope never consumes the whole field. It reports the rejected count.
func roundRobin(scopes []domain.Scope, candidates []rainItem, capacity int) ([]rainItem, int) {
	order := make(map[domain.ScopeIdentity]int, len(scopes))
	byScope := make(map[domain.ScopeIdentity][]rainItem, len(scopes))
	for index, scope := range scopes {
		order[scope.Identity()] = index
	}
	for _, candidate := range candidates {
		byScope[candidate.key.scope] = append(byScope[candidate.key.scope], candidate)
	}

	rounds := 0
	for _, scope := range scopes {
		rounds = max(rounds, len(byScope[scope.Identity()]))
	}
	admitted := make([]rainItem, 0, min(len(candidates), capacity))
	for round := 0; round < rounds && len(admitted) < capacity; round++ {
		for _, scope := range scopes {
			column := byScope[scope.Identity()]
			if round < len(column) && len(admitted) < capacity {
				admitted = append(admitted, column[round])
			}
		}
	}

	// Admitted items are stored in the stable Scope order, then in the same
	// candidate order the column carried, so storage stays deterministic.
	slices.SortStableFunc(admitted, func(a, b rainItem) int {
		if byOrder := order[a.key.scope] - order[b.key.scope]; byOrder != 0 {
			return byOrder
		}
		return compareRainCandidates(a, b)
	})
	return admitted, len(candidates) - len(admitted)
}
