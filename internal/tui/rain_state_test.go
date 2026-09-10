package tui

import (
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// rainPathEvidence builds one retained push of the repository occurring the
// given time before the fixture base, settled at complete event-time evidence
// for the changed paths so a path Scope decides membership from them.
func rainPathEvidence(t *testing.T, id, repository string, since time.Duration, paths ...string) domain.EventEvidence {
	t.Helper()
	evidence := rainEvidence(t, id, repository, since)
	if len(paths) > 0 {
		evidence.Outcome = completeEvidence(t, paths...)
	}
	return evidence
}

// rainKeys returns the admitted field's identities in storage order.
func rainKeys(field rain) []rainKey {
	keys := make([]rainKey, 0, len(field.items))
	for _, item := range field.items {
		keys = append(keys, item.key)
	}
	return keys
}

// TestRainRepagingKeepsAFoundAnchorAtIndexZero guards RG-006's fixed paging: a
// Scope-set change that leaves the retained anchor as the very first Scope
// selects the page holding it, index zero, rather than keeping the page start
// the larger selection had.
func TestRainRepagingKeepsAFoundAnchorAtIndexZero(t *testing.T) {
	selected := rainScopes(t, 8)
	scopes := scopeSet(t, selected...)
	field := startedRain(scopes, rainSnapshot(scopes, oneItemPerScope(t, selected)...), 40, 7)
	field = field.paged(1)
	if got, want := field.anchor, selected[3].Identity(); got != want {
		t.Fatalf("the second page anchored %v, want the fourth Scope", got)
	}

	remaining := scopeSet(t, selected[3:]...)
	field = field.reconciled(remaining, rainSnapshot(remaining), rainAt(time.Minute))
	page := field.field()
	if got, want := page.first, 1; got != want {
		t.Fatalf("the reselected page starts at Scope %d, want %d", got, want)
	}
	if got, want := page.columns[0].scope.Identity(), selected[3].Identity(); got != want {
		t.Errorf("the reselected page shows %v first, want the retained anchor", got)
	}
}

// TestRainRepagingReplacesARemovedAnchorWithItsFinalPredecessor guards RG-006:
// when the anchor Scope disappears and no Scope holds its old index any more,
// its old index names the final predecessor, and the page holding that
// predecessor becomes visible.
func TestRainRepagingReplacesARemovedAnchorWithItsFinalPredecessor(t *testing.T) {
	selected := rainScopes(t, 8)
	scopes := scopeSet(t, selected...)
	field := startedRain(scopes, rainSnapshot(scopes, oneItemPerScope(t, selected)...), 40, 7)
	field = field.paged(1).paged(1)
	if got, want := field.anchor, selected[6].Identity(); got != want {
		t.Fatalf("the third page anchored %v, want the seventh Scope", got)
	}

	remaining := scopeSet(t, selected[:6]...)
	field = field.reconciled(remaining, rainSnapshot(remaining), rainAt(time.Minute))
	page := field.field()
	if got, want := page.first, 4; got != want {
		t.Fatalf("the reselected page starts at Scope %d, want %d", got, want)
	}
	if got, want := page.columns[0].scope.Identity(), selected[3].Identity(); got != want {
		t.Errorf("the reselected page shows %v first, want the fourth Scope", got)
	}
}

// TestRainSurvivingAgeCatchesUpAfterARebasedRefresh guards RG-006: only the
// first input after a resume rebases without advancing. The next successful
// refresh is an ordinary one and reconciles surviving age against its own
// reference, so a frozen age is not carried forward a second time.
func TestRainSurvivingAgeCatchesUpAfterARebasedRefresh(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	snapshot := rainSnapshot(scopes, rainEvidence(t, "one", repository, 0))
	field := startedRain(scopes, snapshot, 40, 7)

	field = field.toggledPause().toggledPause()
	field = field.reconciled(scopes, snapshot, rainAt(10*time.Minute))
	if got := rainItemOf(t, field, "one", scope).age; got != 0 {
		t.Fatalf("the rebasing refresh advanced age to %s, want none", got)
	}

	field = field.reconciled(scopes, snapshot, rainAt(20*time.Minute))
	if got, want := rainItemOf(t, field, "one", scope).age, 20*time.Minute; got != want {
		t.Errorf("the next refresh left age at %s, want %s", got, want)
	}
}

// TestRainRebasingNeedsAReferenceAfterTheFrozenCursor guards RG-006: after a
// resume an equal-time input leaves `needsRebase` set and therefore is not the
// rebasing input, so it reconciles surviving age against the cursor instead of
// freezing it once more.
func TestRainRebasingNeedsAReferenceAfterTheFrozenCursor(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	snapshot := rainSnapshot(scopes, rainEvidence(t, "one", repository, 0))
	field := startedRain(scopes, snapshot, 40, 7)

	field = field.toggledPause().toggledPause()
	field = field.reconciled(scopes, snapshot, rainAt(10*time.Minute))
	field = field.toggledPause().toggledPause()
	if !field.needsRebase {
		t.Fatalf("the second resume did not wait for a rebasing input")
	}
	if field.rebasing(rainAt(10 * time.Minute)) {
		t.Fatalf("an equal-time input is treated as the rebasing input")
	}

	field = field.reconciled(scopes, snapshot, rainAt(10*time.Minute))
	if got, want := rainItemOf(t, field, "one", scope).age, 10*time.Minute; got != want {
		t.Errorf("the equal-time refresh left age at %s, want %s", got, want)
	}
}

// TestRainExpiryRemovesOnlyTheOutOfWindowItem guards RG-006: a tick that takes
// one item past the selected window removes exactly that representation and
// keeps every later one in the field.
func TestRainExpiryRemovesOnlyTheOutOfWindowItem(t *testing.T) {
	first, second := "acme/alpha", "acme/beta"
	scopes := scopeSet(t,
		domain.NewRepositoryScope(testRepository(t, first)),
		domain.NewRepositoryScope(testRepository(t, second)),
	)
	snapshot := rainSnapshot(scopes,
		rainPathEvidence(t, "old", first, 55*time.Minute),
		rainPathEvidence(t, "fresh", second, 0),
	)
	field := startedRainAt(rainWindow60m, scopes, snapshot, 40, 7)
	ordered := scopes.Ordered()
	if !rainAdmits(field, "old", ordered[0]) || !rainAdmits(field, "fresh", ordered[1]) {
		t.Fatalf("the first snapshot admitted %d items, want both", len(field.items))
	}

	field = field.ticked(field.chain, rainAt(6*time.Minute))
	if rainAdmits(field, "old", ordered[0]) {
		t.Errorf("the item past the 60m window is still admitted")
	}
	if !rainAdmits(field, "fresh", ordered[1]) {
		t.Errorf("the in-window item of the next Scope was removed with the out-of-window one")
	}
}

// TestRainWindowKeyBeforeTheFirstSnapshotBuildsNoField guards RG-006: before
// the first successful snapshot no Rain cursor or field exists, so `-` only
// selects the shorter preset and admits nothing.
func TestRainWindowKeyBeforeTheFirstSnapshotBuildsNoField(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	snapshot := rainSnapshot(scopes, rainEvidence(t, "one", repository, 0))

	field := newRain().resized(40, 7).windowed(-1, scopes, snapshot)
	if got, want := field.window.String(), "6h"; got != want {
		t.Errorf("`-` before the first snapshot selected window %s, want %s", got, want)
	}
	if field.started {
		t.Errorf("`-` before the first snapshot started the field")
	}
	if len(field.items) != 0 || field.counts.candidates != 0 {
		t.Errorf("`-` before the first snapshot admitted %d of %d candidates, want none",
			len(field.items), field.counts.candidates)
	}
}

// TestRainPausedQueueSurvivesRepeatedRefreshesWithAnEmptyField guards RG-006:
// a paused Rain whose field is empty keeps its retained arrivals across further
// paused refreshes, which still queue rather than admit them.
func TestRainPausedQueueSurvivesRepeatedRefreshesWithAnEmptyField(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	field := startedRain(scopes, rainSnapshot(scopes), 40, 7)
	field = field.toggledPause()

	arrivals := rainSnapshot(scopes, rainEvidence(t, "arrival", repository, 0))
	field = field.reconciled(scopes, arrivals, rainAt(time.Minute))
	if !queuedHas(field, "arrival", scope) {
		t.Fatalf("the first paused refresh queued %d arrivals, want the one arrival", len(field.queue))
	}

	field = field.reconciled(scopes, arrivals, rainAt(2*time.Minute))
	if !queuedHas(field, "arrival", scope) {
		t.Errorf("the second paused refresh dropped the retained arrival")
	}
	if len(field.items) != 0 {
		t.Errorf("the paused refresh admitted %d items, want none", len(field.items))
	}
	if field.counts.pausedOmitted != 0 {
		t.Errorf("the paused refresh omitted %d arrivals, want none", field.counts.pausedOmitted)
	}
}

// TestRainSkipsNotMemberScopesOfTheSameEvent guards RG-006: only a confirmed
// member outcome creates a candidate, and a not-member Scope of one event never
// hides the confirmed Scopes ordered after it.
func TestRainSkipsNotMemberScopesOfTheSameEvent(t *testing.T) {
	repository := "acme/api"
	unmatched, matched := pathScope(t, repository, "alpha"), pathScope(t, repository, "zeta")
	scopes := scopeSet(t, unmatched, matched)
	snapshot := rainSnapshot(scopes, rainPathEvidence(t, "one", repository, 0, "zeta/main.go"))
	field := startedRain(scopes, snapshot, 40, 7)

	if rainAdmits(field, "one", unmatched) {
		t.Errorf("the not-member Scope admitted a representation")
	}
	if !rainAdmits(field, "one", matched) {
		t.Errorf("the confirmed Scope ordered after a not-member one admitted nothing")
	}
	if got, want := field.counts.candidates, 1; got != want {
		t.Errorf("the snapshot prepared %d candidates, want %d", got, want)
	}
}

// TestRainSkipsOutOfWindowScopesOfTheSameEvent guards RG-006: an out-of-window
// representation of one event never removes the surviving frozen-age
// representation of the same event in a Scope ordered after it.
func TestRainSkipsOutOfWindowScopesOfTheSameEvent(t *testing.T) {
	name := "acme/api"
	repository := domain.NewRepositoryScope(testRepository(t, name))
	path := pathScope(t, name, "src")
	evidence := rainPathEvidence(t, "one", name, 0, "src/main.go")

	only := scopeSet(t, path)
	field := startedRainAt(rainWindow60m, only, rainSnapshot(only, evidence), 40, 7)
	field = field.toggledPause().toggledPause()

	both := scopeSet(t, repository, path)
	field = field.reconciled(both, rainSnapshot(both, evidence), rainAt(65*time.Minute))
	if rainAdmits(field, "one", repository) {
		t.Errorf("the representation older than the 60m window is admitted")
	}
	if !rainAdmits(field, "one", path) {
		t.Errorf("the surviving frozen-age representation was removed with the out-of-window one")
	}
}

// TestRainPausedArrivalsQueueBehindStandingFieldItems guards RG-006: a paused
// refresh queues every newly confirmed representation, including the ones
// ordered after a representation already standing in the frozen field.
func TestRainPausedArrivalsQueueBehindStandingFieldItems(t *testing.T) {
	first, second := "acme/alpha", "acme/beta"
	scopes := scopeSet(t,
		domain.NewRepositoryScope(testRepository(t, first)),
		domain.NewRepositoryScope(testRepository(t, second)),
	)
	standing := rainPathEvidence(t, "standing", first, time.Minute)
	field := startedRain(scopes, rainSnapshot(scopes, standing), 40, 7)
	field = field.toggledPause()

	arrival := rainPathEvidence(t, "arrival", second, 0)
	field = field.reconciled(scopes, rainSnapshot(scopes, standing, arrival), rainAt(time.Minute))

	ordered := scopes.Ordered()
	if !rainAdmits(field, "standing", ordered[0]) {
		t.Errorf("the paused refresh removed the standing field item")
	}
	if !queuedHas(field, "arrival", ordered[1]) {
		t.Errorf("the paused refresh queued %d arrivals, want the one behind the standing item", len(field.queue))
	}
}

// TestRainWindowChangeWhilePausedOmitsNoArrival guards RG-006: a window change
// while paused restores the field and keeps the still-eligible queue, and a
// restoration never consumes paused-arrival capacity.
func TestRainWindowChangeWhilePausedOmitsNoArrival(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, repository)))
	scope := scopes.Ordered()[0]
	standing := rainPathEvidence(t, "standing", repository, time.Minute)
	field := startedRain(scopes, rainSnapshot(scopes, standing), 40, 7)
	field = field.toggledPause()

	arrival := rainPathEvidence(t, "arrival", repository, 0)
	snapshot := rainSnapshot(scopes, standing, arrival)
	field = field.reconciled(scopes, snapshot, rainAt(time.Minute))
	if !queuedHas(field, "arrival", scope) {
		t.Fatalf("the paused refresh did not queue the arrival")
	}

	field = field.windowed(-1, scopes, snapshot)
	if got, want := field.window.String(), "6h"; got != want {
		t.Fatalf("the paused window change selected %s, want %s", got, want)
	}
	if !queuedHas(field, "arrival", scope) {
		t.Errorf("the paused window change dropped the still eligible arrival")
	}
	if !rainAdmits(field, "standing", scope) {
		t.Errorf("the paused window change removed the still eligible field item")
	}
	if got := field.counts.pausedOmitted; got != 0 {
		t.Errorf("the restoration reported %d paused omissions, want none", got)
	}
}

// TestRainStoresAdmittedItemsInScopeThenCandidateOrder guards RG-006: admitted
// items are stored in the stable Scope order and, within one Scope, newest
// event first, so a later Scope's newer event never precedes an earlier
// Scope's older one.
func TestRainStoresAdmittedItemsInScopeThenCandidateOrder(t *testing.T) {
	first, second := "acme/alpha", "acme/beta"
	scopes := scopeSet(t,
		domain.NewRepositoryScope(testRepository(t, first)),
		domain.NewRepositoryScope(testRepository(t, second)),
	)
	ordered := scopes.Ordered()
	snapshot := rainSnapshot(scopes,
		rainPathEvidence(t, "a-new", first, time.Minute),
		rainPathEvidence(t, "b-new", second, 2*time.Minute),
		rainPathEvidence(t, "a-old", first, 3*time.Minute),
		rainPathEvidence(t, "b-old", second, 4*time.Minute),
	)
	field := startedRain(scopes, snapshot, 40, 7)

	want := []rainKey{
		rainKeyOf("a-new", ordered[0]),
		rainKeyOf("a-old", ordered[0]),
		rainKeyOf("b-new", ordered[1]),
		rainKeyOf("b-old", ordered[1]),
	}
	got := rainKeys(field)
	if len(got) != len(want) {
		t.Fatalf("the field stores %d items, want %d", len(got), len(want))
	}
	for index, key := range want {
		if got[index] != key {
			t.Fatalf("item %d is %v, want %v", index, got[index], key)
		}
	}
}

// rainWindowContract is RG-006's closed preset list, transcribed from the spec
// rather than from the implementation, shortest first with the honest
// current-snapshot choice last. A finite preset removes an item at its exact
// boundary; `available` has no finite lifetime at all.
var rainWindowContract = []struct {
	window rainWindow
	name   string
	bound  time.Duration
}{
	{window: rainWindow15m, name: "15m", bound: 15 * time.Minute},
	{window: rainWindow30m, name: "30m", bound: 30 * time.Minute},
	{window: rainWindow60m, name: "60m", bound: time.Hour},
	{window: rainWindow6h, name: "6h", bound: 6 * time.Hour},
	{window: rainWindow24h, name: "24h", bound: 24 * time.Hour},
	{window: rainWindow7d, name: "7d", bound: 7 * 24 * time.Hour},
	{window: rainWindowAvailable, name: "available"},
}

// TestRainWindowStepsBetweenPresetsAndStopsAtBothEndpoints guards RG-006's
// closed preset list: `-` selects the next shorter preset and `+` the next
// longer one, either key at its endpoint is a no-op, and Rain starts at 24h.
func TestRainWindowStepsBetweenPresetsAndStopsAtBothEndpoints(t *testing.T) {
	last := len(rainWindowContract) - 1
	for index, preset := range rainWindowContract {
		if got := preset.window.String(); got != preset.name {
			t.Errorf("preset %d is named %s, want %s", index, got, preset.name)
		}
		shorter, longer := rainWindowContract[max(index-1, 0)], rainWindowContract[min(index+1, last)]
		if got := preset.window.stepped(-1); got != shorter.window {
			t.Errorf("shortening from %s selected %s, want %s", preset.name, got, shorter.name)
		}
		if got := preset.window.stepped(1); got != longer.window {
			t.Errorf("lengthening from %s selected %s, want %s", preset.name, got, longer.name)
		}
	}
	if defaultRainWindow != rainWindow24h || defaultRainWindow.String() != "24h" {
		t.Errorf("Rain defaults to %s, want the 24h preset", defaultRainWindow)
	}
}

// TestRainWindowAdmitsUpToItsExactBoundary guards RG-006: every finite preset
// admits an effective age below its length and removes it at the exact
// boundary, while `available` admits every age because its eligibility is
// bounded snapshot membership alone.
func TestRainWindowAdmitsUpToItsExactBoundary(t *testing.T) {
	for _, preset := range rainWindowContract {
		if preset.bound == 0 {
			for _, age := range []time.Duration{0, time.Hour, 7 * 24 * time.Hour, 100 * 24 * time.Hour} {
				if !preset.window.admits(age) {
					t.Errorf("window %s removed an item at age %s, want every age admitted", preset.name, age)
				}
			}
			continue
		}
		for _, admitted := range []time.Duration{0, preset.bound - time.Nanosecond} {
			if !preset.window.admits(admitted) {
				t.Errorf("window %s removed an item at age %s, want it admitted", preset.name, admitted)
			}
		}
		for _, removed := range []time.Duration{preset.bound, preset.bound + time.Nanosecond} {
			if preset.window.admits(removed) {
				t.Errorf("window %s admitted an item at age %s, want it removed", preset.name, removed)
			}
		}
	}
}

// TestRainWindowClampsACorruptedIndexToARealPreset guards RG-006: an
// out-of-range window degrades to a real preset rather than panicking a view,
// and stepping from one stays inside the closed list.
func TestRainWindowClampsACorruptedIndexToARealPreset(t *testing.T) {
	cases := []struct {
		window  rainWindow
		named   string
		stepped string
	}{
		{window: rainWindow(-5), named: "15m", stepped: "30m"},
		{window: rainWindow(7), named: "available", stepped: "available"},
		{window: rainWindow(97), named: "available", stepped: "available"},
	}
	for _, corrupted := range cases {
		if got := corrupted.window.String(); got != corrupted.named {
			t.Errorf("window index %d is named %s, want %s", int(corrupted.window), got, corrupted.named)
		}
		if got := corrupted.window.stepped(1); got.String() != corrupted.stepped {
			t.Errorf("stepping window index %d longer selected %s, want %s",
				int(corrupted.window), got, corrupted.stepped)
		}
	}
}
