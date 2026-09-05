package cache

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// maintMutPathSet builds count distinct changed paths whose lengths sum to
// exactly total bytes, so a test can state the growth one replacement projects.
func maintMutPathSet(t *testing.T, count, total int) []domain.ChangedPath {
	t.Helper()

	values := make([]string, 0, count)
	remaining := total
	for index := 0; index < count; index++ {
		length := remaining / (count - index)
		remaining -= length
		prefix := fmt.Sprintf("d%d/", index)
		if length <= len(prefix) {
			t.Fatalf("the fixture needs paths longer than %d bytes", len(prefix))
		}
		values = append(values, prefix+strings.Repeat("p", length-len(prefix)))
	}
	return changedPaths(t, values...)
}

// maintMutFillerPaths builds count distinct paths of the given length.
func maintMutFillerPaths(t *testing.T, count, length int) []string {
	t.Helper()

	values := make([]string, 0, count)
	for index := 0; index < count; index++ {
		prefix := fmt.Sprintf("f%d/", index)
		if length <= len(prefix) {
			t.Fatalf("the fixture needs paths longer than %d bytes", len(prefix))
		}
		values = append(values, prefix+strings.Repeat("p", length-len(prefix)))
	}
	return values
}

// maintMutRetainedFileBytes sums the OrgTop-owned files that survive a
// truncating checkpoint. RG-005 projects `page_count * 4,096 + other retained
// files`, and the database, its write-ahead log, and its shared-memory index are
// not among the retained files.
func maintMutRetainedFileBytes(t *testing.T, location Location) int64 {
	t.Helper()

	var total int64
	for _, path := range []string{
		location.Database() + "-journal",
		location.Lock(),
		location.Bootstrap(),
		location.Tombstone(),
	} {
		size, err := statSize(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatalf("statSize(%q) error = %v", path, err)
		}
		total += size
	}
	return total
}

// maintMutCheckpoint truncates the write-ahead log with an independent
// connection, so the page count and the apparent bytes describe one state.
func maintMutCheckpoint(t *testing.T, location Location) {
	t.Helper()

	var busy, log, checkpointed int
	if err := openDirectly(t, location).
		QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &log, &checkpointed); err != nil {
		t.Fatalf("wal_checkpoint error = %v", err)
	}
}

// maintMutGenerousBounds are shrunken bounds whose row limits still admit a
// multi-record fixture, so a test seeds it before narrowing the bound it proves.
func maintMutGenerousBounds() bounds {
	limits := smallBounds()
	limits.evidenceRecords = 1_000
	limits.childRecords = 10_000
	return limits
}

// maintMutSeedExpired saves one expired record per name, each carrying
// pathsPerRecord paths.
func maintMutSeedExpired(t *testing.T, store *Store, names []string, pathsPerRecord int) {
	t.Helper()

	store.withBounds(maintMutGenerousBounds())
	for _, name := range names {
		paths := make([]string, 0, pathsPerRecord)
		for index := 0; index < pathsPerRecord; index++ {
			paths = append(paths, fmt.Sprintf("%s/%d.go", name, index))
		}
		seedEntry(t, store, name, referenceTime.Add(-evidenceTTL), referenceTime.Add(-evidenceTTL), paths...)
	}
}

// maintMutSeedFresh saves one fresh record per name, each carrying
// pathsPerRecord paths, with distinct last-use times so eviction order is total.
func maintMutSeedFresh(t *testing.T, store *Store, names []string, pathsPerRecord int) {
	t.Helper()

	store.withBounds(maintMutGenerousBounds())
	for index, name := range names {
		paths := make([]string, 0, pathsPerRecord)
		for path := 0; path < pathsPerRecord; path++ {
			paths = append(paths, fmt.Sprintf("%s/%d.go", name, path))
		}
		used := referenceTime.Add(-time.Duration(len(names)-index) * time.Hour)
		seedEntry(t, store, name, used, used, paths...)
	}
}

// TestDefaultBoundsMatchTheClosedCapacityTable pins RG-005's capacity table: the
// hard row limits, the retained ceiling and temporary envelope, the fixed 8/32
// MiB reservations and 8 MiB cleanup-selection limit, the one-batch bounds, and
// the 75% cleanup targets of 7,500 evidence rows, 187,500 child rows, and 96 MiB.
func TestDefaultBoundsMatchTheClosedCapacityTable(t *testing.T) {
	t.Parallel()

	const mib = 1 << 20
	limits := defaultBounds()
	for _, bound := range []struct {
		name string
		got  int64
		want int64
	}{
		{"evidence records", int64(limits.evidenceRecords), 10_000},
		{"child records", int64(limits.childRecords), 250_000},
		{"retained ceiling", limits.retainedBytes, 128 * mib},
		{"temporary envelope", limits.temporaryBytes, 160 * mib},
		{"write reservation", limits.writeReservation, 8 * mib},
		{"cleanup reservation", limits.cleanupReservation, 32 * mib},
		{"cleanup selection path bytes", limits.selectionPathBytes, 8 * mib},
		{"evidence batch", int64(limits.evidenceBatch), 100},
		{"child batch", int64(limits.childBatch), 10_000},
		{"evidence target", int64(limits.evidenceTarget()), 7_500},
		{"child target", int64(limits.childTarget()), 187_500},
		{"retained target", limits.retainedTarget(), 96 * mib},
	} {
		if bound.got != bound.want {
			t.Errorf("%s = %d, want %d", bound.name, bound.got, bound.want)
		}
	}
}

// TestAdmissionAllowsExactlyTheTemporaryEnvelope proves RG-005's temporary
// admission inequality is inclusive: apparent bytes plus the fixed reservation
// may reach the envelope, and one byte beyond it skips the write.
func TestAdmissionAllowsExactlyTheTemporaryEnvelope(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	kept := seedEntry(t, store, "kept", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
	used, err := store.PhysicalBytes()
	if err != nil {
		t.Fatalf("PhysicalBytes() error = %v", err)
	}
	replacement := Entry{
		Key:        kept.Key,
		Paths:      changedPaths(t, "b.go"),
		AcquiredAt: referenceTime,
		LastUsedAt: referenceTime,
	}

	limits := smallBounds()
	limits.temporaryBytes = used + limits.writeReservation - 1
	if err := store.withBounds(limits).Save(context.Background(), replacement); !errors.Is(err, ErrOverCapacity) {
		t.Fatalf("Save() error = %v, want ErrOverCapacity one byte beyond the envelope", err)
	}
	limits.temporaryBytes = used + limits.writeReservation
	if err := store.withBounds(limits).Save(context.Background(), replacement); err != nil {
		t.Errorf("Save() error = %v, want a write admitted exactly at the envelope", err)
	}
}

// TestReplacingARecordWithoutStoredPathsAddsNoEvidenceRow proves a replacement
// projects the exact-key deletion: a stored record carrying no child rows is
// still a stored record, so replacing it adds no evidence row to the projection.
func TestReplacingARecordWithoutStoredPathsAddsNoEvidenceRow(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	empty := seedEntry(t, store, "empty", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour))

	limits := smallBounds()
	limits.evidenceRecords = 1
	err := store.withBounds(limits).Save(context.Background(), Entry{
		Key:        empty.Key,
		Paths:      changedPaths(t, "a.go"),
		AcquiredAt: referenceTime,
		LastUsedAt: referenceTime,
	})
	if err != nil {
		t.Errorf("Save() error = %v, want the replacement of the only stored record admitted", err)
	}
}

// TestReplacementProjectsTheExactKeyDeletedChildRows proves the projected child
// rows subtract exactly the rows stored under the replaced key and add exactly
// the rows the replacement carries.
func TestReplacementProjectsTheExactKeyDeletedChildRows(t *testing.T) {
	t.Parallel()

	t.Run("a replacement releases the rows it overwrites", func(t *testing.T) {
		t.Parallel()

		store, _ := fixedClockStore(t)
		stored := seedEntry(t, store, "stored", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour),
			"a.go", "b.go", "c.go")

		limits := smallBounds()
		limits.childRecords = 5
		err := store.withBounds(limits).Save(context.Background(), Entry{
			Key:        stored.Key,
			Paths:      changedPaths(t, "w.go", "x.go", "y.go", "z.go"),
			AcquiredAt: referenceTime,
			LastUsedAt: referenceTime,
		})
		if err != nil {
			t.Errorf("Save() error = %v, want four rows admitted once the three replaced rows are released", err)
		}
	})

	t.Run("a new record adds its own rows", func(t *testing.T) {
		t.Parallel()

		store, _ := fixedClockStore(t)
		seedEntry(t, store, "stored", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour),
			"a.go", "b.go", "c.go")
		key, err := CompareKey(testRepository(t, "owner", "other"), baseSHA, headSHA)
		if err != nil {
			t.Fatalf("CompareKey() error = %v", err)
		}

		limits := smallBounds()
		limits.childRecords = 5
		err = store.withBounds(limits).Save(context.Background(), Entry{
			Key:        key,
			Paths:      changedPaths(t, "w.go", "x.go", "y.go", "z.go"),
			AcquiredAt: referenceTime,
			LastUsedAt: referenceTime,
		})
		if !errors.Is(err, ErrOverCapacity) {
			t.Errorf("Save() error = %v, want ErrOverCapacity for seven projected child rows above five", err)
		}
	})
}

// TestAdmissionAllowsAProjectionExactlyAtAHardRowLimit proves RG-005's logical
// admission inequalities are inclusive: a write projected exactly at a hard row
// limit commits, and only a projection above one is skipped.
func TestAdmissionAllowsAProjectionExactlyAtAHardRowLimit(t *testing.T) {
	t.Parallel()

	t.Run("evidence rows", func(t *testing.T) {
		t.Parallel()

		store, _ := fixedClockStore(t)
		seedEntry(t, store, "first", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
		key, err := CompareKey(testRepository(t, "owner", "second"), baseSHA, headSHA)
		if err != nil {
			t.Fatalf("CompareKey() error = %v", err)
		}

		limits := smallBounds()
		limits.evidenceRecords = 2
		err = store.withBounds(limits).Save(context.Background(), Entry{
			Key:        key,
			Paths:      changedPaths(t, "b.go"),
			AcquiredAt: referenceTime,
			LastUsedAt: referenceTime,
		})
		if err != nil {
			t.Errorf("Save() error = %v, want the second of two admitted evidence records", err)
		}
	})

	t.Run("child rows", func(t *testing.T) {
		t.Parallel()

		store, _ := fixedClockStore(t)
		seedEntry(t, store, "first", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
		key, err := CompareKey(testRepository(t, "owner", "second"), baseSHA, headSHA)
		if err != nil {
			t.Fatalf("CompareKey() error = %v", err)
		}

		limits := smallBounds()
		limits.childRecords = 3
		err = store.withBounds(limits).Save(context.Background(), Entry{
			Key:        key,
			Paths:      changedPaths(t, "b.go", "c.go"),
			AcquiredAt: referenceTime,
			LastUsedAt: referenceTime,
		})
		if err != nil {
			t.Errorf("Save() error = %v, want three projected child rows admitted at a limit of three", err)
		}
	})
}

// The projection fixture is one replacement whose own rows are estimated at
// exactly 21 pages, so the page rounding and every term of the projection are
// observable at the ceiling.
const (
	maintMutProjectedPaths     = 30
	maintMutProjectedPathBytes = 41_312
)

// TestSaveIsAdmittedExactlyAtTheProjectedRetainedCeiling proves RG-005's
// projected admission: a write commits when `page_count * 4,096 + other retained
// files` including its own growth stays at or below the retained ceiling, and is
// skipped one byte above it. The growth is counted in whole pages beyond the
// free pages the database can already reuse.
func TestSaveIsAdmittedExactlyAtTheProjectedRetainedCeiling(t *testing.T) {
	t.Parallel()

	t.Run("without reusable free pages", func(t *testing.T) {
		t.Parallel()

		store, location := fixedClockStore(t)
		maintMutAssertProjectedCeiling(t, store, location)
	})

	t.Run("with reusable free pages", func(t *testing.T) {
		t.Parallel()

		store, location := fixedClockStore(t)
		store = store.withBounds(maintMutGenerousBounds())
		seedEntry(t, store, "released", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour),
			maintMutFillerPaths(t, 20, 900)...)
		// Incremental auto-vacuum keeps the pages a deletion releases on the free
		// list, so the next write reuses them instead of growing the database.
		execDatabase(t, location, "DELETE FROM evidence_path", "DELETE FROM evidence")
		if free := queryScalar[int64](t, location, "PRAGMA freelist_count"); free == 0 {
			t.Fatalf("the fixture needs the deletion to release reusable free pages")
		}
		maintMutAssertProjectedCeiling(t, store, location)
	})
}

// maintMutAssertProjectedCeiling proves one replacement is admitted exactly at
// its projected retained bytes and skipped one byte below them.
func maintMutAssertProjectedCeiling(t *testing.T, store *Store, location Location) {
	t.Helper()

	// Two OrgTop-owned files that survive a checkpoint, so the projection has to
	// account for retained bytes beyond the database itself.
	stageFile(t, location.Bootstrap(), strings.Repeat("b", 700))
	stageFile(t, location.Tombstone(), strings.Repeat("t", 300))
	maintMutCheckpoint(t, location)

	pages := queryScalar[int64](t, location, "PRAGMA page_count")
	free := queryScalar[int64](t, location, "PRAGMA freelist_count")
	retained := maintMutRetainedFileBytes(t, location)

	key, err := CompareKey(testRepository(t, "owner", "projected"), baseSHA, headSHA)
	if err != nil {
		t.Fatalf("CompareKey() error = %v", err)
	}
	entry := Entry{
		Key:        key,
		Paths:      maintMutPathSet(t, maintMutProjectedPaths, maintMutProjectedPathBytes),
		AcquiredAt: referenceTime,
		LastUsedAt: referenceTime,
	}
	// The parent row, plus every stored path's child row and the unique index
	// entry that repeats it. The fixture makes that an exact page multiple.
	growth := int64(evidenceRowBytes + maintMutProjectedPaths*pathRowBytes + 2*maintMutProjectedPathBytes)
	if growth%pageSize != 0 {
		t.Fatalf("the fixture growth %d must be a whole number of %d byte pages", growth, pageSize)
	}
	needed := growth / pageSize
	if needed-free < 8 {
		t.Fatalf("the fixture needs growth beyond the %d free pages and the shared-memory index", free)
	}
	projected := (pages+needed-free)*pageSize + retained

	limits := defaultBounds()
	limits.retainedBytes = projected - 1
	if err := store.withBounds(limits).Save(context.Background(), entry); !errors.Is(err, ErrOverCapacity) {
		t.Fatalf("Save() error = %v, want ErrOverCapacity one byte below the projected %d bytes", err, projected)
	}
	limits.retainedBytes = projected
	if err := store.withBounds(limits).Save(context.Background(), entry); err != nil {
		t.Errorf("Save() error = %v, want a write admitted exactly at the projected %d bytes", err, projected)
	}
}

// TestMaintenanceDueExactlyAtTheChildRowLimit proves the closed maintenance
// trigger is reached rather than exceeded: child rows at the hard limit request
// the bounded cleanup batch.
func TestMaintenanceDueExactlyAtTheChildRowLimit(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	seedEntry(t, store, "full", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go", "b.go", "c.go")

	limits := smallBounds()
	limits.childRecords = 3
	due, err := store.withBounds(limits).MaintenanceDue(context.Background())
	if err != nil {
		t.Fatalf("MaintenanceDue() error = %v", err)
	}
	if !due {
		t.Errorf("MaintenanceDue() = false, want the child-row limit reached to request maintenance")
	}
}

// TestMaintenanceDueExactlyAtTheRetainedTrigger proves apparent bytes reaching
// the retained ceiling request the bounded cleanup batch.
func TestMaintenanceDueExactlyAtTheRetainedTrigger(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	seedEntry(t, store, "kept", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
	used, err := store.PhysicalBytes()
	if err != nil {
		t.Fatalf("PhysicalBytes() error = %v", err)
	}

	limits := smallBounds()
	limits.retainedBytes = used
	due, err := store.withBounds(limits).MaintenanceDue(context.Background())
	if err != nil {
		t.Fatalf("MaintenanceDue() error = %v", err)
	}
	if !due {
		t.Errorf("MaintenanceDue() = false, want apparent bytes at the ceiling to request maintenance")
	}
}

// TestMaintainWithoutASelectionRunsNoCheckpoint proves the bounded vacuum and
// the one truncate checkpoint follow a committed deletion batch: a store below
// every bound selects nothing, so its write-ahead log is left alone.
func TestMaintainWithoutASelectionRunsNoCheckpoint(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	store = store.withBounds(smallBounds())
	seedEntry(t, store, "fresh", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
	before, err := statSize(location.Database() + "-wal")
	if err != nil {
		t.Fatalf("statSize() error = %v", err)
	}
	if before == 0 {
		t.Fatalf("the fixture needs a nonempty write-ahead log")
	}

	report, err := store.Maintain(context.Background())
	if err != nil {
		t.Fatalf("Maintain() error = %v", err)
	}
	if report.DeletedInvalid != 0 || report.DeletedExpired != 0 || report.Evicted != 0 || report.Deferred {
		t.Fatalf("report = %+v, want an empty batch", report)
	}
	after, err := statSize(location.Database() + "-wal")
	if err != nil {
		t.Fatalf("statSize() error = %v", err)
	}
	if after != before {
		t.Errorf("write-ahead log = %d bytes, want the %d bytes an empty batch leaves alone", after, before)
	}
}

// TestSelectionSpendsTheChildRowBudget proves one cleanup transaction is bounded
// by the accumulated child rows of everything it has already selected.
func TestSelectionSpendsTheChildRowBudget(t *testing.T) {
	t.Parallel()

	t.Run("the budget is inclusive", func(t *testing.T) {
		t.Parallel()

		store, location := fixedClockStore(t)
		maintMutSeedExpired(t, store, []string{"aaa", "bbb"}, 2)

		limits := maintMutGenerousBounds()
		limits.childBatch = 4
		report, err := store.withBounds(limits).Maintain(context.Background())
		if err != nil {
			t.Fatalf("Maintain() error = %v", err)
		}
		if report.DeletedExpired != 2 || report.Evicted != 0 {
			t.Fatalf("report = %+v, want four child rows admitted by a budget of four", report)
		}
		if got := repositoryKeys(t, location); len(got) != 0 {
			t.Errorf("surviving records = %v, want none", got)
		}
	})

	t.Run("the budget accumulates across records", func(t *testing.T) {
		t.Parallel()

		store, location := fixedClockStore(t)
		maintMutSeedExpired(t, store, []string{"aaa", "bbb", "ccc"}, 2)

		limits := maintMutGenerousBounds()
		limits.childBatch = 5
		report, err := store.withBounds(limits).Maintain(context.Background())
		if err != nil {
			t.Fatalf("Maintain() error = %v", err)
		}
		if report.DeletedExpired != 2 || report.Evicted != 0 {
			t.Fatalf("report = %+v, want the third record's rows held beyond a budget of five", report)
		}
		if got, want := repositoryKeys(t, location), []string{"owner/ccc"}; !slices.Equal(got, want) {
			t.Errorf("surviving records = %v, want %v", got, want)
		}
	})
}

// TestSelectionAccumulatesStoredPathBytesAcrossRecords proves the cleanup
// selection's stored-path byte budget is spent by every selected record
// together, not by the last one alone.
func TestSelectionAccumulatesStoredPathBytesAcrossRecords(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	limits := maintMutGenerousBounds()
	limits.selectionPathBytes = 12
	store = store.withBounds(limits)
	for _, name := range []string{"aaa", "bbb", "ccc"} {
		seedEntry(t, store, name, referenceTime.Add(-evidenceTTL), referenceTime.Add(-evidenceTTL), name+".go")
	}

	report, err := store.Maintain(context.Background())
	if err != nil {
		t.Fatalf("Maintain() error = %v", err)
	}
	if report.DeletedExpired != 2 {
		t.Fatalf("report = %+v, want two six-byte paths admitted by a twelve-byte budget", report)
	}
	if got, want := repositoryKeys(t, location), []string{"owner/ccc"}; !slices.Equal(got, want) {
		t.Errorf("surviving records = %v, want %v", got, want)
	}
}

// TestEvictionCountsWhatTheBatchAlreadySelected proves eviction measures the 75%
// targets against what the batch leaves behind: records and child rows already
// selected for deletion are gone from the remaining counts.
func TestEvictionCountsWhatTheBatchAlreadySelected(t *testing.T) {
	t.Parallel()

	t.Run("evidence rows", func(t *testing.T) {
		t.Parallel()

		store, _ := fixedClockStore(t)
		maintMutSeedFresh(t, store, []string{"aaa", "bbb", "ccc"}, 1)
		maintMutSeedExpired(t, store, []string{"ddd"}, 1)

		limits := maintMutGenerousBounds()
		limits.evidenceRecords = 4
		report, err := store.withBounds(limits).Maintain(context.Background())
		if err != nil {
			t.Fatalf("Maintain() error = %v", err)
		}
		if report.DeletedExpired != 1 || report.Evicted != 0 {
			t.Errorf("report = %+v, want the expired deletion alone to reach the 75%% evidence target", report)
		}
	})

	t.Run("child rows", func(t *testing.T) {
		t.Parallel()

		store, _ := fixedClockStore(t)
		maintMutSeedFresh(t, store, []string{"aaa", "bbb", "ccc"}, 2)
		maintMutSeedExpired(t, store, []string{"ddd"}, 2)

		limits := maintMutGenerousBounds()
		limits.childRecords = 6
		report, err := store.withBounds(limits).Maintain(context.Background())
		if err != nil {
			t.Fatalf("Maintain() error = %v", err)
		}
		if report.DeletedExpired != 1 || report.Evicted != 1 {
			t.Errorf("report = %+v, want one eviction beyond the expired deletion to reach the 75%% child target", report)
		}
	})
}

// TestEvictionStopsAtTheChildRowTarget proves eviction accounts for the child
// rows of every record it evicts and stops at the 75% child target.
func TestEvictionStopsAtTheChildRowTarget(t *testing.T) {
	t.Parallel()

	t.Run("eviction stops once the target is reached", func(t *testing.T) {
		t.Parallel()

		store, location := fixedClockStore(t)
		maintMutSeedFresh(t, store, []string{"aaa", "bbb", "ccc", "ddd", "eee", "fff"}, 2)

		limits := maintMutGenerousBounds()
		limits.childRecords = 8
		report, err := store.withBounds(limits).Maintain(context.Background())
		if err != nil {
			t.Fatalf("Maintain() error = %v", err)
		}
		if report.Evicted != 3 {
			t.Fatalf("report = %+v, want twelve child rows evicted down to the target of six", report)
		}
		if got, want := repositoryKeys(t, location), []string{"owner/ddd", "owner/eee", "owner/fff"}; !slices.Equal(got, want) {
			t.Errorf("surviving records = %v, want %v", got, want)
		}
	})

	t.Run("the target is inclusive", func(t *testing.T) {
		t.Parallel()

		store, _ := fixedClockStore(t)
		maintMutSeedFresh(t, store, []string{"aaa", "bbb", "ccc"}, 2)

		limits := maintMutGenerousBounds()
		limits.childRecords = 8
		report, err := store.withBounds(limits).Maintain(context.Background())
		if err != nil {
			t.Fatalf("Maintain() error = %v", err)
		}
		if report.Evicted != 0 {
			t.Errorf("report = %+v, want no eviction with child rows exactly at the target", report)
		}
	})
}

// TestEvictionDoesNotRunAtTheRetainedTarget proves the byte target is inclusive:
// apparent bytes exactly at 75% of the retained ceiling evict nothing.
func TestEvictionDoesNotRunAtTheRetainedTarget(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	maintMutSeedFresh(t, store, []string{"aaa", "bbb"}, 1)
	used, err := store.PhysicalBytes()
	if err != nil {
		t.Fatalf("PhysicalBytes() error = %v", err)
	}

	limits := maintMutGenerousBounds()
	limits.retainedBytes = 0
	for ceiling := 4 * used / 3; ceiling <= 4*used/3+4; ceiling++ {
		candidate := limits
		candidate.retainedBytes = ceiling
		if candidate.retainedTarget() == used {
			limits.retainedBytes = ceiling
			break
		}
	}
	if limits.retainedBytes == 0 {
		t.Fatalf("no retained ceiling puts the 75%% target at exactly %d bytes", used)
	}

	report, err := store.withBounds(limits).Maintain(context.Background())
	if err != nil {
		t.Fatalf("Maintain() error = %v", err)
	}
	if report.Evicted != 0 {
		t.Errorf("report = %+v, want no eviction with apparent bytes exactly at the byte target", report)
	}
}

// TestMaintainReportsACompletedCheckpoint proves a batch whose truncate
// checkpoint reports completion is not deferred to a later refresh.
func TestMaintainReportsACompletedCheckpoint(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	store = store.withBounds(smallBounds())
	for _, name := range []string{"aaa", "bbb"} {
		seedEntry(t, store, name, referenceTime.Add(-evidenceTTL), referenceTime.Add(-evidenceTTL), name+".go")
	}

	report, err := store.Maintain(context.Background())
	if err != nil {
		t.Fatalf("Maintain() error = %v", err)
	}
	if report.DeletedExpired != 2 {
		t.Fatalf("report = %+v, want both expired records deleted", report)
	}
	if report.Deferred {
		t.Errorf("report.Deferred = true, want a completed checkpoint reported as complete")
	}
	after, err := statSize(location.Database() + "-wal")
	if err != nil {
		t.Fatalf("statSize() error = %v", err)
	}
	if after != 0 {
		t.Errorf("write-ahead log = %d bytes, want the truncating checkpoint to empty it", after)
	}
}
