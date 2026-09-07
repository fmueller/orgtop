package tui

import (
	"errors"

	"github.com/charmbracelet/x/ansi"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// retryInstant is the instructed rate-limit retry every badge fixture names. It
// is a fixed instant so the rendered badge is identical on every run (NFR-006).
var retryInstant = fixedInstant.Add(15 * time.Minute)

// badgeTexts spells the prepared badges. The assertions here compare the fixed
// narrow-width priority and the badge wording, not the styles each badge
// carries, which the rendered header proves instead.
func badgeTexts(badges []field) []string {
	texts := make([]string, 0, len(badges))
	for _, badge := range badges {
		texts = append(texts, badge.text)
	}
	return texts
}

// combinedState builds the state of a refresh that succeeded while every
// secondary RG-004 condition applied at once: an enrichment rate limit, a
// failed re-expansion, unresolved path evidence, a qualified current-PR member,
// a degraded cache, and a truncated retained set.
func combinedState(t *testing.T) State {
	t.Helper()
	path := pathScope(t, "acme/backend", "internal")
	scopes := scopeSet(t, path, domain.NewRepositoryScope(testRepository(t, "acme/backend")))

	unknown := evidenceFor(t, "unknown", "acme/backend", 3, domain.IncompleteOutcome("the enrichment reported no outcome for the event"))
	member := evidenceFor(t, "member", "acme/backend", 1, domain.CompleteOutcome(domain.ProvenanceCurrentPR, mustChangedPaths(t, "internal/main.go")))

	return State{
		Scopes:             scopes,
		Scoped:             domain.NewRetainedSnapshot(scopes, slices.Concat(unknown, member), true),
		Freshness:          FreshnessCurrent,
		LastSuccess:        fixedInstant,
		CacheDegraded:      "opening the enrichment cache failed",
		EnrichmentRetryAt:  retryInstant,
		SelectionFreshness: SelectionStale,
		SelectionCause:     "expanding acme: github rate limit reached",
	}
}

func TestSecondaryBadgesFollowTheFixedNarrowWidthPriority(t *testing.T) {
	badges := badgeTexts(secondaryBadges(combinedState(t)))

	want := []string{
		"RATE LIMITED " + retryInstant.Format(clockLayout),
		"SELECTION STALE",
		"PATH ?3",
		"CURRENT PR 1",
		"CACHE DEGRADED",
		"TRUNCATED",
	}
	if !slices.Equal(badges, want) {
		t.Fatalf("the prepared badges are %q, want %q", badges, want)
	}
}

func TestSecondaryBadgesReportOnlyTheApplicableConditions(t *testing.T) {
	scopes := testScope(t, "acme/backend")
	state := State{Scopes: scopes, Scoped: domain.NewRetainedSnapshot(scopes, nil, false), Freshness: FreshnessCurrent}

	if badges := badgeTexts(secondaryBadges(state)); len(badges) != 0 {
		t.Errorf("an undegraded current refresh reports the badges %q, want none", badges)
	}
}

// TestRateLimitBadgeReadsPreparedStateNotTheCauseText guards the RG-004
// requirement that the rate limit is carried as prepared state: a cause string
// that merely mentions a rate limit is not a rate limit the header may claim.
func TestRateLimitBadgeReadsPreparedStateNotTheCauseText(t *testing.T) {
	scopes := testScope(t, "acme/backend")
	state := State{
		Scopes:      scopes,
		Scoped:      domain.NewRetainedSnapshot(scopes, nil, false),
		Freshness:   FreshnessStale,
		LastSuccess: fixedInstant,
		Cause:       "refreshing acme/backend: github rate limit reached",
	}

	if badges := badgeTexts(secondaryBadges(state)); slices.ContainsFunc(badges, func(badge string) bool {
		return strings.HasPrefix(badge, "RATE LIMITED")
	}) {
		t.Errorf("the badges %q claim a rate limit that only the cause text mentions", badges)
	}

	state.RateLimited = true
	if badges := badgeTexts(secondaryBadges(state)); !slices.Contains(badges, "RATE LIMITED") {
		t.Errorf("the badges %q omit the prepared rate limit that named no retry instant", badges)
	}

	state.RateLimitRetryAt = retryInstant
	want := "RATE LIMITED " + retryInstant.Format(clockLayout)
	if badges := badgeTexts(secondaryBadges(state)); !slices.Contains(badges, want) {
		t.Errorf("the badges %q omit %q", badges, want)
	}
}

// TestPathUnknownBadgeCountsOnlyPathScopes guards the RG-004 rule that
// repository membership stays known from repository identity: a repository
// Scope contributes no unresolved path coverage.
func TestPathUnknownBadgeCountsOnlyPathScopes(t *testing.T) {
	scopes := scopeSet(t, domain.NewRepositoryScope(testRepository(t, "acme/backend")))
	retained := evidenceFor(t, "unknown", "acme/backend", 2, domain.IncompleteOutcome("no evidence"))
	state := State{
		Scopes:    scopes,
		Scoped:    domain.NewRetainedSnapshot(scopes, retained, false),
		Freshness: FreshnessCurrent,
	}

	if badges := badgeTexts(secondaryBadges(state)); len(badges) != 0 {
		t.Errorf("a repository-only selection reports %q, want no unresolved path coverage", badges)
	}
}

func TestHeaderStatesEveryBadgeAfterThePrimaryStateWhenTheWidthHoldsThem(t *testing.T) {
	state := combinedState(t)
	fields := headerFields(renderHeader(state, ModeOverview, unbounded))

	primary := slices.Index(fields, transportLabel)
	previous := primary
	for _, badge := range badgeTexts(secondaryBadges(state)) {
		index := slices.Index(fields, badge)
		if index < 0 {
			t.Fatalf("the header %q omits the badge %q", fields, badge)
		}
		if index < previous {
			t.Errorf("the header %q states %q before the higher-priority badge at %d", fields, badge, previous)
		}
		previous = index
	}
}

// TestHeaderCollapsesHiddenBadgesIntoTheStatusCount proves RG-004's overflow
// accounting: a width too narrow for every badge never silently drops one, it
// spends the last visible slot on the count of what it held back.
func TestHeaderCollapsesHiddenBadgesIntoTheStatusCount(t *testing.T) {
	state := combinedState(t)
	badges := badgeTexts(secondaryBadges(state))

	full := renderHeader(state, ModeOverview, unbounded)
	for width := lipgloss.Width(full) - 1; width > 0; width-- {
		fields := headerFields(renderHeader(state, ModeOverview, width))
		shown := 0
		for _, badge := range badges {
			if slices.Contains(fields, badge) {
				shown++
			}
		}
		if shown == len(badges) {
			continue
		}
		hidden := len(badges) - shown
		want := "+" + itoa(hidden) + " status"
		if !slices.Contains(fields, want) {
			t.Fatalf("at width %d the header %q hides %d badges without stating %q", width, fields, hidden, want)
		}
		return
	}
	t.Fatal("no width hid a badge, so the status overflow accounting was never exercised")
}

// itoa spells a small count without pulling strconv into the assertion.
func itoa(count int) string {
	return string(rune('0' + count))
}

// TestAFailedPollCarriesItsRateLimitAsPreparedState proves the lifecycle
// publishes the rate limit and its instructed retry instant, so the header
// never has to re-derive either from the sanitized cause text (RG-004).
func TestAFailedPollCarriesItsRateLimitAsPreparedState(t *testing.T) {
	limited := Result{Delay: 15 * time.Minute, RateLimited: true}
	source := &fakeSource{outcomes: []outcome{
		{result: activity(t, "acme/backend")},
		{result: limited, err: errors.New("refreshing acme/backend: github rate limit reached")},
	}}
	timer := &recorder{}
	model := lifecycle(t, source, fixedInstant, timer, "acme/backend")

	model, cmd := run(t, model, model.refresh())
	model, _ = run(t, model, model.refresh())
	_ = cmd

	if !model.state.RateLimited {
		t.Fatal("the rate-limited poll published no prepared rate limit")
	}
	if want := fixedInstant.Add(15 * time.Minute); !model.state.RateLimitRetryAt.Equal(want) {
		t.Errorf("the prepared retry instant is %v, want %v", model.state.RateLimitRetryAt, want)
	}
	if badges := badgeTexts(secondaryBadges(model.state)); !slices.Contains(badges, "RATE LIMITED "+fixedInstant.Add(15*time.Minute).Format(clockLayout)) {
		t.Errorf("the badges %q omit the rate limit the poll reported", badges)
	}
}

// TestASuccessfulPollClearsTheRateLimitItNoLongerReports proves RG-004's
// recovery rule: a badge clears only when no current outcome still requires it,
// and a recovered poll clears exactly the cause it recovered.
func TestASuccessfulPollClearsTheRateLimitItNoLongerReports(t *testing.T) {
	limited := Result{Delay: 15 * time.Minute, RateLimited: true}
	source := &fakeSource{outcomes: []outcome{
		{result: activity(t, "acme/backend")},
		{result: limited, err: errors.New("refreshing acme/backend: github rate limit reached")},
		{result: activity(t, "acme/backend")},
	}}
	timer := &recorder{}
	model := lifecycle(t, source, fixedInstant, timer, "acme/backend")

	model, _ = run(t, model, model.refresh())
	model, _ = run(t, model, model.refresh())
	model, _ = run(t, model, model.refresh())

	if model.state.RateLimited || !model.state.RateLimitRetryAt.IsZero() {
		t.Fatalf("the recovered poll retained the rate limit %v/%v", model.state.RateLimited, model.state.RateLimitRetryAt)
	}
	if badges := badgeTexts(secondaryBadges(model.state)); len(badges) != 0 {
		t.Errorf("the recovered refresh still reports %q", badges)
	}
}

// TestPathCoverageNeverReadsARepositoryScopeAsUnresolved fixes RG-004's rule
// directly on the counter rather than through the domain evaluation that
// happens to make a repository Scope's unknown count zero today. A repository
// Scope's membership is known from repository identity, so even an aggregate
// carrying an unknown count must not reach the unresolved path coverage.
func TestPathCoverageNeverReadsARepositoryScopeAsUnresolved(t *testing.T) {
	aggregates := []domain.ScopeAggregate{
		{Scope: domain.NewRepositoryScope(testRepository(t, "acme/backend")), Unknown: 7, CurrentPR: 1},
		{Scope: pathScope(t, "acme/backend", "internal"), Unknown: 2, CurrentPR: 3},
	}

	unknown, currentPR := pathCoverage(aggregates)

	if unknown != 2 {
		t.Errorf("the unresolved path coverage is %d, want the 2 the path Scope holds", unknown)
	}
	if currentPR != 4 {
		t.Errorf("the qualified current-PR total is %d, want every Scope's 4", currentPR)
	}
}

// TestEarliestNamesTheSoonestInstructedRetry pins which retry a refresh limited
// at both its source and its enrichment names: the soonest one, because that is
// when rate-limited work next becomes eligible.
func TestEarliestNamesTheSoonestInstructedRetry(t *testing.T) {
	soon, late := fixedInstant.Add(time.Minute), fixedInstant.Add(time.Hour)

	if got := earliest(late, soon); !got.Equal(soon) {
		t.Errorf("earliest(late, soon) is %v, want %v", got, soon)
	}
	if got := earliest(soon, late); !got.Equal(soon) {
		t.Errorf("earliest(soon, late) is %v, want %v", got, soon)
	}

	state := State{RateLimitRetryAt: late, EnrichmentRetryAt: soon}
	want := "RATE LIMITED " + soon.Format(clockLayout)
	if got := rateLimitBadge(state); got != want {
		t.Errorf("the rate-limit badge is %q, want %q", got, want)
	}
}

// TestHeaderCollapsesEveryOtherFieldBeforeAnyBadge proves the badge ladder is
// the outermost dimension: at the first width that hides a badge, no optional
// context field is still competing for the space (RG-004).
func TestHeaderCollapsesEveryOtherFieldBeforeAnyBadge(t *testing.T) {
	state := combinedState(t)
	state.Cause = "refreshing acme/backend: github request failed"
	badges := badgeTexts(secondaryBadges(state))

	// The overflow accounting is the other ladder the header carries, so the
	// header is measured with one active: a badge must survive every rung of
	// it, down to the sparsest candidate that still holds every badge.
	rows := overflowRange{kind: "scopes", first: 1, last: 2, total: 9}
	forms := rows.forms()
	sparsest := lipgloss.Width(strings.Join(slices.Concat(
		[]string{ModeOverview.Label(), transportLabel}, badges, forms[len(forms)-1:]), separator))

	optional := []string{appName, state.Cause, "updated " + state.LastSuccess.Format(clockLayout)}
	for _, scope := range state.Scopes.Repositories() {
		optional = append(optional, scope.String())
	}

	full := renderHeader(state, ModeOverview, unbounded, rows)
	for width := lipgloss.Width(full) - 1; width > 0; width-- {
		header := renderHeader(state, ModeOverview, width, rows)
		fields := headerFields(header)
		if !slices.ContainsFunc(badges, func(badge string) bool { return !slices.Contains(fields, badge) }) {
			continue
		}
		for _, field := range optional {
			if strings.Contains(ansi.Strip(header), field) {
				t.Fatalf("at width %d the header %q still holds %q while a badge is already hidden", width, fields, field)
			}
		}
		if width >= sparsest {
			t.Fatalf("at width %d the header %q hides a badge although %d cells hold every one of them", width, fields, sparsest)
		}
		return
	}
	t.Fatal("no width hid a badge, so the badge ladder was never exercised")
}

// TestARateLimitedReexpansionCarriesItsRateLimitAsPreparedState covers the one
// degraded path that never reaches a poll: the expansion itself is rate limited,
// so the attempt reports no source error at all and the prepared rate limit has
// to come from the expansion outcome (RG-004, RG-010).
func TestARateLimitedReexpansionCarriesItsRateLimitAsPreparedState(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	expander := &fakeExpander{attempts: []attempt{
		{expansion: expandedSelection(t, "acme", "acme/backend")},
		{expansion: Expansion{RetryDelay: 5 * time.Minute, RateLimited: true}, err: errors.New("expanding acme: github rate limit reached")},
	}}
	timer := &recorder{}
	model := expanding(t, source, expander, fixedInstant, timer)

	model, _ = run(t, model, initRefresh(t, model))
	due := fixedInstant.Add(16 * time.Minute)
	model = model.at(due)
	model, cmd := apply(t, model, refreshDueMsg{})
	model, _ = run(t, model, cmd)

	if !model.state.RateLimited {
		t.Fatal("the rate-limited re-expansion published no prepared rate limit")
	}
	if want := due.Add(5 * time.Minute); !model.state.RateLimitRetryAt.Equal(want) {
		t.Errorf("the prepared retry instant is %v, want the instructed %v", model.state.RateLimitRetryAt, want)
	}
	want := "RATE LIMITED " + due.Add(5*time.Minute).Format(clockLayout)
	if badges := badgeTexts(secondaryBadges(model.state)); !slices.Contains(badges, want) {
		t.Errorf("the badges %q omit %q", badges, want)
	}
}
