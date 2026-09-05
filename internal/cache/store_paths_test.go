package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// storeMutPermissions reports the permission bits of one cache path.
func storeMutPermissions(t *testing.T, path string) os.FileMode {
	t.Helper()

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %q error = %v", path, err)
	}
	return info.Mode().Perm()
}

// storeMutLockDescriptorIsOpen reports whether the process still holds the
// counted maintenance lock descriptor of one cache location.
func storeMutLockDescriptorIsOpen(location Location) bool {
	lockFiles.mutex.Lock()
	defer lockFiles.mutex.Unlock()

	_, ok := lockFiles.open[location.Lock()]
	return ok
}

// storeMutMaximalPaths builds the largest storable changed-path set: every path
// is exactly the per-path byte bound and the set is exactly the per-entity byte
// bound, so both closed RG-005 bounds are met without being exceeded.
func storeMutMaximalPaths(t *testing.T) []domain.ChangedPath {
	t.Helper()

	count := domain.MaxEvidenceBytes / domain.MaxChangedPathBytes
	values := make([]string, 0, count)
	for index := 0; index < count; index++ {
		prefix := fmt.Sprintf("%03d/", index)
		values = append(values, prefix+strings.Repeat("p", domain.MaxChangedPathBytes-len(prefix)))
	}
	return changedPaths(t, values...)
}

// TestOpenRejectsANonRegularObjectAtALaterOwnedPath guards RG-005's rule that a
// symlink or non-regular object at any owned cache path is refused rather than
// followed. The canonical database is absent, as it is on a first launch, so
// the refusal must come from an owned path inspected after it.
func TestOpenRejectsANonRegularObjectAtALaterOwnedPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally permitted on Windows")
	}
	t.Parallel()

	location := stagedLocation(t)
	target := filepath.Join(t.TempDir(), "elsewhere")
	stageFile(t, target, "")
	if err := os.Symlink(target, location.Tombstone()); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if _, err := Open(location); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Open() error = %v, want ErrUnavailable for a symlinked owned path", err)
	}
	if _, err := os.Lstat(location.Tombstone()); err != nil {
		t.Errorf("the rejected symlink must be preserved, lstat error = %v", err)
	}
	if _, err := os.Lstat(location.Database()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("no database may be created past a rejected owned path, lstat error = %v", err)
	}
}

// TestOpenRejectsACacheFileWithMoreThanOneLink guards RG-005's ownership rule
// that a mutable cache file must have exactly one link, so no other name
// aliases the inode OrgTop writes evidence into.
func TestOpenRejectsACacheFileWithMoreThanOneLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX link counts are not the Windows ownership contract")
	}
	t.Parallel()

	store, location := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	alias := filepath.Join(filepath.Dir(location.Directory()), "alias.db")
	if err := os.Link(location.Database(), alias); err != nil {
		t.Fatalf("Link() error = %v", err)
	}

	if _, err := Open(location); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Open() error = %v, want ErrUnavailable for an aliased cache file", err)
	}
}

// TestOpenNarrowsABroaderExistingDatabaseFile guards RG-005's POSIX mode
// contract for the mutable files themselves: a group- or world-accessible
// database is narrowed to 0600 before use rather than trusted.
func TestOpenNarrowsABroaderExistingDatabaseFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes are not the Windows ownership contract")
	}
	t.Parallel()

	store, location := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := os.Chmod(location.Database(), 0o644); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}

	reopened, err := Open(location)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = reopened.Close() }()

	if got, want := storeMutPermissions(t, location.Database()), os.FileMode(0o600); got != want {
		t.Errorf("database mode = %04o, want %04o", got, want)
	}
}

// TestOpenNarrowsALaterOwnedFileBeyondNarrowAndAbsentOnes guards the same mode
// contract for every owned file rather than the first one: the maintenance lock
// is narrowed even though the database before it needs no repair and the
// sidecars between them are absent.
func TestOpenNarrowsALaterOwnedFileBeyondNarrowAndAbsentOnes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes are not the Windows ownership contract")
	}
	t.Parallel()

	store, location := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := os.Chmod(location.Lock(), 0o666); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	if got, want := storeMutPermissions(t, location.Database()), os.FileMode(0o600); got != want {
		t.Fatalf("the database must already be narrow for this case, mode = %04o, want %04o", got, want)
	}

	reopened, err := Open(location)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = reopened.Close() }()

	if got, want := storeMutPermissions(t, location.Lock()), os.FileMode(0o600); got != want {
		t.Errorf("maintenance lock mode = %04o, want %04o", got, want)
	}
}

// TestOpenClearsATombstoneLeftBesideAnExistingDatabase guards RG-005's restart
// rule: a tombstone is an unfinished reset and forces the sidecar-first cleanup
// before any canonical open, even when a canonical database is already present.
func TestOpenClearsATombstoneLeftBesideAnExistingDatabase(t *testing.T) {
	t.Parallel()

	store, location := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	orphan := location.Database() + "-wal"
	stageFile(t, orphan, "stale wal")
	stageFile(t, location.Tombstone(), "tombstone")

	reopened, err := Open(location)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = reopened.Close() }()

	if _, err := os.Lstat(location.Tombstone()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the tombstone must be removed before the canonical database is opened, lstat error = %v", err)
	}
	if data, err := os.ReadFile(orphan); err == nil && string(data) == "stale wal" {
		t.Error("the canonical database was opened beside the stale write-ahead log")
	}
}

// TestUnreadableCanonicalFileIsUnavailableRatherThanForeign guards RG-005's
// separation of degraded causes: a canonical file this process may not read is
// an unavailable cache with permission guidance, never a foreign database that
// would be reported as another application's file.
func TestUnreadableCanonicalFileIsUnavailableRatherThanForeign(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes are not the Windows ownership contract")
	}
	if os.Geteuid() == 0 {
		t.Skip("the superuser reads every mode")
	}
	t.Parallel()

	location := stagedLocation(t)
	stageFile(t, location.Database(), strings.Repeat("d", headerLength))
	if err := os.Chmod(location.Database(), 0o000); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	root, err := openCacheRoot(location)
	if err != nil {
		t.Fatalf("openCacheRoot() error = %v", err)
	}
	defer func() { _ = root.Close() }()

	err = (&Store{location: location, root: root}).checkHeader()
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("checkHeader() error = %v, want ErrUnavailable", err)
	}
	if errors.Is(err, ErrForeignDatabase) {
		t.Errorf("checkHeader() error = %v, want an unreadable file reported as unavailable rather than foreign", err)
	}
}

// TestCloseReportsAFailureToReleaseTheMaintenanceLock guards RG-005's
// requirement that no lock handle outlives the cache: a release this process
// cannot complete is reported honestly rather than swallowed by the handles
// that were released afterwards.
func TestCloseReportsAFailureToReleaseTheMaintenanceLock(t *testing.T) {
	t.Parallel()

	store, _ := openTestStore(t)
	// Spending the counted descriptor behind the store's back is what a lock
	// release that cannot complete looks like from Close's side.
	if err := store.lock.file.Close(); err != nil {
		t.Fatalf("closing the lock descriptor error = %v", err)
	}

	if err := store.Close(); err == nil {
		t.Error("Close() error = nil, want the failed lock release reported")
	}
}

// TestCloseSpendsEveryCacheHandle guards RG-005's rule that no database or lock
// handle remains open once shared lifecycle access is released.
func TestCloseSpendsEveryCacheHandle(t *testing.T) {
	t.Parallel()

	store, _ := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if store.db != nil {
		t.Error("the database handle outlived Close()")
	}
	if store.lock != nil {
		t.Error("the maintenance lock outlived Close()")
	}
	if store.root != nil {
		t.Error("the cache directory handle outlived Close()")
	}
}

// TestTheLastStoreReleasesTheProcessLockDescriptor guards the counted
// descriptor RG-005's cross-process admission depends on: an earlier store
// closing must keep the shared descriptor open, and the last one closing must
// release it so no lock handle outlives the cache.
func TestTheLastStoreReleasesTheProcessLockDescriptor(t *testing.T) {
	t.Parallel()

	first, location := openTestStore(t)
	second, err := Open(location)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}

	if err := second.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if !storeMutLockDescriptorIsOpen(location) {
		t.Fatal("the shared lock descriptor was released while a store still used it")
	}

	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if storeMutLockDescriptorIsOpen(location) {
		t.Error("the shared lock descriptor outlived the last store using it")
	}
}

// TestCompareKeyRejectsEitherInvalidObject guards RG-005's compare identity:
// both sides are exact normalized objects, so one malformed side alone is
// enough to refuse the key rather than deriving one with an empty base.
func TestCompareKeyRejectsEitherInvalidObject(t *testing.T) {
	t.Parallel()

	repository := testRepository(t, "owner", "repo")

	if key, err := CompareKey(repository, "nope", headSHA); !errors.Is(err, domain.ErrInvalidEvidence) {
		t.Errorf("CompareKey(bad base) = %+v, error = %v, want ErrInvalidEvidence", key, err)
	}
	if key, err := CompareKey(repository, baseSHA, "nope"); !errors.Is(err, domain.ErrInvalidEvidence) {
		t.Errorf("CompareKey(bad head) = %+v, error = %v, want ErrInvalidEvidence", key, err)
	}
}

// TestSaveAcceptsAnEntityAtExactlyTheByteBounds guards the closed RG-005 bounds
// at their boundary: 4,096 bytes in one path and 1 MiB across one evidence set
// are storable, and only more than that is refused.
func TestSaveAcceptsAnEntityAtExactlyTheByteBounds(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	entry := compareEntry(t)
	entry.Paths = storeMutMaximalPaths(t)

	if err := store.Save(context.Background(), entry); err != nil {
		t.Fatalf("Save() error = %v, want a set at exactly the closed byte bounds stored", err)
	}
	got, ok, err := store.Lookup(context.Background(), entry.Key)
	if err != nil || !ok {
		t.Fatalf("Lookup() ok = %v, error = %v, want the stored set", ok, err)
	}
	if len(got.Paths) != len(entry.Paths) {
		t.Errorf("Paths = %d, want the complete %d", len(got.Paths), len(entry.Paths))
	}
}

// TestSaveAcceptsAnEpochRefreshClock guards RG-005's timestamp rule: only a
// negative Unix second predates the epoch, so a refresh clock reading exactly
// the epoch is a usable record rather than an invalid one.
func TestSaveAcceptsAnEpochRefreshClock(t *testing.T) {
	t.Parallel()

	store, location := openTestStore(t)
	epoch := time.Unix(0, 0).UTC()
	entry := compareEntry(t, "src/main.go")

	if err := store.WithClock(func() time.Time { return epoch }).Save(context.Background(), entry); err != nil {
		t.Fatalf("Save() error = %v, want the epoch accepted as a valid time", err)
	}
	if got := queryScalar[int64](t, location, "SELECT acquired_at FROM evidence"); got != 0 {
		t.Errorf("acquired_at = %d, want the epoch stored as 0", got)
	}
}

// TestLookupRejectsStoredPathsOutOfAscendingOrder guards RG-005's storage
// order: a hit requires unique paths in ascending normalized byte order, so a
// record whose stored order was damaged is a whole-record miss.
func TestLookupRejectsStoredPathsOutOfAscendingOrder(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	entry := compareEntry(t, "a/one.go", "b/two.go", "c/three.go")
	if err := store.Save(context.Background(), entry); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	execDatabase(t, location, "UPDATE evidence_path SET path = 'z/last.go' WHERE ordinal = 0")

	if _, ok, err := store.Lookup(context.Background(), entry.Key); ok {
		t.Errorf("Lookup() ok = true, want a miss for paths out of ascending order (error = %v)", err)
	}
}

// TestResetRemovesAnOwnedBootstrapFile guards RG-005's reset procedure: an
// interrupted initialization file OrgTop owns is removed by `--reset-cache`
// together with the database and its sidecars, so the next launch starts from
// no cache state at all.
func TestResetRemovesAnOwnedBootstrapFile(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	seedEntry(t, store, "kept", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	stageFile(t, location.Bootstrap(), "partial")

	if err := Reset(location); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	if _, err := os.Lstat(location.Bootstrap()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the owned bootstrap file survived the reset, lstat error = %v", err)
	}
	if _, err := os.Lstat(location.Database()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the database survived the reset, lstat error = %v", err)
	}
}
