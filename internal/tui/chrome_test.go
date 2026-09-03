package tui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
