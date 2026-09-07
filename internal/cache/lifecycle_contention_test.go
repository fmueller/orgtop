package cache

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// lifecycleEnvironment names the environment variable that turns this test
// binary into a second process holding the maintenance lock's lifecycle region.
// Shared access is what an ordinary running launch holds for its whole
// lifetime, and it is what an exclusive lifecycle step has to wait behind.
const lifecycleEnvironment = "ORGTOP_CACHE_LIFECYCLE_HOLDER"

// holdSharedLifecycle takes the lifecycle region shared at root and holds it for
// a fixed interval, then exits. It opens no database, so a corrupt or absent
// canonical file is not its concern: the point is only that a second process
// occupies the region the reset and the rebuild need exclusively.
func holdSharedLifecycle(root string) int {
	location := LocationIn(root)
	cacheRoot, err := openCacheRoot(location)
	if err != nil {
		return 2
	}
	defer func() { _ = cacheRoot.Close() }()

	lock, err := openMaintenanceLock(cacheRoot, location.Lock())
	if err != nil {
		return 3
	}
	defer func() { _ = lock.close() }()

	if err := lock.acquire(lifecycleRegion, false, lockWaits.busy); err != nil {
		return 4
	}
	defer lock.release(lifecycleRegion)

	if err := signalHolderReady(root); err != nil {
		return 5
	}
	time.Sleep(holdDuration)
	return 0
}

// ownedContents reads every owned cache file that exists, so a test can prove a
// refused lifecycle step left the cache byte-identical.
func ownedContents(t *testing.T, location Location) map[string]string {
	t.Helper()

	contents := map[string]string{}
	for _, path := range location.ownedFiles() {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		contents[path] = string(data)
	}
	return contents
}

// requireUnchanged fails unless the owned files are exactly the ones recorded
// before the refused step, with byte-identical contents.
func requireUnchanged(t *testing.T, before map[string]string, location Location) {
	t.Helper()

	after := ownedContents(t, location)
	for path, want := range before {
		got, ok := after[path]
		if !ok {
			t.Errorf("%q was removed by a contended lifecycle step", path)
			continue
		}
		if got != want {
			t.Errorf("%q changed under a contended lifecycle step: %d -> %d bytes", path, len(want), len(got))
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("%q was created by a contended lifecycle step", path)
		}
	}
}

// TestAContendedResetReportsContentionAndChangesNoFile proves RG-005's reset
// wait bound under genuine cross-process contention: a reset that cannot take
// the lifecycle region inside its wait reports the sanitized contended cause
// rather than hanging or racing, and every owned file survives byte-identical.
func TestAContendedResetReportsContentionAndChangesNoFile(t *testing.T) {
	root := t.TempDir()
	location := LocationIn(root)

	store, err := Open(location)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.WithClock(func() time.Time { return referenceTime }).
		Save(t.Context(), compareEntry(t, "src/main.go")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	startHolder(t, lifecycleEnvironment, root)
	before := ownedContents(t, location)

	err = awaitBounded(t, func() error { return Reset(location) })
	if !errors.Is(err, ErrContended) {
		t.Fatalf("Reset() error = %v, want ErrContended", err)
	}
	if got := err.Error(); strings.Contains(got, location.Directory()) {
		t.Errorf("Reset() error = %q, want a sanitized cause naming no cache path", got)
	}
	requireUnchanged(t, before, location)
}

// TestAContendedLaunchCannotRebuildACorruptDatabase proves the one structural
// rebuild attempt is bound to the lifecycle region too: while a second process
// holds the region, a launch that finds a structurally corrupt database reports
// contention instead of discarding it, and the corrupt database is preserved
// byte-identical for the reset the user still has to run.
func TestAContendedLaunchCannotRebuildACorruptDatabase(t *testing.T) {
	root := t.TempDir()
	location := LocationIn(root)

	store, err := Open(location)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	// Exactly the structural corruption of an owned version-1 database that
	// earns the single rebuild attempt on an uncontended launch.
	execDatabase(t, location, "DROP TABLE evidence_path")

	startHolder(t, lifecycleEnvironment, root)
	before := ownedContents(t, location)
	if _, ok := before[location.Database()]; !ok {
		t.Fatalf("the corrupt database is missing before the contended launch")
	}

	err = awaitBounded(t, func() error {
		contended, openErr := Open(location)
		if openErr == nil {
			_ = contended.Close()
		}
		return openErr
	})
	if !errors.Is(err, ErrContended) {
		t.Fatalf("Open() error = %v, want ErrContended without a rebuild", err)
	}
	if _, err := os.Lstat(location.Tombstone()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a contended launch left a tombstone behind, lstat error = %v", err)
	}
	requireUnchanged(t, before, location)
}
