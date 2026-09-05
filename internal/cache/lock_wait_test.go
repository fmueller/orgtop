package cache

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestDefaultWaitsAreTheClosedProductionBounds pins RG-005's lock waits:
// ordinary work waits at most 250 ms for a busy region, matching the SQLite
// busy limit, and an explicit reset may wait two seconds. Production always
// runs on these; only the test binary shrinks them.
func TestDefaultWaitsAreTheClosedProductionBounds(t *testing.T) {
	t.Parallel()

	if got, want := defaultWaits().busy, 250*time.Millisecond; got != want {
		t.Errorf("defaultWaits().busy = %v, want %v", got, want)
	}
	if got, want := defaultWaits().reset, 2*time.Second; got != want {
		t.Errorf("defaultWaits().reset = %v, want %v", got, want)
	}
}

// TestTheTestBinaryRunsOnShrunkenWaits proves the wait bounds every cache lock
// acquisition uses are injectable rather than compiled in: the test binary
// replaces them with strictly shorter ones, so a mutant that breaks locking or
// budget arithmetic fails fast instead of holding a test open for the
// production bound.
func TestTheTestBinaryRunsOnShrunkenWaits(t *testing.T) {
	t.Parallel()

	if lockWaits.busy >= defaultWaits().busy {
		t.Errorf("lockWaits.busy = %v, want shorter than the %v production bound", lockWaits.busy, defaultWaits().busy)
	}
	if lockWaits.reset >= defaultWaits().reset {
		t.Errorf("lockWaits.reset = %v, want shorter than the %v production bound", lockWaits.reset, defaultWaits().reset)
	}
}

// TestContendedWorkGivesUpWithinTheInjectedBound proves the injected bound is
// the one a contended cache operation actually observes: a read contended by a
// second launch returns its miss well inside the injected wait, not inside the
// production one.
func TestContendedWorkGivesUpWithinTheInjectedBound(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	entry := compareEntry(t, "src/main.go")
	if err := store.Save(context.Background(), entry); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	holder, err := Open(location)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	t.Cleanup(func() { _ = holder.Close() })
	if err := holder.lock.acquire(admissionRegion, true, lockWaits.busy); err != nil {
		t.Fatalf("acquire(admissionRegion) error = %v", err)
	}
	defer holder.lock.release(admissionRegion)

	done := make(chan error, 1)
	go func() {
		_, _, lookupErr := store.Lookup(context.Background(), entry.Key)
		done <- lookupErr
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrContended) {
			t.Fatalf("Lookup() error = %v, want ErrContended", err)
		}
	case <-time.After(defaultWaits().busy):
		t.Fatalf("a contended Lookup did not give up inside the injected %v bound", lockWaits.busy)
	}
}

// testWaits are the shrunken bounds the whole test binary runs on. They stay
// well above the 5 ms retry interval, so an uncontended acquisition still has
// room to succeed on a loaded host, and well below the production bounds, so a
// contended one gives up before it can stretch a test out.
func testWaits() waits {
	return waits{busy: 40 * time.Millisecond, reset: 200 * time.Millisecond}
}
