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

	var cause []field
	if state.Cause != "" {
		cause = []field{{text: state.Cause, style: causeStyle}}
	}
	var listed, counted []field
	if state.Scope.Len() > 0 {
		listed = []field{{text: scopeList(state.Scope), style: contextStyle}}
		counted = []field{{text: scopeCount(state.Scope), style: contextStyle}}
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

	return [][]field{
		slices.Concat(title, core, cause, listed, updated),
		slices.Concat(title, core, cause, counted, updated),
		slices.Concat(title, core, cause, context),
		slices.Concat(core, cause, context),
		slices.Concat(core, cause),
		slices.Concat(core, context),
		core,
	}
}

// renderFooter renders the widest control hint that fits the width. The quit
// hint is retained at every size.
func renderFooter(width int) string {
	for _, candidate := range footerCandidates {
		if fits(lipgloss.Width(candidate), width) {
			return footerStyle.Render(candidate)
		}
	}
	return footerStyle.Render(truncate(footerCandidates[len(footerCandidates)-1], width))
}

// footerCandidates advertises only the controls the shell implements.
var footerCandidates = []string{
	strings.Join([]string{"1 overview", "2 stream", "tab switch", "up/down scroll", "pgup/pgdn page", "q quit"}, separator),
	strings.Join([]string{"1/2/tab switch", "up/down scroll", "pgup/pgdn page", "q quit"}, separator),
	strings.Join([]string{"1/2/tab switch", "up/down scroll", "q quit"}, separator),
	strings.Join([]string{"1/2/tab switch", "q quit"}, separator),
	"q quit",
}

// scopeList names every selected repository in request order.
func scopeList(scope domain.Scope) string {
	names := make([]string, 0, scope.Len())
	for _, repository := range scope.Repositories() {
		names = append(names, repository.String())
	}
	return strings.Join(names, ", ")
}

// scopeCount summarizes the scope when the full list does not fit (FR-002).
func scopeCount(scope domain.Scope) string {
	if scope.Len() == 1 {
		return "1 repository"
	}
	return fmt.Sprintf("%d repositories", scope.Len())
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
