package tui

import (
	"fmt"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// The secondary badges of the shared chrome, in RG-004's fixed narrow-width
// priority. They follow the mandatory primary source state and never replace
// it: a badge qualifies the snapshot the primary state already classified. The
// selection badge is missing here on purpose: its text is the marker
// SelectionFreshness already spells, and it is never respelled.
const (
	rateLimitedBadge   = "RATE LIMITED"
	pathUnknownBadge   = "PATH ?"
	currentPRBadge     = "CURRENT PR "
	cacheDegradedBadge = "CACHE DEGRADED"
	truncatedBadge     = "TRUNCATED"
)

// statusOverflow spells how many badges a width held back. Hidden badges are
// counted rather than dropped, so a narrow header still states that further
// status remains (RG-004).
const statusOverflow = "status"

// secondaryBadges returns the applicable badges of the prepared state in the
// fixed priority order. Every dimension is read from prepared state: no badge
// is inferred from a sanitized cause string, and none is derived by matching
// events or evidence here. Each badge carries the style its own condition
// warrants, so no later stage has to classify badge text to render it: the
// degraded conditions take the stale style the primary marker uses, while the
// qualified current-PR count is context rather than degradation.
func secondaryBadges(state State) []field {
	var badges []field
	if text := rateLimitBadge(state); text != "" {
		badges = append(badges, field{text: text, style: staleStyle})
	}
	if marker := state.SelectionFreshness.Marker(); marker != "" {
		badges = append(badges, field{text: marker, style: staleStyle})
	}
	unknown, currentPR := pathCoverage(state.Scoped.Aggregates())
	if unknown > 0 {
		badges = append(badges, field{text: fmt.Sprintf("%s%d", pathUnknownBadge, unknown), style: staleStyle})
	}
	if currentPR > 0 {
		badges = append(badges, field{text: fmt.Sprintf("%s%d", currentPRBadge, currentPR), style: contextStyle})
	}
	if state.CacheDegraded != "" {
		badges = append(badges, field{text: cacheDegradedBadge, style: staleStyle})
	}
	if state.Scoped.Truncated() {
		badges = append(badges, field{text: truncatedBadge, style: staleStyle})
	}
	return badges
}

// rateLimitBadge renders the rate-limit badge of the prepared state, naming the
// instructed retry instant when one was given. An enrichment-only limit counts
// too: it coexists with a CURRENT snapshot and must stay visible there.
func rateLimitBadge(state State) string {
	retryAt := earliest(state.RateLimitRetryAt, state.EnrichmentRetryAt)
	if !state.RateLimited && retryAt.IsZero() {
		return ""
	}
	if retryAt.IsZero() {
		return rateLimitedBadge
	}
	return rateLimitedBadge + " " + retryAt.Format(clockLayout)
}

// earliest returns the first instructed retry among the given instants, so the
// badge names when rate-limited work next becomes eligible rather than the
// latest limit the refresh happened to hit. Zero instants name no instruction.
func earliest(instants ...time.Time) time.Time {
	var soonest time.Time
	for _, instant := range instants {
		if instant.IsZero() {
			continue
		}
		if soonest.IsZero() || instant.Before(soonest) {
			soonest = instant
		}
	}
	return soonest
}

// pathCoverage totals the unresolved path evidence and the qualified current-PR
// members of the snapshot. Only path Scopes contribute unresolved coverage:
// repository membership stays known from repository identity, so a repository
// Scope is never presented as unresolved (RG-004).
func pathCoverage(aggregates []domain.ScopeAggregate) (unknown, currentPR int) {
	for _, aggregate := range aggregates {
		if aggregate.Scope.Kind() == domain.ScopePath {
			unknown += aggregate.Unknown
		}
		currentPR += aggregate.CurrentPR
	}
	return unknown, currentPR
}

// badgeForms returns the badge ladder from the complete set down to none. Each
// rung keeps the highest-priority badges and spends its last slot on the count
// of the ones it held back, so a narrower header states that status remains
// rather than silently dropping a condition. The count is context: it reports
// what the width hid, not a condition of its own.
func badgeForms(badges []field) [][]field {
	forms := make([][]field, 0, len(badges)+1)
	for shown := len(badges); shown >= 0; shown-- {
		form := badges[:shown:shown]
		if hidden := len(badges) - shown; hidden > 0 {
			counted := fmt.Sprintf("%s%d %s", hiddenMark, hidden, statusOverflow)
			form = append(form, field{text: counted, style: contextStyle})
		}
		forms = append(forms, form)
	}
	return forms
}
