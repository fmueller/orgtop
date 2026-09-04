package tui

import (
	"slices"
	"testing"
)

// TestRainPerPageMatchesTheClosedColumnFormula guards RG-006's
// `K=min(N,max(1,floor((W+1)/13)))`, including A-053's widths 40, 25, 12, and
// the best-effort column below 12.
func TestRainPerPageMatchesTheClosedColumnFormula(t *testing.T) {
	cases := []struct {
		width, scopes, want int
	}{
		{width: 40, scopes: 8, want: 3},
		{width: 25, scopes: 8, want: 2},
		{width: 12, scopes: 8, want: 1},
		{width: 11, scopes: 8, want: 1},
		{width: 1, scopes: 8, want: 1},
		{width: 0, scopes: 8, want: 1},
		{width: -5, scopes: 8, want: 1},
		{width: 40, scopes: 2, want: 2},
		{width: 40, scopes: 0, want: 0},
	}
	for _, want := range cases {
		if got := rainPerPage(want.width, want.scopes); got != want.want {
			t.Errorf("width %d with %d Scopes has K=%d, want %d", want.width, want.scopes, got, want.want)
		}
	}
}

// TestRainColumnWidthsDivideTheFieldLeftmostFirst guards RG-006's column
// arithmetic: `M-1` one-cell separators, the remaining width divided by `M`,
// and every extra cell going to the leftmost columns. A-053's width 40 gives
// 13/13/12 and width 25 gives 12/12.
func TestRainColumnWidthsDivideTheFieldLeftmostFirst(t *testing.T) {
	cases := []struct {
		name          string
		width, actual int
		want          []int
	}{
		{name: "A-053 width 40", width: 40, actual: 3, want: []int{13, 13, 12}},
		{name: "A-053 width 25", width: 25, actual: 2, want: []int{12, 12}},
		{name: "A-053 width 12", width: 12, actual: 1, want: []int{12}},
		{name: "A-053 best effort", width: 11, actual: 1, want: []int{11}},
		{name: "one cell", width: 1, actual: 1, want: []int{1}},
		{name: "no cells", width: 0, actual: 1, want: []int{0}},
		{name: "negative width", width: -4, actual: 1, want: []int{0}},
		{name: "partial final page", width: 40, actual: 2, want: []int{20, 19}},
		{name: "no columns", width: 40, actual: 0, want: nil},
	}
	for _, want := range cases {
		got := rainColumnWidths(want.width, want.actual)
		if !slices.Equal(got, want.want) {
			t.Errorf("%s: column widths %v, want %v", want.name, got, want.want)
		}
		total := 0
		for _, interior := range got {
			total += interior
		}
		if separators := want.actual - 1; want.actual > 0 && want.width > 0 && total+separators != want.width {
			t.Errorf("%s: %d interior cells plus %d separators is not the %d-cell field", want.name, total, separators, want.width)
		}
	}
}

// TestRainPagesAreFixedAndWrap guards RG-006's fixed page starts at
// `0,K,2K...`, the possibly partial final page, and `[`/`]` wrapping at both
// ends. A-053's eight Scopes at width 40 page as 1-3, 4-6, and 7-8.
func TestRainPagesAreFixedAndWrap(t *testing.T) {
	const scopes, perPage = 8, 3
	if got, want := rainPageCount(scopes, perPage), 3; got != want {
		t.Fatalf("%d Scopes at K=%d have %d pages, want %d", scopes, perPage, got, want)
	}
	for start, want := range map[int]int{0: 3, 3: 3, 6: 2} {
		if got := rainPageColumns(scopes, start, perPage); got != want {
			t.Errorf("page starting at %d shows %d columns, want %d", start, got, want)
		}
	}
	forward := []int{3, 6, 0, 3}
	position := 0
	for _, want := range forward {
		position = rainStepPage(position, scopes, perPage, 1)
		if position != want {
			t.Fatalf("paging forward reached %d, want %d", position, want)
		}
	}
	backward := []int{0, 6, 3, 0}
	position = 3
	for _, want := range backward {
		position = rainStepPage(position, scopes, perPage, -1)
		if position != want {
			t.Fatalf("paging backward reached %d, want %d", position, want)
		}
	}
}

// TestRainEmptyScopeSetHasOneEmptyPage guards RG-006's `N=0` case: one
// empty-state page and no column arithmetic at all.
func TestRainEmptyScopeSetHasOneEmptyPage(t *testing.T) {
	if got, want := rainPageCount(0, 0), 1; got != want {
		t.Errorf("an empty selection has %d pages, want %d", got, want)
	}
	if got := rainPageColumns(0, 0, 0); got != 0 {
		t.Errorf("an empty selection prepares %d columns, want 0", got)
	}
	if got := rainStepPage(0, 0, 0, 1); got != 0 {
		t.Errorf("paging an empty selection reached %d, want 0", got)
	}
}

// TestRainPageStartContainsTheAnchor guards RG-006's `floor(anchor-index/K)*K`
// choice of the unique fixed page holding the prior first-visible anchor.
// A-053 resizes an anchor at Scope 5 to width 12 and starts at Scope 5.
func TestRainPageStartContainsTheAnchor(t *testing.T) {
	cases := []struct {
		anchor, perPage, want int
	}{
		{anchor: 4, perPage: 1, want: 4},
		{anchor: 4, perPage: 2, want: 4},
		{anchor: 4, perPage: 3, want: 3},
		{anchor: 0, perPage: 3, want: 0},
		{anchor: 7, perPage: 3, want: 6},
	}
	for _, want := range cases {
		if got := rainPageStart(want.anchor, want.perPage); got != want.want {
			t.Errorf("anchor index %d at K=%d starts page %d, want %d", want.anchor, want.perPage, got, want.want)
		}
	}
}
