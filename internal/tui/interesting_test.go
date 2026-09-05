package tui

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// stripBase is the explicit logical reference every Interesting Now fixture is
// stated against. The strip never reads the host clock, so every vector below
// is a pure offset from it.
var stripBase = time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

// stripAt returns the logical instant the given offset past the fixture base.
func stripAt(offset time.Duration) time.Time { return stripBase.Add(offset) }

// stripEvidence builds one retained push of the repository occurring the given
// time before the fixture base. A repository Scope decides membership from
// identity alone, so the evidence outcome stays the zero value.
func stripEvidence(t *testing.T, id, repository string, since time.Duration) domain.EventEvidence {
	t.Helper()
	event := testEvent(t, id, repository, domain.CategoryPush, domain.EntityCommit)
	event.OccurredAt = stripBase.Add(-since)
	return domain.EventEvidence{Event: event}
}

// stripSnapshot builds the prepared per-Scope snapshot one successful refresh
// publishes for the retained evidence.
func stripSnapshot(scopes domain.ScopeSet, retained ...domain.EventEvidence) domain.ScopedSnapshot {
	return domain.NewRetainedSnapshot(scopes, retained, false)
}

// startedStrip returns the strip after its first successful snapshot at the
// fixture base, which is where a strip reference first exists.
func startedStrip(scopes domain.ScopeSet, snapshot domain.ScopedSnapshot) interesting {
	return newInteresting().reconciled(scopes, snapshot, stripBase)
}

// stripIDs returns the stored entries' source event IDs in stored order.
func stripIDs(strip interesting) []string {
	ids := make([]string, 0, len(strip.entries))
	for _, entry := range strip.entries {
		ids = append(ids, entry.id)
	}
	return ids
}

// repositoryScope returns the repository Scope of the identifier.
func repositoryScope(t *testing.T, repository string) domain.Scope {
	t.Helper()
	return domain.NewRepositoryScope(testRepository(t, repository))
}

// stripRotationFixture returns the three-repository selection and the ten
// pushes per repository the rotation vectors share, which is more candidates
// than the 20-entry bound stores.
func stripRotationFixture(t *testing.T) (domain.ScopeSet, []domain.EventEvidence) {
	t.Helper()
	repositories := []string{"acme/api", "acme/web", "acme/ops"}
	scopes := scopeSet(t,
		repositoryScope(t, repositories[0]),
		repositoryScope(t, repositories[1]),
		repositoryScope(t, repositories[2]),
	)
	var retained []domain.EventEvidence
	for index := range 10 {
		for _, repository := range repositories {
			retained = append(retained, stripEvidence(t, fmt.Sprintf("%s-%02d", repository, index), repository, time.Duration(index)*time.Minute))
		}
	}
	return scopes, retained
}

// TestInterestingStoresTwentyAndShowsFive guards RG-007's bounds: 30 eligible
// events store exactly 20 entries, report the rest as capacity-omitted, and
// leave five visible when height permits.
func TestInterestingStoresTwentyAndShowsFive(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository))
	var retained []domain.EventEvidence
	for index := range 30 {
		retained = append(retained, stripEvidence(t, fmt.Sprintf("e%02d", index), repository, time.Duration(index)*10*time.Second))
	}
	strip := startedStrip(scopes, stripSnapshot(scopes, retained...))

	if got, want := strip.counts, (interestingCounts{eligible: 30, stored: 20, visible: 5, omitted: 10}); got != want {
		t.Fatalf("30 eligible events prepared counts %+v, want %+v", got, want)
	}
	if got, want := len(strip.entries), 20; got != want {
		t.Fatalf("the strip stored %d entries, want %d", got, want)
	}
	if got, want := stripIDs(strip)[0], "e00"; got != want {
		t.Errorf("the first stored entry is %q, want %q", got, want)
	}
	if got, want := len(strip.visible()), 5; got != want {
		t.Errorf("the strip made %d entries visible, want %d", got, want)
	}
	if got, want := stripIDs(strip)[19], "e19"; got != want {
		t.Errorf("the last stored entry is %q, want %q", got, want)
	}
}

// TestInterestingEligibilityWindowIsHalfOpen guards RG-007 eligibility: an
// event at 14m59.999s is eligible and renders `14m`, and one at exactly 15
// minutes is not.
func TestInterestingEligibilityWindowIsHalfOpen(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository))
	inside := stripEvidence(t, "inside", repository, 15*time.Minute-time.Millisecond)
	outside := stripEvidence(t, "outside", repository, 15*time.Minute)
	strip := startedStrip(scopes, stripSnapshot(scopes, inside, outside))

	if got, want := stripIDs(strip), []string{"inside"}; !slices.Equal(got, want) {
		t.Fatalf("the window stored %v, want %v", got, want)
	}
	if got, want := strip.entries[0].ageText(), "14m"; got != want {
		t.Errorf("an age of 14m59.999s renders %q, want %q", got, want)
	}
	if got, want := strip.counts.eligible, 1; got != want {
		t.Errorf("the strip counted %d eligible events, want %d", got, want)
	}
}

// TestInterestingFutureTimestampClampsToZero guards RG-007: an event stamped
// after the strip reference cannot be older than it and renders `0s`.
func TestInterestingFutureTimestampClampsToZero(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository))
	ahead := stripEvidence(t, "ahead", repository, -time.Minute)
	strip := startedStrip(scopes, stripSnapshot(scopes, ahead))

	if got, want := len(strip.entries), 1; got != want {
		t.Fatalf("a future event stored %d entries, want %d", got, want)
	}
	if got, want := strip.entries[0].ageText(), "0s"; got != want {
		t.Errorf("a future event renders age %q, want %q", got, want)
	}
}

// TestInterestingAgeTextTruncatesTowardZero guards RG-007's textual age units.
func TestInterestingAgeTextTruncatesTowardZero(t *testing.T) {
	for _, test := range []struct {
		age  time.Duration
		want string
	}{
		{age: -time.Second, want: "0s"},
		{age: 0, want: "0s"},
		{age: 59*time.Second + 999*time.Millisecond, want: "59s"},
		{age: time.Minute, want: "1m"},
		{age: time.Hour - time.Millisecond, want: "59m"},
		{age: time.Hour, want: "1h"},
		{age: 24*time.Hour - time.Millisecond, want: "23h"},
		{age: 25 * time.Hour, want: "1d"},
	} {
		if got := (interestingEntry{age: test.age}).ageText(); got != test.want {
			t.Errorf("an age of %s renders %q, want %q", test.age, got, test.want)
		}
	}
}

// TestInterestingOrdersTiesByEventID guards RG-007 ordering: equal timestamps
// break by source event ID in byte order, never by payload equality.
func TestInterestingOrdersTiesByEventID(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository))
	strip := startedStrip(scopes, stripSnapshot(scopes,
		stripEvidence(t, "b", repository, time.Minute),
		stripEvidence(t, "a", repository, time.Minute),
		stripEvidence(t, "c", repository, time.Minute),
	))

	if got, want := stripIDs(strip), []string{"a", "b", "c"}; !slices.Equal(got, want) {
		t.Fatalf("tied events stored %v, want %v", got, want)
	}
}

// TestInterestingRepeatedSourceIDStoresOneEntry guards RG-007 identity: the
// source event ID is the strip identity and the first occurrence is kept.
func TestInterestingRepeatedSourceIDStoresOneEntry(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository))
	first := stripEvidence(t, "same", repository, time.Minute)
	first.Event.Description = "first"
	repeat := stripEvidence(t, "same", repository, time.Minute)
	repeat.Event.Description = "repeat"
	strip := startedStrip(scopes, stripSnapshot(scopes, first, repeat))

	if got, want := len(strip.entries), 1; got != want {
		t.Fatalf("a repeated source ID stored %d entries, want %d", got, want)
	}
	if got, want := strip.entries[0].description, "first"; got != want {
		t.Errorf("the stored entry kept description %q, want %q", got, want)
	}
	if got, want := strip.counts.eligible, 1; got != want {
		t.Errorf("a repeated source ID counted %d eligible events, want %d", got, want)
	}
}

// TestInterestingMultiScopeEventIsOneEntry guards RG-007: one event confirmed
// in several Scopes is one entry retaining every confirmed Scope in stable
// order, with the selecting Scope as its sponsor.
func TestInterestingMultiScopeEventIsOneEntry(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository), pathScope(t, repository, "src"))
	shared := stripEvidence(t, "shared", repository, time.Minute)
	shared.Outcome = domain.CompleteOutcome(domain.ProvenanceCurrentPR, mustChangedPaths(t, "src/main.go"))
	strip := startedStrip(scopes, stripSnapshot(scopes, shared))

	if got, want := len(strip.entries), 1; got != want {
		t.Fatalf("a two-Scope event stored %d entries, want %d", got, want)
	}
	entry := strip.entries[0]
	if got, want := len(entry.scopes), 2; got != want {
		t.Fatalf("the entry retained %d confirmed Scopes, want %d", got, want)
	}
	if got, want := entry.sponsor.Identity(), scopes.Ordered()[0].Identity(); got != want {
		t.Errorf("the entry's sponsoring Scope is %v, want %v", got, want)
	}
	if got, want := entry.additionalScopes(), 1; got != want {
		t.Errorf("the entry reports %d additional Scopes, want %d", got, want)
	}
	if got, want := entry.qualifiedScopes(), 1; got != want {
		t.Errorf("the entry reports %d current-PR qualified Scopes, want %d", got, want)
	}
}

// TestInterestingOverlapConsumesTheFairTurn guards RG-007 fairness: a Scope
// whose turn reaches an already selected event spends that turn instead of
// granting the overlapping event a second entry.
func TestInterestingOverlapConsumesTheFairTurn(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository), pathScope(t, repository, "src"))
	shared := stripEvidence(t, "shared", repository, time.Minute)
	shared.Outcome = domain.CompleteOutcome(domain.ProvenanceEventTime, mustChangedPaths(t, "src/main.go"))
	elsewhere := stripEvidence(t, "elsewhere", repository, 2*time.Minute)
	elsewhere.Outcome = domain.CompleteOutcome(domain.ProvenanceEventTime, mustChangedPaths(t, "docs/readme.md"))
	strip := startedStrip(scopes, stripSnapshot(scopes, shared, elsewhere))

	if got, want := stripIDs(strip), []string{"shared", "elsewhere"}; !slices.Equal(got, want) {
		t.Fatalf("the overlap stored %v, want %v", got, want)
	}
	if got, want := strip.entries[1].sponsor.Identity(), scopes.Ordered()[0].Identity(); got != want {
		t.Errorf("the second entry's sponsoring Scope is %v, want the repository Scope %v", got, want)
	}
}

// TestInterestingUnknownAndNotMemberAreIneligible guards RG-007 eligibility:
// only a confirmed member outcome makes a candidate.
func TestInterestingUnknownAndNotMemberAreIneligible(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, pathScope(t, repository, "src"))
	unknown := stripEvidence(t, "unknown", repository, time.Minute)
	unknown.Outcome = domain.IncompleteOutcome("no evidence")
	notMember := stripEvidence(t, "elsewhere", repository, time.Minute)
	notMember.Outcome = domain.CompleteOutcome(domain.ProvenanceEventTime, mustChangedPaths(t, "docs/readme.md"))
	strip := startedStrip(scopes, stripSnapshot(scopes, unknown, notMember))

	if got, want := len(strip.entries), 0; got != want {
		t.Fatalf("undecided and not-member outcomes stored %d entries, want %d", got, want)
	}
	if got, want := strip.counts, (interestingCounts{}); got != want {
		t.Errorf("undecided and not-member outcomes prepared counts %+v, want %+v", got, want)
	}
}

// TestInterestingEventMemberInOneScopeIsEligibleThroughIt guards RG-007: an
// event member in one Scope and unknown in another is eligible through its
// confirmed membership alone.
func TestInterestingEventMemberInOneScopeIsEligibleThroughIt(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository), pathScope(t, repository, "src"))
	partial := stripEvidence(t, "partial", repository, time.Minute)
	partial.Outcome = domain.IncompleteOutcome("no evidence")
	strip := startedStrip(scopes, stripSnapshot(scopes, partial))

	if got, want := len(strip.entries), 1; got != want {
		t.Fatalf("a partially decided event stored %d entries, want %d", got, want)
	}
	if got, want := len(strip.entries[0].scopes), 1; got != want {
		t.Errorf("the entry retained %d confirmed Scopes, want %d", got, want)
	}
}

// TestInterestingTurnsAlternateBetweenScopes guards RG-007 fairness: a busier
// Scope never consumes consecutive turns while another Scope still has
// candidates.
func TestInterestingTurnsAlternateBetweenScopes(t *testing.T) {
	scopes := scopeSet(t, repositoryScope(t, "acme/api"), repositoryScope(t, "acme/web"))
	strip := startedStrip(scopes, stripSnapshot(scopes,
		stripEvidence(t, "a1", "acme/api", time.Minute),
		stripEvidence(t, "a2", "acme/api", 2*time.Minute),
		stripEvidence(t, "a3", "acme/api", 3*time.Minute),
		stripEvidence(t, "b1", "acme/web", 4*time.Minute),
		stripEvidence(t, "b2", "acme/web", 5*time.Minute),
	))

	want := []string{"a1", "b1", "a2", "b2", "a3"}
	if got := stripIDs(strip); !slices.Equal(got, want) {
		t.Fatalf("Scope-fair turns stored %v, want %v", got, want)
	}
}

// TestInterestingRotationAdvancesPastTheTurnThatFilled guards RG-007's
// anti-starvation rotation: the next refresh starts at the Scope after the turn
// that reached the 20-entry bound.
func TestInterestingRotationAdvancesPastTheTurnThatFilled(t *testing.T) {
	scopes, retained := stripRotationFixture(t)
	snapshot := stripSnapshot(scopes, retained...)
	strip := startedStrip(scopes, snapshot)

	// Twenty turns starting at index 0 make turn 20 the Scope at index 1, so the
	// next refresh owes its first turn to the Scope after it.
	if got, want := strip.nextStart, 2; got != want {
		t.Fatalf("the saved next start is Scope %d, want %d", got, want)
	}
	ordered := scopes.Ordered()
	if got, want := strip.entries[0].sponsor.Identity(), ordered[0].Identity(); got != want {
		t.Fatalf("the first refresh started at %v, want %v", got, want)
	}

	next := strip.reconciled(scopes, snapshot, stripBase)
	if got, want := next.entries[0].sponsor.Identity(), ordered[2].Identity(); got != want {
		t.Errorf("the second refresh started at %v, want the saved Scope %v", got, want)
	}
}

// TestInterestingRotationHoldsWhenQueuesExhaust guards RG-007: a refresh that
// stores fewer than 20 entries leaves the saved next start where it was, so an
// idle selection never rotates away from its first Scope.
func TestInterestingRotationHoldsWhenQueuesExhaust(t *testing.T) {
	scopes := scopeSet(t, repositoryScope(t, "acme/api"), repositoryScope(t, "acme/web"))
	snapshot := stripSnapshot(scopes, stripEvidence(t, "b1", "acme/web", time.Minute))
	strip := startedStrip(scopes, snapshot)

	if got, want := strip.nextStart, 0; got != want {
		t.Fatalf("an unfilled refresh saved next start %d, want %d", got, want)
	}
	if got, want := stripIDs(strip), []string{"b1"}; !slices.Equal(got, want) {
		t.Errorf("an unfilled refresh stored %v, want %v", got, want)
	}
}

// TestInterestingScopeRemovalRemapsTheRotation guards RG-007: removing the
// saved Scope hands its turn to the successor now at its old index, and the
// final predecessor when no successor remains.
func TestInterestingScopeRemovalRemapsTheRotation(t *testing.T) {
	api, web, ops := repositoryScope(t, "acme/api"), repositoryScope(t, "acme/web"), repositoryScope(t, "acme/ops")
	scopes := scopeSet(t, api, web, ops)
	ordered := scopes.Ordered()
	strip := startedStrip(scopes, stripSnapshot(scopes))
	strip.nextStart, strip.nextAnchor, strip.nextAnchored = 1, ordered[1].Identity(), true

	remaining := scopeSet(t, ordered[0], ordered[2])
	strip = strip.reconciled(remaining, stripSnapshot(remaining), stripBase)
	if got, want := strip.nextStart, 1; got != want {
		t.Fatalf("removing the saved Scope left next start %d, want %d", got, want)
	}
	if got, want := strip.nextAnchor, remaining.Ordered()[1].Identity(); got != want {
		t.Errorf("the rotation anchor is %v, want the successor %v", got, want)
	}

	last := scopeSet(t, remaining.Ordered()[0])
	strip = strip.reconciled(last, stripSnapshot(last), stripBase)
	if got, want := strip.nextStart, 0; got != want {
		t.Errorf("with no successor the rotation kept next start %d, want %d", got, want)
	}

	empty := domain.ScopeSet{}
	strip = strip.reconciled(empty, stripSnapshot(empty), stripBase)
	if strip.nextAnchored {
		t.Errorf("an empty Scope set kept a rotation anchor, want it cleared")
	}
	if got, want := strip.nextStart, 0; got != want {
		t.Errorf("an empty Scope set left next start %d, want %d", got, want)
	}
}

// TestInterestingTickAgesAndExpiresEntries guards RG-007: an accepted tick
// advances the monotonic strip reference, recomputes ages, and removes entries
// that reach the fixed 15-minute window.
func TestInterestingTickAgesAndExpiresEntries(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository))
	strip := startedStrip(scopes, stripSnapshot(scopes,
		stripEvidence(t, "older", repository, 14*time.Minute),
		stripEvidence(t, "newer", repository, time.Minute),
	))

	strip = strip.ticked(stripAt(59 * time.Second))
	if got, want := stripIDs(strip), []string{"newer", "older"}; !slices.Equal(got, want) {
		t.Fatalf("a 59-second tick stored %v, want %v", got, want)
	}
	if got, want := strip.entries[1].ageText(), "14m"; got != want {
		t.Fatalf("the aging entry renders %q, want %q", got, want)
	}
	strip = strip.ticked(stripAt(time.Minute))
	if got, want := stripIDs(strip), []string{"newer"}; !slices.Equal(got, want) {
		t.Fatalf("reaching 15 minutes stored %v, want %v", got, want)
	}
	if got, want := strip.entries[0].ageText(), "2m"; got != want {
		t.Errorf("the surviving entry renders age %q, want %q", got, want)
	}
	if got, want := strip.counts.eligible, 1; got != want {
		t.Errorf("expiry left %d eligible events, want %d", got, want)
	}
}

// TestInterestingTickRetainsTheSelectionStart guards RG-007: tick recomputation
// of one snapshot keeps the start the refresh used, so ageing never rotates the
// fair order underneath the operator.
func TestInterestingTickRetainsTheSelectionStart(t *testing.T) {
	scopes, retained := stripRotationFixture(t)
	strip := startedStrip(scopes, stripSnapshot(scopes, retained...))
	before := stripIDs(strip)

	ticked := strip.ticked(stripAt(time.Second))
	if got := stripIDs(ticked); !slices.Equal(got, before) {
		t.Fatalf("a tick reordered the selection to %v, want %v", got, before)
	}
	if got, want := ticked.start, strip.start; got != want {
		t.Errorf("a tick moved the selection start to %d, want %d", got, want)
	}
	if got, want := ticked.nextStart, strip.nextStart; got != want {
		t.Errorf("a tick moved the saved next start to %d, want %d", got, want)
	}
}

// TestInterestingRejectsOlderAndEqualTicks guards RG-007: the strip reference is
// monotonic, so a repeated or older tick changes neither reference nor ages.
func TestInterestingRejectsOlderAndEqualTicks(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository))
	strip := startedStrip(scopes, stripSnapshot(scopes, stripEvidence(t, "only", repository, time.Minute)))
	advanced := strip.ticked(stripAt(time.Minute))

	for name, at := range map[string]time.Time{
		"repeated": stripAt(time.Minute),
		"older":    stripAt(30 * time.Second),
	} {
		if got := advanced.ticked(at); got.reference != advanced.reference {
			t.Errorf("a %s tick moved the strip reference to %s, want %s", name, got.reference, advanced.reference)
		}
	}
	if got, want := advanced.entries[0].ageText(), "2m"; got != want {
		t.Errorf("the entry renders age %q, want %q", got, want)
	}
}

// TestInterestingIgnoresTicksBeforeTheFirstSnapshot guards RG-007: no strip
// reference exists before the first success, so a tick establishes none.
func TestInterestingIgnoresTicksBeforeTheFirstSnapshot(t *testing.T) {
	strip := newInteresting().ticked(stripAt(time.Minute))
	if strip.started {
		t.Fatalf("a tick before the first snapshot started the strip")
	}
	if got, want := len(strip.entries), 0; got != want {
		t.Errorf("a tick before the first snapshot stored %d entries, want %d", got, want)
	}
}

// TestInterestingRefreshReplacesWithoutStickySlots guards RG-007: a successful
// refresh recomputes selection from its complete snapshot, so a survivor holds
// no slot a newer direct event should take.
func TestInterestingRefreshReplacesWithoutStickySlots(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository))
	strip := startedStrip(scopes, stripSnapshot(scopes, stripEvidence(t, "survivor", repository, 5*time.Minute)))
	if got, want := stripIDs(strip), []string{"survivor"}; !slices.Equal(got, want) {
		t.Fatalf("the first refresh stored %v, want %v", got, want)
	}

	strip = strip.reconciled(scopes, stripSnapshot(scopes,
		stripEvidence(t, "survivor", repository, 5*time.Minute),
		stripEvidence(t, "arrival", repository, time.Minute),
	), stripBase)
	if got, want := stripIDs(strip), []string{"arrival", "survivor"}; !slices.Equal(got, want) {
		t.Errorf("the second refresh stored %v, want %v", got, want)
	}
}

// TestInterestingOlderRefreshReferenceKeepsTheStripReference guards RG-007: a
// successful publication replaces its snapshot even when its reference is older,
// while the monotonic strip reference and the ages it drives never move back.
func TestInterestingOlderRefreshReferenceKeepsTheStripReference(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository))
	strip := startedStrip(scopes, stripSnapshot(scopes, stripEvidence(t, "first", repository, time.Minute)))
	strip = strip.ticked(stripAt(2 * time.Minute))

	strip = strip.reconciled(scopes, stripSnapshot(scopes, stripEvidence(t, "second", repository, time.Minute)), stripAt(-time.Minute))
	if got, want := strip.reference, stripAt(2*time.Minute); !got.Equal(want) {
		t.Fatalf("an older refresh moved the strip reference to %s, want %s", got, want)
	}
	if got, want := stripIDs(strip), []string{"second"}; !slices.Equal(got, want) {
		t.Fatalf("an older refresh stored %v, want %v", got, want)
	}
	if got, want := strip.entries[0].ageText(), "3m"; got != want {
		t.Errorf("the replaced entry renders age %q, want %q", got, want)
	}
}

// TestInterestingEmptySelectionStoresNothing guards RG-007's empty result: an
// empty snapshot prepares no entries and no counts at all.
func TestInterestingEmptySelectionStoresNothing(t *testing.T) {
	scopes := scopeSet(t, repositoryScope(t, "acme/api"))
	strip := startedStrip(scopes, stripSnapshot(scopes))

	if got, want := len(strip.entries), 0; got != want {
		t.Fatalf("an empty snapshot stored %d entries, want %d", got, want)
	}
	if got, want := strip.counts, (interestingCounts{}); got != want {
		t.Errorf("an empty snapshot prepared counts %+v, want %+v", got, want)
	}
	if got := strip.visible(); len(got) != 0 {
		t.Errorf("an empty snapshot made %d entries visible, want none", len(got))
	}
}

// TestInterestingEntryRetainsExplainableFacts guards RG-007's entry
// explanation: every prepared fact is a direct normalized or application fact.
func TestInterestingEntryRetainsExplainableFacts(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository))
	evidence := stripEvidence(t, "detailed", repository, 90*time.Second)
	evidence.Event.Actor = "octocat"
	evidence.Event.EntityKind = domain.EntityPullRequest
	evidence.Event.EntityRef = "#42"
	evidence.Event.Description = "opened pull request"
	strip := startedStrip(scopes, stripSnapshot(scopes, evidence))

	entry := strip.entries[0]
	if got, want := entry.id, "detailed"; got != want {
		t.Errorf("the entry ID is %q, want %q", got, want)
	}
	if got, want := entry.occurredAt, stripBase.Add(-90*time.Second); !got.Equal(want) {
		t.Errorf("the entry timestamp is %s, want %s", got, want)
	}
	if got, want := entry.repository, evidence.Event.Repository; got != want {
		t.Errorf("the entry repository is %v, want %v", got, want)
	}
	if got, want := entry.actor, "octocat"; got != want {
		t.Errorf("the entry actor is %q, want %q", got, want)
	}
	if got, want := entry.category, domain.CategoryPush; got != want {
		t.Errorf("the entry category is %q, want %q", got, want)
	}
	if got, want := entry.entityKind, domain.EntityPullRequest; got != want {
		t.Errorf("the entry entity kind is %q, want %q", got, want)
	}
	if got, want := entry.entityRef, "#42"; got != want {
		t.Errorf("the entry entity reference is %q, want %q", got, want)
	}
	if got, want := entry.description, "opened pull request"; got != want {
		t.Errorf("the entry description is %q, want %q", got, want)
	}
	if got, want := entry.ageText(), "1m"; got != want {
		t.Errorf("the entry renders age %q, want %q", got, want)
	}
	if got, want := entry.qualifiedScopes(), 0; got != want {
		t.Errorf("the entry reports %d qualified Scopes, want %d", got, want)
	}
}

// TestInterestingRepeatedSourceIDKeepsTheFirstOccurrenceVerdict guards RG-007
// identity: the first canonical occurrence of a repeated source ID decides the
// entry, so a later occurrence of an ineligible ID is not a second payload
// choice the strip is allowed to make.
func TestInterestingRepeatedSourceIDKeepsTheFirstOccurrenceVerdict(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, repositoryScope(t, repository))
	first := stripEvidence(t, "same", repository, 20*time.Minute)
	repeat := stripEvidence(t, "same", repository, time.Minute)
	strip := startedStrip(scopes, stripSnapshot(scopes, first, repeat))

	if got, want := len(strip.entries), 0; got != want {
		t.Fatalf("a repeated ineligible source ID stored %d entries, want %d", got, want)
	}
	if got, want := strip.counts.eligible, 0; got != want {
		t.Errorf("a repeated ineligible source ID counted %d eligible events, want %d", got, want)
	}
}
