package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/fmueller/orgtop/internal/domain"
)

// The field names bounded Stream event detail serializes its prepared facts
// under, in the order RG-012 lists them. Every line is `Name: value`, so a fact
// is named rather than positional and a value that wraps stays attributable.
const (
	detailEventField       = "Event"
	detailTimeField        = "Time"
	detailRepositoryField  = "Repository"
	detailCategoryField    = "Category"
	detailActorField       = "Actor"
	detailEntityField      = "Entity"
	detailDescriptionField = "Description"
	detailMemberField      = "Member"
	detailUnknownField     = "Unknown"
)

// currentPRDetail qualifies a member Scope decided from complete current-PR
// evidence, spelled out rather than marked, because detail has the room the row
// context does not and never shortens a full Scope label.
const currentPRDetail = "current PR"

// wideGraphemePlaceholder stands in for a grapheme wider than the single cell a
// one-column terminal grants it. Splitting the grapheme would emit half a
// cluster, and dropping it would hide prepared text, so the placeholder marks
// the cell instead and the original reappears once the width allows (RG-012).
const wideGraphemePlaceholder = "?"

// detailRange names the detail's own vertical accounting in the shared header,
// so hidden detail lines are never counted as hidden events.
const detailRange = "lines"

// streamDetail is the open-or-closed state of the bounded Stream event detail.
// It follows the stable source event ID rather than an index, so a refresh that
// reorders the snapshot moves the detail with its event instead of onto the
// event that inherited its position.
type streamDetail struct {
	// viewport is the detail's own line offset. Detail has no focused line, so
	// the offset moves directly and clamps against the detail rows alone.
	viewport
	// open reports whether detail is showing instead of the Stream rows.
	open bool
	// eventID is the stable source event ID the open detail follows.
	eventID string
}

// opened returns the detail opened on the source event ID, from the top.
func (d streamDetail) opened(eventID string) streamDetail {
	return streamDetail{open: true, eventID: eventID}
}

// detailLines serializes one prepared event into its logical detail lines. It
// reads the published snapshot alone: no API, cache, normalization, matching,
// aggregation, or ranking work happens here, and no further fact is fetched.
//
// The optional actor and description are omitted when the normalization left
// them empty, so detail never shows a named field with nothing behind it. Every
// member Scope carries its current-PR qualification and every unknown Scope its
// prepared reason; not-member Scopes are omitted, exactly as the row context
// omits them.
func detailLines(scoped domain.ScopedEvent, tokens map[domain.ScopeIdentity]string, anchor time.Time) []string {
	event := scoped.Event
	lines := []string{
		detailField(detailEventField, event.ID),
		detailField(detailTimeField, event.OccurredAt.Format(time.RFC3339)+separator+eventAge(event.OccurredAt, anchor)),
		detailField(detailRepositoryField, event.Repository.String()),
		detailField(detailCategoryField, categoryText(event.Category, registerRich)),
	}
	if event.Actor != "" {
		lines = append(lines, detailField(detailActorField, event.Actor))
	}
	lines = append(lines, detailField(detailEntityField, entityDetail(event)))
	if event.Description != "" {
		lines = append(lines, detailField(detailDescriptionField, event.Description))
	}
	return append(lines, scopeDetailLines(scoped.Memberships, tokens)...)
}

// scopeDetailLines serializes the event's per-Scope outcomes in the stable
// Scope identity order the snapshot published them in, members before unknowns
// so the confirmed membership is never read out of the investigatory context.
func scopeDetailLines(memberships []domain.ScopeMembership, tokens map[domain.ScopeIdentity]string) []string {
	members := make([]string, 0, len(memberships))
	unknowns := make([]string, 0, len(memberships))
	for _, membership := range memberships {
		label := scopeLabel(membership.Scope, tokens)
		switch {
		case membership.Membership.IsMember():
			if membership.Membership.QualifiedCurrentPR() {
				label += separator + currentPRDetail
			}
			members = append(members, detailField(detailMemberField, label))
		case membership.Membership.IsUnknown():
			unknowns = append(unknowns, detailField(detailUnknownField, label+separator+membership.Membership.Reason().String()))
		}
	}
	return append(members, unknowns...)
}

// entityDetail spells the entity the event refers to: its kind, and the
// optional reference behind it. The kind is always populated, so the field is
// never empty even for an event the source gave no reference for.
func entityDetail(event domain.Event) string {
	kind := string(event.EntityKind)
	if event.EntityRef == "" {
		return kind
	}
	return kind + separator + event.EntityRef
}

// detailField renders one named fact. The value is sanitized before it is
// measured or wrapped, so a source spelling can neither become an ANSI control
// sequence nor reorder the line around it (RG-012). Sanitization is idempotent,
// so a Scope label that already escaped its payload passes through unchanged.
func detailField(name, value string) string {
	return name + ": " + escapeControls(value)
}

// wrapDetail wraps every logical line into the physical lines the width holds,
// and totals the graphemes the width replaced with the placeholder. The wrapped
// lines are what the viewport scrolls, so no prepared text is hidden behind a
// mark: it is reached by scrolling instead (RG-012).
func wrapDetail(lines []string, width int) ([]string, int) {
	wrapped := make([]string, 0, len(lines))
	clipped := 0
	for _, line := range lines {
		physical, replaced := wrapDetailLine(line, width)
		wrapped = append(wrapped, physical...)
		clipped += replaced
	}
	return wrapped, clipped
}

// wrapDetailLine greedily splits one logical line into the longest whole-
// grapheme chunks of at most width cells, without dropping text and without
// inventing a continuation prefix, and reports how many graphemes the width
// replaced with the placeholder. A negative width is unbounded and keeps the
// line whole. A zero width renders no cells and an empty line has none to
// render, but both still occupy one physical line, so the prepared line count
// survives a terminal with no room for it and an event field left blank.
// Neither renders a placeholder, so neither clips a grapheme.
func wrapDetailLine(line string, width int) ([]string, int) {
	if width < 0 {
		return []string{line}, 0
	}
	if width == 0 || line == "" {
		return []string{""}, 0
	}
	var chunks []string
	clipped := 0
	for rest := line; rest != ""; {
		chunk, remainder, replaced := leadingChunk(rest, width)
		chunks = append(chunks, chunk)
		if replaced {
			clipped++
		}
		rest = remainder
	}
	return chunks, clipped
}

// leadingChunk takes the longest whole-grapheme prefix of the text that fits
// the positive width, and returns it with the remainder and whether the chunk
// is a placeholder. A leading grapheme wider than the whole width cannot be
// split, so it is consumed as the placeholder that marks the cell the width
// does grant, and the replacement is reported once for that grapheme alone.
func leadingChunk(text string, width int) (chunk, rest string, clipped bool) {
	var builder strings.Builder
	used := 0
	for rest = text; rest != ""; {
		cluster, cells := ansi.FirstGraphemeCluster(rest, ansi.GraphemeWidth)
		if used+cells > width {
			break
		}
		builder.WriteString(cluster)
		used += cells
		rest = rest[len(cluster):]
	}
	if builder.Len() == 0 {
		cluster, _ := ansi.FirstGraphemeCluster(text, ansi.GraphemeWidth)
		return wideGraphemePlaceholder, text[len(cluster):], true
	}
	return builder.String(), rest, false
}

// detailContent returns the wrapped detail lines of the open detail, how many
// graphemes the width replaced with the placeholder, and whether detail is open
// over an event the current snapshot still holds. A detail whose event a refresh
// removed reports closed, so no caller renders or accounts for lines the
// snapshot no longer prepares.
func detailContent(state State, detail streamDetail, width int) ([]string, int, bool) {
	if !detail.open {
		return nil, 0, false
	}
	events := state.Scoped.StreamEvents()
	index, found := eventIndex(events, detail.eventID)
	if !found {
		return nil, 0, false
	}
	lines, clipped := wrapDetail(detailLines(events[index], state.Scopes.Tokens(), state.LastSuccess), width)
	return lines, clipped, true
}

// eventIndex returns the position of the source event ID in the prepared list.
func eventIndex(events []domain.ScopedEvent, eventID string) (int, bool) {
	for index, scoped := range events {
		if scoped.Event.ID == eventID {
			return index, true
		}
	}
	return 0, false
}
