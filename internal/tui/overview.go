package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// noRecentActivity is the explicit empty state of a completely successful
// refresh that returned no events at all (FR-009).
const noRecentActivity = "No recent activity"

// rowGap separates a repository identity from its counts.
const rowGap = "  "

// overview is the Overview view's state slot and rendering seam. Repository
// aggregate rows replace the placeholder body without changing this seam.
type overview struct {
	// offset is the first rendered row. The shell preserves it across mode
	// switches so a view keeps its scroll position (FR-007).
	offset int
}

// render returns the Overview body for the shared content area.
func (o overview) render(state State, width, height int) string {
	return renderBody(overviewLines(state, width), o.offset, width, height)
}

// overviewLines returns the explicit state lines, the aggregate rows, or both.
func overviewLines(state State, width int) []string {
	switch state.Freshness {
	case FreshnessLoading:
		return []string{"Loading repository activity…"}
	case FreshnessError:
		return []string{"Repository activity is unavailable"}
	}

	aggregates := state.Snapshot.Aggregates()
	if len(aggregates) == 0 {
		return []string{noRecentActivity}
	}
	rows := overviewRows(aggregates, width)
	if hasEvents(aggregates) {
		return rows
	}
	return append([]string{noRecentActivity}, rows...)
}

// hasEvents reports whether the snapshot retained any event at all. Nothing is
// summed across repositories: only the per-repository counts are shown (FR-009).
func hasEvents(aggregates []domain.Aggregate) bool {
	return slices.ContainsFunc(aggregates, func(aggregate domain.Aggregate) bool {
		return aggregate.Total > 0
	})
}

// countLabel names one counted dimension in both grammatical numbers.
type countLabel struct {
	singular string
	plural   string
}

// of renders the count with the label that agrees with it.
func (c countLabel) of(count int) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, c.singular)
	}
	return fmt.Sprintf("%d %s", count, c.plural)
}

// rowLayout labels the direct counts of one row: total events, pull-request
// activity, and pushes. No other measure is derived from them (FR-009).
type rowLayout struct {
	events       countLabel
	pullRequests countLabel
	pushes       countLabel
}

// counts renders the labeled counts of one aggregate in their fixed order.
func (r rowLayout) counts(aggregate domain.Aggregate) string {
	return strings.Join([]string{
		r.events.of(aggregate.Total),
		r.pullRequests.of(aggregate.PullRequestActivity),
		r.pushes.of(aggregate.Pushes),
	}, separator)
}

// overviewLayouts orders the row layouts from richest to sparsest.
var overviewLayouts = []rowLayout{
	{
		events:       countLabel{singular: "event", plural: "events"},
		pullRequests: countLabel{singular: "pull request", plural: "pull requests"},
		pushes:       countLabel{singular: "push", plural: "pushes"},
	},
	{
		events:       countLabel{singular: "ev", plural: "ev"},
		pullRequests: countLabel{singular: "pr", plural: "pr"},
		pushes:       countLabel{singular: "push", plural: "push"},
	},
}

// overviewRows renders the widest row layout that fits the width. Every
// selected repository keeps a row, including one with no returned events.
func overviewRows(aggregates []domain.Aggregate, width int) []string {
	sparsest := len(overviewLayouts) - 1
	for _, layout := range overviewLayouts[:sparsest] {
		rows := layoutRows(aggregates, layout)
		if fits(widestWidth(rows), width) {
			return rows
		}
	}
	return layoutRows(aggregates, overviewLayouts[sparsest])
}

// layoutRows renders one aggregate per line in the snapshot's precomputed
// order, aligning the counts behind the widest repository identity.
func layoutRows(aggregates []domain.Aggregate, layout rowLayout) []string {
	identities := make([]string, 0, len(aggregates))
	for _, aggregate := range aggregates {
		identities = append(identities, aggregate.Repository.String())
	}
	identityWidth := widestWidth(identities)

	rows := make([]string, 0, len(aggregates))
	for index, aggregate := range aggregates {
		rows = append(rows, padRight(identities[index], identityWidth)+rowGap+layout.counts(aggregate))
	}
	return rows
}

// padRight pads the text with spaces up to the rendered width.
func padRight(text string, width int) string {
	return text + strings.Repeat(" ", width-lipgloss.Width(text))
}

// widestWidth returns the widest rendered width of the lines.
func widestWidth(lines []string) int {
	widest := 0
	for _, line := range lines {
		widest = max(widest, lipgloss.Width(line))
	}
	return widest
}
