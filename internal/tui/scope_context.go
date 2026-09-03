package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// The non-color marks RG-012 gives one Stream row's Scope context. A member
// decided from current-PR evidence stays visibly qualified, and an undecided
// Scope stays visibly undecided, so neither is readable as a plain member.
const (
	currentPRQualifier = "~"
	unknownMark        = "?"
)

// The words naming the two groups of the complete context form. Members are
// what the snapshot confirmed; unknowns are investigatory context RG-004 keeps
// apart from them.
const (
	memberGroup  = "in "
	unknownGroup = "unresolved "
)

// groupGap separates the labels inside one context group, and hiddenMark leads
// every count of tokens a narrower form left out.
const (
	groupGap   = ", "
	hiddenMark = "+"
)

// scopeToken is one Scope's prepared presentation token inside a Stream row,
// already carrying its RG-012 mark.
type scopeToken struct {
	// text is the marked token as it renders.
	text string
	// qualified reports a member decided from complete current-PR evidence, so
	// the hidden-qualified count stays exact once the token is left out.
	qualified bool
}

// scopeContext is the prepared Scope context of one Stream row: the confirmed
// member tokens and the undecided ones, both in the stable Scope identity order
// the snapshot published them in. Not-member Scopes are omitted, and no
// membership is decided here.
type scopeContext struct {
	members  []scopeToken
	unknowns []scopeToken
}

// newScopeContext reads one event's prepared per-Scope outcomes into the two
// context groups. It consumes the published membership and performs no matching.
func newScopeContext(memberships []domain.ScopeMembership, tokens map[domain.ScopeIdentity]string) scopeContext {
	context := scopeContext{}
	for _, membership := range memberships {
		// A Scope with no prepared token is not part of the published selection
		// the tokens were computed from. Both come from the same atomic
		// publication, so this is unreachable; rendering an ordinal the rest of
		// the view does not share would be worse than omitting the Scope.
		token, prepared := tokens[membership.Scope.Identity()]
		if !prepared {
			continue
		}
		switch {
		case membership.Membership.IsMember():
			qualified := membership.Membership.QualifiedCurrentPR()
			if qualified {
				token += currentPRQualifier
			}
			context.members = append(context.members, scopeToken{text: token, qualified: qualified})
		case membership.Membership.IsUnknown():
			context.unknowns = append(context.unknowns, scopeToken{text: unknownMark + token})
		}
	}
	return context
}

// empty reports whether the row has no Scope context at all.
func (c scopeContext) empty() bool { return len(c.members)+len(c.unknowns) == 0 }

// render returns the most detailed RG-012 rung that fits the budget in rendered
// cells: the complete context, the intermediate form expanded as far as the
// budget allows, the two total forms, then the degraded marks. A negative budget
// is unbounded. Output is never sliced after formatting, so a rendered form is
// always a whole rung.
func (c scopeContext) render(budget int) string {
	if c.empty() {
		return ""
	}
	for _, rung := range c.rungs(budget) {
		if form := rung(); fits(lipgloss.Width(form), budget) {
			return form
		}
	}
	return ""
}

// rungs returns the ladder from the complete form down to the single hidden
// mark. Each rung is formed only when the ladder reaches it, so a row whose
// complete context already fits never grows the expansion the narrower rungs
// would need. The intermediate rung is the widest expansion that already fits,
// so the ladder is walked from the top without re-expanding.
func (c scopeContext) rungs(budget int) []func() string {
	return []func() string{
		c.complete,
		func() string { return c.expanded(budget) },
		c.totals,
		c.minimum,
		func() string { return hiddenCount(len(c.members) + len(c.unknowns)) },
		func() string { return hiddenMark },
	}
}

// complete renders every token of both groups behind the words naming them,
// omitting a group the row has no token for.
func (c scopeContext) complete() string {
	groups := make([]string, 0, 2)
	if len(c.members) > 0 {
		groups = append(groups, memberGroup+joinTokens(c.members))
	}
	if len(c.unknowns) > 0 {
		groups = append(groups, unknownGroup+joinTokens(c.unknowns))
	}
	return strings.Join(groups, separator)
}

// expanded returns the intermediate form grown as far as the budget allows. It
// starts with the first token of each nonempty group and then adds the next
// member and unknown token alternately, member first, keeping every candidate
// that still fits whole. A group whose next token does not fit does not stop the
// other group, and expansion ends once neither next token fits.
func (c scopeContext) expanded(budget int) string {
	shownMembers, shownUnknowns := min(1, len(c.members)), min(1, len(c.unknowns))
	for {
		grown := false
		if shownMembers < len(c.members) && fits(lipgloss.Width(c.intermediate(shownMembers+1, shownUnknowns)), budget) {
			shownMembers++
			grown = true
		}
		if shownUnknowns < len(c.unknowns) && fits(lipgloss.Width(c.intermediate(shownMembers, shownUnknowns+1)), budget) {
			shownUnknowns++
			grown = true
		}
		if !grown {
			return c.intermediate(shownMembers, shownUnknowns)
		}
	}
}

// intermediate renders the leading tokens of each group with the count of the
// tokens it left out, followed by the qualified members among them. The counts
// are recomputed from what is shown, so no hidden member or qualification is
// silently dropped.
func (c scopeContext) intermediate(shownMembers, shownUnknowns int) string {
	groups := make([]string, 0, 3)
	if len(c.members) > 0 {
		groups = append(groups, withHidden(c.members, shownMembers))
	}
	if len(c.unknowns) > 0 {
		groups = append(groups, withHidden(c.unknowns, shownUnknowns))
	}
	if qualified := qualifiedIn(c.members[shownMembers:]); qualified > 0 {
		groups = append(groups, hiddenCount(qualified)+" current PR")
	}
	return strings.Join(groups, separator)
}

// totals renders the counted form of both groups and the qualified members,
// omitting a count the row holds none of.
func (c scopeContext) totals() string {
	return c.counted(" member", " unknown", " current PR", separator)
}

// minimum renders the narrowest counted form, keeping the same three counts
// behind their marks rather than dropping one of them.
func (c scopeContext) minimum() string {
	return c.counted("m", unknownMark, currentPRQualifier, " ")
}

// counted renders the member, unknown, and qualified counts with the given
// suffixes, omitting the zero ones and joining what remains.
func (c scopeContext) counted(member, unknown, qualified, gap string) string {
	counts := make([]string, 0, 3)
	for _, counted := range []struct {
		count  int
		suffix string
	}{
		{count: len(c.members), suffix: member},
		{count: len(c.unknowns), suffix: unknown},
		{count: qualifiedIn(c.members), suffix: qualified},
	} {
		if counted.count > 0 {
			counts = append(counts, fmt.Sprintf("%d%s", counted.count, counted.suffix))
		}
	}
	return strings.Join(counts, gap)
}

// withHidden renders the leading shown tokens of one group and appends the count
// of the ones it left out.
func withHidden(tokens []scopeToken, shown int) string {
	rendered := joinTokens(tokens[:shown])
	if hidden := len(tokens) - shown; hidden > 0 {
		rendered += groupGap + hiddenCount(hidden)
	}
	return rendered
}

// hiddenCount renders how many tokens a narrower form left out, behind the mark
// that names the number as an omission rather than as membership of its own.
func hiddenCount(count int) string {
	return fmt.Sprintf("%s%d", hiddenMark, count)
}

// joinTokens renders the tokens in their prepared order.
func joinTokens(tokens []scopeToken) string {
	texts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		texts = append(texts, token.text)
	}
	return strings.Join(texts, groupGap)
}

// qualifiedIn counts the current-PR qualified members among the tokens.
func qualifiedIn(tokens []scopeToken) int {
	qualified := 0
	for _, token := range tokens {
		if token.qualified {
			qualified++
		}
	}
	return qualified
}
