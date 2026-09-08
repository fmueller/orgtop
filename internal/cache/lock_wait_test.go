package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
// gives up on the ordinary busy bound instead of blocking: a Lookup contended
// by a second launch returns its miss inside contendedGiveUpBound, which no
// operation wired to the longer reset bound, or to no bound at all, can meet.
// The bound it races is the injected one rather than a production constant, so
// the assertion discriminates rather than merely documenting.
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
	case <-time.After(contendedGiveUpBound):
		t.Fatalf("a contended Lookup did not give up inside %v", contendedGiveUpBound)
	}
}

// testWaits are the shrunken bounds the whole test binary runs on. They stay
// below the production bounds, so a contended region gives up long before a
// test could stretch out waiting for one, and well above the 5 ms retry
// interval, so an uncontended acquisition still has room to succeed. They are
// also kept a full order of magnitude apart, which is what lets
// contendedGiveUpBound tell ordinary work from an explicit reset. Neither is
// the SQLite busy timeout: the driver runs on driverBusyTimeout, so these
// values move without changing driver behaviour.
func testWaits() waits {
	return waits{busy: 100 * time.Millisecond, reset: 1500 * time.Millisecond}
}

// contendedGiveUpBound is how long a contended ordinary operation may take
// before the suite calls it a wait-out. It sits an order of magnitude above the
// injected busy bound and strictly below the injected reset bound, so the two
// failure modes it exists for -- an ordinary operation wired to the reset bound,
// or to no bound at all -- both overrun it deterministically. The margin is
// sized against measured host spread, not local timings: the Windows runner has
// been measured running this package roughly fifty times slower than a Linux
// developer host, and what elapses here is a wall-clock deadline plus
// scheduling, so nine hundred milliseconds of slack absorbs that spread.
const contendedGiveUpBound = 1 * time.Second

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

// TestTheDriverBusyTimeoutIsTheProductionBound proves the SQLite busy timeout
// is separated from the injectable lock wait: every functional connection is
// opened with the closed production bound even in the test binary, which runs
// on strictly shorter lock waits. The separation is what lets a timing
// assertion move the injected busy bound without changing driver behaviour.
func TestTheDriverBusyTimeoutIsTheProductionBound(t *testing.T) {
	t.Parallel()

	if lockWaits.busy == defaultWaits().busy {
		t.Fatal("the test binary no longer shrinks the lock wait, so this proves nothing")
	}

	source := functionalDataSource("cache.db")
	production := fmt.Sprintf("&_pragma=busy_timeout(%d)", defaultWaits().busy.Milliseconds())
	injected := fmt.Sprintf("&_pragma=busy_timeout(%d)", lockWaits.busy.Milliseconds())
	if !strings.Contains(source, production) {
		t.Errorf("functionalDataSource() = %q, want it to carry %q", source, production)
	}
	if strings.Contains(source, injected) {
		t.Errorf("functionalDataSource() = %q, want the driver bound not tied to the injected %q", source, injected)
	}
}

// TestTheHolderOutlastsEveryInjectedWait proves the anti-hang deadline
// awaitBounded runs on keeps its documented headroom. That deadline is half of
// holdDuration, and the longest thing it may legitimately wait for is an
// explicit reset waiting lockWaits.reset, so it has to stay an order of
// magnitude above that bound or a fifty-times-slower runner would report a hang
// where the wait was simply met late. holdDuration is hand-written, so without
// this a later change to the injected reset bound would erode the margin
// silently.
func TestTheHolderOutlastsEveryInjectedWait(t *testing.T) {
	t.Parallel()

	if got := holdDuration / 2; got < 10*lockWaits.reset {
		t.Errorf("awaitBounded deadline = %v, want at least ten times the %v reset bound", got, lockWaits.reset)
	}
}

// TestTheInjectedBoundsAreAnOrderOfMagnitudeApart proves the two shrunken waits
// stay far enough apart that a timing assertion can tell them apart. Ordinary
// work waits busy and only an explicit reset waits reset; an assertion sized
// between them can only distinguish the two while the gap is wide enough that a
// host running this package fifty times slower cannot close it.
func TestTheInjectedBoundsAreAnOrderOfMagnitudeApart(t *testing.T) {
	t.Parallel()

	if lockWaits.reset < 10*lockWaits.busy {
		t.Errorf("lockWaits.reset = %v, want at least ten times the %v busy bound", lockWaits.reset, lockWaits.busy)
	}
}
