package cache

import (
	"database/sql"
	"os"
	"strconv"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// stagedLocation returns a cache location whose directory already exists, so a
// test can place a foreign, damaged, or interrupted file at an owned path
// without going through Store.
func stagedLocation(t *testing.T) Location {
	t.Helper()

	location := LocationIn(t.TempDir())
	if err := os.MkdirAll(location.Directory(), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	return location
}

// stageFile writes one file a test needs in place before the store opens.
func stageFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

// openDirectly opens the canonical database with an independent connection, so
// staging, damaging, and assertions never depend on the store's own handle. It
// creates the database at the cache path when none exists yet.
func openDirectly(t *testing.T, location Location) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", location.Database())
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// execDatabase applies statements with an independent connection so a test can
// stage or damage stored rows the way an external process or a bad shutdown
// would.
func execDatabase(t *testing.T, location Location, statements ...string) {
	t.Helper()

	db := openDirectly(t, location)
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("exec %q error = %v", statement, err)
		}
	}
}

// statSize reports one file's apparent length.
func statSize(path string) (int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// commitEntry builds a commit record whose verified sole parent is present, as
// commit evidence requires.
func commitEntry(t *testing.T, paths ...string) Entry {
	t.Helper()

	key, err := CommitKey(testRepository(t, "owner", "repo"), headSHA)
	if err != nil {
		t.Fatalf("CommitKey() error = %v", err)
	}
	return Entry{Key: key, VerifiedParent: baseSHA, Paths: changedPaths(t, paths...)}
}

// unixLiteral renders a time as the signed Unix seconds a stored row carries.
func unixLiteral(value time.Time) string {
	return strconv.FormatInt(value.Unix(), 10)
}

// queryStrings reads one text column from the canonical database with an
// independent connection.
func queryStrings(t *testing.T, location Location, query string) []string {
	t.Helper()

	rows, err := openDirectly(t, location).Query(query)
	if err != nil {
		t.Fatalf("query %q error = %v", query, err)
	}
	defer func() { _ = rows.Close() }()

	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan %q error = %v", query, err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows %q error = %v", query, err)
	}
	return values
}
