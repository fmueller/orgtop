package cache

import (
	"database/sql"
	"os"
	"testing"

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

// execDatabase applies statements with an independent connection so a test can
// stage or damage stored rows the way an external process or a bad shutdown
// would. It creates the database at the cache path when none exists yet.
func execDatabase(t *testing.T, location Location, statements ...string) {
	t.Helper()

	db, err := sql.Open("sqlite", location.Database())
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer func() { _ = db.Close() }()

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
