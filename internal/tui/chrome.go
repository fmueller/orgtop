package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

const (
	appName = "OrgTop"
	// transportLabel is constant: the Events API is near-current, never live.
	transportLabel = "POLLING"
	separator      = " · "
	clockLayout    = "15:04:05"
)

// unbounded is the width or height limit that constrains nothing. It marks a
// terminal size the shell has not been told yet, never a reported size of zero.
const unbounded = -1

// field is one styled header segment. Layout decisions use the plain text, so
// the measured width never depends on the emitted escape sequences.
type field struct {
	text  string
	style lipgloss.Style
}

// renderHeader renders the widest header that fits the width. The active view,
// the transport label, and the freshness marker are always retained; scope
// context, the last success, and the product name give way to them.
func renderHeader(state State, mode Mode, width int) string {
	candidates := headerCandidates(state, mode)
	for _, candidate := range candidates {
		if fits(lipgloss.Width(plainFields(candidate)), width) {
			return joinFields(candidate)
		}
	}
	return truncate(plainFields(candidates[len(candidates)-1]), width)
}

// headerCandidates returns the header layouts from richest to sparsest.
func headerCandidates(state State, mode Mode) [][]field {
	title := []field{{text: appName, style: titleStyle}}
	core := []field{{text: mode.Label(), style: viewStyle}, {text: transportLabel, style: transportStyle}}
	if marker := state.Freshness.Marker(); marker != "" {
		core = append(core, field{text: marker, style: markerStyle(state.Freshness)})
	}

	if marker := state.SelectionFreshness.Marker(); marker != "" {
		core = append(core, field{text: marker, style: staleStyle})
	}

	var cause []field
	if state.Cause != "" {
		cause = append(cause, field{text: state.Cause, style: causeStyle})
	}
	if state.SelectionCause != "" {
		cause = append(cause, field{text: state.SelectionCause, style: causeStyle})
	}
	var listed, counted []field
	if state.Scopes.Len() > 0 {
		listed = []field{{text: scopeList(state.Scopes), style: contextStyle}}
		counted = []field{{text: scopeCount(state.Scopes), style: contextStyle}}
	}
	var updated []field
	if !state.LastSuccess.IsZero() {
		updated = []field{{text: "updated " + state.LastSuccess.Format(clockLayout), style: contextStyle}}
	}

	// context is the last context field a tightening header keeps. A stale
	// header must report the last-success time beside its cause (FR-008), so
	// there the scope summary gives way first; every other state keeps the
	// scope context the operator selected (FR-002).
	context := counted
	if state.Freshness == FreshnessStale {
		context = updated
	}

	layouts := [][]field{
		slices.Concat(title, core, cause, listed, updated),
		slices.Concat(title, core, cause, counted, updated),
		slices.Concat(title, core, cause, context),
		slices.Concat(core, cause, context),
		slices.Concat(core, cause),
		slices.Concat(core, context),
		core,
	}
	return withSelection(layouts, selectionForms(state.Selection))
}

// withSelection appends the RG-010 selection detail to every layout. The detail
// is the lowest-priority segment, so each layout first yields the detail from
// its full form down to nothing before the next layout drops a field the
// selection detail must never displace.
func withSelection(layouts [][]field, forms []string) [][]field {
	if len(forms) == 0 {
		return layouts
	}

	candidates := make([][]field, 0, len(layouts)*(len(forms)+1))
	for _, layout := range layouts {
		for _, form := range forms {
			candidates = append(candidates, append(slices.Clone(layout), field{text: form, style: contextStyle}))
		}
		candidates = append(candidates, layout)
	}
	return candidates
}

// selectionForms returns the RG-010 selection detail from its full summary down
// to its minimum, or nothing for an invocation without an organization
// selector. A bounded result is never presented as the complete organization:
// the omission count and the remaining-page condition survive into the forms
// that can still hold them.
func selectionForms(selection Selection) []string {
	if len(selection.Selectors) == 0 {
		return nil
	}

	omitted := 0
	for _, selector := range selection.Selectors {
		omitted += selector.Omitted
	}
	more := selection.PaginationRemains

	full := fmt.Sprintf("selection: %d repos · %d scopes · %d exact · %d expanded",
		selection.DistinctRepositories, selection.TotalScopes, selection.ExactScopes, selection.ExpandedScopes)
	compact := fmt.Sprintf("sel %d repos · %d scopes", selection.DistinctRepositories, selection.TotalScopes)
	if omitted > 0 {
		full += fmt.Sprintf("%s%d eligible omitted", separator, omitted)
		compact += fmt.Sprintf("%s%d omitted", separator, omitted)
	}
	if more {
		full += separator + "more eligible may be omitted"
		compact += separator + "more?"
	}

	forms := []string{full, compact}
	if minimum := selectionMinimum(omitted, more); minimum != "" {
		forms = append(forms, minimum)
	}
	return forms
}

// selectionMinimum returns the smallest form of the selection detail: the exact
// eligible-omission count, marked with the unknown remainder when more eligible
// repositories may exist. A selection that omitted nothing and left no page
// unconsumed has nothing left to disclose at this size.
func selectionMinimum(omitted int, more bool) string {
	switch {
	case omitted > 0 && more:
		return fmt.Sprintf("SEL %d+?", omitted)
	case omitted > 0:
		return fmt.Sprintf("SEL %d", omitted)
	case more:
		return "SEL ?"
	default:
		return ""
	}
}

// renderFooter renders the widest control hint that fits the width. The quit
// hint is retained at every size, and the active view decides which controls
// are advertised, because Rain's own keys act in Rain alone.
func renderFooter(mode Mode, width int) string {
	candidates := footerCandidates(mode)
	for _, candidate := range candidates {
		if fits(lipgloss.Width(candidate), width) {
			return footerStyle.Render(candidate)
		}
	}
	return footerStyle.Render(truncate(candidates[len(candidates)-1], width))
}

// footerCandidates advertises only the controls the active view implements.
func footerCandidates(mode Mode) []string {
	if mode == ModeRain {
		return rainFooterCandidates
	}
	return scrollFooterCandidates
}

// The shared navigation hint every footer leads with, in its full and its
// compact spelling.
var (
	fullNavigation    = []string{"1 overview", "2 stream", "3 rain", "tab switch"}
	compactNavigation = []string{"1/2/3/tab switch"}
)

// scrollFooterCandidates advertises the scrolling views' controls.
var scrollFooterCandidates = footerLadder(
	[]string{"up/down scroll", "pgup/pgdn page"},
	[]string{"up/down scroll"},
)

// rainFooterCandidates advertises Rain's own controls: it neither scrolls nor
// pages by row, so it advertises its fixed Scope pages, its recency window, and
// its pause instead.
var rainFooterCandidates = footerLadder(
	[]string{"[/] page", "-/+ window", "p pause"},
	[]string{"[/] page", "p pause"},
)

// footerLadder builds one view's hints from richest to sparsest: the full
// navigation with the view's own controls, then the compact navigation with
// those controls and with the shorter set, then navigation alone, and finally
// the quit hint every size retains.
func footerLadder(controls, shorter []string) []string {
	return []string{
		hintLine(fullNavigation, controls),
		hintLine(compactNavigation, controls),
		hintLine(compactNavigation, shorter),
		hintLine(compactNavigation, nil),
		"q quit",
	}
}

// hintLine joins one navigation spelling, the advertised controls, and the quit
// hint that closes every footer.
func hintLine(navigation, controls []string) string {
	return strings.Join(slices.Concat(navigation, controls, []string{"q quit"}), separator)
}

// scopeList names every selected repository in request order.
func scopeList(scopes domain.ScopeSet) string {
	names := make([]string, 0, scopes.Len())
	for _, repository := range scopes.Repositories() {
		names = append(names, repository.String())
	}
	return strings.Join(names, ", ")
}

// scopeCount summarizes the scope when the full list does not fit (FR-002).
func scopeCount(scopes domain.ScopeSet) string {
	if scopes.Len() == 1 {
		return "1 repository"
	}
	return fmt.Sprintf("%d repositories", scopes.Len())
}

// joinFields renders the fields with their semantic styles.
func joinFields(fields []field) string {
	rendered := make([]string, 0, len(fields))
	for _, current := range fields {
		rendered = append(rendered, current.style.Render(current.text))
	}
	return strings.Join(rendered, separator)
}

// plainFields joins the field texts without styling them.
func plainFields(fields []field) string {
	texts := make([]string, 0, len(fields))
	for _, current := range fields {
		texts = append(texts, current.text)
	}
	return strings.Join(texts, separator)
}

// shortenedMark marks content the width could not hold, so a cut line is not
// read as the whole line (FR-010).
const shortenedMark = "…"

// fits reports whether a width satisfies the limit.
func fits(width, limit int) bool { return limit < 0 || width <= limit }

// shorten cuts the text to the limit and marks it as shortened. The mark is
// paid for out of the same limit, so a marked line is never wider than the
// unmarked one it replaces, and a limit too tight to hold the mark yields
// nothing rather than an unmarked cut.
//
// The bodies both views render go through here; the header and the footer do
// not. Each of those already has an explicit ladder that drops whole fields to
// fit, so their shortening is announced by what is missing rather than by a
// mark, and their final cut happens at widths where the mark would consume the
// last readable cells.
func shorten(text string, limit int) string {
	if fits(lipgloss.Width(text), limit) {
		return text
	}
	markWidth := lipgloss.Width(shortenedMark)
	if limit < markWidth {
		return ""
	}
	return truncate(text, limit-markWidth) + shortenedMark
}

// truncate cuts the text to the limit without marking it.
func truncate(text string, limit int) string {
	if fits(lipgloss.Width(text), limit) {
		return text
	}
	var builder strings.Builder
	used := 0
	for _, character := range text {
		width := lipgloss.Width(string(character))
		if used+width > limit {
			break
		}
		builder.WriteRune(character)
		used += width
	}
	return builder.String()
}
