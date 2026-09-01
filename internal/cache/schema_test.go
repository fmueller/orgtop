package cache

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// wantSchema pins the exact stored version-1 declarations. It is the migration
// fixture: every future schema revision must change this fixture together with
// its one-version migration, so a silent schema drift fails here first.
var wantSchema = map[string]string{
	"evidence": strings.Join([]string{
		"CREATE TABLE evidence(",
		"id INTEGER PRIMARY KEY,",
		"repository_key TEXT NOT NULL,",
		"operation TEXT NOT NULL CHECK(operation IN ('commit','compare')),",
		"base_sha TEXT NOT NULL,",
		"head_sha TEXT NOT NULL,",
		"verified_parent_sha TEXT NOT NULL,",
		"complete INTEGER NOT NULL CHECK(complete = 1),",
		"path_count INTEGER NOT NULL CHECK(path_count BETWEEN 0 AND 1000),",
		"acquired_at INTEGER NOT NULL,",
		"last_used_at INTEGER NOT NULL,",
		"UNIQUE(repository_key, operation, base_sha, head_sha)",
		")",
	}, " "),
	"evidence_path": strings.Join([]string{
		"CREATE TABLE evidence_path(",
		"evidence_id INTEGER NOT NULL REFERENCES evidence(id) ON DELETE CASCADE,",
		"ordinal INTEGER NOT NULL,",
		"path TEXT NOT NULL,",
		"PRIMARY KEY(evidence_id, ordinal),",
		"UNIQUE(evidence_id, path)",
		")",
	}, " "),
}

// TestStoredSchemaMatchesTheVersion1Fixture proves the created database carries
// the closed RG-005 declarations exactly, including every CHECK and UNIQUE
// constraint a record invariant depends on.
func TestStoredSchemaMatchesTheVersion1Fixture(t *testing.T) {
	t.Parallel()

	store, location := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	db, err := sql.Open("sqlite", location.Database())
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer func() { _ = db.Close() }()

	for table, want := range wantSchema {
		var stored string
		if err := db.QueryRow("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&stored); err != nil {
			t.Fatalf("read schema of %q error = %v", table, err)
		}
		if got := strings.Join(strings.Fields(stored), " "); got != want {
			t.Errorf("stored schema of %q =\n%s\nwant\n%s", table, got, want)
		}
	}
}

// TestStoredBytesCarryNoSecretOrPayload proves the closed content rule: only the
// typed descriptor, its proof facts, normalized paths, times, and bookkeeping
// reach disk. A credential, header, raw payload, provenance, Scope identity, or
// pull request number must never appear in any cache file.
func TestStoredBytesCarryNoSecretOrPayload(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	const credential = "ghp_secret_token_value_0123456789"
	key, err := CommitKey(testRepository(t, "owner", "repo"), headSHA)
	if err != nil {
		t.Fatalf("CommitKey() error = %v", err)
	}
	entry := Entry{Key: key, VerifiedParent: parentSHA, Paths: changedPaths(t, "src/main.go")}
	if err := store.Save(context.Background(), entry); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, _, err := store.Lookup(context.Background(), key); err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}

	forbidden := []string{
		credential,
		"Authorization",
		"bearer",
		"etag",
		"x-ratelimit",
		"event-time",
		"current-pr",
		"provenance",
		"pr-metadata",
		"scope",
	}
	for _, path := range location.ownedFiles() {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		lowered := bytes.ToLower(data)
		for _, secret := range forbidden {
			if bytes.Contains(lowered, []byte(strings.ToLower(secret))) {
				t.Errorf("%q contains %q, which the cache must never persist", path, secret)
			}
		}
	}
}
