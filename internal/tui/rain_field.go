package tui

import (
	"slices"

	"github.com/fmueller/orgtop/internal/domain"
)

// rainCell is one prepared logical Rain slot. It carries the deterministic
// first occupant's shared category and qualification alone; grouped items stay
// admitted and keep aging and moving behind it.
type rainCell struct {
	// occupied reports whether an admitted item renders in the cell.
	occupied bool
	// category is the shared normalized category the occupant is drawn from.
	category domain.Category
	// recency is the occupant's prepared discrete state, so rendering picks its
	// shared emphasis without measuring an age of its own.
	recency recency
	// qualified reports the occupant's current-PR membership.
	qualified bool
	// clipped reports a qualified occupant whose `~` cannot render because the
	// column is exactly one cell wide. The qualification stays counted here and
	// is never replaced by color.
	clipped bool
	// grouped is how many further admitted items share the cell.
	grouped int
}

// rainColumn is one prepared Scope column of the visible page.
type rainColumn struct {
	// scope is the Scope the column represents.
	scope domain.Scope
	// interior is the column's usable width in cells.
	interior int
	// slots is the exact horizontal slot count of that width.
	slots int
	// rows holds the prepared cells, outermost by field row.
	rows [][]rainCell
	// collisions is how many admitted items on this page could not obtain a
	// unique slot after a full-row probe.
	collisions int
	// clipped is how many visible tokens lost their `~` qualifier.
	clipped int
	// hidden is how many admitted items the column has no usable cell for.
	hidden int
	// recencies are the discrete states of every admitted item of the column,
	// which RG-008's no-color fallback reports as text.
	recencies rainRecencies
}

// rainRecencies counts admitted items by their prepared discrete state. Expiry
// removes an item, so no expired count exists to report.
type rainRecencies struct {
	fresh  int
	recent int
	aging  int
}

// counted adds one item's state to the totals.
func (r rainRecencies) counted(state recency) rainRecencies {
	switch state {
	case recencyNew:
		r.fresh++
	case recencyRecent:
		r.recent++
	case recencyAging:
		r.aging++
	}
	return r
}

// plus merges another column's totals into these.
func (r rainRecencies) plus(other rainRecencies) rainRecencies {
	return rainRecencies{fresh: r.fresh + other.fresh, recent: r.recent + other.recent, aging: r.aging + other.aging}
}

// rainField is the complete prepared page state rendering consumes. Rendering
// only reads it and never hashes, admits, collides, ages, or moves items.
type rainField struct {
	// columns are the actual columns of the visible page.
	columns []rainColumn
	// scopes is the active Scope count the page is one of.
	scopes int
	// page is the one-based visible page and pages the total.
	page, pages int
	// first and last are the one-based inclusive visible Scope range, and stay
	// zero for the empty-state page.
	first, last int
	// hiddenScopes is the off-page Scope count.
	hiddenScopes int
	// hiddenItems is the admitted items in off-page Scopes plus the admitted
	// visible-Scope items with no usable row or slot. A grouped item is never
	// hidden, and one representation has at most one hidden cause.
	hiddenItems int
	// collisions and clipped are the page totals of the per-column counts.
	collisions, clipped int
	// counts are the capacity totals, which responsive hiding never changes.
	counts rainCounts
	// recencies are the page totals of the per-column discrete states.
	recencies rainRecencies
	// window is the selected recency window the page is prepared for.
	window rainWindow
	// paused reports whether movement, age, and expiry are frozen.
	paused bool
}

// field prepares the visible page: its columns, cells, collision grouping, and
// the disjoint hidden, clipped, and capacity counters.
func (r rain) field() rainField {
	perPage := rainPerPage(r.width, len(r.scopes))
	columns := rainPageColumns(len(r.scopes), r.start, perPage)
	field := rainField{
		scopes:  len(r.scopes),
		pages:   rainPageCount(len(r.scopes), perPage),
		counts:  r.counts,
		window:  r.window,
		paused:  r.paused,
		columns: make([]rainColumn, 0, columns),
	}
	if columns > 0 {
		field.page = r.start/perPage + 1
		field.first, field.last = r.start+1, r.start+columns
		field.hiddenScopes = len(r.scopes) - columns
	} else {
		field.pages, field.page = 1, 1
	}

	visible := make(map[domain.ScopeIdentity]struct{}, columns)
	widths := rainColumnWidths(r.width, columns)
	for index, interior := range widths {
		scope := r.scopes[r.start+index]
		visible[scope.Identity()] = struct{}{}
		column := r.column(scope, interior)
		field.collisions += column.collisions
		field.clipped += column.clipped
		field.hiddenItems += column.hidden
		field.recencies = field.recencies.plus(column.recencies)
		field.columns = append(field.columns, column)
	}
	for _, item := range r.items {
		if _, onPage := visible[item.key.scope]; !onPage {
			field.hiddenItems++
		}
	}
	return field
}

// column prepares one Scope column. A column with no usable row or slot renders
// no token at all and reports its admitted items as hidden instead.
func (r rain) column(scope domain.Scope, interior int) rainColumn {
	slots := rainSlots(interior)
	items := make([]rainItem, 0, len(r.items))
	for _, item := range r.items {
		if item.key.scope == scope.Identity() {
			items = append(items, item)
		}
	}
	column := rainColumn{scope: scope, interior: interior, slots: slots}
	for _, item := range items {
		column.recencies = column.recencies.counted(recencyAt(item.age))
	}
	if r.height <= 0 || slots <= 0 {
		column.hidden = len(items)
		return column
	}

	column.rows = make([][]rainCell, r.height)
	byRow := make([][]rainItem, r.height)
	for _, item := range items {
		row := item.row(r.height)
		byRow[row] = append(byRow[row], item)
	}
	clips := rainClipsQualification(interior)
	for row, occupants := range byRow {
		column.rows[row] = make([]rainCell, slots)
		slices.SortStableFunc(occupants, compareRainCandidates)
		desired := make([]int, len(occupants))
		for index, item := range occupants {
			desired[index] = reduce(rainSlotHash(item.key.event, item.scope), slots)
		}
		assigned, collisions := assignRainSlots(desired, slots)
		column.collisions += collisions
		for index, slot := range assigned {
			cell := &column.rows[row][slot]
			if cell.occupied {
				cell.grouped++
				continue
			}
			occupant := occupants[index]
			*cell = rainCell{
				occupied:  true,
				category:  occupant.category,
				recency:   recencyAt(occupant.age),
				qualified: occupant.qualified,
				clipped:   clips && occupant.qualified,
			}
			if cell.clipped {
				column.clipped++
			}
		}
	}
	return column
}

// assignRainSlots places one row's ordered items. Each takes its desired slot
// or probes right with wraparound to the first free one; once every slot is
// occupied, further items group into their desired slot and count as
// collisions. A successful probe never counts as one.
func assignRainSlots(desired []int, slots int) ([]int, int) {
	if slots <= 0 || len(desired) == 0 {
		return nil, 0
	}
	taken := make([]bool, slots)
	free, collisions := slots, 0
	assigned := make([]int, len(desired))
	for index, want := range desired {
		if free == 0 {
			assigned[index], collisions = want, collisions+1
			continue
		}
		slot := want
		for taken[slot] {
			slot = (slot + 1) % slots
		}
		taken[slot], free, assigned[index] = true, free-1, slot
	}
	return assigned, collisions
}
