package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// geometryMutScopeAt returns the repository Scope of the numbered fixture
// repository. The numbering keeps the stable lowercase-key Scope order RG-006
// pages over.
func geometryMutScopeAt(t *testing.T, index int) domain.Scope {
	t.Helper()
	return domain.NewRepositoryScope(testRepository(t, fmt.Sprintf("acme/g%d", index)))
}

// geometryMutItems returns count retained pushes of the repository, each one
// minute older than the last, so candidate ordering is total and stable.
func geometryMutItems(t *testing.T, prefix, repository string, count int) []domain.EventEvidence {
	t.Helper()
	retained := make([]domain.EventEvidence, 0, count)
	for index := range count {
		retained = append(retained, rainEvidence(t,
			fmt.Sprintf("%s-%d", prefix, index), repository, time.Duration(index+1)*time.Minute))
	}
	return retained
}

// geometryMutOccupant returns the column's first occupied cell.
func geometryMutOccupant(column rainColumn) (rainCell, bool) {
	for _, row := range column.rows {
		for _, cell := range row {
			if cell.occupied {
				return cell, true
			}
		}
	}
	return rainCell{}, false
}

// TestRainColumnHelpersAreTotalForDegenerateCounts guards RG-006's `N=0` case
// and the stale inputs a resize or Scope removal can still present: an empty
// selection does no column arithmetic at all, and a page start or per-page
// count left over from an earlier geometry never yields a negative column
// count, a zero page count, or a division by a per-page count of zero.
func TestRainColumnHelpersAreTotalForDegenerateCounts(t *testing.T) {
	if got := rainPerPage(40, -1); got != 0 {
		t.Errorf("a selection reported as -1 Scopes has K=%d, want 0", got)
	}
	if got := rainPageColumns(0, 1, 3); got != 0 {
		t.Errorf("an empty selection with a stale page start prepares %d columns, want 0", got)
	}
	if got := rainPageColumns(3, 5, 0); got != 0 {
		t.Errorf("a page start past the end prepares %d columns, want 0", got)
	}
	if got := rainPageCount(0, 3); got != 1 {
		t.Errorf("an empty selection with a stale K has %d pages, want the one empty-state page", got)
	}
	if got := rainPageCount(8, 0); got != 1 {
		t.Errorf("eight Scopes without a usable K have %d pages, want the one empty-state page", got)
	}
	if got := rainPageStart(4, 0); got != 0 {
		t.Errorf("an anchor without a usable K starts page %d, want 0", got)
	}
}

// TestRainFieldNumbersTheVisiblePage guards RG-006's one-based `scopes A-B of
// N` header input: the fixed pages of eight Scopes at width 40 begin at Scope
// indexes 0, 3, and 6, and the page holding start `S` is numbered `S/K+1`.
func TestRainFieldNumbersTheVisiblePage(t *testing.T) {
	selected := make([]domain.Scope, 0, 8)
	retained := make([]domain.EventEvidence, 0, 8)
	for index := range 8 {
		scope := geometryMutScopeAt(t, index)
		selected = append(selected, scope)
		retained = append(retained, rainEvidence(t, fmt.Sprintf("e%d", index), scope.Repository().String(), time.Minute))
	}
	scopes := scopeSet(t, selected...)
	state := startedRain(scopes, rainSnapshot(scopes, retained...), 40, 7)

	cases := []struct {
		steps, page, first, last int
	}{
		{steps: 0, page: 1, first: 1, last: 3},
		{steps: 1, page: 2, first: 4, last: 6},
		{steps: 2, page: 3, first: 7, last: 8},
	}
	for _, want := range cases {
		paged := state
		for range want.steps {
			paged = paged.paged(1)
		}
		field := paged.field()
		if field.page != want.page {
			t.Errorf("after %d forward pages the visible page is %d, want %d", want.steps, field.page, want.page)
		}
		if field.pages != 3 {
			t.Errorf("after %d forward pages eight Scopes report %d pages, want 3", want.steps, field.pages)
		}
		if field.first != want.first || field.last != want.last {
			t.Errorf("after %d forward pages the visible Scope range is %d-%d, want %d-%d",
				want.steps, field.first, field.last, want.first, want.last)
		}
	}
}

// TestRainFieldTotalsCollisionsOverEveryColumn guards RG-006's page total: the
// prepared `N collisions` is every visible column's count together, not the
// last column's alone. Two 12-cell columns of six slots with eight admitted
// items each group two items apiece, so the page reports four.
func TestRainFieldTotalsCollisionsOverEveryColumn(t *testing.T) {
	first, second := geometryMutScopeAt(t, 0), geometryMutScopeAt(t, 1)
	scopes := scopeSet(t, first, second)
	retained := append(
		geometryMutItems(t, "a", first.Repository().String(), 8),
		geometryMutItems(t, "b", second.Repository().String(), 8)...)
	// Width 25 seats exactly two 12-cell columns of six slots each; one row
	// makes every item past the sixth of a column group.
	state := startedRain(scopes, rainSnapshot(scopes, retained...), 25, 1)
	field := state.field()

	if got, want := len(field.columns), 2; got != want {
		t.Fatalf("width 25 prepared %d columns, want %d", got, want)
	}
	for index, column := range field.columns {
		if got, want := column.collisions, 2; got != want {
			t.Fatalf("column %d prepared %d collisions, want %d", index+1, got, want)
		}
	}
	if got, want := field.collisions, 4; got != want {
		t.Errorf("the page prepared %d collisions, want every column's %d", got, want)
	}
	if field.hiddenItems != 0 {
		t.Errorf("grouped items counted %d hidden items, want 0", field.hiddenItems)
	}
}

// TestRainFieldTotalsHiddenItemsOverEveryColumn guards RG-006's disjoint hidden
// accounting: `hidden items` is every visible column's unplaceable items plus
// the off-page ones. Without a single field row no column can place anything,
// so all sixteen admitted items are hidden.
func TestRainFieldTotalsHiddenItemsOverEveryColumn(t *testing.T) {
	selected := make([]domain.Scope, 0, 8)
	retained := make([]domain.EventEvidence, 0, 16)
	for index := range 8 {
		scope := geometryMutScopeAt(t, index)
		selected = append(selected, scope)
		retained = append(retained, geometryMutItems(t, fmt.Sprintf("s%d", index), scope.Repository().String(), 2)...)
	}
	scopes := scopeSet(t, selected...)
	state := startedRain(scopes, rainSnapshot(scopes, retained...), 40, 0)
	field := state.field()

	if got, want := len(field.columns), 3; got != want {
		t.Fatalf("width 40 prepared %d columns, want %d", got, want)
	}
	for index, column := range field.columns {
		if got, want := column.hidden, 2; got != want {
			t.Fatalf("column %d reports %d hidden items, want %d", index+1, got, want)
		}
	}
	if got, want := field.hiddenItems, 16; got != want {
		t.Errorf("the page reports %d hidden items, want every admitted item's %d", got, want)
	}
	if got, want := field.counts.admitted, 16; got != want {
		t.Errorf("responsive hiding changed admission to %d items, want %d", got, want)
	}
}

// TestRainColumnTotalsCollisionsOverEveryRow guards RG-006's per-column count:
// a column totals the groupings of every one of its field rows. With one slot
// per row, each row keeps its deterministic first occupant and every further
// item of that row groups into it, so a ten-item two-row column reports eight
// collisions and both rows carry their groupings.
func TestRainColumnTotalsCollisionsOverEveryRow(t *testing.T) {
	scope := geometryMutScopeAt(t, 0)
	scopes := scopeSet(t, scope)
	retained := geometryMutItems(t, "row", scope.Repository().String(), 10)
	// Interior width two is one slot, so a row holds exactly one occupant.
	state := startedRain(scopes, rainSnapshot(scopes, retained...), 2, 2)
	field := state.field()

	if got, want := len(field.columns), 1; got != want {
		t.Fatalf("width 2 prepared %d columns, want %d", got, want)
	}
	column := field.columns[0]
	if got, want := column.slots, 1; got != want {
		t.Fatalf("a two-cell column has %d slots, want %d", got, want)
	}
	grouped := 0
	for row := range column.rows {
		cell := column.rows[row][0]
		if !cell.occupied {
			t.Fatalf("row %d holds no occupant, so the fixture no longer spans both rows", row)
		}
		if cell.grouped <= 0 {
			t.Errorf("row %d reports %d grouped items, want a positive count", row, cell.grouped)
		}
		grouped += cell.grouped
	}
	if got, want := grouped, 8; got != want {
		t.Errorf("the column grouped %d items over its rows, want %d", got, want)
	}
	if got, want := column.collisions, 8; got != want {
		t.Errorf("the column prepared %d collisions, want every row's %d", got, want)
	}
	if got, want := field.counts.admitted, 10; got != want {
		t.Errorf("grouping changed admission to %d items, want %d", got, want)
	}
}

// TestRainFieldClipsQualificationOnlyWhenTheColumnIsOneCell guards RG-006 and
// RG-008: a qualified occupant of a column wide enough for its `~` renders the
// qualifier and is never counted as a clipped qualification, so clipping needs
// the one-cell column and the qualification together.
func TestRainFieldClipsQualificationOnlyWhenTheColumnIsOneCell(t *testing.T) {
	repository := "acme/api"
	scopes := scopeSet(t, pathScope(t, repository, "src"))
	evidence := rainEvidence(t, "qualified", repository, time.Minute)
	evidence.Outcome = domain.CompleteOutcome(domain.ProvenanceCurrentPR, mustChangedPaths(t, "src/main.go"))
	state := startedRain(scopes, rainSnapshot(scopes, evidence), 40, 3)
	field := state.field()

	if got, want := len(field.columns), 1; got != want {
		t.Fatalf("width 40 with one Scope prepared %d columns, want %d", got, want)
	}
	column := field.columns[0]
	if column.interior <= 1 {
		t.Fatalf("the column interior is %d cells, want a column wide enough for the qualifier", column.interior)
	}
	occupant, found := geometryMutOccupant(column)
	if !found {
		t.Fatal("the admitted qualified item occupies no cell")
	}
	if !occupant.qualified {
		t.Error("the occupant lost its current-PR qualification")
	}
	if occupant.clipped {
		t.Error("a qualified occupant of a wide column is marked clipped")
	}
	if got := field.clipped; got != 0 {
		t.Errorf("a wide column prepared %d clipped qualifications, want 0", got)
	}
}

// TestPlacementIsStableForLongEventIdentifiers guards RG-006's placement byte
// stream for the normalized event IDs real sources produce: a long identifier
// places deterministically, its row and slot purposes stay independent, and
// both reduce inside the field.
func TestPlacementIsStableForLongEventIdentifiers(t *testing.T) {
	// A GitHub node ID is far longer than the purpose prefix and Scope key.
	const event = "PR_kwDOAbCdEf4AbCdEfGhIjKlMnOpQrStUvWxYz"
	scope := domain.NewRepositoryScope(testRepository(t, "acme/api"))

	row, slot := rainRowHash(event, scope), rainSlotHash(event, scope)
	if again := rainRowHash(event, scope); again != row {
		t.Errorf("the row hash of a long event ID is %x on a second call, want the stable %x", again, row)
	}
	if row == slot {
		t.Errorf("the row and slot purposes of a long event ID share the hash %x, want independent results", row)
	}
	for _, count := range []int{1, 7, 500} {
		if got := reduce(row, count); got < 0 || got >= count {
			t.Errorf("the row of a long event ID reduced onto %d positions is %d, want a position of the field", count, got)
		}
	}
}

// TestReduceHasNoPositionWithoutAPositiveCount guards RG-006: hash modulo runs
// only for positive row and slot counts, so a field with no usable row or slot
// reduces to no position at all rather than to position zero by division.
func TestReduceHasNoPositionWithoutAPositiveCount(t *testing.T) {
	hash := rainRowHash("12345", domain.NewRepositoryScope(testRepository(t, "acme/api")))
	for _, count := range []int{0, -1, -7} {
		if got := reduce(hash, count); got != 0 {
			t.Errorf("reducing onto %d positions gave %d, want 0", count, got)
		}
	}
}
