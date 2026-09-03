package cache

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"slices"
	"time"

	"github.com/fmueller/orgtop/internal/domain"

	// modernc.org/sqlite is the pure-Go driver the CGO_ENABLED=0 cross-compile
	// requires. Only this package may import it.
	_ "modernc.org/sqlite"
)

// Cache state errors. Each one is a bypass rather than a launch failure: OrgTop
// keeps working against GitHub and reports a sanitized degraded cause.
var (
	// ErrForeignDatabase reports a database OrgTop does not own. A foreign
	// nonzero application ID and an ambiguous pre-existing zero-ID file are both
	// preserved and bypassed, never deleted or migrated.
	ErrForeignDatabase = errors.New("cache file does not belong to orgtop")
	// ErrIncompatibleVersion reports a newer or unknown older schema version.
	// v0.2.0 creates version 1 only and performs no in-place migration.
	ErrIncompatibleVersion = errors.New("cache schema version is not supported")
	// ErrCorrupt reports structural corruption of a database carrying OrgTop's
	// exact application ID and a known version.
	ErrCorrupt = errors.New("cache structure is corrupt")
)

// SQLite header offsets of the two fields that identify a database without
// opening it. Reading the raw header keeps a foreign or truncated file from
// being handed to the driver at all.
const (
	headerLength            = 100
	headerUserVersionOffset = 60
	headerApplicationOffset = 68
)

// walAutocheckpointPages is RG-005's fixed WAL autocheckpoint threshold.
const walAutocheckpointPages = 1000

// Store is the version-1 enrichment cache. It is a narrow evidence cache, not a
// general persistence layer: it exposes exact-key lookup and exact-key
// replacement of complete changed-file records and nothing else.
type Store struct {
	location Location
	root     *os.Root
	lock     *maintenanceLock
	db       *sql.DB
	now      func() time.Time
	limits   bounds
	// invalid holds the exact keys of rows that failed their invariants on
	// read. Reads open no invalidation transaction, so the next bounded
	// cleanup batch deletes exactly these records first.
	invalid []Key
	// touched holds the proven hits of this refresh for its single batched
	// touch transaction.
	touched []Key
	// disabled stops cache work for this process after an unrecovered
	// physical state. OrgTop stays interactive and falls back to GitHub.
	disabled bool
	// rebuilt records that this process spent its one structural rebuild
	// attempt. A second confirmed corruption is reported rather than
	// discarding another database.
	rebuilt bool
}

// Open prepares the fixed cache location and returns a usable version-1 store.
// Setup runs under exclusive lifecycle access and is downgraded to shared access
// for the store's lifetime, so a concurrent process cannot rebuild or reset the
// database while this one uses it.
func Open(location Location) (*Store, error) {
	root, err := openCacheRoot(location)
	if err != nil {
		return nil, err
	}
	lock, err := openMaintenanceLock(root, location.Lock())
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	store := &Store{location: location, root: root, lock: lock, limits: defaultBounds()}

	// Ordinary cache use holds shared lifecycle access, so concurrent launches
	// do not exclude each other. Only creation, permission repair, and rebuild
	// take the region exclusively.
	if err := lock.acquire(lifecycleRegion, false, busyWait); err != nil {
		_ = store.closeHandles()
		return nil, err
	}
	if err := store.prepare(); err != nil {
		_ = store.closeHandles()
		return nil, err
	}
	return store, nil
}

// prepare brings the cache to a usable version-1 state. It probes under shared
// lifecycle access and escalates to exclusive access only for the work that
// mutates paths: permission repair and fresh creation.
func (s *Store) prepare() error {
	broad, err := inspectOwnedPaths(s.root, s.location)
	if err != nil {
		return err
	}
	exists, err := s.canonicalExists()
	if err != nil {
		return err
	}
	// A tombstone is an unfinished reset. It forces the same sidecar-first
	// cleanup before any canonical create or open, so no database is used
	// beside write-ahead state that no longer describes it.
	pending, err := s.ownedExists(tombstoneName)
	if err != nil {
		return err
	}
	if broad || pending || !exists {
		if err := s.underExclusiveLifecycle(s.setUp); err != nil {
			return err
		}
	}
	if err := s.openCanonical(); err != nil {
		if !errors.Is(err, ErrCorrupt) || s.rebuilt {
			return err
		}
		// Confirmed structural corruption of exact v1 gets one rebuild
		// attempt per process. A failed rebuild reports its cause and leaves
		// cache work disabled for this launch rather than retrying.
		if err := s.underExclusiveLifecycle(s.rebuild); err != nil {
			return err
		}
		if err := s.openCanonical(); err != nil {
			return err
		}
	}
	// SQLite creates its WAL and SHM sidecars on first open. They are mutable
	// cache files, so a broader inherited mode is narrowed like any other.
	broad, err = inspectOwnedPaths(s.root, s.location)
	if err != nil {
		return err
	}
	if !broad {
		return nil
	}
	return s.underExclusiveLifecycle(func() error { return repairOwnedPaths(s.root, s.location) })
}

// underExclusiveLifecycle runs one path-mutating step under exclusive lifecycle
// access and returns to shared access afterwards. A contended region leaves
// every file unchanged.
func (s *Store) underExclusiveLifecycle(step func() error) error {
	s.lock.release(lifecycleRegion)
	if err := s.lock.acquire(lifecycleRegion, true, busyWait); err != nil {
		return err
	}
	if err := step(); err != nil {
		return err
	}
	return s.lock.convert(lifecycleRegion, busyWait)
}

// openCanonical proves the canonical file is OrgTop's exact version 1 and opens
// it. Both steps are one unit because either one can confirm the corruption the
// single rebuild attempt answers.
func (s *Store) openCanonical() error {
	if err := s.checkHeader(); err != nil {
		return err
	}
	return s.openDatabase()
}

// rebuild spends this process's one structural rebuild attempt: the corrupt
// database goes through the same tombstone procedure `--reset-cache` uses and a
// complete empty version 1 replaces it. The attempt is recorded before the work
// so a failure is never retried. It must hold exclusive lifecycle access.
func (s *Store) rebuild() error {
	s.rebuilt = true
	if err := discardDatabase(s.root, s.location); err != nil {
		return err
	}
	// bootstrap repeats the orphan cleanup discardDatabase just ran. That is
	// deliberate: bootstrap owns the precondition for its own exclusive create
	// on every path that reaches it, and removing an absent file succeeds.
	return s.bootstrap()
}

// WithClock returns the store reading time from the refresh's fixed clock. Tests
// and the refresh pipeline supply it so record ages never depend on wall time
// read at an arbitrary moment.
func (s *Store) WithClock(now func() time.Time) *Store {
	s.now = now
	return s
}

// withBounds returns the store using shrunken capacities. Only tests use it:
// production always runs on the closed defaultBounds.
func (s *Store) withBounds(limits bounds) *Store {
	s.limits = limits
	return s
}

// Close releases the database handle and every lock region. No database handle
// remains open after shared lifecycle access is released.
func (s *Store) Close() error {
	return s.closeHandles()
}

// Location reports the fixed location this store uses.
func (s *Store) Location() Location { return s.location }

// setUp repairs permissions and creates a fresh version-1 database when none is
// canonical. It must hold exclusive lifecycle access.
func (s *Store) setUp() error {
	if err := repairOwnedPaths(s.root, s.location); err != nil {
		return err
	}
	pending, err := s.ownedExists(tombstoneName)
	if err != nil {
		return err
	}
	if pending {
		if err := clearInterruptedReset(s.root); err != nil {
			return err
		}
	}
	exists, err := s.canonicalExists()
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.bootstrap()
}

func (s *Store) canonicalExists() (bool, error) {
	return s.ownedExists(databaseName)
}

// ownedExists reports whether one owned cache file is present.
func (s *Store) ownedExists(name string) (bool, error) {
	_, err := s.root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return true, nil
}

// bootstrap creates a complete version-1 database beside the canonical name and
// renames it into place. With no canonical database, only orphaned OrgTop
// sidecars and a previous owned bootstrap file are removed first; a crash
// therefore leaves no canonical file rather than a partial one.
func (s *Store) bootstrap() error {
	if err := clearInterruptedReset(s.root); err != nil {
		return err
	}
	if err := removeOwnedFile(s.root, bootstrapName); err != nil {
		return err
	}
	file, err := s.root.OpenFile(bootstrapName, os.O_RDWR|os.O_CREATE|os.O_EXCL, fileMode)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if err := createSchema(s.location.Bootstrap()); err != nil {
		_ = removeOwnedFile(s.root, bootstrapName)
		return err
	}
	if err := s.root.Rename(bootstrapName, databaseName); err != nil {
		_ = removeOwnedFile(s.root, bootstrapName)
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return nil
}

// createSchema writes the physical layout, the complete v1 schema, and the
// header identity into the exclusively created bootstrap file. Page size and
// incremental auto-vacuum precede schema creation because neither can be
// changed once a page is written.
func createSchema(path string) error {
	db, err := sql.Open("sqlite", dataSource(path))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	for _, pragma := range []string{
		fmt.Sprintf("PRAGMA page_size = %d", pageSize),
		"PRAGMA auto_vacuum = INCREMENTAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
	}
	transaction, err := db.Begin()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() { _ = transaction.Rollback() }()

	statements := slices.Concat(schemaStatements, []string{
		fmt.Sprintf("PRAGMA application_id = %d", applicationID),
		fmt.Sprintf("PRAGMA user_version = %d", schemaVersion),
	})
	for _, statement := range statements {
		if _, err := transaction.Exec(statement); err != nil {
			return fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return nil
}

// checkHeader applies the closed unknown-version policy by reading the raw
// SQLite header. A foreign or ambiguous file is never opened as OrgTop's.
func (s *Store) checkHeader() error {
	identity, err := readIdentity(s.root)
	if errors.Is(err, fs.ErrNotExist) {
		// Only a concurrent reset removes the canonical file between setup and
		// open. That is an unavailable cache, not a cause without a sentinel
		// the caller can act on.
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if err != nil {
		return err
	}
	if identity.application != applicationID {
		return fmt.Errorf("%w: application id %#08x", ErrForeignDatabase, identity.application)
	}
	if identity.version != schemaVersion {
		return fmt.Errorf("%w: schema version %d", ErrIncompatibleVersion, identity.version)
	}
	return nil
}

// databaseIdentity is the pair of raw SQLite header fields that names a
// database without opening it.
type databaseIdentity struct {
	application uint32
	version     uint32
}

// readIdentity reads the header fields of the canonical database through the
// directory-relative handle. An absent file reports fs.ErrNotExist so a caller
// can distinguish "no cache" from "not OrgTop's cache"; anything that does not
// read as a database header at all is foreign and preserved.
func readIdentity(root *os.Root) (databaseIdentity, error) {
	file, err := root.Open(databaseName)
	if errors.Is(err, fs.ErrNotExist) {
		return databaseIdentity{}, err
	}
	if err != nil {
		return databaseIdentity{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() { _ = file.Close() }()

	header := make([]byte, headerLength)
	// io.ReadFull distinguishes an empty file from a short one, so a legitimate
	// database is never misclassified as foreign by a partial read.
	if _, err := io.ReadFull(file, header); err != nil {
		if errors.Is(err, io.EOF) {
			return databaseIdentity{}, fmt.Errorf("%w: cache header is unreadable", ErrForeignDatabase)
		}
		return databaseIdentity{}, fmt.Errorf("%w: cache header is truncated", ErrForeignDatabase)
	}
	return databaseIdentity{
		application: binary.BigEndian.Uint32(header[headerApplicationOffset:]),
		version:     binary.BigEndian.Uint32(header[headerUserVersionOffset:]),
	}, nil
}

// openDatabase opens the canonical database and keeps the handle only once it
// is proven usable.
func (s *Store) openDatabase() error {
	db, err := sql.Open("sqlite", functionalDataSource(s.location.Database()))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	// One connection keeps transactions serialized inside the process, so the
	// admission region alone decides cross-process ordering.
	db.SetMaxOpenConns(1)

	if err := verifyUsable(db); err != nil {
		_ = db.Close()
		return err
	}
	s.db = db
	return nil
}

// verifyUsable enables and verifies WAL outside a transaction and proves the
// version-1 schema is structurally intact.
func verifyUsable(db *sql.DB) error {
	if err := verifyIdentity(db); err != nil {
		return err
	}
	var journal string
	if err := db.QueryRow("PRAGMA journal_mode = WAL").Scan(&journal); err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if journal != "wal" {
		return fmt.Errorf("%w: journal mode %q", ErrCorrupt, journal)
	}
	return verifyStructure(db)
}

// verifyIdentity re-reads the application ID and schema version through the
// connection the driver actually opened. database/sql takes a path rather than
// a descriptor, so the driver's own open is not covered by the no-follow,
// directory-relative inspection every other path operation uses; a same-user
// process could in principle swap the canonical name between that inspection
// and this open. Re-proving the identity on the live connection closes that
// window for anything that is not an OrgTop version-1 database. The residual
// gap — a swap for another OrgTop v1 database owned by the same user — is not
// closable without a descriptor-based driver open.
func verifyIdentity(db *sql.DB) error {
	var identifier int64
	if err := db.QueryRow("PRAGMA application_id").Scan(&identifier); err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if identifier != applicationID {
		return fmt.Errorf("%w: application id %#08x", ErrForeignDatabase, identifier)
	}
	var version int64
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if version != schemaVersion {
		return fmt.Errorf("%w: schema version %d", ErrIncompatibleVersion, version)
	}
	return nil
}

// verifyStructure proves the closed logical tables exist and are readable. A
// database carrying OrgTop's exact application ID and version that fails this is
// structural corruption, never a partial hit.
func verifyStructure(db *sql.DB) error {
	for _, table := range schemaTables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: table %q is missing", ErrCorrupt, table)
		}
		if err != nil {
			return fmt.Errorf("%w: %w", ErrCorrupt, err)
		}
	}
	var integrity string
	if err := db.QueryRow("PRAGMA quick_check(1)").Scan(&integrity); err != nil {
		return fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if integrity != "ok" {
		return fmt.Errorf("%w: %s", ErrCorrupt, integrity)
	}
	return nil
}

// functionalDataSource builds the driver URI of the canonical database. Foreign
// keys and the 250 ms busy limit apply to every functional connection.
func functionalDataSource(path string) string {
	return dataSource(path) +
		"&_pragma=foreign_keys(1)" +
		fmt.Sprintf("&_pragma=busy_timeout(%d)", busyWait.Milliseconds()) +
		"&_pragma=synchronous(1)" +
		fmt.Sprintf("&_pragma=wal_autocheckpoint(%d)", walAutocheckpointPages)
}

// dataSource builds the bare driver URI. Bootstrap uses it directly, because it
// sets its own physical layout before any schema exists.
func dataSource(path string) string {
	return "file:" + (&url.URL{Path: path}).EscapedPath() + "?_txlock=immediate"
}

func (s *Store) closeHandles() error {
	var err error
	if s.db != nil {
		err = s.db.Close()
		s.db = nil
	}
	if s.lock != nil {
		if closeErr := s.lock.close(); err == nil {
			err = closeErr
		}
		s.lock = nil
	}
	if s.root != nil {
		if closeErr := s.root.Close(); err == nil {
			err = closeErr
		}
		s.root = nil
	}
	return err
}

func (s *Store) clock() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

// PhysicalBytes reports the apparent length of every file the cache owns: the
// main database, its WAL, SHM, and rollback journal sidecars, the maintenance
// lock, and any OrgTop-owned bootstrap or tombstone file. The metric is the sum
// of apparent file lengths, not allocated filesystem blocks.
func (s *Store) PhysicalBytes() (int64, error) {
	if s.root == nil {
		return 0, fmt.Errorf("%w: the store is closed", ErrUnavailable)
	}
	var total int64
	for _, name := range ownedNames() {
		size, err := s.ownedSize(name)
		if err != nil {
			return 0, err
		}
		total += size
	}
	return total, nil
}

// usable reports whether cache work is still allowed in this process. An
// unrecovered physical state disables the cache without failing the launch.
func (s *Store) usable() error {
	if s.db == nil {
		return fmt.Errorf("%w: the store is closed", ErrUnavailable)
	}
	if s.disabled {
		return fmt.Errorf("%w: cache use is disabled for this process, run --reset-cache", ErrOverCapacity)
	}
	return nil
}

// ownedSize reports the apparent length of one owned file, zero when absent.
func (s *Store) ownedSize(name string) (int64, error) {
	info, err := s.root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return info.Size(), nil
}

// Lookup returns the complete record stored under an exact typed key. A
// contended lock, an unusable row, or a failed invariant is a miss for this
// refresh: the returned error reports the degradation cause and never turns
// incomplete state into complete evidence.
func (s *Store) Lookup(ctx context.Context, key Key) (Entry, bool, error) {
	if err := s.usable(); err != nil {
		return Entry{}, false, err
	}
	if key.IsZero() {
		return Entry{}, false, fmt.Errorf("%w: no typed key", ErrInvalidRecord)
	}
	if err := s.lock.acquire(admissionRegion, false, busyWait); err != nil {
		return Entry{}, false, err
	}
	defer s.lock.release(admissionRegion)

	// The physical proof runs inside the admission region, so two processes
	// cannot both pass admission against the same stale byte figures. A read
	// reserves no temporary headroom, but a cache already above the retained
	// ceiling admits no hit at all.
	if err := s.admit(0); err != nil {
		return Entry{}, false, err
	}

	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Entry{}, false, fmt.Errorf("%w: %w", ErrContended, err)
	}
	defer func() { _ = transaction.Rollback() }()

	entry, found, err := readEntry(ctx, transaction, key)
	if err != nil {
		// A row that failed its structural invariants is queued first for
		// bounded cleanup; the read itself changes nothing.
		if errors.Is(err, ErrInvalidRecord) {
			s.queueInvalid(key)
		}
		return Entry{}, false, err
	}
	if !found {
		return Entry{}, false, nil
	}
	now := s.clock()
	if err := entry.validate(now); err != nil {
		// A failed row invariant is a miss. Reads never open an invalidation
		// transaction; the record is queued first for bounded cleanup.
		s.queueInvalid(key)
		return Entry{}, false, err
	}
	if entry.expired(now) {
		// Expired evidence never satisfies complete evidence. It stays stored
		// until bounded cleanup or a successful exact-key replacement.
		return Entry{}, false, fmt.Errorf("%w: acquired at %s", ErrExpired, entry.AcquiredAt.Format(time.RFC3339))
	}
	s.queueTouch(key)
	return entry, true, nil
}

// readEntry loads one parent row and its children and proves the structural
// facts a hit requires: complete evidence, the exact child count, contiguous
// ordinals, and unique validated paths.
func readEntry(ctx context.Context, transaction *sql.Tx, key Key) (Entry, bool, error) {
	var (
		id             int64
		verifiedParent string
		complete       int
		pathCount      int
		acquiredAt     int64
		lastUsedAt     int64
	)
	err := transaction.QueryRowContext(ctx,
		`SELECT id, verified_parent_sha, complete, path_count, acquired_at, last_used_at
		 FROM evidence
		 WHERE repository_key = ? AND operation = ? AND base_sha = ? AND head_sha = ?`,
		key.Repository(), key.Operation(), key.Base(), key.Head(),
	).Scan(&id, &verifiedParent, &complete, &pathCount, &acquiredAt, &lastUsedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if complete != 1 {
		return Entry{}, false, fmt.Errorf("%w: record is not complete", ErrInvalidRecord)
	}
	paths, err := readPaths(ctx, transaction, id, pathCount)
	if err != nil {
		return Entry{}, false, err
	}
	return Entry{
		Key:            key,
		VerifiedParent: verifiedParent,
		Paths:          paths,
		AcquiredAt:     time.Unix(acquiredAt, 0).UTC(),
		LastUsedAt:     time.Unix(lastUsedAt, 0).UTC(),
	}, true, nil
}

func readPaths(ctx context.Context, transaction *sql.Tx, id int64, pathCount int) ([]domain.ChangedPath, error) {
	rows, err := transaction.QueryContext(ctx,
		`SELECT ordinal, path FROM evidence_path WHERE evidence_id = ? ORDER BY ordinal`, id)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	defer func() { _ = rows.Close() }()

	paths := make([]domain.ChangedPath, 0, pathCount)
	for expected := 0; rows.Next(); expected++ {
		var (
			ordinal int
			value   string
		)
		if err := rows.Scan(&ordinal, &value); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrCorrupt, err)
		}
		if ordinal != expected {
			return nil, fmt.Errorf("%w: ordinal %d is not contiguous", ErrInvalidRecord, ordinal)
		}
		path, err := domain.NewChangedPath(value)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidRecord, err)
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if len(paths) != pathCount {
		return nil, fmt.Errorf("%w: %d child rows for a declared count of %d", ErrInvalidRecord, len(paths), pathCount)
	}
	return paths, nil
}

// Save replaces the record under one exact typed key in a single immediate
// transaction: the old record is removed and the complete parent and all its
// children are written and counted before the commit. A reader observes either
// the previous complete record or the replacement, never working rows, and a
// rolled-back write never invalidates the API evidence of the current refresh.
func (s *Store) Save(ctx context.Context, entry Entry) error {
	if err := s.usable(); err != nil {
		return err
	}
	now := s.clock()
	entry.Paths = sortedPaths(entry.Paths)
	if entry.AcquiredAt.IsZero() {
		entry.AcquiredAt = now
	}
	if entry.LastUsedAt.IsZero() {
		entry.LastUsedAt = entry.AcquiredAt
	}
	if err := entry.validate(now); err != nil {
		return err
	}
	if err := s.lock.acquire(admissionRegion, true, busyWait); err != nil {
		return err
	}
	defer s.lock.release(admissionRegion)

	// Admission is proven under the region a competing process must also take,
	// so no two processes can both admit a write against the same figures.
	if err := s.admitWrite(ctx, entry); err != nil {
		return err
	}

	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrContended, err)
	}
	defer func() { _ = transaction.Rollback() }()

	if err := writeEntry(ctx, transaction, entry, now); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("%w: %w", ErrContended, err)
	}
	return nil
}

func writeEntry(ctx context.Context, transaction *sql.Tx, entry Entry, reference time.Time) error {
	key := entry.Key
	stored, err := readStoredTimes(ctx, transaction, key)
	if err != nil {
		return err
	}
	// Replacement never moves a record's clock backward: a racing refresh whose
	// fixed clock is behind must not rewind the acquisition time a record's age
	// is measured from. Invalid stored times are discarded, not carried forward.
	entry.AcquiredAt = latestValid(entry.AcquiredAt, stored.acquired, reference)
	entry.LastUsedAt = latestValid(entry.LastUsedAt, stored.lastUsed, reference)
	if _, err := transaction.ExecContext(ctx,
		`DELETE FROM evidence WHERE repository_key = ? AND operation = ? AND base_sha = ? AND head_sha = ?`,
		key.Repository(), key.Operation(), key.Base(), key.Head(),
	); err != nil {
		return fmt.Errorf("%w: %w", ErrContended, err)
	}
	result, err := transaction.ExecContext(ctx,
		`INSERT INTO evidence(
			repository_key, operation, base_sha, head_sha, verified_parent_sha,
			complete, path_count, acquired_at, last_used_at)
		 VALUES(?, ?, ?, ?, ?, 1, ?, ?, ?)`,
		key.Repository(), key.Operation(), key.Base(), key.Head(), entry.VerifiedParent,
		len(entry.Paths), entry.AcquiredAt.Unix(), entry.LastUsedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrContended, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrContended, err)
	}
	for ordinal, path := range entry.Paths {
		if _, err := transaction.ExecContext(ctx,
			`INSERT INTO evidence_path(evidence_id, ordinal, path) VALUES(?, ?, ?)`,
			id, ordinal, path.String(),
		); err != nil {
			return fmt.Errorf("%w: %w", ErrContended, err)
		}
	}
	var written int
	if err := transaction.QueryRowContext(ctx,
		`SELECT count(*) FROM evidence_path WHERE evidence_id = ?`, id).Scan(&written); err != nil {
		return fmt.Errorf("%w: %w", ErrContended, err)
	}
	if written != len(entry.Paths) {
		return fmt.Errorf("%w: wrote %d of %d child rows", ErrInvalidRecord, written, len(entry.Paths))
	}
	return nil
}

// storedTimes are the timestamps of the record an exact-key replacement is
// about to remove.
type storedTimes struct {
	acquired time.Time
	lastUsed time.Time
}

// readStoredTimes reads the timestamps of the record under an exact key inside
// the replacement transaction. A missing record contributes nothing.
func readStoredTimes(ctx context.Context, transaction *sql.Tx, key Key) (storedTimes, error) {
	var acquired, lastUsed int64
	err := transaction.QueryRowContext(ctx,
		`SELECT acquired_at, last_used_at
		 FROM evidence
		 WHERE repository_key = ? AND operation = ? AND base_sha = ? AND head_sha = ?`,
		key.Repository(), key.Operation(), key.Base(), key.Head(),
	).Scan(&acquired, &lastUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return storedTimes{}, nil
	}
	if err != nil {
		return storedTimes{}, fmt.Errorf("%w: %w", ErrContended, err)
	}
	return storedTimes{acquired: time.Unix(acquired, 0).UTC(), lastUsed: time.Unix(lastUsed, 0).UTC()}, nil
}

// latestValid reports the later of the writing refresh's time and a stored
// time, ignoring a stored value the closed timestamp rules reject.
func latestValid(writing, stored, reference time.Time) time.Time {
	if stored.IsZero() || validateTimestamp(stored, reference, "stored") != nil {
		return writing
	}
	if stored.After(writing) {
		return stored
	}
	return writing
}
