package cache

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// smallBounds shrinks the closed row limits so every boundary can be proven
// with a temporary store instead of a 128 MiB fixture. A test that needs one
// more boundary sharpened adjusts the returned value. Production always uses
// defaultBounds.
func smallBounds() bounds {
	limits := defaultBounds()
	limits.evidenceRecords = 4
	limits.childRecords = 40
	return limits
}

// seedEntry saves one record under a distinct key with explicit times.
func seedEntry(t *testing.T, store *Store, name string, acquired, lastUsed time.Time, paths ...string) Entry {
	t.Helper()

	key, err := CompareKey(testRepository(t, "owner", name), baseSHA, headSHA)
	if err != nil {
		t.Fatalf("CompareKey() error = %v", err)
	}
	entry := Entry{Key: key, Paths: changedPaths(t, paths...), AcquiredAt: acquired, LastUsedAt: lastUsed}
	if err := store.Save(context.Background(), entry); err != nil {
		t.Fatalf("Save(%q) error = %v", name, err)
	}
	return entry
}

// repositoryKeys reports the stored repository keys in ascending order, so a
// test can assert exactly which records survived a batch.
func repositoryKeys(t *testing.T, location Location) []string {
	t.Helper()

	return queryStrings(t, location, "SELECT repository_key FROM evidence ORDER BY repository_key")
}

// TestMaintainDeletesQueuedInvalidRecordsFirst proves a record whose invariants
// failed on read is queued and removed ahead of expired and evictable records.
func TestMaintainDeletesQueuedInvalidRecordsFirst(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	limits := smallBounds()
	limits.evidenceBatch = 1
	store = store.withBounds(limits)
	broken := seedEntry(t, store, "broken", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
	seedEntry(t, store, "expired", referenceTime.Add(-evidenceTTL), referenceTime.Add(-evidenceTTL), "b.go")
	execDatabase(t, location, "DELETE FROM evidence_path WHERE evidence_id = (SELECT id FROM evidence WHERE repository_key = 'owner/broken')")

	if _, ok, err := store.Lookup(context.Background(), broken.Key); ok || !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("Lookup() ok = %v, error = %v, want an invalid-record miss", ok, err)
	}
	report, err := store.Maintain(context.Background())
	if err != nil {
		t.Fatalf("Maintain() error = %v", err)
	}
	if report.DeletedInvalid != 1 || report.DeletedExpired != 0 || report.Evicted != 0 {
		t.Fatalf("report = %+v, want exactly the queued invalid record deleted", report)
	}
	if got, want := repositoryKeys(t, location), []string{"owner/expired"}; !slices.Equal(got, want) {
		t.Errorf("surviving records = %v, want %v", got, want)
	}
}

// TestMaintainDeletesExpiredBeforeEvicting proves the closed order: expired
// records go before any least-recently-used eviction.
func TestMaintainDeletesExpiredBeforeEvicting(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	limits := smallBounds()
	limits.evidenceBatch = 1
	store = store.withBounds(limits)
	seedEntry(t, store, "expired", referenceTime.Add(-evidenceTTL), referenceTime.Add(-time.Minute), "a.go")
	seedEntry(t, store, "fresh", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "b.go")

	report, err := store.Maintain(context.Background())
	if err != nil {
		t.Fatalf("Maintain() error = %v", err)
	}
	if report.DeletedExpired != 1 || report.Evicted != 0 {
		t.Fatalf("report = %+v, want the expired record deleted first", report)
	}
	if got, want := repositoryKeys(t, location), []string{"owner/fresh"}; !slices.Equal(got, want) {
		t.Errorf("surviving records = %v, want %v", got, want)
	}
}

// TestMaintainEvictsLeastRecentlyUsedToTheTarget proves eviction orders by
// last_used_at, then acquired_at, then the stable typed key, and stops at the
// 75% target rather than emptying the store.
func TestMaintainEvictsLeastRecentlyUsedToTheTarget(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	store = store.withBounds(smallBounds())
	// The 75% target of four records is three, so exactly one record is evicted.
	seedEntry(t, store, "aaa", referenceTime.Add(-4*time.Hour), referenceTime.Add(-time.Hour), "a.go")
	seedEntry(t, store, "bbb", referenceTime.Add(-3*time.Hour), referenceTime.Add(-time.Hour), "b.go")
	seedEntry(t, store, "ccc", referenceTime.Add(-2*time.Hour), referenceTime.Add(-time.Minute), "c.go")
	seedEntry(t, store, "ddd", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Minute), "d.go")

	report, err := store.Maintain(context.Background())
	if err != nil {
		t.Fatalf("Maintain() error = %v", err)
	}
	if report.Evicted != 1 {
		t.Fatalf("report = %+v, want exactly one eviction to the 75%% target", report)
	}
	// aaa and bbb tie on last_used_at; aaa has the older acquisition time.
	if got, want := repositoryKeys(t, location), []string{"owner/bbb", "owner/ccc", "owner/ddd"}; !slices.Equal(got, want) {
		t.Errorf("surviving records = %v, want %v", got, want)
	}
}

// TestMaintainEvictionTieBreaksOnTheStableTypedKey proves records that tie on
// both timestamps are evicted in ascending typed-key order.
func TestMaintainEvictionTieBreaksOnTheStableTypedKey(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	store = store.withBounds(smallBounds())
	same := referenceTime.Add(-time.Hour)
	for _, name := range []string{"aaa", "bbb", "ccc", "ddd"} {
		seedEntry(t, store, name, same, same, name+".go")
	}

	if _, err := store.Maintain(context.Background()); err != nil {
		t.Fatalf("Maintain() error = %v", err)
	}
	if got, want := repositoryKeys(t, location), []string{"owner/bbb", "owner/ccc", "owner/ddd"}; !slices.Equal(got, want) {
		t.Errorf("surviving records = %v, want the lowest typed key evicted, got %v", got, want)
	}
}

// TestMaintainStopsAtTheBatchBound proves one refresh performs one bounded
// batch and the next refresh continues the same deterministic order.
func TestMaintainStopsAtTheBatchBound(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	limits := smallBounds()
	limits.evidenceBatch = 2
	store = store.withBounds(limits)
	for _, name := range []string{"aaa", "bbb", "ccc"} {
		seedEntry(t, store, name, referenceTime.Add(-evidenceTTL), referenceTime.Add(-evidenceTTL), name+".go")
	}

	first, err := store.Maintain(context.Background())
	if err != nil {
		t.Fatalf("Maintain() error = %v", err)
	}
	if first.DeletedExpired != 2 {
		t.Fatalf("first batch = %+v, want the batch bound of 2", first)
	}
	if got, want := repositoryKeys(t, location), []string{"owner/ccc"}; !slices.Equal(got, want) {
		t.Errorf("surviving records = %v, want %v", got, want)
	}
	second, err := store.Maintain(context.Background())
	if err != nil {
		t.Fatalf("second Maintain() error = %v", err)
	}
	if second.DeletedExpired != 1 {
		t.Errorf("second batch = %+v, want the remaining expired record", second)
	}
}

// TestMaintainShortensSelectionAtTheStoredPathByteBound proves selection stops
// at the stored-path byte bound even when the record batch is not spent.
func TestMaintainShortensSelectionAtTheStoredPathByteBound(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	limits := smallBounds()
	limits.selectionPathBytes = 6
	store = store.withBounds(limits)
	for _, name := range []string{"aaa", "bbb", "ccc"} {
		seedEntry(t, store, name, referenceTime.Add(-evidenceTTL), referenceTime.Add(-evidenceTTL), name+".go")
	}

	report, err := store.Maintain(context.Background())
	if err != nil {
		t.Fatalf("Maintain() error = %v", err)
	}
	if report.DeletedExpired != 1 {
		t.Fatalf("report = %+v, want selection shortened to one record", report)
	}
	if got, want := repositoryKeys(t, location), []string{"owner/bbb", "owner/ccc"}; !slices.Equal(got, want) {
		t.Errorf("surviving records = %v, want %v", got, want)
	}
}

// TestMaintainTruncatesTheWriteAheadLog proves a committed batch is followed by
// bounded incremental vacuum and one truncate checkpoint.
func TestMaintainTruncatesTheWriteAheadLog(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	store = store.withBounds(smallBounds())
	for _, name := range []string{"aaa", "bbb", "ccc", "ddd"} {
		seedEntry(t, store, name, referenceTime.Add(-evidenceTTL), referenceTime.Add(-evidenceTTL), name+".go")
	}
	before, err := statSize(location.Database() + "-wal")
	if err != nil {
		t.Fatalf("statSize() error = %v", err)
	}
	if before == 0 {
		t.Fatalf("the fixture needs a nonempty write-ahead log")
	}

	if _, err := store.Maintain(context.Background()); err != nil {
		t.Fatalf("Maintain() error = %v", err)
	}
	after, err := statSize(location.Database() + "-wal")
	if err != nil {
		t.Fatalf("statSize() error = %v", err)
	}
	if after != 0 {
		t.Errorf("write-ahead log = %d bytes, want a truncating checkpoint to empty it", after)
	}
}

// TestMaintenanceDueReportsEveryTrigger proves expiry, either hard logical
// limit, and the physical trigger each request maintenance.
func TestMaintenanceDueReportsEveryTrigger(t *testing.T) {
	t.Parallel()

	t.Run("quiet store", func(t *testing.T) {
		t.Parallel()

		store, _ := fixedClockStore(t)
		store = store.withBounds(smallBounds())
		seedEntry(t, store, "fresh", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")

		due, err := store.MaintenanceDue(context.Background())
		if err != nil {
			t.Fatalf("MaintenanceDue() error = %v", err)
		}
		if due {
			t.Errorf("MaintenanceDue() = true, want no trigger below every bound")
		}
	})

	t.Run("expiry", func(t *testing.T) {
		t.Parallel()

		store, _ := fixedClockStore(t)
		store = store.withBounds(smallBounds())
		seedEntry(t, store, "expired", referenceTime.Add(-evidenceTTL), referenceTime.Add(-evidenceTTL), "a.go")

		due, err := store.MaintenanceDue(context.Background())
		if err != nil || !due {
			t.Errorf("MaintenanceDue() = %v, error = %v, want an expiry trigger", due, err)
		}
	})

	t.Run("evidence rows", func(t *testing.T) {
		t.Parallel()

		store, _ := fixedClockStore(t)
		store = store.withBounds(smallBounds())
		for _, name := range []string{"aaa", "bbb", "ccc", "ddd"} {
			seedEntry(t, store, name, referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), name+".go")
		}

		due, err := store.MaintenanceDue(context.Background())
		if err != nil || !due {
			t.Errorf("MaintenanceDue() = %v, error = %v, want a row-limit trigger", due, err)
		}
	})

	t.Run("physical bytes", func(t *testing.T) {
		t.Parallel()

		store, _ := fixedClockStore(t)
		seedEntry(t, store, "fresh", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
		limits := smallBounds()
		limits.retainedBytes = 1
		limits.temporaryBytes = 1 << 30
		store = store.withBounds(limits)

		due, err := store.MaintenanceDue(context.Background())
		if err != nil || !due {
			t.Errorf("MaintenanceDue() = %v, error = %v, want a physical trigger", due, err)
		}
	})
}

// TestSaveIsSkippedWhenAdmissionCannotReserve proves a write that cannot prove
// its temporary reservation is skipped and leaves the stored record intact.
func TestSaveIsSkippedWhenAdmissionCannotReserve(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	kept := seedEntry(t, store, "kept", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
	limits := smallBounds()
	limits.temporaryBytes = limits.writeReservation // no room for the reservation
	store = store.withBounds(limits)

	err := store.Save(context.Background(), Entry{
		Key:        kept.Key,
		Paths:      changedPaths(t, "b.go"),
		AcquiredAt: referenceTime,
		LastUsedAt: referenceTime,
	})
	if !errors.Is(err, ErrOverCapacity) {
		t.Fatalf("Save() error = %v, want ErrOverCapacity", err)
	}
	if got := queryScalar[int](t, location, "SELECT count(*) FROM evidence_path WHERE path = 'a.go'"); got != 1 {
		t.Errorf("the previously stored record must survive a skipped write, path rows = %d", got)
	}
}

// TestSaveRejectsAProjectionAboveAHardRowLimit proves admission rejects a write
// projected above a hard logical limit.
func TestSaveRejectsAProjectionAboveAHardRowLimit(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	limits := smallBounds()
	limits.evidenceRecords = 1
	store = store.withBounds(limits)
	seedEntry(t, store, "first", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")

	key, err := CompareKey(testRepository(t, "owner", "second"), baseSHA, headSHA)
	if err != nil {
		t.Fatalf("CompareKey() error = %v", err)
	}
	err = store.Save(context.Background(), Entry{
		Key:        key,
		Paths:      changedPaths(t, "b.go"),
		AcquiredAt: referenceTime,
		LastUsedAt: referenceTime,
	})
	if !errors.Is(err, ErrOverCapacity) {
		t.Errorf("Save() error = %v, want ErrOverCapacity", err)
	}
}

// TestOversizedCacheAdmitsNoHitOrMutation proves a cache already above the
// retained ceiling admits nothing and directs the user to --reset-cache.
func TestOversizedCacheAdmitsNoHitOrMutation(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	entry := seedEntry(t, store, "kept", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
	limits := smallBounds()
	limits.retainedBytes = 1
	store = store.withBounds(limits)

	_, ok, err := store.Lookup(context.Background(), entry.Key)
	if ok || !errors.Is(err, ErrOverCapacity) {
		t.Fatalf("Lookup() ok = %v, error = %v, want ErrOverCapacity", ok, err)
	}
	if !strings.Contains(err.Error(), "--reset-cache") {
		t.Errorf("Lookup() error = %q, want it to direct the user to --reset-cache", err)
	}
	if err := store.Save(context.Background(), entry); !errors.Is(err, ErrOverCapacity) {
		t.Errorf("Save() error = %v, want ErrOverCapacity", err)
	}
}

// TestMaintainKeepsEvidenceUsableAfterCleanup proves cleanup returns the store
// to the documented bounds and leaves surviving evidence a complete hit.
func TestMaintainKeepsEvidenceUsableAfterCleanup(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	store = store.withBounds(smallBounds())
	var survivor Entry
	for index, name := range []string{"aaa", "bbb", "ccc", "ddd"} {
		entry := seedEntry(t, store, name,
			referenceTime.Add(-time.Duration(4-index)*time.Hour),
			referenceTime.Add(-time.Duration(4-index)*time.Hour),
			fmt.Sprintf("%s.go", name))
		survivor = entry
	}

	report, err := store.Maintain(context.Background())
	if err != nil {
		t.Fatalf("Maintain() error = %v", err)
	}
	if report.EvidenceAfter > store.limits.evidenceTarget() {
		t.Errorf("evidence rows after cleanup = %d, want at most the 75%% target %d",
			report.EvidenceAfter, store.limits.evidenceTarget())
	}
	got, ok, err := store.Lookup(context.Background(), survivor.Key)
	if !ok || err != nil {
		t.Fatalf("Lookup() ok = %v, error = %v, want the survivor to remain a hit", ok, err)
	}
	if len(got.Paths) != 1 || got.Paths[0].String() != "ddd.go" {
		t.Errorf("survivor paths = %v, want the complete stored set", pathStrings(got.Paths))
	}
}

func pathStrings(paths []domain.ChangedPath) []string {
	values := make([]string, 0, len(paths))
	for _, path := range paths {
		values = append(values, path.String())
	}
	return values
}

// TestTouchRetainsHitsWhenTheBatchIsSkipped proves a skipped touch keeps its
// queued hits: a contended or over-capacity refresh may try again, and a later
// Touch must not report success for an update that never ran.
func TestTouchRetainsHitsWhenTheBatchIsSkipped(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	entry := seedEntry(t, store, "kept", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
	if _, ok, err := store.Lookup(context.Background(), entry.Key); !ok || err != nil {
		t.Fatalf("Lookup() ok = %v, error = %v, want a hit", ok, err)
	}

	limits := smallBounds()
	limits.temporaryBytes = limits.writeReservation
	store = store.withBounds(limits)
	if err := store.Touch(context.Background()); !errors.Is(err, ErrOverCapacity) {
		t.Fatalf("Touch() error = %v, want ErrOverCapacity", err)
	}
	if got := store.pendingTouches(); got != 1 {
		t.Fatalf("pending touches = %d, want the skipped hit retained", got)
	}

	store = store.withBounds(smallBounds())
	if err := store.Touch(context.Background()); err != nil {
		t.Fatalf("retried Touch() error = %v", err)
	}
	if got, want := queryScalar[int64](t, location, "SELECT last_used_at FROM evidence"), referenceTime.Unix(); got != want {
		t.Errorf("last_used_at = %d, want the retried touch to apply %d", got, want)
	}
}

// TestAdmissionIsProvenUnderTheAdmissionRegion proves the physical admission
// proof happens under the exclusive admission region, so two processes cannot
// both pass admission against the same stale byte figures.
func TestAdmissionIsProvenUnderTheAdmissionRegion(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	entry := seedEntry(t, store, "kept", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
	limits := smallBounds()
	limits.retainedBytes = 1
	store = store.withBounds(limits)

	// A second launch holding the region makes every admitted operation
	// contended. An over-capacity verdict here would prove the byte proof ran
	// before the region was held.
	holder, err := Open(location)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = holder.Close() })
	if err := holder.lock.acquire(admissionRegion, true, lockWaits.busy); err != nil {
		t.Fatalf("acquire(admissionRegion) error = %v", err)
	}
	defer holder.lock.release(admissionRegion)

	if _, _, err := store.Lookup(context.Background(), entry.Key); !errors.Is(err, ErrContended) {
		t.Errorf("Lookup() error = %v, want ErrContended", err)
	}
	if err := store.Save(context.Background(), entry); !errors.Is(err, ErrContended) {
		t.Errorf("Save() error = %v, want ErrContended", err)
	}
	if _, err := store.Maintain(context.Background()); !errors.Is(err, ErrContended) {
		t.Errorf("Maintain() error = %v, want ErrContended", err)
	}
}

// TestDisabledCacheStopsEveryOperation proves the disabled state is sticky for
// the process: no read, write, touch, or maintenance resumes without a reset.
func TestDisabledCacheStopsEveryOperation(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	entry := seedEntry(t, store, "kept", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
	store.queueTouch(entry.Key)
	store.disabled = true

	if _, _, err := store.Lookup(context.Background(), entry.Key); !errors.Is(err, ErrOverCapacity) {
		t.Errorf("Lookup() error = %v, want ErrOverCapacity", err)
	}
	if err := store.Save(context.Background(), entry); !errors.Is(err, ErrOverCapacity) {
		t.Errorf("Save() error = %v, want ErrOverCapacity", err)
	}
	if err := store.Touch(context.Background()); !errors.Is(err, ErrOverCapacity) {
		t.Errorf("Touch() error = %v, want ErrOverCapacity", err)
	}
	if _, err := store.Maintain(context.Background()); !errors.Is(err, ErrOverCapacity) {
		t.Errorf("Maintain() error = %v, want ErrOverCapacity", err)
	}
	if _, err := store.MaintenanceDue(context.Background()); !errors.Is(err, ErrOverCapacity) {
		t.Errorf("MaintenanceDue() error = %v, want ErrOverCapacity", err)
	}
}

// TestAdmissionAllowsExactlyTheRetainedCeiling proves the physical ceiling is
// inclusive: a store exactly at the ceiling still admits work, and one byte
// above it admits none.
func TestAdmissionAllowsExactlyTheRetainedCeiling(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	entry := seedEntry(t, store, "kept", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
	used, err := store.PhysicalBytes()
	if err != nil {
		t.Fatalf("PhysicalBytes() error = %v", err)
	}

	limits := smallBounds()
	limits.retainedBytes = used
	if _, ok, err := store.withBounds(limits).Lookup(context.Background(), entry.Key); !ok || err != nil {
		t.Fatalf("Lookup() ok = %v, error = %v, want a hit exactly at the ceiling", ok, err)
	}
	limits.retainedBytes = used - 1
	if _, ok, err := store.withBounds(limits).Lookup(context.Background(), entry.Key); ok || !errors.Is(err, ErrOverCapacity) {
		t.Errorf("Lookup() ok = %v, error = %v, want ErrOverCapacity one byte above the ceiling", ok, err)
	}
}

// TestMaintainKeepsQueuedInvalidRecordsThatDidNotFitTheBatch proves invalid
// records the batch budget could not reach keep their first-place priority.
// Nothing else rediscovers them: invalid tracking exists only for this process.
func TestMaintainKeepsQueuedInvalidRecordsThatDidNotFitTheBatch(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	limits := smallBounds()
	limits.evidenceBatch = 1
	store = store.withBounds(limits)
	first := seedEntry(t, store, "aaa", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
	second := seedEntry(t, store, "bbb", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "b.go")
	execDatabase(t, location, "DELETE FROM evidence_path")

	for _, key := range []Key{first.Key, second.Key} {
		if _, ok, err := store.Lookup(context.Background(), key); ok || !errors.Is(err, ErrInvalidRecord) {
			t.Fatalf("Lookup() ok = %v, error = %v, want an invalid-record miss", ok, err)
		}
	}
	report, err := store.Maintain(context.Background())
	if err != nil {
		t.Fatalf("Maintain() error = %v", err)
	}
	if report.DeletedInvalid != 1 {
		t.Fatalf("report = %+v, want the batch bound of one invalid record", report)
	}
	if got := len(store.invalid); got != 1 {
		t.Fatalf("queued invalid records = %d, want the unreached record retained", got)
	}
	due, err := store.MaintenanceDue(context.Background())
	if err != nil || !due {
		t.Fatalf("MaintenanceDue() = %v, error = %v, want the retained record to request maintenance", due, err)
	}
	next, err := store.Maintain(context.Background())
	if err != nil {
		t.Fatalf("second Maintain() error = %v", err)
	}
	if next.DeletedInvalid != 1 {
		t.Errorf("second batch = %+v, want the retained invalid record deleted first", next)
	}
	if got := len(store.invalid); got != 0 {
		t.Errorf("queued invalid records = %d, want the queue drained", got)
	}
}
