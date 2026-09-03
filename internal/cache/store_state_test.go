package cache

import (
	"errors"
	"os"
	"testing"
	"time"
)

// TestOpenPreservesAForeignApplicationID proves a nonzero application ID that is
// not OrgTop's is bypassed, never migrated or deleted.
func TestOpenPreservesAForeignApplicationID(t *testing.T) {
	t.Parallel()

	location := stagedLocation(t)
	execDatabase(t, location, "CREATE TABLE other(id INTEGER)", "PRAGMA application_id = 12345")
	before, err := os.ReadFile(location.Database())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if _, err := Open(location); !errors.Is(err, ErrForeignDatabase) {
		t.Fatalf("Open() error = %v, want ErrForeignDatabase", err)
	}
	after, err := os.ReadFile(location.Database())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(before) != len(after) {
		t.Errorf("the foreign database changed size: %d -> %d", len(before), len(after))
	}
}

// TestOpenPreservesAnAmbiguousZeroApplicationID proves a pre-existing zero-ID
// database is ambiguous: only an exclusively created bootstrap file is eligible
// for zero-to-v1 initialization.
func TestOpenPreservesAnAmbiguousZeroApplicationID(t *testing.T) {
	t.Parallel()

	location := stagedLocation(t)
	execDatabase(t, location, "CREATE TABLE other(id INTEGER)")

	if _, err := Open(location); !errors.Is(err, ErrForeignDatabase) {
		t.Fatalf("Open() error = %v, want ErrForeignDatabase", err)
	}
	if _, err := os.Stat(location.Database()); err != nil {
		t.Errorf("the ambiguous database must be preserved, stat error = %v", err)
	}
}

// TestOpenBypassesAnUnknownSchemaVersion proves a newer or unknown older
// user_version is neither deleted nor migrated. v0.2.0 creates version 1 only.
func TestOpenBypassesAnUnknownSchemaVersion(t *testing.T) {
	t.Parallel()

	for name, version := range map[string]string{
		"newer": "PRAGMA user_version = 2",
		"older": "PRAGMA user_version = 0",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			location := LocationIn(t.TempDir())
			store, err := Open(location)
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if err := store.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			execDatabase(t, location, version)

			if _, err := Open(location); !errors.Is(err, ErrIncompatibleVersion) {
				t.Fatalf("Open() error = %v, want ErrIncompatibleVersion", err)
			}
			if _, err := os.Stat(location.Database()); err != nil {
				t.Errorf("an unknown version must be preserved, stat error = %v", err)
			}
		})
	}
}

// TestOpenDiscardsAStructurallyCorruptDatabase proves a database carrying
// OrgTop's exact application ID and version but a damaged schema is corruption,
// never a partial hit against whichever table survived: the rebuild replaces the
// whole database rather than serving the rows that happen to have survived.
func TestOpenDiscardsAStructurallyCorruptDatabase(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	seedEntry(t, store, "kept", referenceTime.Add(-time.Hour), referenceTime.Add(-time.Hour), "a.go")
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	execDatabase(t, location, "DROP TABLE evidence_path")

	rebuilt, err := Open(location)
	if err != nil {
		t.Fatalf("Open() error = %v, want the corrupt database discarded", err)
	}
	defer func() { _ = rebuilt.Close() }()

	if got := queryScalar[int](t, location, "SELECT count(*) FROM evidence"); got != 0 {
		t.Errorf("evidence rows = %d, want the surviving table discarded with the corrupt database", got)
	}
}

// TestOpenTreatsAnUnreadableHeaderAsForeign proves a truncated or non-SQLite
// file at the cache path is bypassed rather than opened as OrgTop's database.
func TestOpenTreatsAnUnreadableHeaderAsForeign(t *testing.T) {
	t.Parallel()

	location := stagedLocation(t)
	stageFile(t, location.Database(), "not a database")

	if _, err := Open(location); !errors.Is(err, ErrForeignDatabase) {
		t.Fatalf("Open() error = %v, want ErrForeignDatabase", err)
	}
	if _, err := os.Stat(location.Database()); err != nil {
		t.Errorf("the foreign file must be preserved, stat error = %v", err)
	}
}

// TestOpenDiscardsAnInterruptedBootstrap proves a crash during initialization
// leaves an owned bootstrap file that the next locked launch discards, and never
// exposes partial v1 tables as canonical.
func TestOpenDiscardsAnInterruptedBootstrap(t *testing.T) {
	t.Parallel()

	location := stagedLocation(t)
	stageFile(t, location.Bootstrap(), "partial")

	store, err := Open(location)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = store.Close() }()

	if _, err := os.Stat(location.Bootstrap()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the interrupted bootstrap file must be discarded, stat error = %v", err)
	}
	if got := queryScalar[int](t, location, "PRAGMA user_version"); got != schemaVersion {
		t.Errorf("user_version = %d, want %d", got, schemaVersion)
	}
}

// TestOpenRemovesOrphanedSidecarsBeforeCreating proves no fresh main database
// opens beside a stale WAL left by an interrupted or reset cache.
func TestOpenRemovesOrphanedSidecarsBeforeCreating(t *testing.T) {
	t.Parallel()

	location := stagedLocation(t)
	orphan := location.Database() + "-wal"
	stageFile(t, orphan, "stale wal")
	stageFile(t, location.Tombstone(), "tombstone")

	store, err := Open(location)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = store.Close() }()

	if _, err := os.Stat(location.Tombstone()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the tombstone must be removed, stat error = %v", err)
	}
	if data, err := os.ReadFile(orphan); err == nil && string(data) == "stale wal" {
		t.Error("the orphaned WAL sidecar must be removed before the canonical database is created")
	}
}
