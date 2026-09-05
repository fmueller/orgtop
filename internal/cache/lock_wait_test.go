package cache

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
// replaces them with strictly shorter ones, so a broken lock or budget fails
// the suite fast instead of holding a test open for the production bound.
func TestTheTestBinaryRunsOnShrunkenWaits(t *testing.T) {
	t.Parallel()

	if lockWaits.busy >= defaultWaits().busy {
		t.Errorf("lockWaits.busy = %v, want shorter than the %v production bound", lockWaits.busy, defaultWaits().busy)
	}
	if lockWaits.reset >= defaultWaits().reset {
		t.Errorf("lockWaits.reset = %v, want shorter than the %v production bound", lockWaits.reset, defaultWaits().reset)
	}
}

// TestContendedWorkReturnsPromptlyRatherThanWaitingOut proves a contended read
// gives up instead of blocking: a Lookup contended by a second launch returns
// its miss well inside the two seconds an explicit reset is allowed, so no
// ordinary cache operation can be wired to a bound that long or to none at all.
// It deliberately does not race the injected bound against the production busy
// bound — those are the same order of magnitude, and the host speeds this suite
// runs on span roughly fifty to one. That the injected bound is the shorter of
// the two is pinned by TestTheTestBinaryRunsOnShrunkenWaits instead.
func TestContendedWorkReturnsPromptlyRatherThanWaitingOut(t *testing.T) {
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
	case <-time.After(defaultWaits().reset):
		t.Fatalf("a contended Lookup did not give up inside %v", defaultWaits().reset)
	}
}

// testWaits are the shrunken bounds the whole test binary runs on. They stay
// well below the production bounds, so a contended region gives up long before
// a test could stretch out waiting for one, and well above the 5 ms retry
// interval, so an uncontended acquisition still has room to succeed. The
// headroom matters: the Windows runner has been measured running this package
// roughly fifty times slower than a Linux developer host, and the same bound
// serves as the SQLite busy timeout.
func testWaits() waits {
	return waits{busy: 100 * time.Millisecond, reset: 500 * time.Millisecond}
}

// TestHolderReadinessGivesUpAsSoonAsTheHolderExits proves the cross-process
// proof stops waiting the moment the second process is gone rather than sitting
// out its readiness bound. A setup path that cannot open the cache at all
// leaves the holder exiting without ever writing its marker, and that is the
// state the proof has to fail fast in — the bound itself only governs a holder
// that is alive but slow, which a loaded host legitimately produces.
func TestHolderReadinessGivesUpAsSoonAsTheHolderExits(t *testing.T) {
	t.Parallel()

	// A cache root underneath a regular file: the holder's own Open fails, so
	// it exits before it can hold a region or signal readiness.
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	stageFile(t, blocker, "not a directory")

	holder := exec.Command(os.Args[0], "-test.run", "TestMain")
	holder.Env = append(os.Environ(), holdEnvironment+"="+filepath.Join(blocker, "cache"))
	if err := holder.Start(); err != nil {
		t.Fatalf("start holder error = %v", err)
	}
	exited := make(chan error, 1)
	go func() {
		exited <- holder.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		_ = holder.Process.Kill()
		<-exited
	})

	err := awaitHolderReady(exited, filepath.Join(root, "holding"))
	if !errors.Is(err, errHolderExited) {
		t.Errorf("awaitHolderReady() error = %v, want %v", err, errHolderExited)
	}
}
