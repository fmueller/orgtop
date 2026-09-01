package cache

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

var referenceTime = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// fixedClockStore opens a store reading time from a fixed refresh clock.
func fixedClockStore(t *testing.T) (*Store, Location) {
	t.Helper()

	store, location := openTestStore(t)
	return store.WithClock(func() time.Time { return referenceTime }), location
}

func changedPaths(t *testing.T, values ...string) []domain.ChangedPath {
	t.Helper()

	paths := make([]domain.ChangedPath, 0, len(values))
	for _, value := range values {
		path, err := domain.NewChangedPath(value)
		if err != nil {
			t.Fatalf("NewChangedPath(%q) error = %v", value, err)
		}
		paths = append(paths, path)
	}
	return paths
}

func compareEntry(t *testing.T, paths ...string) Entry {
	t.Helper()

	key, err := CompareKey(testRepository(t, "owner", "repo"), baseSHA, headSHA)
	if err != nil {
		t.Fatalf("CompareKey() error = %v", err)
	}
	return Entry{Key: key, Paths: changedPaths(t, paths...)}
}

// TestLookupMissesAnEmptyStore proves an absent record is a plain miss and never
// an empty complete changed-file set.
func TestLookupMissesAnEmptyStore(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	entry := compareEntry(t)

	got, ok, err := store.Lookup(context.Background(), entry.Key)
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if ok {
		t.Errorf("Lookup() hit = %+v, want a miss", got)
	}
}

// TestSaveThenLookupReturnsTheCompleteSet proves a stored complete record is
// reusable in ascending normalized order without a GitHub lookup.
func TestSaveThenLookupReturnsTheCompleteSet(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	entry := compareEntry(t, "src/main.go", "README.md", "docs/guide.md")

	if err := store.Save(context.Background(), entry); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, ok, err := store.Lookup(context.Background(), entry.Key)
	if err != nil || !ok {
		t.Fatalf("Lookup() ok = %v, error = %v, want a hit", ok, err)
	}

	want := []string{"README.md", "docs/guide.md", "src/main.go"}
	if len(got.Paths) != len(want) {
		t.Fatalf("Paths = %v, want %v", got.Paths, want)
	}
	for index, path := range got.Paths {
		if path.String() != want[index] {
			t.Errorf("Paths[%d] = %q, want %q", index, path.String(), want[index])
		}
	}
	if !got.AcquiredAt.Equal(referenceTime) {
		t.Errorf("AcquiredAt = %v, want %v", got.AcquiredAt, referenceTime)
	}
}

// TestEmptySetIsCompleteNegativeEvidence proves a complete empty path set is
// stored and reused as proof of non-membership, not as a missing record.
func TestEmptySetIsCompleteNegativeEvidence(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	entry := compareEntry(t)

	if err := store.Save(context.Background(), entry); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, ok, err := store.Lookup(context.Background(), entry.Key)
	if err != nil || !ok {
		t.Fatalf("Lookup() ok = %v, error = %v, want a hit", ok, err)
	}
	if len(got.Paths) != 0 {
		t.Errorf("Paths = %v, want the complete empty set", got.Paths)
	}
	outcome := got.Outcome(domain.ProvenanceEventTime)
	if !outcome.IsComplete() {
		t.Errorf("Outcome().IsComplete() = false, want complete negative evidence")
	}
}

// TestCommitRecordCarriesItsVerifiedParent proves a commit hit is usable only
// for the event whose before SHA equals the proven sole parent.
func TestCommitRecordCarriesItsVerifiedParent(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	key, err := CommitKey(testRepository(t, "owner", "repo"), headSHA)
	if err != nil {
		t.Fatalf("CommitKey() error = %v", err)
	}
	entry := Entry{Key: key, VerifiedParent: parentSHA, Paths: changedPaths(t, "src/main.go")}

	if err := store.Save(context.Background(), entry); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, ok, err := store.Lookup(context.Background(), key)
	if err != nil || !ok {
		t.Fatalf("Lookup() ok = %v, error = %v, want a hit", ok, err)
	}
	if got.VerifiedParent != parentSHA {
		t.Errorf("VerifiedParent = %q, want %q", got.VerifiedParent, parentSHA)
	}

	outcome := got.Outcome(domain.ProvenanceEventTime)
	if matched := outcome.ForSoleParent(parentSHA); !matched.IsComplete() {
		t.Errorf("ForSoleParent(proven parent) is not complete")
	}
	if mismatched := outcome.ForSoleParent(baseSHA); mismatched.IsComplete() {
		t.Errorf("ForSoleParent(other before SHA) must not be complete")
	}
}

// TestRecordStoresNoProvenance proves one immutable compare serves either
// event-time or qualified current-PR use, because provenance belongs to the
// requesting descriptor and is never persisted.
func TestRecordStoresNoProvenance(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	entry := compareEntry(t, "src/main.go")
	if err := store.Save(context.Background(), entry); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, ok, err := store.Lookup(context.Background(), entry.Key)
	if err != nil || !ok {
		t.Fatalf("Lookup() ok = %v, error = %v, want a hit", ok, err)
	}

	for _, provenance := range []domain.EvidenceProvenance{domain.ProvenanceEventTime, domain.ProvenanceCurrentPR} {
		outcome := got.Outcome(provenance)
		if !outcome.IsComplete() {
			t.Fatalf("Outcome(%s) is not complete", provenance)
		}
		if outcome.Provenance() != provenance {
			t.Errorf("Outcome(%s).Provenance() = %s, want the requested provenance", provenance, outcome.Provenance())
		}
	}
}

// TestSaveReplacesTheExactKeyRecord proves replacement removes the old record
// and its children, so a stale path can never survive beside a new set.
func TestSaveReplacesTheExactKeyRecord(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	first := compareEntry(t, "old/removed.go")
	if err := store.Save(context.Background(), first); err != nil {
		t.Fatalf("Save(first) error = %v", err)
	}
	second := compareEntry(t, "new/added.go")
	if err := store.Save(context.Background(), second); err != nil {
		t.Fatalf("Save(second) error = %v", err)
	}

	got, ok, err := store.Lookup(context.Background(), second.Key)
	if err != nil || !ok {
		t.Fatalf("Lookup() ok = %v, error = %v, want a hit", ok, err)
	}
	if len(got.Paths) != 1 || got.Paths[0].String() != "new/added.go" {
		t.Errorf("Paths = %v, want [new/added.go]", got.Paths)
	}
}

// TestSaveRejectsUnprovableRecords proves the closed per-entity invariants are
// enforced before anything is written, so no record can be stored that a read
// would then have to reject.
func TestSaveRejectsUnprovableRecords(t *testing.T) {
	t.Parallel()

	repository := testRepository(t, "owner", "repo")
	commitKey, err := CommitKey(repository, headSHA)
	if err != nil {
		t.Fatalf("CommitKey() error = %v", err)
	}
	compareKey, err := CompareKey(repository, baseSHA, headSHA)
	if err != nil {
		t.Fatalf("CompareKey() error = %v", err)
	}

	tooLong, err := domain.NewChangedPath(strings.Repeat("a", domain.MaxChangedPathBytes+1))
	if err != nil {
		t.Fatalf("NewChangedPath() error = %v", err)
	}
	many := make([]domain.ChangedPath, 0, domain.MaxEvidencePaths+1)
	for index := 0; index <= domain.MaxEvidencePaths; index++ {
		many = append(many, changedPaths(t, "path/"+strings.Repeat("x", index+1))...)
	}

	for name, entry := range map[string]Entry{
		"commit without a verified parent":   {Key: commitKey},
		"commit with an invalid parent":      {Key: commitKey, VerifiedParent: "nope"},
		"compare carrying a parent":          {Key: compareKey, VerifiedParent: parentSHA},
		"no typed key":                       {},
		"a path beyond the byte bound":       {Key: compareKey, Paths: []domain.ChangedPath{tooLong}},
		"more paths than the entity bound":   {Key: compareKey, Paths: many},
		"a timestamp far ahead of the clock": {Key: compareKey, AcquiredAt: referenceTime.Add(time.Hour), LastUsedAt: referenceTime},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store, _ := fixedClockStore(t)
			if err := store.Save(context.Background(), entry); !errors.Is(err, ErrInvalidRecord) {
				t.Errorf("Save() error = %v, want ErrInvalidRecord", err)
			}
		})
	}
}

// TestSaveRollsBackACanceledReplacement proves an interrupted write cannot
// expose partial file evidence: the previous complete record survives intact.
func TestSaveRollsBackACanceledReplacement(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	first := compareEntry(t, "kept/original.go")
	if err := store.Save(context.Background(), first); err != nil {
		t.Fatalf("Save(first) error = %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Save(canceled, compareEntry(t, "never/written.go")); err == nil {
		t.Fatal("Save(canceled) error = nil, want a canceled write")
	}

	got, ok, err := store.Lookup(context.Background(), first.Key)
	if err != nil || !ok {
		t.Fatalf("Lookup() ok = %v, error = %v, want the previous record", ok, err)
	}
	if len(got.Paths) != 1 || got.Paths[0].String() != "kept/original.go" {
		t.Errorf("Paths = %v, want [kept/original.go]", got.Paths)
	}
}

// TestLookupRejectsAnInvalidStoredTimestamp proves a record whose stored time is
// negative or far in the future is a miss, never evidence with a repaired clock.
func TestLookupRejectsAnInvalidStoredTimestamp(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	entry := compareEntry(t, "src/main.go")
	if err := store.Save(context.Background(), entry); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	execDatabase(t, location, "UPDATE evidence SET acquired_at = -1")

	if _, ok, err := store.Lookup(context.Background(), entry.Key); ok {
		t.Errorf("Lookup() ok = true, want a miss for an invalid timestamp (error = %v)", err)
	}
}

// TestLookupRejectsBrokenChildRows proves a wrong child count or a gap in the
// stored ordinals is a whole-record miss, never a partial changed-file set.
func TestLookupRejectsBrokenChildRows(t *testing.T) {
	t.Parallel()

	for name, damage := range map[string]string{
		"missing child row": "DELETE FROM evidence_path WHERE ordinal = 1",
		"ordinal gap":       "UPDATE evidence_path SET ordinal = 7 WHERE ordinal = 1",
		"count mismatch":    "UPDATE evidence SET path_count = 9",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store, location := fixedClockStore(t)
			entry := compareEntry(t, "a/one.go", "b/two.go", "c/three.go")
			if err := store.Save(context.Background(), entry); err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			execDatabase(t, location, damage)

			if _, ok, err := store.Lookup(context.Background(), entry.Key); ok {
				t.Errorf("Lookup() ok = true, want a miss for %s (error = %v)", name, err)
			}
		})
	}
}

// TestPhysicalBytesAccountsForSidecars proves accounting sums the apparent
// lengths of the database, its WAL and SHM sidecars, and the maintenance lock.
func TestPhysicalBytesAccountsForSidecars(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	if err := store.Save(context.Background(), compareEntry(t, "src/main.go")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	total, err := store.PhysicalBytes()
	if err != nil {
		t.Fatalf("PhysicalBytes() error = %v", err)
	}
	var sum int64
	for _, path := range location.ownedFiles() {
		if info, err := statSize(path); err == nil {
			sum += info
		}
	}
	if total != sum {
		t.Errorf("PhysicalBytes() = %d, want the summed apparent length %d", total, sum)
	}
	if total <= 0 {
		t.Errorf("PhysicalBytes() = %d, want a positive apparent length", total)
	}
}
