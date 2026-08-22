package tui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// minDetailWidth is how much of the actor and description a row must still be
// able to show for its layout to count as fitting. Descriptions are free-form,
// so measuring a layout against the longest one would drop every terminal to
// the sparsest spelling; the shared body truncates whatever still overflows.
const minDetailWidth = 20

// stream is the Stream view's state slot and rendering seam. It renders the
// snapshot's events in their deterministic reverse-chronological order and
// windows them manually, so no component library is needed (FR-010).
type stream struct {
	// viewport is the Stream's own scroll position, moved and clamped by the
	// mechanism both views share.
	viewport
}

// render returns the Stream body for the shared content area.
func (s stream) render(state State, width, height int) string {
	return renderBody(streamLines(state, width), s.viewport, width, height)
}

// scrolled returns the view moved by one scrolling keystroke.
func (s stream) scrolled(keystroke string, state State, width, height int) stream {
	s.viewport = s.viewport.scrolled(keystroke, len(streamLines(state, width)), height)
	return s
}

// streamLines returns the explicit state line or the event rows.
func streamLines(state State, width int) []string {
	switch state.Freshness {
	case FreshnessLoading:
		return []string{"Loading recent events…"}
	case FreshnessError:
		return []string{"Recent events are unavailable"}
	}

	events := state.Snapshot.Events()
	if len(events) == 0 {
		return []string{noRecentActivity}
	}
	return streamRows(events, width)
}

// streamLayout labels one row layout: how precisely the occurrence time is
// spelled and how each category is named. The category is always text, so a
// monochrome terminal keeps the full encoding (FR-010).
type streamLayout struct {
	clock       string
	push        string
	pullRequest string
	review      string
	comment     string
	other       string
}

// name returns the layout's text encoding of the category. A category the
// layout does not name is spelled as the catch-all one.
func (l streamLayout) name(category domain.Category) string {
	switch category {
	case domain.CategoryPush:
		return l.push
	case domain.CategoryPullRequest:
		return l.pullRequest
	case domain.CategoryReview:
		return l.review
	case domain.CategoryComment:
		return l.comment
	default:
		return l.other
	}
}

// streamLayouts orders the row layouts from richest to sparsest.
var streamLayouts = []streamLayout{
	{clock: clockLayout, push: "push", pullRequest: "pull request", review: "review", comment: "comment", other: "other"},
	{clock: "15:04", push: "push", pullRequest: "pr", review: "rev", comment: "com", other: "oth"},
}

// streamRow is one laid-out event: the aligned occurrence time, repository, and
// category, followed by the optional actor and description.
type streamRow struct {
	columns string
	detail  string
}

// String renders the row. A row without detail keeps no padding behind its
// category.
func (r streamRow) String() string {
	if r.detail == "" {
		return strings.TrimRight(r.columns, " ")
	}
	return r.columns + rowGap + r.detail
}

// required is the width the row needs to stay readable: its aligned columns and
// enough of the detail to start reading it.
func (r streamRow) required() int {
	if r.detail == "" {
		return lipgloss.Width(r.columns)
	}
	return lipgloss.Width(r.columns) + lipgloss.Width(rowGap) + min(lipgloss.Width(r.detail), minDetailWidth)
}

// streamRows renders the widest row layout that fits the width. Every snapshot
// event keeps a row; the shared body truncates what still overflows.
func streamRows(events []domain.Event, width int) []string {
	sparsest := len(streamLayouts) - 1
	for _, layout := range streamLayouts[:sparsest] {
		rows := layoutStreamRows(events, layout)
		if fits(requiredWidth(rows), width) {
			return renderStreamRows(rows)
		}
	}
	return renderStreamRows(layoutStreamRows(events, streamLayouts[sparsest]))
}

// requiredWidth returns the widest width the rows need.
func requiredWidth(rows []streamRow) int {
	widest := 0
	for _, row := range rows {
		widest = max(widest, row.required())
	}
	return widest
}

// renderStreamRows renders the laid-out rows as body lines.
func renderStreamRows(rows []streamRow) []string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, row.String())
	}
	return lines
}

// layoutStreamRows lays out one event per row in the snapshot's order, aligning
// the detail behind the widest repository identity and category name.
func layoutStreamRows(events []domain.Event, layout streamLayout) []streamRow {
	identities := make([]string, 0, len(events))
	categories := make([]string, 0, len(events))
	for _, event := range events {
		identities = append(identities, event.Repository.String())
		categories = append(categories, layout.name(event.Category))
	}
	identityWidth, categoryWidth := widestWidth(identities), widestWidth(categories)

	rows := make([]streamRow, 0, len(events))
	for index, event := range events {
		rows = append(rows, streamRow{
			columns: strings.Join([]string{
				event.OccurredAt.Local().Format(layout.clock),
				padRight(identities[index], identityWidth),
				padRight(categories[index], categoryWidth),
			}, rowGap),
			detail: streamDetail(event),
		})
	}
	return rows
}

// streamDetail joins the optional actor and description of one event, so an
// event without either keeps a row rather than an empty field.
func streamDetail(event domain.Event) string {
	present := make([]string, 0, 2)
	for _, field := range []string{event.Actor, event.Description} {
		if field != "" {
			present = append(present, field)
		}
	}
	return strings.Join(present, separator)
}
