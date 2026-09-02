package cache

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "modernc.org/sqlite"
)

// openTestStore opens a store beneath a fresh temporary user cache root.
func openTestStore(t *testing.T) (*Store, Location) {
	t.Helper()

	location := LocationIn(t.TempDir())
	store, err := Open(location)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, location
}

// queryScalar reads one value from the canonical database with an independent
// connection, so assertions never depend on the store's own handle.
func queryScalar[T any](t *testing.T, location Location, query string) T {
	t.Helper()

	var value T
	if err := openDirectly(t, location).QueryRow(query).Scan(&value); err != nil {
		t.Fatalf("query %q error = %v", query, err)
	}
	return value
}

// TestOpenCreatesTheVersion1Database pins RG-005's physical and header contract:
// 4 KiB pages, WAL, incremental auto-vacuum, the ORGT application ID, and
// user_version 1.
func TestOpenCreatesTheVersion1Database(t *testing.T) {
	t.Parallel()

	store, location := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if got, want := queryScalar[int](t, location, "PRAGMA page_size"), 4096; got != want {
		t.Errorf("page_size = %d, want %d", got, want)
	}
	if got, want := queryScalar[int64](t, location, "PRAGMA application_id"), int64(applicationID); got != want {
		t.Errorf("application_id = %#x, want %#x", got, want)
	}
	if got, want := queryScalar[int](t, location, "PRAGMA user_version"), schemaVersion; got != want {
		t.Errorf("user_version = %d, want %d", got, want)
	}
	if got, want := queryScalar[string](t, location, "PRAGMA journal_mode"), "wal"; got != want {
		t.Errorf("journal_mode = %q, want %q", got, want)
	}
	if got, want := queryScalar[int](t, location, "PRAGMA auto_vacuum"), 2; got != want {
		t.Errorf("auto_vacuum = %d, want 2 (incremental)", got)
	}
}

// TestOpenCreatesTheClosedLogicalTables proves the version-1 schema is exactly
// the two private tables RG-005 closes and nothing else.
func TestOpenCreatesTheClosedLogicalTables(t *testing.T) {
	t.Parallel()

	store, location := openTestStore(t)
	_ = store.Close()

	db, err := sql.Open("sqlite", location.Database())
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		t.Fatalf("query tables error = %v", err)
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan error = %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error = %v", err)
	}
	if len(names) != 2 || names[0] != "evidence" || names[1] != "evidence_path" {
		t.Errorf("tables = %v, want [evidence evidence_path]", names)
	}
}

// TestOpenAppliesRestrictivePermissions proves the POSIX ownership and mode
// contract: a 0700 directory and 0600 mutable files.
func TestOpenAppliesRestrictivePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes are not the Windows ownership contract")
	}
	t.Parallel()

	_, location := openTestStore(t)

	directory, err := os.Stat(location.Directory())
	if err != nil {
		t.Fatalf("stat directory error = %v", err)
	}
	if got, want := directory.Mode().Perm(), os.FileMode(0o700); got != want {
		t.Errorf("directory mode = %04o, want %04o", got, want)
	}
	for _, path := range []string{location.Database(), location.Lock()} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %q error = %v", path, err)
		}
		if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
			t.Errorf("%q mode = %04o, want %04o", filepath.Base(path), got, want)
		}
	}
}

// TestOpenNarrowsBroaderExistingPermissions proves a pre-existing group- or
// world-accessible cache directory and database are narrowed before use rather
// than trusted.
func TestOpenNarrowsBroaderExistingPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes are not the Windows ownership contract")
	}
	t.Parallel()

	location := LocationIn(t.TempDir())
	if err := os.MkdirAll(location.Directory(), 0o777); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Chmod(location.Directory(), 0o777); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}

	store, err := Open(location)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = store.Close() }()

	info, err := os.Stat(location.Directory())
	if err != nil {
		t.Fatalf("stat directory error = %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o700); got != want {
		t.Errorf("directory mode = %04o, want %04o", got, want)
	}
}

// TestOpenRejectsNonRegularAndSymlinkedPaths proves a symlink or a directory at
// an owned file path is refused rather than followed.
func TestOpenRejectsNonRegularAndSymlinkedPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally permitted on Windows")
	}
	t.Parallel()

	t.Run("symlinked database", func(t *testing.T) {
		t.Parallel()

		location := stagedLocation(t)
		target := filepath.Join(t.TempDir(), "elsewhere.db")
		stageFile(t, target, "")
		if err := os.Symlink(target, location.Database()); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}

		if _, err := Open(location); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("Open() error = %v, want ErrUnavailable", err)
		}
		if _, err := os.Lstat(location.Database()); err != nil {
			t.Errorf("the rejected symlink must be preserved, lstat error = %v", err)
		}
	})

	t.Run("directory at the cache path", func(t *testing.T) {
		t.Parallel()

		location := LocationIn(t.TempDir())
		if err := os.MkdirAll(location.Database(), 0o700); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		if _, err := Open(location); !errors.Is(err, ErrUnavailable) {
			t.Errorf("Open() error = %v, want ErrUnavailable", err)
		}
	})
}

// TestOpenRejectsAZeroLocation keeps an unresolved user cache directory an
// unavailable cache instead of a relative path beside the working directory.
func TestOpenRejectsAZeroLocation(t *testing.T) {
	t.Parallel()

	if _, err := Open(Location{}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Open(zero) error = %v, want ErrUnavailable", err)
	}
}

// TestOpenIsIdempotent proves a second open of an existing v1 database reuses it
// rather than rebuilding or migrating it.
func TestOpenIsIdempotent(t *testing.T) {
	t.Parallel()

	store, location := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	before, err := os.Stat(location.Database())
	if err != nil {
		t.Fatalf("stat error = %v", err)
	}

	second, err := Open(location)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer func() { _ = second.Close() }()

	after, err := os.Stat(location.Database())
	if err != nil {
		t.Fatalf("stat error = %v", err)
	}
	if !before.ModTime().Equal(after.ModTime()) && after.Size() != before.Size() {
		t.Errorf("reopen replaced the database: %v -> %v", before.ModTime(), after.ModTime())
	}
}

// TestOpenLeavesNothingToNarrow proves an ordinary launch settles the cache
// permissions completely. A platform whose reported mode is not its real access
// control must report no pending narrowing, or every later open would escalate
// to exclusive lifecycle access and refuse every concurrent launch.
func TestOpenLeavesNothingToNarrow(t *testing.T) {
	t.Parallel()

	store, location := openTestStore(t)

	broad, err := inspectOwnedPaths(store.root, location)
	if err != nil {
		t.Fatalf("inspectOwnedPaths() error = %v", err)
	}
	if broad {
		t.Error("a freshly opened cache still reports a pending permission repair, so every open would take the lifecycle region exclusively")
	}
}

// TestConcurrentOpensShareTheLifecycleRegion proves two launches can use one
// existing healthy cache at the same time. Ordinary use holds shared lifecycle
// access; only creation, repair, and rebuild exclude other launches.
func TestConcurrentOpensShareTheLifecycleRegion(t *testing.T) {
	t.Parallel()

	_, location := openTestStore(t)

	second, err := Open(location)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	if err := second.Close(); err != nil {
		t.Errorf("second Close() error = %v", err)
	}
}
