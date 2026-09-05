package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// The rows RG-007's height collapse reserves. The strip spends one row on its
// title, the Rain activity field keeps at least three rows whenever the body
// has them, and a body of exactly those four rows holds the field plus the
// count-only strip line.
const (
	stripTitleRow   = 1
	stripFieldFloor = 3
	stripCollapse   = stripFieldFloor + stripTitleRow
)

// stripEmptyState is the explicit line the strip renders when the retained
// snapshot carries no eligible event at all. It states the absence rather than
// leaving the reader to read an empty area as a rendering fault.
const stripEmptyState = "Interesting Now: no recent activity"

// stripOverflowHint is RG-007's explicit indicator that the collapsed strip is
// holding entries the size cannot show. `2` reaches every retained eligible
// source event in Stream.
const stripOverflowHint = "q I+"

// rows returns how many of the available body rows the strip takes and how many
// stored entries it renders in them, applying RG-007's height collapse: no
// strip line at all below four rows, the count-only line at exactly four, and
// the title plus `D=min(5,stored,B-4)` entries above that. Collapse changes
// only the visible count and never the stored selection, its order, or the
// 20-entry bound.
//
// A strip that has not published a first snapshot yet reports nothing, because
// it has no eligible/stored facts to account for; a negative row count is an
// unbounded body that renders the whole visible strip.
func (s interesting) rows(available int) (rows, shown int) {
	switch {
	case !s.started:
		return 0, 0
	case s.counts.eligible == 0:
		if available < 0 || available >= stripCollapse {
			return stripTitleRow, 0
		}
		return 0, 0
	case available < 0:
		shown = len(s.visible())
		return stripTitleRow + shown, shown
	case available < stripCollapse:
		return 0, 0
	default:
		shown = min(stripVisible, len(s.entries), available-stripCollapse)
		return stripTitleRow + shown, shown
	}
}

// render returns the strip lines a Rain body appends below its field: the
// accounting title, or the empty state, followed by the entries the height
// granted. It consumes prepared state alone and reranks, reselects, and reages
// nothing (RG-007).
func (s interesting) render(tokens map[domain.ScopeIdentity]string, set charset, capability colorCapability, width, rows, shown int) []string {
	if rows <= 0 {
		return nil
	}
	lines := make([]string, 0, rows)
	lines = append(lines, contextStyle.Render(shorten(s.titleLine(shown, width), width)))
	for _, entry := range s.entries[:min(shown, len(s.entries))] {
		lines = append(lines, entry.render(tokens, set, capability, width))
	}
	return lines
}

// titleLine returns the line above the rendered entries: the explicit empty
// state, the count-only line of a body that has room for nothing else, or the
// accounting title of the entries beneath it.
func (s interesting) titleLine(shown, width int) string {
	if s.counts.eligible == 0 {
		return stripEmptyState
	}
	forms := stripTitleForms
	if shown == 0 {
		forms = stripCountForms
	}
	return s.accounting(shown).line(forms, width)
}

// stripAccounting is RG-007's disjoint strip accounting: the entries rendered,
// the stored ones a height left hidden, and the eligible ones the 20-entry
// capacity omitted. The three are separate facts, so a narrower spelling
// shortens all of them rather than merging or dropping one.
type stripAccounting struct {
	shown   int
	hidden  int
	omitted int
}

// accounting returns the disjoint totals for the rendered entry count.
func (s interesting) accounting(shown int) stripAccounting {
	return stripAccounting{
		shown:   shown,
		hidden:  max(s.counts.stored-shown, 0),
		omitted: s.counts.omitted,
	}
}

// overflowing reports whether the strip is holding entries the current size
// does not render, which is what RG-007's `I+` indicator marks.
func (a stripAccounting) overflowing() bool { return a.hidden+a.omitted > 0 }

// stripAccountingForms spells the accounting from its full form down to its
// narrowest one. Both ladders end in the same compact spelling, so the tightest
// footer and the tightest title state the identical three counts.
type stripAccountingForms []string

// stripTitleForms names the strip above its entries.
var stripTitleForms = stripAccountingForms{
	"Interesting Now: %d shown" + separator + "%d hidden" + separator + "%d omitted",
	"I: %d shown %d hidden %d omitted",
	"I:%d/%d/%d",
}

// stripCountForms names the accounting where the strip has no entry row of its
// own: the count-only body line and the collapsed Rain footer.
var stripCountForms = stripAccountingForms{
	"interesting: %d shown/%d hidden/%d omitted",
	"I: %d shown %d hidden %d omitted",
	"I:%d/%d/%d",
}

// line returns the widest spelling of the accounting that fits the width, and
// the narrowest one when none does; the caller shortens what still overflows.
func (a stripAccounting) line(forms stripAccountingForms, width int) string {
	for _, form := range forms {
		if spelled := a.spell(form); fits(lipgloss.Width(spelled), width) {
			return spelled
		}
	}
	return a.spell(forms[len(forms)-1])
}

// spell renders one accounting form with the three disjoint counts.
func (a stripAccounting) spell(form string) string {
	return fmt.Sprintf(form, a.shown, a.hidden, a.omitted)
}

// footerAccounting returns the accounting the Rain footer states in place of
// its optional hints, and whether the current body owes it: a strip with
// eligible entries and fewer than four body rows renders no line of its own, so
// the footer is where its counts stay visible (RG-007).
func (s interesting) footerAccounting(available int) (stripAccounting, bool) {
	if !s.started || s.counts.eligible == 0 || available < 0 || available >= stripCollapse {
		return stripAccounting{}, false
	}
	return s.accounting(0), true
}

// stripDetail names how much of one entry a form still carries. The levels drop
// RG-007's lowest-priority facts first: the actor and entity detail, then the
// additional-Scope count, then the current-PR qualification. Action, category,
// repository, sponsoring Scope, and age survive every level.
type stripDetail int

const (
	// stripDetailCore is action or category, repository, sponsoring Scope, age.
	stripDetailCore stripDetail = iota
	// stripDetailQualified adds the current-PR qualification.
	stripDetailQualified
	// stripDetailScoped adds the additional-Scope count.
	stripDetailScoped
	// stripDetailFull adds the actor and entity detail.
	stripDetailFull
)

// render returns the widest form of the entry that fits the width, emphasized
// by its prepared recency. Emphasis reinforces recency alone: the category
// stays encoded by glyph and text and the age by its own spelling, so nothing
// about the entry depends on color (RG-008).
func (e interestingEntry) render(tokens map[domain.ScopeIdentity]string, set charset, capability colorCapability, width int) string {
	style := recencyAt(e.age).style(capability)
	for _, register := range []categoryRegister{registerRich, registerCompact} {
		for _, detail := range []stripDetail{stripDetailFull, stripDetailScoped, stripDetailQualified, stripDetailCore} {
			if form := e.form(tokens, set, register, detail); fits(lipgloss.Width(form), width) {
				return style.Render(form)
			}
		}
	}
	return style.Render(e.minimal(tokens, set, width))
}

// form renders one rung of the entry ladder: the category in the given
// register, the optional actor and entity detail, the repository, the
// sponsoring Scope with its current-PR qualification, the additional confirmed
// Scopes, the qualified Scope count, and the textual age.
func (e interestingEntry) form(tokens map[domain.ScopeIdentity]string, set charset, register categoryRegister, detail stripDetail) string {
	segments := make([]string, 0, 8)
	segments = append(segments, categoryGlyph(e.category, set)+" "+categoryText(e.category, register))
	if detail >= stripDetailFull {
		if e.actor != "" {
			segments = append(segments, e.actor)
		}
		if entity := e.entityText(); entity != "" {
			segments = append(segments, entity)
		}
	}
	segments = append(segments, e.repository.String(), e.sponsorText(tokens, detail >= stripDetailQualified))
	if detail >= stripDetailScoped && e.additionalScopes() > 0 {
		segments = append(segments, fmt.Sprintf("%s%d scopes", hiddenMark, e.additionalScopes()))
	}
	if detail >= stripDetailQualified && e.qualifiedScopes() > 0 {
		segments = append(segments, fmt.Sprintf("%d current PR", e.qualifiedScopes()))
	}
	return strings.Join(append(segments, e.ageText()), separator)
}

// minimal renders RG-007's final logical form, `glyph repository scope age`.
// The glyph and the age are fixed cells at every width, because RG-012 may
// shorten this form's two labels but never substitute the facts around them;
// the repository and the sponsoring Scope therefore share whatever the width
// leaves, the repository first as the higher-priority label. The sponsoring
// Scope appears as its prepared presentation token here, which is the same
// token Stream and Rain name it by.
//
// A width too tight even for the fixed cells is below the dimensions FR-011
// closes, where the shortened labels fall away and the line stays deterministic
// best effort rather than becoming something other than this form.
func (e interestingEntry) minimal(tokens map[domain.ScopeIdentity]string, set charset, width int) string {
	glyph, age := categoryGlyph(e.category, set), e.ageText()
	repository, scope := e.repository.String(), preparedScopeToken(e.sponsor, tokens)
	if width >= 0 {
		// The three single-cell gaps between the four facts are spent before
		// either label is measured, so a fitting form is never one cell wide.
		budget := width - lipgloss.Width(glyph) - lipgloss.Width(age) - 3
		repository = shorten(repository, max(budget-1, 0))
		scope = shorten(scope, max(budget-lipgloss.Width(repository), 0))
	}
	return strings.Join(nonEmpty(glyph, repository, scope, age), " ")
}

// nonEmpty returns the parts a width left something of, so a label the tightest
// form could not hold at all costs no gap of its own.
func nonEmpty(parts ...string) []string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return kept
}

// entityText names the entity the event refers to, or nothing when the
// normalized event carries no reference. Missing optional detail is omitted
// rather than inferred (RG-007).
func (e interestingEntry) entityText() string {
	if e.entityRef == "" {
		return ""
	}
	return string(e.entityKind) + " " + e.entityRef
}

// sponsorText names the sponsoring Scope, with its own current-PR
// qualification when the form still carries qualification. Sponsorship explains
// selection and order alone and never narrows membership.
func (e interestingEntry) sponsorText(tokens map[domain.ScopeIdentity]string, qualified bool) string {
	label := scopeLabel(e.sponsor, tokens)
	if qualified && e.sponsorQualified() {
		return label + " current PR"
	}
	return label
}

// sponsorQualified reports whether the sponsoring Scope itself qualified
// through current-PR evidence.
func (e interestingEntry) sponsorQualified() bool {
	identity := e.sponsor.Identity()
	for _, scope := range e.scopes {
		if scope.scope.Identity() == identity {
			return scope.qualified
		}
	}
	return false
}

// preparedScopeToken returns the Scope's prepared presentation token, falling back to
// its own spelling for a Scope the publication prepared no token for.
func preparedScopeToken(scope domain.Scope, tokens map[domain.ScopeIdentity]string) string {
	if token, prepared := tokens[scope.Identity()]; prepared {
		return token
	}
	return scope.String()
}
