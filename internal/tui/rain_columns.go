package tui

// rainColumnStride is the field width one further Rain column costs: twelve
// interior cells plus its one-cell separator. RG-006 derives the per-page
// column count from it as `floor((W+1)/13)`, so the leading column pays no
// separator of its own.
const rainColumnStride = 13

// rainPerPage returns RG-006's `K=min(N,max(1,floor((W+1)/13)))`, the number of
// consecutive Scopes one fixed page holds. A width below twelve still yields
// one best-effort column, and an empty selection yields none because `N=0` has
// one empty-state page and no column arithmetic.
func rainPerPage(width, scopes int) int {
	if scopes <= 0 {
		return 0
	}
	return min(scopes, max(1, (width+1)/rainColumnStride))
}

// rainPageColumns returns the `M=min(K,N-S)` actual columns of the fixed page
// starting at zero-based Scope index S. The final page may be partial.
func rainPageColumns(scopes, start, perPage int) int {
	if scopes <= 0 || perPage <= 0 {
		return 0
	}
	return min(perPage, scopes-start)
}

// rainColumnWidths divides the field among the page's actual columns: the
// `M-1` one-cell separators come off first, the remainder is divided evenly,
// and every extra cell goes to the leftmost columns (RG-006). A field too
// narrow to seat its separators leaves zero-interior columns, which stay in
// state and render their explicit constrained form.
func rainColumnWidths(width, columns int) []int {
	if columns <= 0 {
		return nil
	}
	interior := max(width-(columns-1), 0)
	share, extra := interior/columns, interior%columns
	widths := make([]int, columns)
	for index := range widths {
		widths[index] = share
		if index < extra {
			widths[index]++
		}
	}
	return widths
}

// rainPageCount returns how many fixed pages the Scope set has. An empty
// selection has exactly one empty-state page.
func rainPageCount(scopes, perPage int) int {
	if scopes <= 0 || perPage <= 0 {
		return 1
	}
	return (scopes + perPage - 1) / perPage
}

// rainPageStart returns the unique fixed page start holding the Scope at the
// given zero-based index, RG-006's `floor(anchor-index/K)*K`.
func rainPageStart(anchor, perPage int) int {
	if perPage <= 0 {
		return 0
	}
	return max(anchor, 0) / perPage * perPage
}

// rainStepPage returns the start of the page a step of `+1` (`]`) or `-1`
// (`[`) selects. It moves one fixed page in either direction with wraparound —
// past the end to the first page, and before the beginning to the final,
// possibly partial, one — so manual paging reaches every Scope without
// starvation.
func rainStepPage(start, scopes, perPage, step int) int {
	pages := rainPageCount(scopes, perPage)
	if perPage <= 0 || pages <= 1 {
		return 0
	}
	page := (start/perPage + step + pages) % pages
	return page * perPage
}
