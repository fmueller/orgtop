package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// contextOf builds one prepared Stream row context from token spellings. A
// member token carrying the current-PR qualifier is recorded as qualified, so
// the fixtures read the way the rendered row does.
func contextOf(members []string, unknowns []string) scopeContext {
	built := scopeContext{}
	for _, member := range members {
		built.members = append(built.members, scopeToken{
			text:      member,
			qualified: strings.HasSuffix(member, currentPRQualifier),
		})
	}
	for _, unknown := range unknowns {
		built.unknowns = append(built.unknowns, scopeToken{text: unknownMark + unknown})
	}
	return built
}

// TestScopeContextRendersTheMostDetailedFittingRung guards RG-012's Stream
// overlap ladder: the complete form, the expanding intermediate form, the
// minimum totals, and the degraded marks each appear at the budget the contract
// gives them, and no rendered form outruns its budget.
func TestScopeContextRendersTheMostDetailedFittingRung(t *testing.T) {
	context := contextOf([]string{"R1", "P2~", "P4~", "R6"}, []string{"P3", "P5"})

	cases := []struct {
		name   string
		budget int
		want   string
	}{
		{name: "complete", budget: unbounded, want: "in R1, P2~, P4~, R6 · unresolved ?P3, ?P5"},
		{name: "intermediate expanded", budget: 35, want: "R1, +3 · ?P3, ?P5 · +2 current PR"},
		{name: "intermediate minimal", budget: 32, want: "R1, +3 · ?P3, +1 · +2 current PR"},
		{name: "minimum totals", budget: 31, want: "4m 2? 2~"},
		{name: "hidden total", budget: 3, want: "+6"},
		{name: "single cell", budget: 1, want: "+"},
		{name: "nothing", budget: 0, want: ""},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := context.render(testCase.budget)
			if got != testCase.want {
				t.Errorf("scope context at budget %d is %q, want %q", testCase.budget, got, testCase.want)
			}
			if !fits(lipgloss.Width(got), testCase.budget) {
				t.Errorf("scope context %q is %d cells wide, want at most %d", got, lipgloss.Width(got), testCase.budget)
			}
		})
	}
}

// largeContext builds the context of an event matching many Scopes, where the
// spelled-out counts are narrower than the token form they replace. It is the
// only shape that reaches the counted rung, because a handful of Scopes always
// spells its tokens more cheaply than the words counting them.
func largeContext(t *testing.T) scopeContext {
	t.Helper()
	const scopes = 500
	members, unknowns := make([]string, 0, scopes), make([]string, 0, scopes)
	for index := range scopes {
		members = append(members, fmt.Sprintf("P%03d~", scopes-index))
		unknowns = append(unknowns, fmt.Sprintf("P%03d", 999-index))
	}
	return contextOf(members, unknowns)
}

// TestScopeContextCountsWhatTheIntermediateFormCannotSpell guards RG-012's
// counted rung: once the token form outruns the budget, the row still states how
// many Scopes confirmed it, how many stayed undecided, and how many members are
// current-PR qualified, in the contract's order.
func TestScopeContextCountsWhatTheIntermediateFormCannotSpell(t *testing.T) {
	context := largeContext(t)

	cases := []struct {
		name   string
		budget int
		want   string
	}{
		{name: "counted", budget: 42, want: "500 member · 500 unknown · 500 current PR"},
		{name: "minimum", budget: 40, want: "500m 500? 500~"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := context.render(testCase.budget)
			if got != testCase.want {
				t.Errorf("scope context at budget %d is %q, want %q", testCase.budget, got, testCase.want)
			}
			if !fits(lipgloss.Width(got), testCase.budget) {
				t.Errorf("scope context %q is %d cells wide, want at most %d", got, lipgloss.Width(got), testCase.budget)
			}
		})
	}
}

// TestScopeContextOmitsAnEmptyGroup guards RG-012: a row with only members or
// only unknowns renders one group rather than an empty second one.
func TestScopeContextOmitsAnEmptyGroup(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{name: "members only", got: contextOf([]string{"R1", "P2"}, nil).render(unbounded), want: "in R1, P2"},
		{name: "unknowns only", got: contextOf(nil, []string{"P2"}).render(unbounded), want: "unresolved ?P2"},
		{name: "no scope at all", got: contextOf(nil, nil).render(unbounded), want: ""},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Errorf("scope context is %q, want %q", testCase.got, testCase.want)
			}
		})
	}
}
