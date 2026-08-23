package tui

import (
	"testing"
	"time"
)

// ageAnchor is the last-success instant the age tests measure against. It is a
// wall-clock instant far from the machine's own, so a spelling that reached for
// the current clock instead of the anchor could not accidentally agree.
var ageAnchor = time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)

// TestEventAgeSpellsOneWholeUnit guards FR-010: an age is spelled in one unit at
// whole-unit resolution, so a partly filled unit rounds down rather than up.
// Both sides of every bucket boundary are pinned, because rounding the wrong way
// is the failure that makes an age disagree with the row above it.
func TestEventAgeSpellsOneWholeUnit(t *testing.T) {
	const day = 24 * time.Hour

	tests := []struct {
		name    string
		elapsed time.Duration
		want    string
	}{
		{name: "the anchor instant itself", elapsed: 0, want: "<1m"},
		{name: "one second short of a minute", elapsed: 59 * time.Second, want: "<1m"},
		{name: "exactly one minute", elapsed: time.Minute, want: "1m"},
		{name: "one second short of two minutes", elapsed: 119 * time.Second, want: "1m"},
		{name: "exactly two minutes", elapsed: 2 * time.Minute, want: "2m"},
		{name: "one second short of an hour", elapsed: time.Hour - time.Second, want: "59m"},
		{name: "exactly one hour", elapsed: time.Hour, want: "1h"},
		{name: "one second short of a day", elapsed: day - time.Second, want: "23h"},
		{name: "exactly one day", elapsed: day, want: "1d"},
		{name: "one second short of a week", elapsed: 7*day - time.Second, want: "6d"},
		{name: "exactly one week", elapsed: 7 * day, want: "1w"},
		{name: "one second short of two weeks", elapsed: 14*day - time.Second, want: "1w"},
		{name: "exactly two weeks", elapsed: 14 * day, want: "2w"},
		{name: "one second short of a year", elapsed: 365*day - time.Second, want: "52w"},
		{name: "exactly one year", elapsed: 365 * day, want: "1y"},
		{name: "one second short of two years", elapsed: 730*day - time.Second, want: "1y"},
		{name: "exactly two years", elapsed: 730 * day, want: "2y"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := eventAge(ageAnchor.Add(-test.elapsed), ageAnchor)
			if got != test.want {
				t.Errorf("an age of %v is spelled %q, want %q", test.elapsed, got, test.want)
			}
		})
	}
}

// TestEventAgeClampsAnEventAheadOfTheAnchor guards FR-010: a source clock ahead
// of the anchor must not render a negative or empty age. The youngest spelling
// is the honest one, because the event cannot be older than the snapshot that
// carries it.
func TestEventAgeClampsAnEventAheadOfTheAnchor(t *testing.T) {
	tests := []struct {
		name  string
		ahead time.Duration
	}{
		{name: "a second ahead", ahead: time.Second},
		{name: "an hour ahead", ahead: time.Hour},
		{name: "a week ahead", ahead: 7 * 24 * time.Hour},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := eventAge(ageAnchor.Add(test.ahead), ageAnchor)
			if got != youngestAge {
				t.Errorf("an event %v ahead of the anchor is spelled %q, want %q", test.ahead, got, youngestAge)
			}
		})
	}
}

// TestEventAgeClampsAZeroAnchorToTheYoungestAge guards the state before any
// refresh has succeeded: State.LastSuccess is the zero instant there, and every
// event is ahead of it. The clamp must carry that case rather than spelling an
// age of two thousand years.
func TestEventAgeClampsAZeroAnchorToTheYoungestAge(t *testing.T) {
	if got := eventAge(ageAnchor, time.Time{}); got != youngestAge {
		t.Errorf("an age against the zero anchor is spelled %q, want %q", got, youngestAge)
	}
}
