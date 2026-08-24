package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// minDetailWidth is how much of the actor and description a row must still be
// able to show for its layout to count as fitting. Descriptions are free-form,
// so measuring a layout against the longest one would drop every terminal to
// the sparsest spelling; the shared body truncates whatever still overflows.
const minDetailWidth = 20

// streamChrome is the number of content-area lines Stream reserves for its own
// chrome: the coverage disclosure and the column headings. FR-007 lets a view
// reserve further fixed lines of its own beyond the shared header and footer;
// these are Stream's.
const streamChrome = 2

// boundedDisclosure is what Stream adds to its event count when the FR-006 bound
// discarded older events, so a list that ends at the bound says so rather than
// ending without explanation (FR-010).
const boundedDisclosure = "older activity was discarded at this limit"

// eventCount labels the number of events one snapshot holds.
var eventCount = countLabel{singular: "event", plural: "events"}

// stream is the Stream view's state slot and rendering seam. It renders the
// snapshot's events in their deterministic reverse-chronological order and
// windows them manually, so no component library is needed (FR-010).
type stream struct {
	// viewport is the Stream's own scroll position, moved and clamped by the
	// mechanism both views share.
	viewport
}

// render returns the Stream body for the shared content area: the sticky chrome
// lines the view reserves, above the windowed event rows.
func (s stream) render(state State, width, height int) string {
	chrome, lines, rowHeight := streamContent(state, width, height)
	rendered := make([]string, 0, len(chrome)+1)
	for _, line := range chrome {
		rendered = append(rendered, bodyStyle.Render(shorten(line, width)))
	}
	rendered = append(rendered, renderBody(lines, s.viewport, width, rowHeight))
	return strings.Join(rendered, "\n")
}

// scrolled returns the view moved by one scrolling keystroke over the rows that
// remain once Stream's own chrome has taken its lines.
func (s stream) scrolled(keystroke string, state State, width, height int) stream {
	_, lines, rowHeight := streamContent(state, width, height)
	s.viewport = s.viewport.scrolled(keystroke, len(lines), rowHeight)
	return s
}

// streamContent returns Stream's sticky chrome lines, the scrollable lines
// beneath them, and the height those lines are windowed and clamped against.
// Rendering and scrolling both go through it, so the clamp and the render cannot
// disagree about how many rows the content area holds.
//
// The chrome describes rows, so the explicit states are given none, and it is
// subordinate to content: a content area that cannot hold all of it keeps one
// event row, dropping the coverage disclosure before the column headings that
// name what the remaining row means (FR-010, A-010).
func streamContent(state State, width, height int) (chrome []string, lines []string, rowHeight int) {
	events := state.Snapshot.Events()
	if line := streamStateLine(state.Freshness, events); line != "" {
		return nil, []string{line}, height
	}

	laid := layoutFittingWidth(events, state.LastSuccess, width)
	chrome = []string{streamCoverage(len(events), state.Snapshot.Truncated()), laid.heading.String()}
	// A non-positive height is unbounded and holds all of it; a bounded one
	// gives up chrome lines, the disclosure first, to keep one event row.
	for height > 0 && len(chrome) >= height {
		chrome = chrome[1:]
	}
	return chrome, renderStreamRows(laid.rows), height - len(chrome)
}

// streamCoverage states how much activity the list represents: the number of
// events the snapshot holds, and whether the FR-006 bound discarded older ones.
// Without it a short list cannot be told from a bounded one, and scrolling to
// the bottom cannot be told from having seen everything (FR-010).
func streamCoverage(count int, truncated bool) string {
	showing := "showing " + eventCount.of(count)
	if !truncated {
		return showing
	}
	return showing + separator + boundedDisclosure
}

// streamStateLine returns the single explicit line Stream renders in place of
// its rows, or the empty string when the snapshot has events to lay out.
func streamStateLine(freshness Freshness, events []domain.Event) string {
	switch freshness {
	case FreshnessLoading:
		return "Loading recent events…"
	case FreshnessError:
		return "Recent events are unavailable"
	}
	if len(events) == 0 {
		return noRecentActivity
	}
	return ""
}

// streamLayout labels one row layout: how each category is named, and how the
// heading row names the columns. The category is always text, so a monochrome
// terminal keeps the full encoding (FR-010). The age column has one spelling at
// every width, because it is already the narrowest honest one.
type streamLayout struct {
	push        string
	pullRequest string
	review      string
	comment     string
	other       string
	// headings names the columns in this layout's own register, so a sparser
	// row keeps sparser headings above it.
	headings streamHeadings
}

// streamHeadings names the four rendered columns of one layout.
type streamHeadings struct {
	age      string
	identity string
	category string
	detail   string
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
	{
		push: "push", pullRequest: "pull request", review: "review", comment: "comment", other: "other",
		headings: streamHeadings{age: "age", identity: "repository", category: "category", detail: "actor" + separator + "description"},
	},
	{
		push: "push", pullRequest: "pr", review: "rev", comment: "com", other: "oth",
		headings: streamHeadings{age: "age", identity: "repo", category: "type", detail: "detail"},
	},
}

// streamRow is one laid-out line: the aligned age, repository, and category,
// followed by the optional actor and description.
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

// laidOutStream is one layout applied to the snapshot: the column heading row
// and the event rows, sharing one set of column widths.
type laidOutStream struct {
	heading streamRow
	rows    []streamRow
}

// required returns the widest width the heading and the rows need.
func (l laidOutStream) required() int {
	widest := l.heading.required()
	for _, row := range l.rows {
		widest = max(widest, row.required())
	}
	return widest
}

// layoutFittingWidth returns the widest row layout that fits the width. Every
// snapshot event keeps a row; the shared body truncates what still overflows.
func layoutFittingWidth(events []domain.Event, lastSuccess time.Time, width int) laidOutStream {
	sparsest := len(streamLayouts) - 1
	for _, layout := range streamLayouts[:sparsest] {
		laid := layoutStream(events, lastSuccess, layout)
		if fits(laid.required(), width) {
			return laid
		}
	}
	return layoutStream(events, lastSuccess, streamLayouts[sparsest])
}

// renderStreamRows renders the laid-out rows as body lines.
func renderStreamRows(rows []streamRow) []string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, row.String())
	}
	return lines
}

// layoutStream lays out the heading row and one event per row in the snapshot's
// order, aligning the detail behind the widest repository identity and category
// name. The headings share those widths, so they stay above the values they
// name. Ages are right-aligned in their own column so the snapshot's
// reverse-chronological order reads straight down it (FR-010).
func layoutStream(events []domain.Event, lastSuccess time.Time, layout streamLayout) laidOutStream {
	// The headings lead every column, so they are measured and laid out as the
	// first row and split back off once the widths are shared.
	ages := []string{layout.headings.age}
	identities := []string{layout.headings.identity}
	categories := []string{layout.headings.category}
	details := []string{layout.headings.detail}
	for _, event := range events {
		ages = append(ages, eventAge(event.OccurredAt, lastSuccess))
		identities = append(identities, event.Repository.String())
		categories = append(categories, layout.name(event.Category))
		details = append(details, streamDetail(event))
	}
	ageWidth, identityWidth, categoryWidth := widestWidth(ages), widestWidth(identities), widestWidth(categories)

	laid := make([]streamRow, 0, len(ages))
	for index := range ages {
		laid = append(laid, streamRow{
			columns: strings.Join([]string{
				padLeft(ages[index], ageWidth),
				padRight(identities[index], identityWidth),
				padRight(categories[index], categoryWidth),
			}, rowGap),
			detail: details[index],
		})
	}
	return laidOutStream{heading: laid[0], rows: laid[1:]}
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
