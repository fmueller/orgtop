package cache

// Version-1 header identity. The application ID is the ASCII bytes `ORGT`, so a
// database that is not OrgTop's is recognized without opening it as one.
const (
	applicationID = 0x4f524754
	schemaVersion = 1
	pageSize      = 4096
)

// schemaStatements is the complete version-1 logical schema. It is created in
// one transaction with the header identity, so a crash never exposes partial v1
// tables as canonical.
var schemaStatements = []string{
	`CREATE TABLE evidence(
		id INTEGER PRIMARY KEY,
		repository_key TEXT NOT NULL,
		operation TEXT NOT NULL CHECK(operation IN ('commit','compare')),
		base_sha TEXT NOT NULL,
		head_sha TEXT NOT NULL,
		verified_parent_sha TEXT NOT NULL,
		complete INTEGER NOT NULL CHECK(complete = 1),
		path_count INTEGER NOT NULL CHECK(path_count BETWEEN 0 AND 1000),
		acquired_at INTEGER NOT NULL,
		last_used_at INTEGER NOT NULL,
		UNIQUE(repository_key, operation, base_sha, head_sha)
	)`,
	`CREATE TABLE evidence_path(
		evidence_id INTEGER NOT NULL REFERENCES evidence(id) ON DELETE CASCADE,
		ordinal INTEGER NOT NULL,
		path TEXT NOT NULL,
		PRIMARY KEY(evidence_id, ordinal),
		UNIQUE(evidence_id, path)
	)`,
}

// schemaTables names every table the version-1 schema must carry. A missing or
// unreadable table under OrgTop's exact application ID is structural corruption,
// never a partial hit.
var schemaTables = []string{"evidence", "evidence_path"}
