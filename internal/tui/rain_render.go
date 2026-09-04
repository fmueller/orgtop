package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// The lines Rain reserves inside the shared content area for its own chrome:
// the Scope column headings above the field and the context line below it. A
// third line holds the glyph legend when the width can hold it. FR-007 lets a
// view reserve fixed lines of its own beyond the shared header and footer.
const (
	rainHeadings = 1
	rainContext  = 1
	rainLegend   = 1
)

// columnGap is the one-cell separator between two Rain columns, which RG-006's
// column arithmetic already reserves.
const columnGap = " "

// legendHiddenContext is the RG-012 indicator a constrained Rain keeps when its
// dimensions cannot hold the complete glyph-to-text legend (RG-008).
const legendHiddenContext = "legend hidden"

// pausedContext marks the frozen field, which is the one Rain state a still
// picture is otherwise indistinguishable from.
const pausedContext = "PAUSED"

// rainVisibleLegend returns the complete glyph-to-text legend when the content
// area can hold it beside the headings, one field row, and the context, and the
// empty string when either dimension cannot. It is the one answer the height
// ladder and the rendered line both read, so a size never reserves a legend row
// the width then leaves empty.
func rainVisibleLegend(set charset, width, height int) string {
	if height >= 0 && height <= rainContext+rainHeadings+rainLegend {
		return ""
	}
	return rainLegendLine(set, width)
}

// fieldHeight returns the field rows the content area leaves once Rain's
// own chrome has taken its lines. The field is the primary content, so it keeps
// a row at every positive height and the chrome yields instead: the legend
// first, then the headings, then the context line (RG-012). A negative height
// is unbounded and holds the whole prepared field.
func (r rain) fieldHeight(set charset, width, height int) int {
	switch {
	case height < 0:
		return r.height
	case height <= 0:
		return 0
	case height <= rainContext+rainHeadings:
		return 1
	case rainVisibleLegend(set, width, height) != "":
		return height - rainContext - rainHeadings - rainLegend
	default:
		return height - rainContext - rainHeadings
	}
}

// render returns the Rain body for the shared content area: the Scope column
// headings, the bounded field, its context line, and the legend when the width
// permits it. Rendering only consumes prepared state; it never hashes, admits,
// collides, ages, or moves an item, and no glyph, accent, or motion claims
// severity, importance, anomaly, or a source outcome (RG-006).
func (r rain) render(state State, set charset, capability colorCapability, width, height int) string {
	if height == 0 {
		return ""
	}
	field := r.field()
	legend := rainVisibleLegend(set, width, height)

	lines := make([]string, 0, max(height, 1))
	if height < 0 || height > rainContext+rainHeadings {
		lines = append(lines, rainHeadingLine(field, state.Scopes.Tokens(), width))
	}
	lines = append(lines, rainFieldLines(field, state, set, capability, width, r.fieldHeight(set, width, height))...)
	if height < 0 || height > rainContext {
		lines = append(lines, rainContextLine(field, capability, legend == "", width))
	}
	if legend != "" {
		lines = append(lines, legend)
	}
	return strings.Join(lines, "\n")
}

// rainHeadingLine names every visible Scope above its column, shortened to the
// interior width the column was granted. A label is content inside a column and
// never changes the column count or width (RG-012).
func rainHeadingLine(field rainField, tokens map[domain.ScopeIdentity]string, width int) string {
	headings := make([]string, 0, len(field.columns))
	for _, column := range field.columns {
		headings = append(headings, padColumn(shorten(scopeLabel(column.scope, tokens), column.interior), column.interior))
	}
	return contextStyle.Render(shorten(strings.Join(headings, columnGap), width))
}

// rainFieldLines returns the field rows, or the one explicit line that replaces
// them above blank rows that keep the reserved height. A field yields to an
// explicit state exactly as the other views do: a snapshot that does not exist
// yet, or one that failed, is never drawn as a quiet field.
func rainFieldLines(field rainField, state State, set charset, capability colorCapability, width, height int) []string {
	if line := rainStateLine(state.Freshness, field.counts.candidates); line != "" {
		lines := make([]string, max(height, 1))
		lines[0] = bodyStyle.Render(shorten(line, width))
		return lines
	}
	lines := make([]string, 0, height)
	for row := range height {
		lines = append(lines, rainFieldRow(field, set, capability, row, width))
	}
	return lines
}

// rainStateLine returns the single explicit line Rain renders in place of its
// field, or the empty string when the field has candidates to draw.
func rainStateLine(freshness Freshness, candidates int) string {
	switch freshness {
	case FreshnessLoading:
		return "Loading recent activity…"
	case FreshnessError:
		return "Recent activity is unavailable"
	}
	if candidates == 0 {
		return noRecentActivity
	}
	return ""
}

// rainFieldRow draws one field row across every visible column.
func rainFieldRow(field rainField, set charset, capability colorCapability, row, width int) string {
	drawn := make([]string, 0, len(field.columns))
	for _, column := range field.columns {
		drawn = append(drawn, rainColumnRow(column, set, capability, row))
	}
	return shorten(strings.Join(drawn, columnGap), width)
}

// rainColumnRow draws one column's slots on one field row, padded to the
// interior width the column was granted. A column with no usable row or slot
// draws no token at all; its admitted items are already counted as hidden.
func rainColumnRow(column rainColumn, set charset, capability colorCapability, row int) string {
	if row < 0 || row >= len(column.rows) {
		return padColumn("", column.interior)
	}
	cellWidth := rainCellWidth(column.interior)
	var drawn strings.Builder
	for _, cell := range column.rows[row] {
		drawn.WriteString(rainCellToken(cell, set, capability, cellWidth))
	}
	return padColumn(drawn.String(), column.interior)
}

// rainCellWidth is how many terminal cells one logical slot of the column
// occupies: the glyph alone in RG-006's exact one-cell column, and the glyph
// plus its current-PR qualifier otherwise.
func rainCellWidth(interior int) int {
	if rainClipsQualification(interior) {
		return 1
	}
	return 2
}

// rainCellToken draws one prepared cell. The category is encoded by the shared
// glyph and the current-PR membership by the `~` qualifier, so colour and
// intensity reinforce recency alone and never carry a meaning of their own
// (RG-008).
func rainCellToken(cell rainCell, set charset, capability colorCapability, cellWidth int) string {
	if !cell.occupied {
		return strings.Repeat(" ", cellWidth)
	}
	token := categoryGlyph(cell.category, set)
	if cellWidth > 1 {
		qualifier := " "
		if cell.qualified {
			qualifier = currentPRQualifier
		}
		token += qualifier
	}
	return cell.recency.style(capability).Render(token)
}

// padColumn pads the drawn column to its granted interior width, so the next
// column always starts where the geometry places it.
func padColumn(drawn string, interior int) string {
	if width := lipgloss.Width(drawn); width < interior {
		return drawn + strings.Repeat(" ", interior-width)
	}
	return drawn
}

// rainContextLine renders the widest context that fits the width. The segments
// are ordered by how much of Rain's honesty depends on them, and a width that
// cannot hold a segment drops it and every lower-priority one after it; the
// prepared counts reappear unchanged after a resize (RG-012).
func rainContextLine(field rainField, capability colorCapability, legendHidden bool, width int) string {
	for _, candidate := range rainContextCandidates(field, capability, legendHidden) {
		if line := strings.Join(candidate, separator); fits(lipgloss.Width(line), width) {
			return contextStyle.Render(line)
		}
	}
	return contextStyle.Render(shorten(rainWindowContext(field.window), width))
}

// rainContextCandidates returns the context layouts from richest to sparsest:
// the full spelling, the same segments in their compact spelling, and then that
// layout with its lowest-priority segments dropped one at a time.
func rainContextCandidates(field rainField, capability colorCapability, legendHidden bool) [][]string {
	full := rainContextSegments(field, capability, legendHidden, false)
	compact := rainContextSegments(field, capability, legendHidden, true)
	candidates := make([][]string, 0, len(compact)+1)
	candidates = append(candidates, full)
	for kept := len(compact); kept > 0; kept-- {
		candidates = append(candidates, compact[:kept])
	}
	return candidates
}

// rainContextSegments returns the context segments in priority order, in their
// full or their compact spelling. The disjoint RG-006 counters keep their own
// words, so a hidden Scope, a hidden item, a capacity omission, a grouped
// collision, and a clipped qualification are never merged or substituted for
// one another. The selected window keeps its full spelling in both, because it
// is already short and names the lifetime every other count is measured under.
func rainContextSegments(field rainField, capability colorCapability, legendHidden, shortened bool) []string {
	segments := make([]string, 0, 9)
	if field.paused {
		segments = append(segments, pausedContext)
	}
	segments = append(segments, rainWindowContext(field.window))
	if field.scopes > 0 {
		segments = append(segments, rainScopeRange(field, shortened))
	}
	if legendHidden {
		segments = append(segments, pick(shortened, "no legend", legendHiddenContext))
	}
	if !capability.rendersIntensity() {
		segments = append(segments, rainRecencyContext(field.recencies, shortened))
	}
	for _, counted := range rainDisjointCounts(field) {
		if counted.count > 0 {
			segments = append(segments, fmt.Sprintf("%s%d%s", hiddenMark, counted.count, pick(shortened, counted.mark, " "+counted.text)))
		}
	}
	return segments
}

// rainDisjointCount is one of RG-006's disjoint counters with the word and the
// mark that name it.
type rainDisjointCount struct {
	count int
	text  string
	mark  string
}

// rainDisjointCounts returns the page's counters in their fixed priority order.
func rainDisjointCounts(field rainField) []rainDisjointCount {
	return []rainDisjointCount{
		{count: field.hiddenScopes, text: "scopes hidden", mark: "s"},
		{count: field.hiddenItems, text: "items hidden", mark: "i"},
		{count: rainOmitted(field.counts), text: "omitted", mark: "o"},
		{count: field.collisions, text: "collisions", mark: "c"},
		{count: field.clipped, text: "clipped", mark: "q"},
	}
}

// pick returns the compact spelling when the layout is shortened.
func pick(shortened bool, compact, full string) string {
	if shortened {
		return compact
	}
	return full
}

// rainWindowContext spells the selected recency window, which survives view
// changes and resize and is not persisted after exit (RG-006).
func rainWindowContext(window rainWindow) string { return "window " + window.String() }

// rainScopeRange states which Scopes of the selection the fixed page holds, in
// one-based inclusive positions, in its full or its compact spelling.
func rainScopeRange(field rainField, shortened bool) string {
	if shortened {
		if field.first == field.last {
			return fmt.Sprintf("%d/%d", field.first, field.scopes)
		}
		return fmt.Sprintf("%d-%d/%d", field.first, field.last, field.scopes)
	}
	if field.first == field.last {
		return fmt.Sprintf("scope %d of %d", field.first, field.scopes)
	}
	return fmt.Sprintf("scopes %d-%d of %d", field.first, field.last, field.scopes)
}

// rainRecencyContext spells the visible page's discrete recency totals, which
// RG-008 requires whenever the effective profile cannot separate the states by
// intensity, in its full and its shortened form.
func rainRecencyContext(recencies rainRecencies, shortened bool) string {
	if shortened {
		return fmt.Sprintf("age %d/%d/%d", recencies.fresh, recencies.recent, recencies.aging)
	}
	return fmt.Sprintf("recency: %d %s%s%d %s%s%d %s",
		recencies.fresh, recencyNew, separator,
		recencies.recent, recencyRecent, separator,
		recencies.aging, recencyAging)
}

// rainOmitted totals the capacity rejections, which are only that: an omission
// is never responsive hiding, and responsive hiding is never an omission.
func rainOmitted(counts rainCounts) int {
	return counts.columnOmitted + counts.globalOmitted + counts.pausedOmitted
}

// rainLegendLine renders the complete glyph-to-text legend, or the empty string
// at a width that cannot hold every category. RG-008 exposes the legend
// whenever dimensions permit and accounts for a hidden one otherwise, so a
// partial legend that silently spells fewer categories is never rendered.
func rainLegendLine(set charset, width int) string {
	for _, register := range []categoryRegister{registerRich, registerCompact} {
		entries := make([]string, 0, len(categoryOrder))
		for _, category := range categoryOrder {
			entries = append(entries, categoryGlyph(category, set)+" "+categoryText(category, register))
		}
		if line := strings.Join(entries, separator); fits(lipgloss.Width(line), width) {
			return contextStyle.Render(line)
		}
	}
	return ""
}
