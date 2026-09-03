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
	// viewport is the Overview's own scroll position, moved and clamped by the
	// mechanism both views share.
	viewport
}

// render returns the Overview body for the shared content area.
func (o overview) render(state State, width, height int) string {
	return renderBody(overviewLines(state, width), o.viewport, width, height)
}

// scrolled returns the view moved by one scrolling keystroke.
func (o overview) scrolled(keystroke string, state State, width, height int) overview {
	o.viewport = o.viewport.scrolled(keystroke, len(overviewLines(state, width)), height)
	return o
}

// overviewLines returns the explicit state lines, the Scope rows, or both.
func overviewLines(state State, width int) []string {
	switch state.Freshness {
	case FreshnessLoading:
		return []string{"Loading repository activity…"}
	case FreshnessError:
		return []string{"Repository activity is unavailable"}
	}

	aggregates := state.Scoped.Aggregates()
	if len(aggregates) == 0 {
		return []string{noRecentActivity}
	}
	rows := overviewRows(aggregates, state.Scopes.Tokens(), width)
	if hasObservations(aggregates) {
		return rows
	}
	return append([]string{noRecentActivity}, rows...)
}

// hasObservations reports whether the snapshot confirmed activity or left any
// evidence undecided. Nothing is summed across Scopes: overlapping membership
// would double count, so only the per-Scope counts are shown (FR-009, RG-004).
func hasObservations(aggregates []domain.ScopeAggregate) bool {
	return slices.ContainsFunc(aggregates, func(aggregate domain.ScopeAggregate) bool {
		return aggregate.Activity > 0 || aggregate.Unknown > 0
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

// The empty states of one Scope row. RG-004 keeps them apart: a Scope with no
// confirmed member but undecided evidence has not been shown to be quiet, so it
// never renders as the Scope that complete evidence found empty.
const (
	noActivity          = "No activity"
	noConfirmedActivity = "No confirmed activity"
)

// rowLayout labels the direct counts of one Scope row: confirmed activity, the
// qualified current-PR subset of it, undecided evidence, and the pull-request
// and push categories the confirmed members fall into. No other measure is
// derived from them (FR-009, RG-004).
type rowLayout struct {
	activity     countLabel
	currentPR    countLabel
	unknown      countLabel
	pullRequests countLabel
	pushes       countLabel
}

// counts renders the labeled counts of one Scope in their fixed order. The
// activity clause leads, carrying the qualified current-PR subset and the
// unknown coverage the activity total is a lower bound against; the category
// subcounts, which only confirmed members contribute to, follow it.
func (r rowLayout) counts(aggregate domain.ScopeAggregate) string {
	if aggregate.Activity == 0 {
		if aggregate.Unknown == 0 {
			return noActivity
		}
		return noConfirmedActivity + separator + r.unknown.of(aggregate.Unknown)
	}

	fields := []string{r.activity.of(aggregate.Activity)}
	if aggregate.CurrentPR > 0 {
		fields = append(fields, r.currentPR.of(aggregate.CurrentPR))
	}
	if aggregate.Unknown > 0 {
		fields = append(fields, r.unknown.of(aggregate.Unknown))
	}
	fields = append(fields,
		r.pullRequests.of(aggregate.PullRequestActivity),
		r.pushes.of(aggregate.Pushes),
	)
	return strings.Join(fields, separator)
}

// overviewLayouts orders the row layouts from richest to sparsest. Both spell
// the empty states identically, because those are the contract's own words
// rather than a count a narrow terminal may abbreviate.
var overviewLayouts = []rowLayout{
	{
		activity:     countLabel{singular: "activity", plural: "activity"},
		currentPR:    countLabel{singular: "current PR", plural: "current PR"},
		unknown:      countLabel{singular: "unknown", plural: "unknown"},
		pullRequests: countLabel{singular: "pull request", plural: "pull requests"},
		pushes:       countLabel{singular: "push", plural: "pushes"},
	},
	{
		activity:     countLabel{singular: "act", plural: "act"},
		currentPR:    countLabel{singular: "cur PR", plural: "cur PR"},
		unknown:      countLabel{singular: "unk", plural: "unk"},
		pullRequests: countLabel{singular: "pr", plural: "pr"},
		pushes:       countLabel{singular: "push", plural: "push"},
	},
}

// overviewRows renders the widest row layout that fits the width. Every
// selected Scope keeps a row, including a zero-activity and an all-unknown one.
// A Scope label names the same Scope in every layout, so the labels are rendered
// once and only the counts are laid out again.
func overviewRows(aggregates []domain.ScopeAggregate, tokens map[domain.ScopeIdentity]string, width int) []string {
	labels := make([]string, 0, len(aggregates))
	for _, aggregate := range aggregates {
		labels = append(labels, scopeLabel(aggregate.Scope, tokens))
	}

	sparsest := len(overviewLayouts) - 1
	for _, layout := range overviewLayouts[:sparsest] {
		rows := layoutRows(aggregates, labels, layout)
		if fits(widestWidth(rows), width) {
			return rows
		}
	}
	return layoutRows(aggregates, labels, overviewLayouts[sparsest])
}

// layoutRows renders one Scope per line in the snapshot's prepared order,
// aligning the counts behind the widest Scope label.
func layoutRows(aggregates []domain.ScopeAggregate, labels []string, layout rowLayout) []string {
	labelWidth := widestWidth(labels)
	rows := make([]string, 0, len(aggregates))
	for index, aggregate := range aggregates {
		rows = append(rows, padRight(labels[index], labelWidth)+rowGap+layout.counts(aggregate))
	}
	return rows
}

// scopeLabel renders the full RG-012 label of one Scope: its compact
// presentation token and the requested repository, and for a path Scope the
// requested pattern behind it. A path Scope is therefore always distinguishable
// from a repository one and is never spelled as a synthetic repository.
func scopeLabel(scope domain.Scope, tokens map[domain.ScopeIdentity]string) string {
	token, prepared := tokens[scope.Identity()]
	if !prepared {
		return scope.String()
	}
	return token + " " + scope.String()
}

// padLeft pads the text with leading spaces up to the rendered width.
func padLeft(text string, width int) string {
	return strings.Repeat(" ", width-lipgloss.Width(text)) + text
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
