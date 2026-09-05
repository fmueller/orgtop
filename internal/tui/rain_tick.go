package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// rainTickMsg is one message of the single Rain timer chain. It carries the
// generation it belongs to and the explicit logical timestamp the transition
// advances to, so no Rain state transition ever reads the host clock (RG-006).
type rainTickMsg struct {
	chain uint64
	at    time.Time
}

// applyRainTick advances the field by one accepted tick and continues the one
// chain. Only the generation the field expects is handled; a duplicate or
// older-generation message schedules nothing, so a second chain never starts.
// A message the field itself rejects — before its first snapshot, while paused,
// or at a timestamp that does not advance the cursor — still continues the one
// chain from the generation the field reports afterwards.
func (m Model) applyRainTick(message rainTickMsg) (tea.Model, tea.Cmd) {
	if message.chain != m.rain.chain {
		return m, nil
	}
	m.rain = m.rain.ticked(message.chain, message.at)
	// The strip advances on the same explicit tick even while the field is
	// paused or hidden, so an entry leaving the fixed 15-minute window
	// disappears from the strip while the frozen field keeps its picture
	// (RG-007).
	m.interesting = m.interesting.ticked(message.at)
	return m.resizedRain(), m.rainTick(m.rain.chain, rainStep)
}

// rainTickAfter is the production Rain timer seam: it schedules the next
// generation of the chain with the explicit instant it fires at.
func rainTickAfter(chain uint64, delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(at time.Time) tea.Msg {
		return rainTickMsg{chain: chain, at: at}
	})
}
