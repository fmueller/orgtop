package cache

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// TestClosingASecondHandleKeepsTheHeldRegion proves the cross-process admission
// invariant survives a second store handle being closed in this process. POSIX
// record locks belong to the process, not to one descriptor, so a naive second
// lock file would release the regions the first store still holds and let a
// genuine second process mutate the cache at the same time.
func TestClosingASecondHandleKeepsTheHeldRegion(t *testing.T) {
	root := t.TempDir()

	store, err := Open(LocationIn(root))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = store.Close() }()
	if err := store.lock.acquire(admissionRegion, true, busyWait); err != nil {
		t.Fatalf("acquire admission error = %v", err)
	}
	defer store.lock.release(admissionRegion)

	// A second handle on the same cache, opened and closed while the first one
	// still holds exclusive admission.
	second, err := Open(LocationIn(root))
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	if code := runContenderProcess(t, root); code != contenderContended {
		t.Errorf("a separate process reported exit %d, want %d (contended)", code, contenderContended)
	}
}

// Exit codes the contender child process reports back to the parent.
const (
	contenderContended = 0
	contenderMutated   = 1
	contenderFailed    = 2
)

const contendEnvironment = "ORGTOP_CACHE_LOCK_CONTENDER"

// runContenderProcess re-executes this binary as an independent process that
// tries one cache mutation and reports whether admission refused it.
func runContenderProcess(t *testing.T, root string) int {
	t.Helper()

	contender := exec.Command(os.Args[0], "-test.run", "TestMain")
	contender.Env = append(os.Environ(), contendEnvironment+"="+root)
	err := contender.Run()
	if err == nil {
		return contenderContended
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	t.Fatalf("running the contender process failed: %v", err)
	return contenderFailed
}

// attemptMutation opens the cache at root and reports whether exclusive
// admission refused one write.
func attemptMutation(root string) int {
	store, err := Open(LocationIn(root))
	if err != nil {
		return contenderFailed
	}
	defer func() { _ = store.Close() }()

	repository, err := domain.ParseRepository("owner/repo")
	if err != nil {
		return contenderFailed
	}
	key, err := CompareKey(repository, baseSHA, headSHA)
	if err != nil {
		return contenderFailed
	}
	err = store.Save(context.Background(), Entry{Key: key})
	if errors.Is(err, ErrContended) {
		return contenderContended
	}
	return contenderMutated
}

// TestSaveRejectsAnEntitySpanningTooManyBytes proves the aggregate per-entity
// byte bound: a set whose paths are each within the per-path limit but together
// exceed 1 MiB is incomplete rather than truncated.
func TestSaveRejectsAnEntitySpanningTooManyBytes(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	key, err := CompareKey(testRepository(t, "owner", "repo"), baseSHA, headSHA)
	if err != nil {
		t.Fatalf("CompareKey() error = %v", err)
	}

	// Each path stays well under the 4,096-byte per-path bound while the set
	// crosses the 1 MiB per-entity bound.
	const segment = 2048
	paths := make([]domain.ChangedPath, 0, domain.MaxEvidencePaths)
	for index := 0; len(paths) < domain.MaxEvidencePaths; index++ {
		value := strconv.Itoa(index) + "/" + strings.Repeat("p", segment)
		path, err := domain.NewChangedPath(value)
		if err != nil {
			t.Fatalf("NewChangedPath() error = %v", err)
		}
		paths = append(paths, path)
	}

	err = store.Save(context.Background(), Entry{Key: key, Paths: paths})
	if !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("Save() error = %v, want ErrInvalidRecord for a set beyond %d bytes", err, domain.MaxEvidenceBytes)
	}
}

// TestReplacementNeverMovesAcquisitionBackward proves exact-key replacement
// takes the maximum of the refresh time and the valid stored timestamps, so a
// racing refresh with an earlier clock cannot rewind the acquisition time a
// record's age is measured from.
func TestReplacementNeverMovesAcquisitionBackward(t *testing.T) {
	t.Parallel()

	store, _ := openTestStore(t)
	key, err := CompareKey(testRepository(t, "owner", "repo"), baseSHA, headSHA)
	if err != nil {
		t.Fatalf("CompareKey() error = %v", err)
	}

	later := store.WithClock(func() time.Time { return referenceTime })
	if err := later.Save(context.Background(), Entry{Key: key, Paths: changedPaths(t, "a/one.go")}); err != nil {
		t.Fatalf("Save(later) error = %v", err)
	}

	// A concurrent refresh runs a little behind. Its own stored view stays
	// inside the closed forward-skew window, so it is a valid concurrent
	// timestamp rather than a discarded far-future one.
	earlier := referenceTime.Add(-2 * time.Minute)
	behind := store.WithClock(func() time.Time { return earlier })
	if err := behind.Save(context.Background(), Entry{Key: key, Paths: changedPaths(t, "b/two.go")}); err != nil {
		t.Fatalf("Save(earlier) error = %v", err)
	}

	got, ok, err := store.WithClock(func() time.Time { return referenceTime }).Lookup(context.Background(), key)
	if err != nil || !ok {
		t.Fatalf("Lookup() ok = %v, error = %v, want the replacement", ok, err)
	}
	if got.AcquiredAt.Before(referenceTime) {
		t.Errorf("AcquiredAt = %v, want no earlier than the previously stored %v", got.AcquiredAt, referenceTime)
	}
	if len(got.Paths) != 1 || got.Paths[0].String() != "b/two.go" {
		t.Errorf("Paths = %v, want the replacement [b/two.go]", got.Paths)
	}
}

// TestReplacementDiscardsInvalidStoredTimestamps proves a negative, overflowing,
// or too-far-future stored timestamp is discarded by replacement rather than
// carried forward as the new maximum.
func TestReplacementDiscardsInvalidStoredTimestamps(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	key, err := CompareKey(testRepository(t, "owner", "repo"), baseSHA, headSHA)
	if err != nil {
		t.Fatalf("CompareKey() error = %v", err)
	}
	if err := store.Save(context.Background(), Entry{Key: key, Paths: changedPaths(t, "a/one.go")}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	execDatabase(t, location, "UPDATE evidence SET acquired_at = 253402300799, last_used_at = -1")

	if err := store.Save(context.Background(), Entry{Key: key, Paths: changedPaths(t, "b/two.go")}); err != nil {
		t.Fatalf("Save(replacement) error = %v", err)
	}
	got, ok, err := store.Lookup(context.Background(), key)
	if err != nil || !ok {
		t.Fatalf("Lookup() ok = %v, error = %v, want the replacement", ok, err)
	}
	if !got.AcquiredAt.Equal(referenceTime) {
		t.Errorf("AcquiredAt = %v, want the refresh time %v", got.AcquiredAt, referenceTime)
	}
	if !got.LastUsedAt.Equal(referenceTime) {
		t.Errorf("LastUsedAt = %v, want the refresh time %v", got.LastUsedAt, referenceTime)
	}
}
