package tui

import (
	"fmt"
	"time"
)

// The coarse age units. A day is a fixed 24 hours and a year a fixed 365 days:
// an age is a rendered approximation of elapsed time, not a calendar
// calculation, so no zone or leap correction is implied.
const (
	day  = 24 * time.Hour
	week = 7 * day
	year = 365 * day
)

// youngestAge spells an event younger than a minute. It is also what an event
// timestamped ahead of the anchor clamps to, because an event cannot be older
// than the snapshot carrying it, and a negative or empty age would read as a
// rendering fault rather than as a source clock running ahead.
const youngestAge = "<1m"

// eventAge spells how long before the anchor an event occurred, in one unit at
// whole-unit resolution: the unit is the coarsest one the elapsed time fills
// completely, and a partly filled unit rounds down. The anchor is the last
// successful refresh, never the current clock, so every rendered age agrees with
// the last-success time the header reports and freezes with a stale snapshot
// (FR-010). The zero anchor of a shell that has not yet succeeded puts every
// event ahead of it, which the clamp carries.
func eventAge(occurred, anchor time.Time) string {
	elapsed := anchor.Sub(occurred)
	switch {
	case elapsed < time.Minute:
		return youngestAge
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm", elapsed/time.Minute)
	case elapsed < day:
		return fmt.Sprintf("%dh", elapsed/time.Hour)
	case elapsed < week:
		return fmt.Sprintf("%dd", elapsed/day)
	case elapsed < year:
		return fmt.Sprintf("%dw", elapsed/week)
	default:
		return fmt.Sprintf("%dy", elapsed/year)
	}
}
