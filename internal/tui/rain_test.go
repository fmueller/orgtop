package tui

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// rainBase is the explicit logical reference every Rain fixture is stated
// against. Rain never reads the host clock, so every vector below is a pure
// offset from it.
var rainBase = time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

// rainAt returns the logical instant the given offset past the fixture base.
func rainAt(offset time.Duration) time.Time { return rainBase.Add(offset) }

// rainEvidence builds one retained push of the repository occurring the given
// time before the fixture base. A repository Scope decides membership from
// identity alone, so the evidence outcome stays the zero value.
func rainEvidence(t *testing.T, id, repository string, since time.Duration) domain.EventEvidence {
	t.Helper()
	event := testEvent(t, id, repository, domain.CategoryPush, domain.EntityCommit)
	event.OccurredAt = rainBase.Add(-since)
	return domain.EventEvidence{Event: event}
}

// rainSnapshot builds the prepared per-Scope snapshot one successful refresh
// publishes for the retained evidence.
func rainSnapshot(scopes domain.ScopeSet, retained ...domain.EventEvidence) domain.ScopedSnapshot {
	return domain.NewRetainedSnapshot(scopes, retained, false)
}

// startedRain returns Rain after its first successful snapshot at the fixture
// base, sized to the given field, which is where a Rain cursor first exists.
func startedRain(scopes domain.ScopeSet, snapshot domain.ScopedSnapshot, width, height int) rain {
	return newRain().resized(width, height).reconciled(scopes, snapshot, rainBase)
}

// rainItemOf returns the admitted item of one event/Scope identity.
func rainItemOf(t *testing.T, field rain, event string, scope domain.Scope) rainItem {
	t.Helper()
	for _, item := range field.items {
		if item.key == rainKeyOf(event, scope) {
			return item
		}
	}
	t.Fatalf("no admitted Rain item for event %q in Scope %s", event, scope)
	return rainItem{}
}

// rainAdmits reports whether the event/Scope identity is admitted.
func rainAdmits(field rain, event string, scope domain.Scope) bool {
	for _, item := range field.items {
		if item.key == rainKeyOf(event, scope) {
			return true
		}
	}
	return false
}

// TestRainRepeatingTickMovesThroughEveryRow guards A-049: accepted 500 ms ticks
// move a runtime arrival at row zero of a four-row field through rows one, two,
// three, and back to zero, so items repeat until their lifetime ends.
func TestRainRepeatingTickMovesThroughEveryRow(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	field := startedRain(scopes, rainSnapshot(scopes), 40, 4)
	arrival := rainEvidence(t, "arrival", repository, 0)
	field = field.reconciled(scopes, rainSnapshot(scopes, arrival), rainBase)
	if got := rainItemOf(t, field, "arrival", scopes.Ordered()[0]).row(4); got != 0 {
		t.Fatalf("a runtime arrival enters at row %d, want row 0", got)
	}
	for step, want := range []int{1, 2, 3, 0} {
		field = field.ticked(field.chain, rainAt(time.Duration(step+1)*500*time.Millisecond))
		if got := rainItemOf(t, field, "arrival", scopes.Ordered()[0]).row(4); got != want {
			t.Fatalf("tick %d put the item on row %d, want %d", step+1, got, want)
		}
	}
}

// TestRainDelayedTickAdvancesOnceAndKeepsRemainder guards A-049: a tick delayed
// by 2.25 seconds advances four rows in one transition, retains a 250 ms
// remainder, advances factual age by the complete 2.25 seconds, and replays no
// frames.
func TestRainDelayedTickAdvancesOnceAndKeepsRemainder(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	field := startedRain(scopes, rainSnapshot(scopes), 40, 7)
	arrival := rainEvidence(t, "arrival", repository, 0)
	field = field.reconciled(scopes, rainSnapshot(scopes, arrival), rainBase)
	field = field.ticked(field.chain, rainAt(2250*time.Millisecond))

	item := rainItemOf(t, field, "arrival", scopes.Ordered()[0])
	if got := item.row(7); got != 4 {
		t.Errorf("the delayed tick moved the item to row %d, want row 4", got)
	}
	if got, want := field.remainder, 250*time.Millisecond; got != want {
		t.Errorf("the delayed tick retained remainder %s, want %s", got, want)
	}
	if got, want := item.age, 2250*time.Millisecond; got != want {
		t.Errorf("the delayed tick advanced age to %s, want %s", got, want)
	}
	if got, want := field.cursor, rainAt(2250*time.Millisecond); !got.Equal(want) {
		t.Errorf("the delayed tick left the cursor at %s, want %s", got, want)
	}
}

// TestRainIgnoresDuplicateOlderAndForeignChainTicks guards RG-006's single
// timer chain: a duplicate, an older-generation, an equal-time, and an
// older-time message neither advance state nor start another chain.
func TestRainIgnoresDuplicateOlderAndForeignChainTicks(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	started := startedRain(scopes, rainSnapshot(scopes, rainEvidence(t, "one", repository, 0)), 40, 4)
	advanced := started.ticked(started.chain, rainAt(500*time.Millisecond))

	cases := []struct {
		name  string
		chain uint64
		at    time.Time
	}{
		{name: "duplicate generation", chain: advanced.chain - 1, at: rainAt(time.Second)},
		{name: "older generation", chain: 0, at: rainAt(time.Second)},
		{name: "equal time", chain: advanced.chain, at: rainAt(500 * time.Millisecond)},
		{name: "older time", chain: advanced.chain, at: rainAt(250 * time.Millisecond)},
	}
	for _, ignored := range cases {
		if got := advanced.ticked(ignored.chain, ignored.at); !reflect.DeepEqual(got, advanced) {
			t.Errorf("a %s message changed Rain state", ignored.name)
		}
	}
}

// TestRainRefreshRebasesTheCursorThroughTheSameAdvance guards A-049: a snapshot
// at `0s` sets the cursor; after a `0.5s` tick a refresh at `1.25s` advances one
// more row with a `250ms` remainder and rebases the cursor, so a `1.5s` tick
// advances exactly one row and only `1.5s` of age has accrued.
func TestRainRefreshRebasesTheCursorThroughTheSameAdvance(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	snapshot := rainSnapshot(scopes, rainEvidence(t, "one", repository, 0))
	field := startedRain(scopes, snapshot, 40, 7)
	scope := scopes.Ordered()[0]
	start := rainItemOf(t, field, "one", scope).row(7)

	field = field.ticked(field.chain, rainAt(500*time.Millisecond))
	field = field.reconciled(scopes, snapshot, rainAt(1250*time.Millisecond))
	if got, want := field.remainder, 250*time.Millisecond; got != want {
		t.Errorf("the refresh retained remainder %s, want %s", got, want)
	}
	if got, want := rainItemOf(t, field, "one", scope).row(7), (start+2)%7; got != want {
		t.Errorf("after the refresh the item is on row %d, want %d", got, want)
	}

	field = field.ticked(field.chain, rainAt(1500*time.Millisecond))
	if got, want := rainItemOf(t, field, "one", scope).row(7), (start+3)%7; got != want {
		t.Errorf("after the last tick the item is on row %d, want %d", got, want)
	}
	if got, want := rainItemOf(t, field, "one", scope).age, 1500*time.Millisecond; got != want {
		t.Errorf("total accrued age is %s, want %s", got, want)
	}
}

// TestRainRefreshBeforeTheCursorAdvancesNothing guards A-049's alternate
// reference: a refresh whose reference is not after the cursor advances
// nothing, and newly admitted ages still use the current cursor.
func TestRainRefreshBeforeTheCursorAdvancesNothing(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	field := startedRain(scopes, rainSnapshot(scopes), 40, 7)
	field = field.ticked(field.chain, rainAt(500*time.Millisecond))

	arrival := rainEvidence(t, "late", repository, 0)
	field = field.reconciled(scopes, rainSnapshot(scopes, arrival), rainAt(250*time.Millisecond))
	if got, want := field.cursor, rainAt(500*time.Millisecond); !got.Equal(want) {
		t.Errorf("the earlier reference moved the cursor to %s, want %s", got, want)
	}
	if got, want := rainItemOf(t, field, "late", scopes.Ordered()[0]).age, 500*time.Millisecond; got != want {
		t.Errorf("the newly admitted item aged %s, want the current cursor age %s", got, want)
	}
}

// TestRainRemovesItemsAtTheExactWindowBoundary guards RG-006's half-open
// lifetime: an item is eligible while its effective age is below the selected
// window and the exact boundary removes it, in one delayed transition too.
func TestRainRemovesItemsAtTheExactWindowBoundary(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	inside := rainEvidence(t, "inside", repository, time.Hour-time.Nanosecond)
	boundary := rainEvidence(t, "boundary", repository, time.Hour)
	field := startedRain(scopes, rainSnapshot(scopes, inside, boundary), 40, 4)
	if !rainAdmits(field, "inside", scope) {
		t.Error("an item one nanosecond inside the window is not admitted")
	}
	if rainAdmits(field, "boundary", scope) {
		t.Error("an item at the exact window boundary is still admitted")
	}
	field = field.ticked(field.chain, rainAt(time.Nanosecond))
	if rainAdmits(field, "inside", scope) {
		t.Error("a delayed transition past the boundary did not remove the item")
	}
}

// TestRainColumnCapKeepsTheNewestFifty guards A-051: 51 candidates in one Scope
// leave its newest 50 and record exactly one column omission.
func TestRainColumnCapKeepsTheNewestFifty(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	retained := make([]domain.EventEvidence, 0, 51)
	for index := range 51 {
		retained = append(retained, rainEvidence(t, fmt.Sprintf("e%02d", index), repository, time.Duration(index)*time.Minute))
	}
	field := startedRain(scopes, rainSnapshot(scopes, retained...), 40, 4)
	if got, want := field.counts.admitted, 50; got != want {
		t.Errorf("admitted %d items, want %d", got, want)
	}
	if got, want := field.counts.columnOmitted, 1; got != want {
		t.Errorf("recorded %d column omissions, want %d", got, want)
	}
	if rainAdmits(field, "e50", scopes.Ordered()[0]) {
		t.Error("the oldest candidate survived the 50-item column cap")
	}
}

// TestRainGlobalAdmissionIsScopeFair guards A-051: 100 candidates in each of 11
// ordered Scopes leave 550 after the column caps, and the global round robin
// admits 46 from Scopes 1-5 and 45 from Scopes 6-11.
func TestRainGlobalAdmissionIsScopeFair(t *testing.T) {
	selected := make([]domain.Scope, 0, 11)
	for index := range 11 {
		selected = append(selected, domain.NewRepositoryScope(testRepository(t, fmt.Sprintf("acme/r%02d", index))))
	}
	scopes := scopeSet(t, selected...)
	retained := make([]domain.EventEvidence, 0, 1100)
	for index, scope := range scopes.Ordered() {
		for event := range 100 {
			id := fmt.Sprintf("s%02d-e%03d", index, event)
			retained = append(retained, rainEvidence(t, id, scope.Repository().String(), time.Duration(event)*time.Second))
		}
	}
	field := startedRain(scopes, rainSnapshot(scopes, retained...), 200, 4)

	counts := map[domain.ScopeIdentity]int{}
	for _, item := range field.items {
		counts[item.key.scope]++
	}
	for index, scope := range scopes.Ordered() {
		want := 45
		if index < 5 {
			want = 46
		}
		if got := counts[scope.Identity()]; got != want {
			t.Errorf("Scope %d admitted %d items, want %d", index+1, got, want)
		}
	}
	want := rainCounts{candidates: 1100, admitted: 500, columnOmitted: 550, globalOmitted: 50}
	if field.counts != want {
		t.Errorf("prepared counts are %+v, want %+v", field.counts, want)
	}
}

// TestRainCreatesOneItemPerMatchingScope guards RG-006 overlap honesty: one
// event confirmed in three Scopes consumes three independently bounded items.
func TestRainCreatesOneItemPerMatchingScope(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t,
		domain.NewRepositoryScope(testRepository(t, repository)),
		pathScope(t, repository, "docs"),
		pathScope(t, repository, "src"),
	)
	evidence := rainEvidence(t, "shared", repository, 0)
	evidence.Outcome = domain.CompleteOutcome(domain.ProvenanceEventTime, mustChangedPaths(t, "docs/readme.md", "src/main.go"))
	field := startedRain(scopes, rainSnapshot(scopes, evidence), 60, 4)
	if got, want := field.counts.admitted, 3; got != want {
		t.Fatalf("a three-Scope event admitted %d items, want %d", got, want)
	}
	rows := map[int]struct{}{}
	for _, item := range field.items {
		rows[item.row(4)] = struct{}{}
	}
	if len(field.items) != 3 {
		t.Fatalf("the field holds %d items, want 3", len(field.items))
	}
	if got := field.counts.candidates; got != 3 {
		t.Errorf("a three-Scope event produced %d candidates, want 3", got)
	}
}

// TestRainUnknownAndNotMemberOutcomesCreateNoItems guards RG-006 density: only
// a confirmed member outcome creates a Rain candidate, so the field represents
// direct displayed events alone.
func TestRainUnknownAndNotMemberOutcomesCreateNoItems(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, pathScope(t, repository, "docs"), pathScope(t, repository, "src"))
	unknown := rainEvidence(t, "unknown", repository, 0)
	unknown.Outcome = domain.IncompleteOutcome("no evidence")
	notMember := rainEvidence(t, "elsewhere", repository, 0)
	notMember.Outcome = domain.CompleteOutcome(domain.ProvenanceEventTime, mustChangedPaths(t, "other/file.txt"))
	field := startedRain(scopes, rainSnapshot(scopes, unknown, notMember), 60, 4)
	if got := field.counts.candidates; got != 0 {
		t.Errorf("undecided and not-member outcomes produced %d candidates, want 0", got)
	}
	if len(field.items) != 0 {
		t.Errorf("the field holds %d items, want none", len(field.items))
	}
}

// TestRainRefreshRetainsRemovesAndAdds guards A-052: a refresh that retains one
// membership, removes another, and adds a third keeps the survivor's phase,
// drops the removed item, and starts the new one at row zero.
func TestRainRefreshRetainsRemovesAndAdds(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	survivor := rainEvidence(t, "survivor", repository, time.Minute)
	removed := rainEvidence(t, "removed", repository, 2*time.Minute)
	field := startedRain(scopes, rainSnapshot(scopes, survivor, removed), 40, 7)
	field = field.ticked(field.chain, rainAt(500*time.Millisecond))
	phase := rainItemOf(t, field, "survivor", scope).rowPhase

	added := rainEvidence(t, "added", repository, 0)
	field = field.reconciled(scopes, rainSnapshot(scopes, survivor, added), rainAt(time.Second))
	if got := rainItemOf(t, field, "survivor", scope).rowPhase; got != (phase+1)%7 {
		t.Errorf("the survivor's phase is %d, want the advanced retained phase %d", got, (phase+1)%7)
	}
	if rainAdmits(field, "removed", scope) {
		t.Error("a representation whose membership disappeared is still admitted")
	}
	if got := rainItemOf(t, field, "added", scope).row(7); got != 0 {
		t.Errorf("a newly confirmed membership entered at row %d, want row 0", got)
	}
}

// TestRainFirstSnapshotDistributesPlacement guards RG-006's steady startup: the
// first snapshot distributes initial rows with the stable placement function
// rather than stacking every item on row zero.
func TestRainFirstSnapshotDistributesPlacement(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	field := startedRain(scopes, rainSnapshot(scopes, rainEvidence(t, "12345", repository, 0)), 40, 7)
	if got, want := rainItemOf(t, field, "12345", scope).row(7), 4; got != want {
		t.Errorf("the distributed row of the A-050 fixture is %d, want %d", got, want)
	}
}

// TestRainHistoricalReadmissionUsesDistributedPlacement guards RG-006: a
// continuously eligible candidate first admitted after a prior capacity
// omission is historical and never pretends to be a new arrival.
func TestRainHistoricalReadmissionUsesDistributedPlacement(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	retained := make([]domain.EventEvidence, 0, 51)
	for index := range 51 {
		retained = append(retained, rainEvidence(t, fmt.Sprintf("e%02d", index), repository, time.Duration(index)*time.Minute))
	}
	field := startedRain(scopes, rainSnapshot(scopes, retained...), 40, 7)
	if rainAdmits(field, "e50", scope) {
		t.Fatal("the oldest candidate was admitted despite the column cap")
	}

	// Dropping the newest candidate frees exactly the one omitted representation.
	field = field.reconciled(scopes, rainSnapshot(scopes, retained[1:]...), rainBase)
	item := rainItemOf(t, field, "e50", scope)
	if item.rowPhase != rainRowHash("e50", scope) {
		t.Errorf("the readmitted historical item has phase %d, want the distributed row hash %d", item.rowPhase, rainRowHash("e50", scope))
	}
}

// TestRainPauseFreezesMovementAgeAndExpiry guards A-052: while paused, ticks
// freeze phase, age, expiry, and the cursor remainder.
func TestRainPauseFreezesMovementAgeAndExpiry(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	expiring := rainEvidence(t, "expiring", repository, time.Hour-time.Second)
	field := startedRain(scopes, rainSnapshot(scopes, expiring), 40, 7)
	field = field.toggledPause()
	frozen := rainItemOf(t, field, "expiring", scope)

	field = field.ticked(field.chain, rainAt(10*time.Minute))
	current := rainItemOf(t, field, "expiring", scope)
	if current.rowPhase != frozen.rowPhase {
		t.Errorf("a paused tick moved the item to phase %d, want the frozen %d", current.rowPhase, frozen.rowPhase)
	}
	if current.age != frozen.age {
		t.Errorf("a paused tick aged the item to %s, want the frozen %s", current.age, frozen.age)
	}
	if !field.cursor.Equal(rainBase) {
		t.Errorf("a paused tick moved the cursor to %s, want the frozen %s", field.cursor, rainBase)
	}
	if field.remainder != 0 {
		t.Errorf("a paused tick accrued remainder %s, want none", field.remainder)
	}
}

// TestRainPauseBeforeFirstSuccessIsNoOp guards A-052: before the first
// successful snapshot no Rain cursor or field exists, so `p` does nothing.
func TestRainPauseBeforeFirstSuccessIsNoOp(t *testing.T) {
	field := newRain().resized(40, 7)
	if got := field.toggledPause(); !reflect.DeepEqual(got, field) {
		t.Error("pausing before the first successful snapshot changed Rain state")
	}
}

// TestRainPausedRefreshRemovesLostTruthAndQueuesArrivals guards A-052: a paused
// refresh still removes representations whose membership disappeared and queues
// newly confirmed ones rather than admitting them.
func TestRainPausedRefreshRemovesLostTruthAndQueuesArrivals(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	survivor := rainEvidence(t, "survivor", repository, time.Minute)
	removed := rainEvidence(t, "removed", repository, 2*time.Minute)
	field := startedRain(scopes, rainSnapshot(scopes, survivor, removed), 40, 7)
	field = field.toggledPause()

	arrival := rainEvidence(t, "arrival", repository, 0)
	field = field.reconciled(scopes, rainSnapshot(scopes, survivor, arrival), rainAt(time.Minute))
	if !rainAdmits(field, "survivor", scope) {
		t.Error("the surviving representation left the frozen field")
	}
	if rainAdmits(field, "removed", scope) {
		t.Error("a representation whose membership disappeared stayed in the frozen field")
	}
	if rainAdmits(field, "arrival", scope) {
		t.Error("a paused arrival was admitted to the field instead of queued")
	}
	if len(field.queue) != 1 || field.queue[0].key != rainKeyOf("arrival", scope) {
		t.Fatalf("the paused queue holds %v, want the one arrival", field.queue)
	}
	if got := field.queue[0].age; got != 0 {
		t.Errorf("a paused arrival after the frozen reference has age %s, want the clamped zero", got)
	}
}

// TestRainPausedQueueRetainsScopeFairly guards A-052: with 34, 34, and 33
// current queue candidates in three ordered Scopes, retention is 34, 33, and 33
// and the one displaced candidate is the single current paused omission.
func TestRainPausedQueueRetainsScopeFairly(t *testing.T) {
	selected := make([]domain.Scope, 0, 3)
	for index := range 3 {
		selected = append(selected, domain.NewRepositoryScope(testRepository(t, fmt.Sprintf("acme/r%d", index))))
	}
	scopes := scopeSet(t, selected...)
	field := startedRain(scopes, rainSnapshot(scopes), 60, 7).toggledPause()

	ordered := scopes.Ordered()
	var retained []domain.EventEvidence
	for index, count := range []int{34, 34, 33} {
		for event := range count {
			id := fmt.Sprintf("s%d-e%02d", index, event)
			retained = append(retained, rainEvidence(t, id, ordered[index].Repository().String(), time.Duration(event)*time.Second))
		}
	}
	field = field.reconciled(scopes, rainSnapshot(scopes, retained...), rainAt(time.Minute))

	queued := map[domain.ScopeIdentity]int{}
	for _, item := range field.queue {
		queued[item.key.scope]++
	}
	for index, want := range []int{34, 33, 33} {
		if got := queued[ordered[index].Identity()]; got != want {
			t.Errorf("Scope %d retained %d queue entries, want %d", index+1, got, want)
		}
	}
	if got, want := len(field.queue), 100; got != want {
		t.Errorf("the paused queue holds %d entries, want the %d cap", got, want)
	}
	if got, want := field.counts.pausedOmitted, 1; got != want {
		t.Errorf("recorded %d paused omissions, want %d", got, want)
	}
	if rainAdmits(field, "s1-e33", ordered[1]) || queuedHas(field, "s1-e33", ordered[1]) {
		t.Error("the oldest Scope 2 candidate was retained, want it as the one paused omission")
	}
}

// queuedHas reports whether the paused queue retains the event/Scope identity.
func queuedHas(field rain, event string, scope domain.Scope) bool {
	for _, item := range field.queue {
		if item.key == rainKeyOf(event, scope) {
			return true
		}
	}
	return false
}

// TestRainResumeMergesQueueAndRebasesOnce guards A-052: resume performs one
// fair merge, starts queued admissions at row zero, clears the queue, and the
// first post-resume tick only establishes its cursor.
func TestRainResumeMergesQueueAndRebasesOnce(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	survivor := rainEvidence(t, "survivor", repository, time.Minute)
	field := startedRain(scopes, rainSnapshot(scopes, survivor), 40, 7).toggledPause()
	arrival := rainEvidence(t, "arrival", repository, 0)
	field = field.reconciled(scopes, rainSnapshot(scopes, survivor, arrival), rainAt(time.Minute))

	field = field.toggledPause()
	if len(field.queue) != 0 {
		t.Errorf("resume left %d queue entries, want the queue cleared", len(field.queue))
	}
	if got := rainItemOf(t, field, "arrival", scope).row(7); got != 0 {
		t.Errorf("a resumed arrival entered at row %d, want row 0", got)
	}
	frozenAge := rainItemOf(t, field, "survivor", scope).age

	field = field.ticked(field.chain, rainAt(10*time.Minute))
	if got := rainItemOf(t, field, "survivor", scope).age; got != frozenAge {
		t.Errorf("the first post-resume tick aged the survivor to %s, want the frozen %s", got, frozenAge)
	}
	if got, want := field.cursor, rainAt(10*time.Minute); !got.Equal(want) {
		t.Errorf("the first post-resume tick left the cursor at %s, want %s", got, want)
	}
	field = field.ticked(field.chain, rainAt(10*time.Minute+500*time.Millisecond))
	if got, want := rainItemOf(t, field, "survivor", scope).age, frozenAge+500*time.Millisecond; got != want {
		t.Errorf("the second post-resume tick aged the survivor to %s, want %s", got, want)
	}
}

// TestRainResumeRebasesOnARefreshReference guards A-052's alternate sequence: a
// successful refresh reaching Rain first performs the same zero-advance rebase.
func TestRainResumeRebasesOnARefreshReference(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	snapshot := rainSnapshot(scopes, rainEvidence(t, "one", repository, time.Minute))
	field := startedRain(scopes, snapshot, 40, 7).toggledPause().toggledPause()
	frozen := rainItemOf(t, field, "one", scope)

	field = field.reconciled(scopes, snapshot, rainAt(10*time.Minute))
	current := rainItemOf(t, field, "one", scope)
	if current.age != frozen.age || current.rowPhase != frozen.rowPhase {
		t.Errorf("the rebasing refresh advanced the item to %+v, want the frozen %+v", current, frozen)
	}
	if got, want := field.cursor, rainAt(10*time.Minute); !got.Equal(want) {
		t.Errorf("the rebasing refresh left the cursor at %s, want %s", got, want)
	}
}

// TestRainWindowStepsThroughTheClosedPresets guards A-054: Rain starts at 60m,
// `-` steps through 30m and 15m and stops, and `+` reverses and stops at 60m.
func TestRainWindowStepsThroughTheClosedPresets(t *testing.T) {
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, "acme/api")))
	empty := rainSnapshot(scopes)
	field := startedRain(scopes, empty, 40, 7)
	if got, want := field.window.String(), "60m"; got != want {
		t.Fatalf("Rain starts at window %s, want %s", got, want)
	}
	for _, want := range []string{"30m", "15m", "15m"} {
		field = field.windowed(-1, scopes, empty)
		if got := field.window.String(); got != want {
			t.Fatalf("shortening reached window %s, want %s", got, want)
		}
	}
	for _, want := range []string{"30m", "60m", "60m"} {
		field = field.windowed(1, scopes, empty)
		if got := field.window.String(); got != want {
			t.Fatalf("lengthening reached window %s, want %s", got, want)
		}
	}
}

// TestRainWindowReconcilesAgainstTheCurrentSnapshot guards A-054: shortening
// removes newly out-of-window items and lengthening restores eligible ones with
// distributed placement while surviving phases remain.
func TestRainWindowReconcilesAgainstTheCurrentSnapshot(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	young := rainEvidence(t, "young", repository, time.Minute)
	old := rainEvidence(t, "old", repository, 20*time.Minute)
	snapshot := rainSnapshot(scopes, young, old)
	field := startedRain(scopes, snapshot, 40, 7)
	field = field.ticked(field.chain, rainAt(500*time.Millisecond))
	phase := rainItemOf(t, field, "young", scope).rowPhase

	field = field.windowed(-1, scopes, snapshot).windowed(-1, scopes, snapshot)
	if rainAdmits(field, "old", scope) {
		t.Error("shortening the window kept an item beyond the new window")
	}
	if got := rainItemOf(t, field, "young", scope).rowPhase; got != phase {
		t.Errorf("shortening the window moved the survivor to phase %d, want the retained %d", got, phase)
	}

	field = field.windowed(1, scopes, snapshot)
	restored := rainItemOf(t, field, "old", scope)
	if restored.rowPhase != rainRowHash("old", scope) {
		t.Errorf("the restored historical item has phase %d, want the distributed row hash %d", restored.rowPhase, rainRowHash("old", scope))
	}
	if got := rainItemOf(t, field, "young", scope).rowPhase; got != phase {
		t.Errorf("lengthening the window moved the survivor to phase %d, want the retained %d", got, phase)
	}
}

// TestRainResizePreservesAgeAdmissionAndPhase guards A-053: resize maps the row
// modulo the new height and leaves age, admission, pause state, queue order,
// and omission totals unchanged.
func TestRainResizePreservesAgeAdmissionAndPhase(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	retained := make([]domain.EventEvidence, 0, 51)
	for index := range 51 {
		retained = append(retained, rainEvidence(t, fmt.Sprintf("e%02d", index), repository, time.Duration(index)*time.Minute))
	}
	field := startedRain(scopes, rainSnapshot(scopes, retained...), 40, 7)
	field = field.ticked(field.chain, rainAt(500*time.Millisecond))
	before := field.items

	resized := field.resized(25, 3)
	if !reflect.DeepEqual(resized.items, before) {
		t.Error("resize changed the admitted items")
	}
	if resized.counts != field.counts {
		t.Errorf("resize changed the prepared counts to %+v, want %+v", resized.counts, field.counts)
	}
	item := rainItemOf(t, resized, "e00", scopes.Ordered()[0])
	if got, want := item.row(3), int(item.rowPhase%3); got != want {
		t.Errorf("the resized row is %d, want the phase modulo the new height %d", got, want)
	}
}

// TestRainZeroHeightKeepsPhaseAndStillExpires guards A-053: at height zero,
// age and expiry continue while the row phase is unchanged and no modulo runs.
func TestRainZeroHeightKeepsPhaseAndStillExpires(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	expiring := rainEvidence(t, "expiring", repository, time.Hour-time.Second)
	surviving := rainEvidence(t, "surviving", repository, 0)
	field := startedRain(scopes, rainSnapshot(scopes, expiring, surviving), 40, 0)
	phase := rainItemOf(t, field, "surviving", scope).rowPhase

	field = field.ticked(field.chain, rainAt(time.Second))
	if rainAdmits(field, "expiring", scope) {
		t.Error("a zero-height field did not expire an item at its window boundary")
	}
	if got := rainItemOf(t, field, "surviving", scope).rowPhase; got != phase {
		t.Errorf("a zero-height tick changed the phase to %d, want the unchanged %d", got, phase)
	}
	if got, want := rainItemOf(t, field, "surviving", scope).age, time.Second; got != want {
		t.Errorf("a zero-height tick aged the item to %s, want %s", got, want)
	}
}

// TestRainAnchorFollowsScopeChanges guards A-053: the fixed page containing the
// anchor is chosen after a resize or Scope change, a removed anchor selects the
// successor now at its index or the final predecessor, an empty set clears the
// anchor, and the next nonempty set selects its first Scope.
func TestRainAnchorFollowsScopeChanges(t *testing.T) {
	all := make([]domain.Scope, 0, 8)
	for index := range 8 {
		all = append(all, domain.NewRepositoryScope(testRepository(t, fmt.Sprintf("acme/r%d", index))))
	}
	eight := scopeSet(t, all...)
	field := startedRain(eight, rainSnapshot(eight), 25, 7)
	field = field.paged(1).paged(1)
	if got, want := field.start, 4; got != want {
		t.Fatalf("two forward pages at K=2 start at Scope index %d, want %d", got, want)
	}

	resized := field.resized(12, 7)
	if got, want := resized.start, 4; got != want {
		t.Errorf("resizing to width 12 starts at Scope index %d, want the anchor's %d", got, want)
	}

	withoutAnchor := scopeSet(t, append(append([]domain.Scope{}, all[:4]...), all[5:]...)...)
	successor := resized.reconciled(withoutAnchor, rainSnapshot(withoutAnchor), rainBase)
	if got, want := successor.scopes[successor.start].Identity(), all[5].Identity(); got != want {
		t.Errorf("a removed anchor selected %v, want the successor at its index %v", got, want)
	}

	shorter := scopeSet(t, all[:3]...)
	predecessor := resized.reconciled(shorter, rainSnapshot(shorter), rainBase)
	if got, want := predecessor.scopes[predecessor.start].Identity(), all[2].Identity(); got != want {
		t.Errorf("a removed anchor with no successor selected %v, want the final predecessor %v", got, want)
	}
}

// TestRainPagingWrapsAtBothEnds guards RG-006: `[` and `]` move one fixed page
// and wrap, so manual paging reaches every Scope without starvation.
func TestRainPagingWrapsAtBothEnds(t *testing.T) {
	all := make([]domain.Scope, 0, 8)
	for index := range 8 {
		all = append(all, domain.NewRepositoryScope(testRepository(t, fmt.Sprintf("acme/r%d", index))))
	}
	scopes := scopeSet(t, all...)
	field := startedRain(scopes, rainSnapshot(scopes), 40, 7)
	for _, want := range []int{3, 6, 0} {
		field = field.paged(1)
		if field.start != want {
			t.Fatalf("paging forward reached Scope index %d, want %d", field.start, want)
		}
	}
	for _, want := range []int{6, 3, 0} {
		field = field.paged(-1)
		if field.start != want {
			t.Fatalf("paging backward reached Scope index %d, want %d", field.start, want)
		}
	}
}

// TestRainIsDeterministic guards RG-006: identical inputs yield identical state.
func TestRainIsDeterministic(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)), pathScope(t, repository, "src"))
	var retained []domain.EventEvidence
	for index := range 40 {
		evidence := rainEvidence(t, fmt.Sprintf("e%02d", index), repository, time.Duration(index)*time.Minute)
		evidence.Outcome = domain.CompleteOutcome(domain.ProvenanceEventTime, mustChangedPaths(t, "src/main.go"))
		retained = append(retained, evidence)
	}
	snapshot := rainSnapshot(scopes, retained...)

	run := func() rain {
		field := startedRain(scopes, snapshot, 40, 7)
		field = field.ticked(field.chain, rainAt(750*time.Millisecond))
		field = field.toggledPause().reconciled(scopes, snapshot, rainAt(time.Second)).toggledPause()
		return field.windowed(-1, scopes, snapshot).resized(25, 5).paged(1)
	}
	first, second := run(), run()
	if !reflect.DeepEqual(first.items, second.items) {
		t.Error("identical inputs produced different Rain items")
	}
	if first.counts != second.counts || first.start != second.start || first.window != second.window {
		t.Error("identical inputs produced different prepared Rain state")
	}
}

// TestRainPausedWindowChangeRestoresHistoricalItemsToTheField guards RG-006's
// paused window rule: "Window changes while paused reconcile both frozen field
// and current queue eligibility; restored historical field items use
// distributed placement and do not consume paused-arrival capacity." A
// lengthening window therefore readmits the historical representation to the
// field at its stable distributed row rather than routing it through the
// bounded arrival queue.
func TestRainPausedWindowChangeRestoresHistoricalItemsToTheField(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	young := rainEvidence(t, "young", repository, time.Minute)
	old := rainEvidence(t, "old", repository, 20*time.Minute)
	snapshot := rainSnapshot(scopes, young, old)

	field := startedRain(scopes, snapshot, 40, 7)
	field = field.windowed(-1, scopes, snapshot).windowed(-1, scopes, snapshot)
	if rainAdmits(field, "old", scope) {
		t.Fatal("shortening to 15m kept the out-of-window item")
	}

	field = field.toggledPause().windowed(1, scopes, snapshot)
	if !rainAdmits(field, "old", scope) {
		t.Fatal("lengthening the window while paused did not restore the historical item to the field")
	}
	if got, want := rainItemOf(t, field, "old", scope).rowPhase, rainRowHash("old", scope); got != want {
		t.Errorf("the restored historical item has phase %d, want the distributed row hash %d", got, want)
	}
	if len(field.queue) != 0 {
		t.Errorf("the restoration consumed %d paused-arrival slots, want none", len(field.queue))
	}
	if field.counts.pausedOmitted != 0 {
		t.Errorf("the restoration recorded %d paused omissions, want none", field.counts.pausedOmitted)
	}
	if !rainAdmits(field, "young", scope) {
		t.Error("the surviving frozen item left the field")
	}
}

// TestRainPausedWindowChangeReconcilesQueueEligibility guards the same RG-006
// sentence's other half: a window change while paused also reconciles current
// queue eligibility, so a queued arrival outside the shortened window is
// removed while the queue keeps the eligible ones.
func TestRainPausedWindowChangeReconcilesQueueEligibility(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	field := startedRain(scopes, rainSnapshot(scopes), 40, 7).toggledPause()

	fresh := rainEvidence(t, "fresh", repository, time.Minute)
	stale := rainEvidence(t, "stale", repository, 20*time.Minute)
	snapshot := rainSnapshot(scopes, fresh, stale)
	field = field.reconciled(scopes, snapshot, rainBase)
	if len(field.queue) != 2 {
		t.Fatalf("the paused refresh queued %d arrivals, want 2", len(field.queue))
	}

	field = field.windowed(-1, scopes, snapshot).windowed(-1, scopes, snapshot)
	if queuedHas(field, "stale", scope) {
		t.Error("shortening the window while paused kept an out-of-window queue entry")
	}
	if !queuedHas(field, "fresh", scope) {
		t.Error("shortening the window while paused dropped an eligible queue entry")
	}
	if rainAdmits(field, "fresh", scope) {
		t.Error("a queued paused arrival was promoted into the frozen field by a window change")
	}
}

// TestRainCandidateTiesBreakByEventIDByteOrder guards RG-006's candidate
// ordering: equal event timestamps are separated by event ID byte order, which
// decides which representation survives the column cap.
func TestRainCandidateTiesBreakByEventIDByteOrder(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	retained := make([]domain.EventEvidence, 0, rainColumnCap+1)
	for index := range rainColumnCap + 1 {
		retained = append(retained, rainEvidence(t, fmt.Sprintf("tie%02d", index), repository, time.Minute))
	}
	// The fixture is published newest-last so a stable order that ignored the
	// event ID would retain a different representation than byte order does.
	slices.Reverse(retained)

	field := startedRain(scopes, rainSnapshot(scopes, retained...), 40, 7)
	if got, want := field.counts.admitted, rainColumnCap; got != want {
		t.Fatalf("admitted %d tied candidates, want the %d-item column cap", got, want)
	}
	if !rainAdmits(field, "tie00", scope) {
		t.Error("the lowest event ID lost the tie, want it retained by byte order")
	}
	if rainAdmits(field, fmt.Sprintf("tie%02d", rainColumnCap), scope) {
		t.Error("the highest event ID won the tie, want it omitted by byte order")
	}
}
