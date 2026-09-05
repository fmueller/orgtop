package cache

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// TestResetRemovesTheDatabaseAndItsSidecars proves `--reset-cache` runs RG-005's
// exact procedure: the main database is renamed to the tombstone, the
// original-name sidecars go first, and the tombstone itself is gone afterwards.
func TestResetRemovesTheDatabaseAndItsSidecars(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	seedEntry(t, store, "kept", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	// A rollback journal is the sidecar recovery can leave behind, so the
	// procedure has to account for it beside the WAL and SHM files.
	stageFile(t, location.Database()+"-journal", "journal")

	if err := Reset(location); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	for _, path := range append([]string{location.Database(), location.Tombstone(), location.Bootstrap()},
		location.Database()+"-wal", location.Database()+"-shm", location.Database()+"-journal") {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%q survived the reset, lstat error = %v", path, err)
		}
	}
}

// TestResetLeavesNoFreshDatabaseBesideAStaleWriteAheadLog proves the recovery
// path a reset exists for: the next launch creates a complete empty version-1
// database and never adopts write-ahead state from the removed one.
func TestResetLeavesNoFreshDatabaseBesideAStaleWriteAheadLog(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	seedEntry(t, store, "kept", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	stageFile(t, location.Database()+"-wal", "stale wal")

	if err := Reset(location); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	reopened, err := Open(location)
	if err != nil {
		t.Fatalf("Open() after reset error = %v", err)
	}
	defer func() { _ = reopened.Close() }()

	if got := queryScalar[int](t, location, "SELECT count(*) FROM evidence"); got != 0 {
		t.Errorf("evidence rows after a reset = %d, want an empty version-1 database", got)
	}
	if data, err := os.ReadFile(location.Database() + "-wal"); err == nil && string(data) == "stale wal" {
		t.Error("a fresh main database opened beside the stale write-ahead log")
	}
}

// TestResetSucceedsWithAnAbsentCache proves an absent database is a successful
// reset once the orphaned sidecars and tombstone are cleaned up.
func TestResetSucceedsWithAnAbsentCache(t *testing.T) {
	t.Parallel()

	location := stagedLocation(t)
	stageFile(t, location.Database()+"-wal", "stale wal")
	stageFile(t, location.Tombstone(), "tombstone")

	if err := Reset(location); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	for _, path := range []string{location.Database() + "-wal", location.Tombstone()} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%q survived the reset of an absent cache, lstat error = %v", path, err)
		}
	}
}

// TestResetNarrowsBroaderExistingPermissions proves a reset repairs the cache
// directory it uses. A reset creates the maintenance lock inside that directory
// under exclusive lifecycle access, which is exactly when RG-005 allows
// permission repair, so it must not leave a group- or world-accessible cache
// directory behind for the next launch to write into.
func TestResetNarrowsBroaderExistingPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes are not the Windows ownership contract")
	}
	t.Parallel()

	location := stagedLocation(t)
	if err := os.Chmod(location.Directory(), 0o777); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}

	if err := Reset(location); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	info, err := os.Stat(location.Directory())
	if err != nil {
		t.Fatalf("stat directory error = %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o700); got != want {
		t.Errorf("directory mode after a reset = %04o, want %04o", got, want)
	}
}

// TestResetPreservesADatabaseOrgTopDoesNotOwn proves the ownership guard: a
// foreign nonzero application ID and an ambiguous pre-existing zero-ID database
// are both preserved, and the reset fails with a sanitized ownership cause.
func TestResetPreservesADatabaseOrgTopDoesNotOwn(t *testing.T) {
	t.Parallel()

	for name, statements := range map[string][]string{
		"foreign application id": {"CREATE TABLE other(id INTEGER)", "PRAGMA application_id = 12345"},
		"ambiguous zero id":      {"CREATE TABLE other(id INTEGER)"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			location := stagedLocation(t)
			execDatabase(t, location, statements...)
			before, err := os.ReadFile(location.Database())
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}

			err = Reset(location)
			if !errors.Is(err, ErrForeignDatabase) {
				t.Fatalf("Reset() error = %v, want ErrForeignDatabase", err)
			}
			after, err := os.ReadFile(location.Database())
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			if len(before) != len(after) {
				t.Errorf("the preserved database changed size: %d -> %d", len(before), len(after))
			}
			if _, err := os.Lstat(location.Tombstone()); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("a refused reset left a tombstone, lstat error = %v", err)
			}
		})
	}
}

// TestResetRejectsAZeroLocation keeps an unresolved user cache directory an
// unavailable cache rather than a relative path the reset would delete.
func TestResetRejectsAZeroLocation(t *testing.T) {
	t.Parallel()

	if err := Reset(Location{}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Reset(zero) error = %v, want ErrUnavailable", err)
	}
}

// TestOpenRebuildsConfirmedStructuralCorruptionOnce proves a database carrying
// OrgTop's exact application ID and version but a damaged schema gets one
// rebuild attempt per process through the same tombstone procedure, and that a
// healthy cache never spends that attempt.
func TestOpenRebuildsConfirmedStructuralCorruptionOnce(t *testing.T) {
	t.Parallel()

	store, location := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	execDatabase(t, location, "DROP TABLE evidence_path")

	rebuilt, err := Open(location)
	if err != nil {
		t.Fatalf("Open() error = %v, want the one structural rebuild attempt to recover", err)
	}
	defer func() { _ = rebuilt.Close() }()

	if !rebuilt.rebuilt {
		t.Error("the store did not record spending its one structural rebuild attempt")
	}
	if _, err := os.Lstat(location.Tombstone()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the rebuild left a tombstone behind, lstat error = %v", err)
	}
	entry := compareEntry(t, "src/main.go")
	if err := rebuilt.WithClock(func() time.Time { return referenceTime }).Save(context.Background(), entry); err != nil {
		t.Fatalf("Save() after a rebuild error = %v", err)
	}

	healthy, err := Open(location)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer func() { _ = healthy.Close() }()
	if healthy.rebuilt {
		t.Error("an intact cache spent a structural rebuild attempt")
	}
}

// TestASpentRebuildAttemptIsNotRetried proves the rebuild budget is one attempt
// per process: a store that already spent it reports the corruption and
// preserves the database rather than discarding a second one.
func TestASpentRebuildAttemptIsNotRetried(t *testing.T) {
	t.Parallel()

	store, location := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	execDatabase(t, location, "DROP TABLE evidence_path")
	before, err := os.ReadFile(location.Database())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	spent := stagedStore(t, location)
	spent.rebuilt = true
	if err := spent.prepare(); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("prepare() error = %v, want ErrCorrupt without a second rebuild", err)
	}
	after, err := os.ReadFile(location.Database())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(before) != len(after) {
		t.Errorf("a spent rebuild attempt still replaced the database: %d -> %d bytes", len(before), len(after))
	}
}

// stagedStore builds a store over an existing cache directory without running
// Open's setup, so a test can drive prepare with a chosen rebuild budget.
func stagedStore(t *testing.T, location Location) *Store {
	t.Helper()

	root, err := openCacheRoot(location)
	if err != nil {
		t.Fatalf("openCacheRoot() error = %v", err)
	}
	lock, err := openMaintenanceLock(root, location.Lock())
	if err != nil {
		t.Fatalf("openMaintenanceLock() error = %v", err)
	}
	store := &Store{location: location, root: root, lock: lock, limits: defaultBounds()}
	if err := lock.acquire(lifecycleRegion, false, lockWaits.busy); err != nil {
		t.Fatalf("acquire lifecycle error = %v", err)
	}
	t.Cleanup(func() { _ = store.closeHandles() })
	return store
}

// TestCheckHeaderReportsAnAbsentDatabaseAsUnavailable keeps a canonical file
// that disappeared between setup and open an unavailable cache rather than a
// bare filesystem error matching no package sentinel. A concurrent reset is
// exactly that window, and the caller decides what to do from the sentinel.
func TestCheckHeaderReportsAnAbsentDatabaseAsUnavailable(t *testing.T) {
	t.Parallel()

	location := stagedLocation(t)
	store := stagedStore(t, location)

	if err := store.checkHeader(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("checkHeader() error = %v, want ErrUnavailable", err)
	}
}

// TestSaveProjectsTheReplacementsOwnPageGrowth proves write admission measures
// the pending replacement's own growth against the projected post-checkpoint
// retained ceiling rather than only the pre-write state.
func TestSaveProjectsTheReplacementsOwnPageGrowth(t *testing.T) {
	t.Parallel()

	// A ceiling only a few pages above the current state leaves room for the
	// pre-write figures but none for the rows this replacement writes; one
	// covering the growth admits the same maximum-size replacement.
	for name, testCase := range map[string]struct {
		headroom int64
		admitted bool
	}{
		"above the projected ceiling":  {headroom: 4 * pageSize},
		"within the projected ceiling": {headroom: 8 << 20, admitted: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store, _ := fixedClockStore(t)
			used, err := store.PhysicalBytes()
			if err != nil {
				t.Fatalf("PhysicalBytes() error = %v", err)
			}
			limits := defaultBounds()
			limits.retainedBytes = used + testCase.headroom

			err = store.withBounds(limits).Save(context.Background(), maximumEntry(t))
			if testCase.admitted {
				if err != nil {
					t.Errorf("Save() error = %v, want a maximum-size replacement within the projected ceiling", err)
				}
				return
			}
			if !errors.Is(err, ErrOverCapacity) {
				t.Errorf("Save() error = %v, want ErrOverCapacity for a replacement projected above the ceiling", err)
			}
		})
	}
}

// maximumEntry builds the largest storable record: the closed path count at the
// closed total byte bound.
func maximumEntry(t *testing.T) Entry {
	t.Helper()

	// Twelve bytes of every path are its fixed prefix, suffix, and ordinal, so
	// the padded name stops the set short of the closed 1 MiB total.
	width := domain.MaxEvidenceBytes/domain.MaxEvidencePaths - 12
	values := make([]string, 0, domain.MaxEvidencePaths)
	for index := range domain.MaxEvidencePaths {
		values = append(values, "src/"+strings.Repeat("a", width)+strconv.Itoa(index)+".go")
	}
	entry := compareEntry(t, values...)
	entry.Paths = sortedPaths(entry.Paths)
	if err := validatePaths(entry.Paths); err != nil {
		t.Fatalf("the maximum fixture is not storable: %v", err)
	}
	return entry
}
