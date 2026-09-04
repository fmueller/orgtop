package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// TestRainSlotAssignmentProbesThenGroups guards A-050: with two slots and four
// ordered items all desiring slot zero, the first takes zero, the second probes
// right to one, the last two group into their desired slot, and exactly
// `2 collisions` is prepared. A successful probe never increments the count.
func TestRainSlotAssignmentProbesThenGroups(t *testing.T) {
	assigned, collisions := assignRainSlots([]int{0, 0, 0, 0}, 2)
	if want := []int{0, 1, 0, 0}; fmt.Sprint(assigned) != fmt.Sprint(want) {
		t.Errorf("slot assignment is %v, want %v", assigned, want)
	}
	if collisions != 2 {
		t.Errorf("prepared %d collisions, want 2", collisions)
	}
}

// TestRainSlotAssignmentWrapsWhileProbing guards RG-006's probe: an item whose
// desired slot is taken walks right with wraparound to the first free slot and
// counts no collision.
func TestRainSlotAssignmentWrapsWhileProbing(t *testing.T) {
	assigned, collisions := assignRainSlots([]int{2, 2, 2}, 3)
	if want := []int{2, 0, 1}; fmt.Sprint(assigned) != fmt.Sprint(want) {
		t.Errorf("slot assignment is %v, want %v", assigned, want)
	}
	if collisions != 0 {
		t.Errorf("successful probes prepared %d collisions, want 0", collisions)
	}
}

// TestRainSlotAssignmentHasNoPositionsWithoutSlots guards RG-006: hash modulo
// and collision probing run only for a positive slot count.
func TestRainSlotAssignmentHasNoPositionsWithoutSlots(t *testing.T) {
	if assigned, collisions := assignRainSlots([]int{0, 0}, 0); assigned != nil || collisions != 0 {
		t.Errorf("a column with no slots assigned %v with %d collisions, want none", assigned, collisions)
	}
}

// rainScopes builds count repository Scopes in the stable Scope identity order.
func rainScopes(t *testing.T, count int) []domain.Scope {
	t.Helper()
	scopes := make([]domain.Scope, 0, count)
	for index := range count {
		scopes = append(scopes, domain.NewRepositoryScope(testRepository(t, fmt.Sprintf("acme/r%d", index))))
	}
	return scopes
}

// oneItemPerScope returns one retained push per Scope, so every column holds
// exactly one admitted item.
func oneItemPerScope(t *testing.T, scopes []domain.Scope) []domain.EventEvidence {
	t.Helper()
	retained := make([]domain.EventEvidence, 0, len(scopes))
	for index, scope := range scopes {
		retained = append(retained, rainEvidence(t, fmt.Sprintf("e%d", index), scope.Repository().String(), time.Minute))
	}
	return retained
}

// TestRainFieldPagesEightScopesAtWidthForty guards A-053: eight Scopes with one
// admitted item each at width 40 give `K=3`, column widths 13/13/12, and a
// first page reporting five hidden Scopes and five hidden items.
func TestRainFieldPagesEightScopesAtWidthForty(t *testing.T) {
	selected := rainScopes(t, 8)
	scopes := scopeSet(t, selected...)
	state := startedRain(scopes, rainSnapshot(scopes, oneItemPerScope(t, selected)...), 40, 7)
	field := state.field()

	if got, want := len(field.columns), 3; got != want {
		t.Fatalf("page one prepared %d columns, want %d", got, want)
	}
	for index, want := range []int{13, 13, 12} {
		if got := field.columns[index].interior; got != want {
			t.Errorf("column %d has interior width %d, want %d", index+1, got, want)
		}
	}
	if got, want := field.hiddenScopes, 5; got != want {
		t.Errorf("page one reports %d hidden Scopes, want %d", got, want)
	}
	if got, want := field.hiddenItems, 5; got != want {
		t.Errorf("page one reports %d hidden items, want %d", got, want)
	}
	if got, want := field.pages, 3; got != want {
		t.Errorf("eight Scopes prepare %d pages, want %d", got, want)
	}
	if got, want := field.first, 1; got != want {
		t.Errorf("page one starts at Scope %d, want %d", got, want)
	}
	if got, want := field.last, 3; got != want {
		t.Errorf("page one ends at Scope %d, want %d", got, want)
	}
}

// TestRainFieldClipsQualificationInOneCellColumns guards A-053: at width one
// and height one, one category cell renders and a qualified item increments
// `clipped qualifications`.
func TestRainFieldClipsQualificationInOneCellColumns(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, pathScope(t, repository, "src"))
	evidence := rainEvidence(t, "qualified", repository, time.Minute)
	evidence.Outcome = domain.CompleteOutcome(domain.ProvenanceCurrentPR, mustChangedPaths(t, "src/main.go"))
	state := startedRain(scopes, rainSnapshot(scopes, evidence), 1, 1)
	field := state.field()

	if got, want := len(field.columns), 1; got != want {
		t.Fatalf("width one prepared %d columns, want %d", got, want)
	}
	column := field.columns[0]
	if got, want := column.slots, 1; got != want {
		t.Fatalf("a one-cell column has %d slots, want %d", got, want)
	}
	if !column.rows[0][0].occupied {
		t.Fatal("the one usable cell renders no category token")
	}
	if !column.rows[0][0].clipped {
		t.Error("the qualified occupant of a one-cell column is not marked clipped")
	}
	if got, want := field.clipped, 1; got != want {
		t.Errorf("prepared %d clipped qualifications, want %d", got, want)
	}
	if field.hiddenItems != 0 {
		t.Errorf("a clipped qualification counted %d hidden items, want 0", field.hiddenItems)
	}
}

// TestRainFieldHidesItemsWithoutUsableCells guards A-053 and RG-006's disjoint
// counters: a visible Scope with no usable slot or row keeps its admitted items
// in state and reports them as hidden items rather than as omissions.
func TestRainFieldHidesItemsWithoutUsableCells(t *testing.T) {
	selected := rainScopes(t, 1)
	scopes := scopeSet(t, selected...)
	retained := oneItemPerScope(t, selected)
	for _, dimensions := range []struct{ width, height int }{{width: 0, height: 7}, {width: 40, height: 0}} {
		state := startedRain(scopes, rainSnapshot(scopes, retained...), dimensions.width, dimensions.height)
		field := state.field()
		if got, want := field.hiddenItems, 1; got != want {
			t.Errorf("at %dx%d the field reports %d hidden items, want %d", dimensions.width, dimensions.height, got, want)
		}
		if got := field.counts.globalOmitted + field.counts.columnOmitted; got != 0 {
			t.Errorf("at %dx%d responsive hiding recorded %d capacity omissions, want 0", dimensions.width, dimensions.height, got)
		}
		if got, want := len(state.items), 1; got != want {
			t.Errorf("at %dx%d the field retains %d admitted items, want %d", dimensions.width, dimensions.height, got, want)
		}
	}
}

// TestRainFieldEmptySelectionHasOneEmptyPage guards RG-006: `N=0` prepares one
// empty-state page and no column arithmetic at all.
func TestRainFieldEmptySelectionHasOneEmptyPage(t *testing.T) {
	state := newRain().resized(40, 7)
	field := state.field()
	if len(field.columns) != 0 {
		t.Errorf("an empty selection prepared %d columns, want none", len(field.columns))
	}
	if got, want := field.pages, 1; got != want {
		t.Errorf("an empty selection prepared %d pages, want %d", got, want)
	}
	if field.first != 0 || field.last != 0 {
		t.Errorf("an empty selection prepared the Scope range %d-%d, want none", field.first, field.last)
	}
}

// TestRainFieldGroupsCollisionsWithoutHidingThem guards RG-006: grouped items
// stay admitted, are counted as collisions, and are never counted hidden.
func TestRainFieldGroupsCollisionsWithoutHidingThem(t *testing.T) {
	repository := "acme/api"
	selected := domain.NewRepositoryScope(testRepository(t, repository))
	scopes := scopeSet(t, selected)
	retained := make([]domain.EventEvidence, 0, 8)
	for index := range 8 {
		retained = append(retained, rainEvidence(t, fmt.Sprintf("e%d", index), repository, time.Duration(index)*time.Minute))
	}
	// One row and one slot force every item after the first to group.
	state := startedRain(scopes, rainSnapshot(scopes, retained...), 2, 1)
	field := state.field()
	if got, want := field.collisions, 7; got != want {
		t.Errorf("prepared %d collisions, want %d", got, want)
	}
	if field.hiddenItems != 0 {
		t.Errorf("grouped items counted %d hidden items, want 0", field.hiddenItems)
	}
	if got, want := field.counts.admitted, 8; got != want {
		t.Errorf("grouping changed admission to %d items, want %d", got, want)
	}
}

// TestRainFieldKeepsTheDeterministicFirstOccupant guards RG-006: a grouped cell
// keeps the deterministic first occupant's category and qualification token.
func TestRainFieldKeepsTheDeterministicFirstOccupant(t *testing.T) {
	repository := "acme/api"
	selected := domain.NewRepositoryScope(testRepository(t, repository))
	scopes := scopeSet(t, selected)
	newest := rainEvidence(t, "newest", repository, time.Minute)
	newest.Event.Category = domain.CategoryReview
	older := rainEvidence(t, "older", repository, 2*time.Minute)
	state := startedRain(scopes, rainSnapshot(scopes, newest, older), 2, 1)

	cell := state.field().columns[0].rows[0][0]
	if got, want := cell.category, domain.CategoryReview; got != want {
		t.Errorf("the grouped cell shows category %q, want the newest occupant's %q", got, want)
	}
}
