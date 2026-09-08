package tui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fmueller/orgtop/internal/domain"
)

// expanded returns the published selection of an organization expansion that
// admitted three of its repositories beside one exact Scope.
func expandedState(t *testing.T, omitted int, more bool) State {
	t.Helper()
	scopes := testScope(t, "acme/backend", "acme/frontend", "acme/infra", "other/exact")
	return State{
		Scopes:    scopes,
		Freshness: FreshnessCurrent,
		Selection: Selection{
			Scopes:               scopes,
			ExactScopes:          1,
			ExpandedScopes:       3,
			TotalScopes:          4,
			DistinctRepositories: 4,
			Selectors:            []SelectorSelection{{Organization: "acme", Omitted: omitted, HasMore: more}},
			PaginationRemains:    more,
		},
	}
}

func TestSelectionFormsDiscloseOmissionAndRemainingPages(t *testing.T) {
	tests := map[string]struct {
		omitted     int
		more        bool
		wantFull    string
		wantCompact string
		wantMinimum string
	}{
		"complete": {
			wantFull:    "selection: 4 repos · 4 scopes · 1 exact · 3 expanded",
			wantCompact: "sel 4 repos · 4 scopes",
			wantMinimum: "",
		},
		"exact omission": {
			omitted:     480,
			wantFull:    "selection: 4 repos · 4 scopes · 1 exact · 3 expanded · 480 eligible omitted",
			wantCompact: "sel 4 repos · 4 scopes · 480 omitted",
			wantMinimum: "SEL 480",
		},
		"exact and unknown omission": {
			omitted:     480,
			more:        true,
			wantFull:    "selection: 4 repos · 4 scopes · 1 exact · 3 expanded · 480 eligible omitted · more eligible may be omitted",
			wantCompact: "sel 4 repos · 4 scopes · 480 omitted · more?",
			wantMinimum: "SEL 480+?",
		},
		"unknown omission only": {
			more:        true,
			wantFull:    "selection: 4 repos · 4 scopes · 1 exact · 3 expanded · more eligible may be omitted",
			wantCompact: "sel 4 repos · 4 scopes · more?",
			wantMinimum: "SEL ?",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			forms := selectionForms(expandedState(t, test.omitted, test.more).Selection)

			want := []string{test.wantFull, test.wantCompact}
			if test.wantMinimum != "" {
				want = append(want, test.wantMinimum)
			}
			if len(forms) != len(want) {
				t.Fatalf("selection forms are %q, want %q", forms, want)
			}
			for index, value := range want {
				if forms[index] != value {
					t.Errorf("selection form %d is %q, want %q", index, forms[index], value)
				}
			}
		})
	}
}

func TestSelectionFormsAreEmptyWithoutAnOrganizationSelector(t *testing.T) {
	state := State{Scopes: testScope(t, "acme/backend"), Selection: exactSelection(testScope(t, "acme/backend"))}

	if forms := selectionForms(state.Selection); len(forms) != 0 {
		t.Errorf("an exact selection reports the selection forms %q, want none", forms)
	}
}

func TestHeaderDisclosesTheSelectionAtEveryWidthItFits(t *testing.T) {
	state := expandedState(t, 480, true)
	forms := selectionForms(state.Selection)
	if len(forms) != 3 {
		t.Fatalf("the disclosed selection has %d forms, want the full, compact, and minimum ones", len(forms))
	}

	seen := make(map[string]bool, len(forms))
	for width := 1; width <= 200; width++ {
		header := renderHeader(state, ModeOverview, width)
		if lipgloss.Width(header) > width {
			t.Fatalf("the header is %d cells wide at width %d:\n%s", lipgloss.Width(header), width, header)
		}

		rendered := 0
		for _, form := range forms {
			if strings.Contains(header, form) {
				rendered++
				seen[form] = true
			}
		}
		// The compact form is a prefix of nothing else, and the minimum form
		// shares no text with either, so a header discloses one form at most.
		if rendered > 1 {
			t.Errorf("the header discloses %d selection forms at width %d:\n%s", rendered, width, header)
		}
	}

	for _, form := range forms {
		if !seen[form] {
			t.Errorf("no width disclosed the selection form %q", form)
		}
	}
}

func TestHeaderNeverDisclosesASelectionWithoutAnOrganizationSelector(t *testing.T) {
	scopes := testScope(t, "acme/backend")
	state := State{Scopes: scopes, Freshness: FreshnessCurrent, Selection: exactSelection(scopes)}

	for width := 1; width <= 200; width++ {
		header := renderHeader(state, ModeOverview, width)
		for _, disclosure := range []string{"selection:", "sel ", "SEL "} {
			if strings.Contains(header, disclosure) {
				t.Fatalf("the exact-selection header contains %q at width %d:\n%s", disclosure, width, header)
			}
		}
	}
}

// headerFields splits a rendered header into its separated segments, with the
// styling removed so a segment is compared as the text it states.
func headerFields(header string) []string {
	fields := strings.Split(header, separator)
	for index, value := range fields {
		fields[index] = strings.TrimSpace(ansi.Strip(value))
	}
	return fields
}

func TestHeaderMarksAStaleSelectionBesideThePrimaryState(t *testing.T) {
	state := expandedState(t, 0, false)
	state.Freshness = FreshnessStale
	state.Cause = "refreshing acme/backend: github request failed"
	state.SelectionFreshness = SelectionStale
	state.SelectionCause = "expanding acme: github rate limit reached"

	fields := headerFields(renderHeader(state, ModeOverview, unbounded))

	// The selection marker stands beside the primary one, never instead of it,
	// so the header states both as their own segments.
	for _, want := range []string{transportLabel, "STALE", "SELECTION STALE", state.Cause, state.SelectionCause} {
		if !slices.Contains(fields, want) {
			t.Errorf("the stale-selection header states %q, want a %q segment", fields, want)
		}
	}
	if index := slices.Index(fields, "STALE"); index >= 0 && index > slices.Index(fields, "SELECTION STALE") {
		t.Errorf("the header states %q, want the primary state before the selection marker", fields)
	}

	current := expandedState(t, 0, false)
	current.Freshness = FreshnessStale
	current.Cause = state.Cause
	currentFields := headerFields(renderHeader(current, ModeOverview, unbounded))
	if slices.Contains(currentFields, "SELECTION STALE") {
		t.Errorf("a current selection is marked stale: %q", currentFields)
	}
	if !slices.Contains(currentFields, "STALE") {
		t.Errorf("the header states %q, want the primary stale state retained on its own", currentFields)
	}
}

// mixedScopes is the selection that motivated T-087: two repositories, one of
// them carrying two path Scopes of its own, so the Scope count and the
// repository count differ.
func mixedScopes(t *testing.T) domain.ScopeSet {
	t.Helper()
	return scopeSet(t,
		domain.NewRepositoryScope(testRepository(t, "acme/backend")),
		pathScope(t, "acme/backend", "api"),
		pathScope(t, "acme/backend", "cmd"),
		domain.NewRepositoryScope(testRepository(t, "acme/frontend")),
	)
}

// TestScopeContextFormsCountRepositoriesAndScopesSeparately pins RG-012's
// Scope-context ladder. The segment reports distinct repositories and Scopes as
// their own counts, so a mixed selection never spells its Scope count as a
// repository count.
func TestScopeContextFormsCountRepositoriesAndScopesSeparately(t *testing.T) {
	tests := map[string]struct {
		scopes domain.ScopeSet
		want   []string
	}{
		"repository only": {
			scopes: testScope(t, "acme/backend", "acme/frontend"),
			want:   []string{"2 repos · 2 scopes", "2 scopes", "2S"},
		},
		"one repository": {
			scopes: testScope(t, "acme/backend"),
			want:   []string{"1 repos · 1 scopes", "1 scopes", "1S"},
		},
		"mixed repository and path": {
			scopes: mixedScopes(t),
			want:   []string{"2 repos · 4 scopes", "4 scopes", "4S"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			forms := scopeContextForms(test.scopes)
			if !slices.Equal(forms, test.want) {
				t.Errorf("scope context forms are %q, want %q", forms, test.want)
			}
		})
	}
}

// TestScopeContextFormsAreEmptyWithoutASelection keeps an unselected State from
// claiming header space with a zero-count summary.
func TestScopeContextFormsAreEmptyWithoutASelection(t *testing.T) {
	if forms := scopeContextForms(domain.ScopeSet{}); len(forms) != 0 {
		t.Errorf("an empty selection reports the scope context forms %q, want none", forms)
	}
}

// TestHeaderRendersEveryScopeContextRung walks the widths of a mixed selection
// and pins that every rung of the ladder is reachable, that no rung outruns its
// width, and that no width labels the three Scopes as three repositories.
func TestHeaderRendersEveryScopeContextRung(t *testing.T) {
	scopes := mixedScopes(t)
	state := State{Scopes: scopes, Freshness: FreshnessCurrent, Selection: exactSelection(scopes)}
	forms := scopeContextForms(scopes)

	seen := make(map[string]bool, len(forms))
	for width := 1; width <= 200; width++ {
		header := ansi.Strip(renderHeader(state, ModeOverview, width))
		if lipgloss.Width(header) > width {
			t.Fatalf("the header is %d cells wide at width %d:\n%s", lipgloss.Width(header), width, header)
		}
		if strings.Contains(header, "repositories") || strings.Contains(header, "4 repos") {
			t.Fatalf("the header labels the Scope count as a repository count at width %d:\n%s", width, header)
		}
		// The ladder is checked from full to minimum, because every shorter
		// rung is contained in the one above it.
		for _, form := range forms {
			if strings.Contains(header, form) {
				seen[form] = true
				break
			}
		}
	}

	for _, form := range forms {
		if !seen[form] {
			t.Errorf("no width between 1 and 200 rendered the scope context rung %q", form)
		}
	}
}

// TestHeaderDropsTheCauseBeforeTheScopeContext pins RG-012's closed header
// priority order: Scope context outranks the stale last-success and the concise
// sanitized error, and "if no form of a segment fits, that segment and all
// lower-priority segments are omitted". A tightening header therefore has to
// exhaust the Scope-context rung ladder before it keeps the cause text, so no
// width may render the cause without the context segment it outranks.
func TestHeaderDropsTheCauseBeforeTheScopeContext(t *testing.T) {
	const cause = "github: request failed after three attempts"
	scopes := mixedScopes(t)

	tests := map[string]struct {
		state State
		// context is the ladder the cause must never outlive.
		context []string
		// narrowest is the first width whose header holds the cause, and
		// below/at are the exact headers rendered one cell under it and on it.
		// Pinning the boundary keeps the sweep from passing on a richer form
		// that happens to survive at some wider width.
		narrowest int
		below, at string
	}{
		"error keeps the scope summary": {
			state:     State{Scopes: scopes, Freshness: FreshnessError, Cause: cause},
			context:   append([]string{scopeList(scopes)}, scopeContextForms(scopes)...),
			narrowest: 77,
			below:     "OVERVIEW · POLLING · ERROR · 2 repos · 4 scopes",
			at:        "OVERVIEW · POLLING · ERROR · " + cause + " · 4S",
		},
		"stale keeps the last success": {
			state: State{
				Scopes:      scopes,
				Freshness:   FreshnessStale,
				Cause:       cause,
				LastSuccess: fixedInstant,
			},
			context:   []string{scopeList(scopes), "updated " + fixedInstant.Format(clockLayout)},
			narrowest: 91,
			below:     "OVERVIEW · POLLING · STALE · updated 12:00:00",
			at:        "OVERVIEW · POLLING · STALE · " + cause + " · updated 12:00:00",
		},
		"a selection cause yields the same way": {
			state:     State{Scopes: scopes, Freshness: FreshnessError, SelectionCause: cause},
			context:   append([]string{scopeList(scopes)}, scopeContextForms(scopes)...),
			narrowest: 77,
			below:     "OVERVIEW · POLLING · ERROR · 2 repos · 4 scopes",
			at:        "OVERVIEW · POLLING · ERROR · " + cause + " · 4S",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			narrowest := 0
			for width := 1; width <= 200; width++ {
				header := ansi.Strip(renderHeader(test.state, ModeOverview, width))
				if rendered := lipgloss.Width(header); rendered > width {
					t.Fatalf("the header is %d cells wide at width %d:\n%s", rendered, width, header)
				}
				if !strings.Contains(header, cause) {
					continue
				}
				if narrowest == 0 {
					narrowest = width
				}
				if !slices.ContainsFunc(test.context, func(form string) bool {
					return strings.Contains(header, form)
				}) {
					t.Errorf("width %d renders the cause without any of the higher-priority context forms %q:\n%s",
						width, test.context, header)
				}
			}

			if narrowest != test.narrowest {
				t.Errorf("the cause first fits at width %d, want %d", narrowest, test.narrowest)
			}
			// One cell under the boundary the header spends its cells on the
			// higher-priority context segment instead of the cause, and on the
			// boundary the cause is only admitted beside a surviving rung.
			if got := ansi.Strip(renderHeader(test.state, ModeOverview, test.narrowest-1)); got != test.below {
				t.Errorf("the header at width %d is %q, want %q", test.narrowest-1, got, test.below)
			}
			if got := ansi.Strip(renderHeader(test.state, ModeOverview, test.narrowest)); got != test.at {
				t.Errorf("the header at width %d is %q, want %q", test.narrowest, got, test.at)
			}
		})
	}
}
