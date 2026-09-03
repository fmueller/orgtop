package tui

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// attempt is one scripted expansion outcome.
type attempt struct {
	expansion Expansion
	err       error
}

// fakeExpander is the injected organization expansion under test. Like the
// activity source it is deterministic: an attempt runs only when the lifecycle
// command that owns it is executed.
type fakeExpander struct {
	// attempts are returned in order; the last one repeats.
	attempts []attempt
	// calls counts started expansions.
	calls int
	// contexts records the context of every started expansion.
	contexts []context.Context
}

func (e *fakeExpander) Expand(ctx context.Context) (Expansion, error) {
	e.calls++
	e.contexts = append(e.contexts, ctx)
	if len(e.attempts) == 0 {
		return Expansion{}, nil
	}
	next := e.attempts[min(e.calls-1, len(e.attempts)-1)]
	return next.expansion, next.err
}

// expandedSelection returns a scripted successful expansion of the named
// repositories, with the provenance and per-selector disclosure one selector
// would have produced.
func expandedSelection(t *testing.T, organization string, values ...string) Expansion {
	t.Helper()
	scopes := testScope(t, values...)
	provenance := make([]ScopeProvenance, 0, len(values))
	for _, scope := range scopes.Scopes() {
		provenance = append(provenance, ScopeProvenance{Scope: scope, Selector: organization})
	}
	return Expansion{Selection: Selection{
		Scopes:               scopes,
		ExpandedScopes:       scopes.Len(),
		TotalScopes:          scopes.Len(),
		DistinctRepositories: len(scopes.Repositories()),
		Selectors: []SelectorSelection{{
			Organization: organization,
			Listed:       len(values),
			Eligible:     len(values),
			Retained:     len(values),
		}},
		Provenance: provenance,
	}}
}

// emptyExpansion returns a scripted successful expansion that found no eligible
// repository.
func emptyExpansion(organization string) Expansion {
	return Expansion{Selection: Selection{
		Selectors: []SelectorSelection{{Organization: organization}},
	}}
}

// expanding builds a lifecycle model whose selection comes from an expander and
// whose clock and timer are deterministic. The exact selection may be empty,
// which is the organization-only invocation.
func expanding(t *testing.T, source Source, expander Expander, at time.Time, timer *recorder, exact ...string) Model {
	t.Helper()
	scopes := domain.ScopeSet{}
	if len(exact) > 0 {
		scopes = testScope(t, exact...)
	}
	model, err := New(context.Background(), scopes, source, WithExpander(expander))
	if err != nil {
		t.Fatalf("building the expanding lifecycle model failed: %v", err)
	}
	model.now = func() time.Time { return at }
	model.tick = timer.tick
	return model
}

// at replaces the model clock, which is how a test advances time between two
// refreshes.
func (m Model) at(instant time.Time) Model {
	m.now = func() time.Time { return instant }
	return m
}

// names returns the repository names of a scope in scope order.
func names(scopes domain.ScopeSet) []string {
	values := make([]string, 0, scopes.Len())
	for _, repository := range scopes.Repositories() {
		values = append(values, repository.String())
	}
	return values
}

// assertScopes fails when the scope does not name exactly the wanted
// repositories in order.
func assertScopes(t *testing.T, label string, scopes domain.ScopeSet, want ...string) {
	t.Helper()
	got := names(scopes)
	if len(got) != len(want) {
		t.Fatalf("%s holds %v, want %v", label, got, want)
	}
	for index, value := range want {
		if got[index] != value {
			t.Fatalf("%s holds %v, want %v", label, got, want)
		}
	}
}

func TestOrganizationOnlyStartupExpandsBeforeItPollsAnything(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend", "acme/frontend")}}}
	expander := &fakeExpander{attempts: []attempt{{expansion: expandedSelection(t, "acme", "acme/backend", "acme/frontend")}}}
	model := expanding(t, source, expander, fixedInstant, &recorder{})

	cmd := model.Init()
	if model.state.Freshness != FreshnessLoading {
		t.Errorf("freshness is %v before the first expansion, want FreshnessLoading", model.state.Freshness)
	}
	if expander.calls != 0 || source.calls != 0 {
		t.Errorf("Init itself ran %d expansions and %d polls, want the command to own both", expander.calls, source.calls)
	}

	model, _ = run(t, model, cmd)

	if expander.calls != 1 {
		t.Errorf("the first refresh ran %d expansions, want 1", expander.calls)
	}
	if source.calls != 1 {
		t.Fatalf("the first refresh ran %d polls, want 1", source.calls)
	}
	assertScopes(t, "the polled selection", source.scopes[0], "acme/backend", "acme/frontend")
	if model.state.Freshness != FreshnessCurrent {
		t.Errorf("freshness is %v after expansion and polling succeeded, want FreshnessCurrent", model.state.Freshness)
	}
	assertScopes(t, "the published selection", model.state.Scopes, "acme/backend", "acme/frontend")
	if model.state.Selection.ExpandedScopes != 2 {
		t.Errorf("the published selection reports %d expanded scopes, want 2", model.state.Selection.ExpandedScopes)
	}
}

func TestSuccessfulEmptyExpansionIsACurrentEmptySelectionThatPollsNothing(t *testing.T) {
	source := &fakeSource{}
	expander := &fakeExpander{attempts: []attempt{{expansion: emptyExpansion("acme")}}}
	model := expanding(t, source, expander, fixedInstant, &recorder{})

	model, _ = run(t, model, model.Init())

	if source.calls != 0 {
		t.Errorf("an empty selection ran %d polls, want none", source.calls)
	}
	if model.state.Freshness != FreshnessCurrent {
		t.Errorf("freshness is %v after an empty expansion, want FreshnessCurrent", model.state.Freshness)
	}
	if !model.state.LastSuccess.Equal(fixedInstant) {
		t.Errorf("last success is %v after an empty expansion, want %v", model.state.LastSuccess, fixedInstant)
	}
	if got := len(model.state.Snapshot.Events()); got != 0 {
		t.Errorf("the published snapshot holds %d events, want 0", got)
	}
	if got := len(model.state.Selection.Selectors); got != 1 {
		t.Errorf("the published selection reports %d selectors, want the empty selector context retained", got)
	}
	if model.state.SelectionFreshness != SelectionCurrent {
		t.Errorf("selection freshness is %v after a successful empty expansion, want SelectionCurrent", model.state.SelectionFreshness)
	}
}

func TestInitialExpansionFailureIsErrorAndPollsNoSubset(t *testing.T) {
	source := &fakeSource{}
	expander := &fakeExpander{attempts: []attempt{{err: errors.New("expanding acme: organization not found or inaccessible")}}}
	model := expanding(t, source, expander, fixedInstant, &recorder{}, "other/exact")

	model, _ = run(t, model, model.Init())

	if source.calls != 0 {
		t.Errorf("a failed initial expansion ran %d polls, want none of the exact or expanded subset", source.calls)
	}
	if model.state.Freshness != FreshnessError {
		t.Errorf("freshness is %v after a failed initial expansion, want FreshnessError", model.state.Freshness)
	}
	if model.state.Cause == "" {
		t.Error("a failed initial expansion reports no cause")
	}
	if model.state.SelectionFreshness != SelectionCurrent {
		t.Errorf("selection freshness is %v with no previous selection, want SelectionCurrent", model.state.SelectionFreshness)
	}
	if model.state.Selection.TotalScopes != 0 {
		t.Errorf("a failed initial expansion published %d scopes, want none", model.state.Selection.TotalScopes)
	}
}

func TestReexpansionBeforeItIsDueReusesTheFixedSelection(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	expander := &fakeExpander{attempts: []attempt{{expansion: expandedSelection(t, "acme", "acme/backend")}}}
	model := expanding(t, source, expander, fixedInstant, &recorder{})

	model, _ = run(t, model, model.Init())
	model = model.at(fixedInstant.Add(time.Minute))
	_, cmd := apply(t, model, refreshDueMsg{})
	run(t, model, cmd)

	if expander.calls != 1 {
		t.Errorf("a refresh before the expansion is due ran %d expansions, want the fixed selection reused", expander.calls)
	}
	if source.calls != 2 {
		t.Fatalf("the second refresh ran %d polls in total, want 2", source.calls)
	}
	assertScopes(t, "the reused selection", source.scopes[1], "acme/backend")
}

func TestFailedReexpansionPollsTheLastSuccessfulSelectionAndMarksItStale(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	expander := &fakeExpander{attempts: []attempt{
		{expansion: expandedSelection(t, "acme", "acme/backend")},
		{err: errors.New("expanding acme: github request failed")},
	}}
	model := expanding(t, source, expander, fixedInstant, &recorder{})

	model, _ = run(t, model, model.Init())
	model = model.at(fixedInstant.Add(16 * time.Minute))
	model, cmd := apply(t, model, refreshDueMsg{})
	model, _ = run(t, model, cmd)

	if expander.calls != 2 {
		t.Errorf("the due refresh ran %d expansions, want 2", expander.calls)
	}
	if source.calls != 2 {
		t.Fatalf("a non-rate-limited expansion failure ran %d polls in total, want the previous selection polled", source.calls)
	}
	assertScopes(t, "the polled selection", source.scopes[1], "acme/backend")
	if model.state.Freshness != FreshnessCurrent {
		t.Errorf("freshness is %v after polling the previous selection succeeded, want FreshnessCurrent", model.state.Freshness)
	}
	if model.state.SelectionFreshness != SelectionStale {
		t.Errorf("selection freshness is %v after a failed re-expansion, want SelectionStale", model.state.SelectionFreshness)
	}
	if model.state.SelectionCause == "" {
		t.Error("a failed re-expansion reports no selection cause")
	}
	assertScopes(t, "the retained selection", model.state.Scopes, "acme/backend")
}

func TestRateLimitedReexpansionDispatchesNoPollAndNarrowsNothing(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	expander := &fakeExpander{attempts: []attempt{
		{expansion: expandedSelection(t, "acme", "acme/backend")},
		{expansion: Expansion{RetryDelay: 5 * time.Minute, RateLimited: true}, err: errors.New("expanding acme: github rate limit reached")},
	}}
	timer := &recorder{}
	model := expanding(t, source, expander, fixedInstant, timer)

	model, _ = run(t, model, model.Init())
	model = model.at(fixedInstant.Add(16 * time.Minute))
	model, cmd := apply(t, model, refreshDueMsg{})
	model, _ = run(t, model, cmd)

	if source.calls != 1 {
		t.Errorf("a rate-limited expansion ran %d polls in total, want no further dispatch", source.calls)
	}
	if model.state.Freshness != FreshnessStale {
		t.Errorf("freshness is %v after a rate-limited re-expansion, want FreshnessStale", model.state.Freshness)
	}
	if model.state.SelectionFreshness != SelectionStale {
		t.Errorf("selection freshness is %v after a rate-limited re-expansion, want SelectionStale", model.state.SelectionFreshness)
	}
	assertScopes(t, "the retained selection", model.state.Scopes, "acme/backend")
	if got := timer.delays[len(timer.delays)-1]; got < 5*time.Minute {
		t.Errorf("the next attempt is scheduled in %v, want at least the instructed 5m0s", got)
	}
}

func TestSuccessfulReexpansionReplacesTheSelectionAndClearsItsCause(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	expander := &fakeExpander{attempts: []attempt{
		{expansion: expandedSelection(t, "acme", "acme/backend")},
		{err: errors.New("expanding acme: github request failed")},
		{expansion: expandedSelection(t, "acme", "acme/backend", "acme/frontend")},
	}}
	model := expanding(t, source, expander, fixedInstant, &recorder{})

	model, _ = run(t, model, model.Init())
	model = model.at(fixedInstant.Add(16 * time.Minute))
	model, cmd := apply(t, model, refreshDueMsg{})
	model, _ = run(t, model, cmd)
	model = model.at(fixedInstant.Add(40 * time.Minute))
	model, cmd = apply(t, model, refreshDueMsg{})
	model, _ = run(t, model, cmd)

	if model.state.SelectionFreshness != SelectionCurrent {
		t.Errorf("selection freshness is %v after a successful re-expansion, want SelectionCurrent", model.state.SelectionFreshness)
	}
	if model.state.SelectionCause != "" {
		t.Errorf("selection cause is %q after recovery, want it cleared", model.state.SelectionCause)
	}
	assertScopes(t, "the replaced selection", model.state.Scopes, "acme/backend", "acme/frontend")
	assertScopes(t, "the polled selection", source.scopes[len(source.scopes)-1], "acme/backend", "acme/frontend")
}

func TestExpansionSuccessWithFailedPollingKeepsTheNewSelectionHidden(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{
		{err: errors.New("refreshing acme/backend: github request failed")},
		{result: activity(t, "acme/backend")},
	}}
	expander := &fakeExpander{attempts: []attempt{{expansion: expandedSelection(t, "acme", "acme/backend")}}}
	model := expanding(t, source, expander, fixedInstant, &recorder{})

	model, _ = run(t, model, model.Init())

	if model.state.Freshness != FreshnessError {
		t.Errorf("freshness is %v after an initial poll failure, want FreshnessError", model.state.Freshness)
	}
	if model.state.Scopes.Len() != 0 {
		t.Errorf("the failed refresh published %v, want the expanded selection to stay hidden", names(model.state.Scopes))
	}

	model = model.at(fixedInstant.Add(time.Minute))
	model, cmd := apply(t, model, refreshDueMsg{})
	model, _ = run(t, model, cmd)

	if expander.calls != 1 {
		t.Errorf("the next refresh ran %d expansions, want the internally retained selection reused", expander.calls)
	}
	if source.calls != 2 {
		t.Fatalf("the next refresh ran %d polls in total, want 2", source.calls)
	}
	assertScopes(t, "the polled selection", source.scopes[1], "acme/backend")
	if model.state.Freshness != FreshnessCurrent {
		t.Errorf("freshness is %v after the matching poll succeeded, want FreshnessCurrent", model.state.Freshness)
	}
	assertScopes(t, "the published selection", model.state.Scopes, "acme/backend")
}

func TestEveryPollOfOneRefreshUsesTheSelectionThatRefreshExpanded(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	expander := &fakeExpander{attempts: []attempt{
		{expansion: expandedSelection(t, "acme", "acme/backend")},
		{expansion: expandedSelection(t, "acme", "acme/frontend")},
	}}
	model := expanding(t, source, expander, fixedInstant, &recorder{})

	model, _ = run(t, model, model.Init())

	assertScopes(t, "the polled selection", source.scopes[0], "acme/backend")
	assertScopes(t, "the published selection", model.state.Scopes, "acme/backend")
	aggregates := model.state.Snapshot.Aggregates()
	if len(aggregates) != 1 || aggregates[0].Repository.String() != "acme/backend" {
		t.Errorf("the published snapshot aggregates %v, want it built from the selection that refresh expanded", aggregates)
	}
}

func TestCancellationBeforePublicationDiscardsTheExpandedSelection(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	expander := &fakeExpander{attempts: []attempt{{expansion: expandedSelection(t, "acme", "acme/backend")}}}
	model := expanding(t, source, expander, fixedInstant, &recorder{})

	cmd := model.Init()
	message := cmd()
	model, _ = apply(t, model, tea.KeyPressMsg{Code: 'q', Text: "q"})
	model, next := apply(t, model, message)

	if model.state.Freshness != FreshnessLoading {
		t.Errorf("freshness is %v after cancellation, want the loading state left intact", model.state.Freshness)
	}
	if model.state.Scopes.Len() != 0 {
		t.Errorf("cancellation published %v, want neither a partial selection nor an empty fallback", names(model.state.Scopes))
	}
	if next != nil {
		t.Error("cancellation scheduled a further attempt, want the discarded attempt to schedule nothing")
	}
	if !model.expansionDue() {
		t.Error("expansion is not due after cancellation, want the discarded selection to leave it immediately due")
	}
}

func TestPublishedSelectionRetainsTruncationDisclosureAndProvenance(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	expansion := expandedSelection(t, "acme", "acme/backend")
	expansion.Selection.PaginationRemains = true
	expansion.Selection.Selectors[0].Listed = 120
	expansion.Selection.Selectors[0].Eligible = 90
	expansion.Selection.Selectors[0].Archived = 30
	expansion.Selection.Selectors[0].Retained = 1
	expansion.Selection.Selectors[0].Omitted = 89
	expansion.Selection.Selectors[0].HasMore = true
	expander := &fakeExpander{attempts: []attempt{{expansion: expansion}}}
	model := expanding(t, source, expander, fixedInstant, &recorder{})

	model, _ = run(t, model, model.Init())

	published := model.state.Selection
	if !published.PaginationRemains {
		t.Error("the published selection dropped the remaining-pagination disclosure")
	}
	if got := len(published.Selectors); got != 1 {
		t.Fatalf("the published selection reports %d selectors, want 1", got)
	}
	selector := published.Selectors[0]
	if selector.Listed != 120 || selector.Eligible != 90 || selector.Archived != 30 || selector.Retained != 1 || selector.Omitted != 89 || !selector.HasMore {
		t.Errorf("the published selector disclosure is %+v, want the expansion's counts retained", selector)
	}
	if got := len(published.Provenance); got != 1 {
		t.Fatalf("the published selection reports %d provenance entries, want one per scope", got)
	}
	if published.Provenance[0].Selector != "acme" {
		t.Errorf("the published provenance names %q, want the contributing selector acme", published.Provenance[0].Selector)
	}
}

func TestExpandedAndExactScopesPublishIdenticalDownstreamState(t *testing.T) {
	exactSource := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend", "acme/frontend")}}}
	exact := lifecycle(t, exactSource, fixedInstant, &recorder{}, "acme/backend", "acme/frontend")
	exact, _ = run(t, exact, exact.Init())

	expandedSource := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend", "acme/frontend")}}}
	expander := &fakeExpander{attempts: []attempt{{expansion: expandedSelection(t, "acme", "acme/backend", "acme/frontend")}}}
	expanded := expanding(t, expandedSource, expander, fixedInstant, &recorder{})
	expanded, _ = run(t, expanded, expanded.Init())

	if got, want := len(expanded.state.Snapshot.Events()), len(exact.state.Snapshot.Events()); got != want {
		t.Errorf("the expanded selection published %d events, want the %d an exact selection publishes", got, want)
	}
	if got, want := expanded.state.Snapshot.Aggregates(), exact.state.Snapshot.Aggregates(); !slices.Equal(got, want) {
		t.Errorf("the expanded selection aggregates %v, want the %v an exact selection aggregates", got, want)
	}
	if expanded.state.Freshness != exact.state.Freshness {
		t.Errorf("the expanded selection is %v, want the %v an exact selection reaches", expanded.state.Freshness, exact.state.Freshness)
	}
	assertScopes(t, "the expanded selection", expanded.state.Scopes, names(exact.state.Scopes)...)
}

func TestFailedExpansionSchedulesTheNextAttemptAtItsOwnRetryBound(t *testing.T) {
	tests := map[string]struct {
		expansion Expansion
		want      time.Duration
	}{
		"no instructed time": {want: expansionRetry},
		"later instructed time": {
			expansion: Expansion{RetryDelay: expansionRetry + 5*time.Minute, RateLimited: true},
			want:      expansionRetry + 5*time.Minute,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			timer := &recorder{}
			expander := &fakeExpander{attempts: []attempt{{expansion: test.expansion, err: errors.New("expanding acme: github request failed")}}}
			model := expanding(t, &fakeSource{}, expander, fixedInstant, timer)

			model, _ = run(t, model, model.Init())

			if got := timer.delays[len(timer.delays)-1]; got != test.want {
				t.Errorf("the next attempt after a failed expansion is scheduled in %s, want the RG-010 bound %s", got, test.want)
			}
			if want := fixedInstant.Add(test.want); !model.nextExpansion.Equal(want) {
				t.Errorf("the next expansion is due at %s, want %s", model.nextExpansion, want)
			}
		})
	}
}
