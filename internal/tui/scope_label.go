package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fmueller/orgtop/internal/domain"
)

// scopeLabel renders the complete RG-012 label of one Scope: its compact
// presentation token and the requested repository, and for a path Scope the
// requested pattern behind it. A path Scope is therefore always distinguishable
// from a repository one and is never spelled as a synthetic repository.
func scopeLabel(scope domain.Scope, tokens map[domain.ScopeIdentity]string) string {
	return shortenScopeLabel(scope, tokens, -1)
}

// shortenScopeLabel renders the Scope's label inside a budget of terminal
// cells. Overview, Stream, Rain, and Interesting Now share this one function,
// so a Scope reads the same way in every view; a negative budget is unbounded.
func shortenScopeLabel(scope domain.Scope, tokens map[domain.ScopeIdentity]string, budget int) string {
	return shortenLabel(tokens[scope.Identity()], scope.String(), budget)
}

// shortenLabel renders the token and payload of one label inside the budget
// (RG-012). The token identifies the Scope, so it is retained ahead of the
// payload: a budget too tight for both keeps the token and marks the payload,
// and one too tight for the complete token renders its longest fitting prefix
// rather than an unmarked payload fragment that could name another Scope.
//
// A Scope the publication prepared no token for carries an empty token and
// shortens as its payload alone.
func shortenLabel(token, payload string, budget int) string {
	token, payload = escapeControls(token), escapeControls(payload)
	if token == "" {
		return shortenPayload(payload, budget)
	}

	complete := token + " " + payload
	if fits(lipgloss.Width(complete), budget) {
		return complete
	}

	// The token and the space after it are spent first, and the payload is
	// shortened into whatever cells remain. The remainder is clamped at zero
	// because a negative budget means unbounded to every measurement here.
	tokenWidth := lipgloss.Width(token)
	remaining := max(budget-tokenWidth-1, 0)
	if shortened := shortenPayload(payload, remaining); shortened != "" {
		return token + " " + shortened
	}
	if budget >= tokenWidth {
		return token
	}
	return ansi.Truncate(token, max(budget, 0), "")
}

// shortenPayload shortens one payload into a budget of cells: the split while
// the budget can hold it, then the mark alone, which still says content is
// missing. An empty render is the caller's signal that not even the mark fits,
// so a label with a token can fall back to its token-only forms.
func shortenPayload(payload string, budget int) string {
	if fits(lipgloss.Width(payload), budget) {
		return payload
	}
	if split := splitPayload(payload, budget); split != "" {
		return split
	}
	if budget >= lipgloss.Width(shortenedMark) {
		return shortenedMark
	}
	return ""
}

// splitPayload splits the payload around one mark inside the budget of cells.
// The prefix takes the larger half of the cells the mark leaves and the suffix
// the smaller half, both cut on whole grapheme clusters; cells a wide boundary
// leaves unspent extend the prefix first and then the suffix. A budget that
// cannot hold the mark and one whole cluster on each side yields nothing, which
// is the caller's signal to fall back to its next form.
func splitPayload(payload string, budget int) string {
	cells := budget - lipgloss.Width(shortenedMark)
	if cells < 2 {
		return ""
	}
	suffixShare := cells / 2
	suffix := longestSuffix(payload, suffixShare)
	prefix := ansi.Truncate(payload, cells-lipgloss.Width(suffix), "")
	suffix = longestSuffix(payload, cells-lipgloss.Width(prefix))
	if prefix == "" || suffix == "" {
		return ""
	}
	return prefix + shortenedMark + suffix
}

// longestSuffix returns the longest whole-cluster suffix of the payload that
// fits the budget. Dropping cells from the left may leave a wide cluster
// straddling the cut, so the drop grows until what remains measures inside it.
func longestSuffix(payload string, budget int) string {
	if budget <= 0 {
		return ""
	}
	total := lipgloss.Width(payload)
	for dropped := max(total-budget, 0); dropped < total; dropped++ {
		if suffix := ansi.TruncateLeft(payload, dropped, ""); lipgloss.Width(suffix) <= budget {
			return suffix
		}
	}
	return ""
}

// escapeControls renders the code points a terminal must never receive verbatim
// as visible text, before anything is measured (RG-012). Presentation payloads
// carry requested and source spellings, so a Scope cannot become an ANSI
// control sequence, reorder the line around it, or occupy no cell at all.
func escapeControls(text string) string {
	return escapeUnattachedClusters(escapeCodePoints(text))
}

// escapeCodePoints escapes the control and bidi code points RG-012 names,
// wherever in the payload they appear.
func escapeCodePoints(text string) string {
	if !strings.ContainsFunc(text, escaped) {
		return text
	}
	var builder strings.Builder
	for _, character := range text {
		if escaped(character) {
			writeEscape(&builder, character)
			continue
		}
		builder.WriteRune(character)
	}
	return builder.String()
}

// escapeUnattachedClusters escapes every code point of a grapheme cluster that
// occupies no cell, which is a cluster no positive-width grapheme carries: a
// leading combining mark, joiner, or variation selector would otherwise render
// as nothing at all and hide itself from the reader and from every measurement.
// The same code points inside a positive-width grapheme are left alone, so an
// emoji sequence stays one grapheme rather than becoming a row of escapes.
func escapeUnattachedClusters(text string) string {
	var builder strings.Builder
	for rest := text; rest != ""; {
		cluster, width := ansi.FirstGraphemeCluster(rest, ansi.GraphemeWidth)
		rest = rest[len(cluster):]
		if width > 0 {
			builder.WriteString(cluster)
			continue
		}
		for _, character := range cluster {
			writeEscape(&builder, character)
		}
	}
	return builder.String()
}

// writeEscape writes one code point as RG-012's visible uppercase escape.
func writeEscape(builder *strings.Builder, character rune) {
	fmt.Fprintf(builder, `\u{%X}`, character)
}

// escaped reports whether the code point renders as a visible escape: the C0
// and C1 control ranges, DEL, and the bidi controls RG-012 names.
func escaped(character rune) bool {
	switch {
	case character < 0x20, character == 0x7F:
		return true
	case character >= 0x80 && character <= 0x9F:
		return true
	case character == 0x061C, character == 0x200E, character == 0x200F:
		return true
	case character >= 0x202A && character <= 0x202E:
		return true
	case character >= 0x2066 && character <= 0x2069:
		return true
	}
	return false
}
