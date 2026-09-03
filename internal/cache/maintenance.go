package cache

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"
)

// ErrExpired reports evidence past its freshness horizon. It is a miss like an
// absent record: expired evidence never satisfies complete evidence, and it
// remains stored only until bounded cleanup or exact-key replacement.
var ErrExpired = errors.New("cache record expired")

// ErrOverCapacity reports an operation that cannot prove its physical or
// logical admission bounds. The cache is bypassed rather than grown: OrgTop
// falls back to GitHub and reports cache degradation.
var ErrOverCapacity = errors.New("cache is over capacity")

// evidenceTTL is the closed freshness horizon. Complete evidence is fresh for
// exactly 30 days from acquisition and expired at the boundary.
const evidenceTTL = 30 * 24 * time.Hour

// bounds are the closed cache capacities. Production always uses
// defaultBounds; tests shrink them so every boundary is provable with a
// temporary store instead of a multi-megabyte fixture.
type bounds struct {
	// evidenceRecords and childRecords are the hard logical row limits.
	evidenceRecords int
	childRecords    int
	// retainedBytes is the retained physical ceiling and cleanup trigger;
	// temporaryBytes is the envelope one operation may occupy while running.
	retainedBytes  int64
	temporaryBytes int64
	// writeReservation and cleanupReservation are the fixed temporary
	// headroom a replacement or touch and a cleanup must prove.
	writeReservation   int64
	cleanupReservation int64
	// selectionPathBytes shortens one cleanup selection.
	selectionPathBytes int64
	// evidenceBatch and childBatch bound one cleanup transaction.
	evidenceBatch int
	childBatch    int
	// vacuumPages bounds the incremental vacuum after a committed batch.
	vacuumPages int
}

func defaultBounds() bounds {
	return bounds{
		evidenceRecords:    10_000,
		childRecords:       250_000,
		retainedBytes:      128 << 20,
		temporaryBytes:     160 << 20,
		writeReservation:   8 << 20,
		cleanupReservation: 32 << 20,
		selectionPathBytes: 8 << 20,
		evidenceBatch:      100,
		childBatch:         10_000,
		vacuumPages:        1024,
	}
}

// The cleanup targets are 75% of each retained bound: a batch stops there
// rather than emptying a store that is merely full.
func (b bounds) evidenceTarget() int   { return b.evidenceRecords * 3 / 4 }
func (b bounds) childTarget() int      { return b.childRecords * 3 / 4 }
func (b bounds) retainedTarget() int64 { return b.retainedBytes * 3 / 4 }

// Maintenance reports one bounded cleanup batch: what it removed, and the row
// and byte bounds before and after it ran.
type Maintenance struct {
	DeletedInvalid int
	DeletedExpired int
	Evicted        int
	EvidenceBefore int
	EvidenceAfter  int
	PathsBefore    int
	PathsAfter     int
	BytesBefore    int64
	BytesAfter     int64
	// Deferred reports a busy or incomplete checkpoint that stayed within the
	// retained ceiling and is retried by a later refresh.
	Deferred bool
}

// expired reports whether a record is past the freshness horizon of the
// refresh's fixed clock. The boundary itself is expired.
func (e Entry) expired(now time.Time) bool {
	return !now.Before(e.AcquiredAt.Add(evidenceTTL))
}

// admit proves the physical preconditions of one operation. A cache already
// above the retained ceiling admits no hit or mutation at all; otherwise the
// operation must fit its fixed reservation inside the temporary envelope.
func (s *Store) admit(reservation int64) error {
	used, err := s.PhysicalBytes()
	if err != nil {
		return err
	}
	if used > s.limits.retainedBytes {
		return fmt.Errorf("%w: cache files use %d bytes above the %d byte ceiling, run --reset-cache",
			ErrOverCapacity, used, s.limits.retainedBytes)
	}
	if used+reservation > s.limits.temporaryBytes {
		return fmt.Errorf("%w: %d bytes plus a %d byte reservation exceed the %d byte envelope",
			ErrOverCapacity, used, reservation, s.limits.temporaryBytes)
	}
	return nil
}

// admitWrite adds a replacement's logical projection to the physical
// admission: the committed store must stay within both hard row limits and the
// projected post-checkpoint retained ceiling.
func (s *Store) admitWrite(ctx context.Context, entry Entry) error {
	if err := s.admit(s.limits.writeReservation); err != nil {
		return err
	}
	counts, err := s.rowCounts(ctx)
	if err != nil {
		return err
	}
	replaced, err := s.storedPathCount(ctx, entry.Key)
	if err != nil {
		return err
	}
	evidence := counts.evidence
	if replaced < 0 {
		// No record under this exact key, so the replacement adds one.
		evidence++
		replaced = 0
	}
	paths := counts.paths - replaced + len(entry.Paths)
	if evidence > s.limits.evidenceRecords || paths > s.limits.childRecords {
		return fmt.Errorf("%w: a replacement projects %d evidence and %d child rows above the %d and %d limits",
			ErrOverCapacity, evidence, paths, s.limits.evidenceRecords, s.limits.childRecords)
	}
	return s.admitProjectedBytes(ctx, replacementBytes(entry))
}

// Storage estimates of one replacement's own rows. A B-tree cell carries its
// payload plus a header and rowid, and evidence_path repeats every path in the
// unique index that proves paths are stored once, so a stored path is counted
// twice. The estimates are deliberately generous: admission has to bound the
// growth a write can cause, and overstating it skips a write while
// understating it would grow the cache past the ceiling it must respect.
const (
	evidenceRowBytes = 512
	pathRowBytes     = 96
)

// replacementBytes estimates the physical bytes one replacement's own rows add:
// the parent row and, for every path, its child row and index entry.
func replacementBytes(entry Entry) int64 {
	total := int64(evidenceRowBytes)
	for _, path := range entry.Paths {
		total += 2*int64(len(path.String())) + pathRowBytes
	}
	return total
}

// admitProjectedBytes proves the store stays within the retained ceiling once
// the pending operation's own growth is written and the write-ahead log is
// checkpointed back into the main database. Measuring only the pre-write state
// would admit a replacement whose own rows cross the ceiling.
func (s *Store) admitProjectedBytes(ctx context.Context, growth int64) error {
	var pages, free int64
	if err := s.db.QueryRowContext(ctx,
		"SELECT (SELECT * FROM pragma_page_count), (SELECT * FROM pragma_freelist_count)").Scan(&pages, &free); err != nil {
		return fmt.Errorf("%w: %w", ErrContended, err)
	}
	retained, err := s.retainedSidecarBytes()
	if err != nil {
		return err
	}
	projected := (pages+growthPages(growth, free))*pageSize + retained
	if projected > s.limits.retainedBytes {
		return fmt.Errorf("%w: the operation projects %d retained bytes above the %d byte ceiling",
			ErrOverCapacity, projected, s.limits.retainedBytes)
	}
	return nil
}

// growthPages reports the pages an operation adds beyond the free pages the
// database can already reuse. Incremental auto-vacuum keeps freed pages on the
// free list rather than returning them to the filesystem, so the pages an
// exact-key deletion releases are exactly the ones its replacement rewrites.
func growthPages(growth, free int64) int64 {
	pages := (growth + pageSize - 1) / pageSize
	if pages <= free {
		return 0
	}
	return pages - free
}

// retainedSidecarBytes sums the owned files that survive a checkpoint. The main
// database is accounted for by its page count and the write-ahead log and its
// shared-memory index do not survive a truncating checkpoint.
func (s *Store) retainedSidecarBytes() (int64, error) {
	transient := map[string]bool{
		databaseName:          true,
		databaseName + "-wal": true,
		databaseName + "-shm": true,
	}
	var total int64
	for _, name := range ownedNames() {
		if transient[name] {
			continue
		}
		size, err := s.ownedSize(name)
		if err != nil {
			return 0, err
		}
		total += size
	}
	return total, nil
}

// counts are the store's current logical row counts.
type counts struct {
	evidence int
	paths    int
}

func (s *Store) rowCounts(ctx context.Context) (counts, error) {
	var result counts
	if err := s.db.QueryRowContext(ctx,
		"SELECT (SELECT count(*) FROM evidence), (SELECT count(*) FROM evidence_path)",
	).Scan(&result.evidence, &result.paths); err != nil {
		return counts{}, fmt.Errorf("%w: %w", ErrContended, err)
	}
	return result, nil
}

// storedPathCount reports the child rows under one exact key, or -1 when no
// record exists there. The count is a correlated subquery so an absent parent
// row stays distinguishable from a stored record that carries no child rows.
func (s *Store) storedPathCount(ctx context.Context, key Key) (int, error) {
	var stored int
	err := s.db.QueryRowContext(ctx,
		`SELECT (SELECT count(*) FROM evidence_path p WHERE p.evidence_id = e.id)
		 FROM evidence e
		 WHERE e.repository_key = ? AND e.operation = ? AND e.base_sha = ? AND e.head_sha = ?`,
		key.Repository(), key.Operation(), key.Base(), key.Head(),
	).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return -1, nil
	}
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrContended, err)
	}
	return stored, nil
}

// queueInvalid records the exact key of a row that failed its invariants on
// read. Reads open no invalidation transaction; the next bounded cleanup batch
// deletes these records first.
func (s *Store) queueInvalid(key Key) {
	s.invalid = appendOnce(s.invalid, key)
}

// queueTouch records a proven hit for the refresh's single batched touch.
func (s *Store) queueTouch(key Key) {
	s.touched = appendOnce(s.touched, key)
}

// appendOnce keeps a queue free of repeats, so one refresh reading the same key
// twice still yields one deletion or one touch.
func appendOnce(keys []Key, key Key) []Key {
	if slices.Contains(keys, key) {
		return keys
	}
	return append(keys, key)
}

// pendingTouches reports how many proven hits still await the batched touch.
func (s *Store) pendingTouches() int { return len(s.touched) }

// Touch applies the refresh's one best-effort batched hit-touch transaction. It
// updates only last_used_at, never acquired_at, so reuse never extends the
// freshness horizon. A failure is reported as cache degradation and never
// invalidates a hit that was already proven.
func (s *Store) Touch(ctx context.Context) error {
	if err := s.usable(); err != nil {
		return err
	}
	if len(s.touched) == 0 {
		return nil
	}
	if err := s.lock.acquire(admissionRegion, true, busyWait); err != nil {
		return err
	}
	defer s.lock.release(admissionRegion)

	if err := s.admit(s.limits.writeReservation); err != nil {
		return err
	}

	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrContended, err)
	}
	defer func() { _ = transaction.Rollback() }()

	now := s.clock().Unix()
	skew := s.clock().Add(futureSkew).Unix()
	for _, key := range s.touched {
		// A stored time is carried forward only while it is valid and later
		// than this refresh; an invalid one is discarded, not preserved.
		if _, err := transaction.ExecContext(ctx,
			`UPDATE evidence
			 SET last_used_at = CASE
				WHEN last_used_at > ? THEN ?
				WHEN last_used_at > ? THEN last_used_at
				ELSE ? END
			 WHERE repository_key = ? AND operation = ? AND base_sha = ? AND head_sha = ?`,
			skew, now, now, now,
			key.Repository(), key.Operation(), key.Base(), key.Head(),
		); err != nil {
			return fmt.Errorf("%w: %w", ErrContended, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("%w: %w", ErrContended, err)
	}
	// The queue is drained only once the batch is committed. A skipped or
	// contended touch keeps its hits, so a later refresh may try again instead
	// of reporting success for an update that never ran.
	s.touched = nil
	return nil
}

// MaintenanceDue reports whether expiry, either hard logical limit, or the
// retained physical trigger requests the single bounded cleanup batch.
func (s *Store) MaintenanceDue(ctx context.Context) (bool, error) {
	if err := s.usable(); err != nil {
		return false, err
	}
	if len(s.invalid) > 0 {
		return true, nil
	}
	counts, err := s.rowCounts(ctx)
	if err != nil {
		return false, err
	}
	if counts.evidence >= s.limits.evidenceRecords || counts.paths >= s.limits.childRecords {
		return true, nil
	}
	used, err := s.PhysicalBytes()
	if err != nil {
		return false, err
	}
	if used >= s.limits.retainedBytes {
		return true, nil
	}
	var expired int
	if err := s.db.QueryRowContext(ctx,
		"SELECT count(*) FROM evidence WHERE acquired_at <= ?", s.expiryCutoff()).Scan(&expired); err != nil {
		return false, fmt.Errorf("%w: %w", ErrContended, err)
	}
	return expired > 0, nil
}

// expiryCutoff reports the acquisition time at or below which a record is
// expired under the refresh's fixed clock.
func (s *Store) expiryCutoff() int64 {
	return s.clock().Add(-evidenceTTL).Unix()
}

// Maintain performs at most one bounded cleanup batch: queued invalid records
// first, then expired records by acquisition time, then least-recently-used
// eviction down to the 75% targets. The batch is bounded by record, child-row,
// and stored-path byte budgets, so it never becomes an unbounded foreground
// pause; a later refresh continues the same deterministic order.
func (s *Store) Maintain(ctx context.Context) (Maintenance, error) {
	if err := s.usable(); err != nil {
		return Maintenance{}, err
	}
	if err := s.lock.acquire(admissionRegion, true, busyWait); err != nil {
		return Maintenance{}, err
	}
	defer s.lock.release(admissionRegion)

	if err := s.admit(s.limits.cleanupReservation); err != nil {
		return Maintenance{}, err
	}

	before, err := s.rowCounts(ctx)
	if err != nil {
		return Maintenance{}, err
	}
	usedBefore, err := s.PhysicalBytes()
	if err != nil {
		return Maintenance{}, err
	}
	report := Maintenance{
		EvidenceBefore: before.evidence,
		PathsBefore:    before.paths,
		BytesBefore:    usedBefore,
	}
	batch, err := s.selectBatch(ctx, before, usedBefore)
	if err != nil {
		return Maintenance{}, err
	}
	if len(batch.ids) > 0 {
		if err := s.deleteBatch(ctx, batch.ids); err != nil {
			return Maintenance{}, err
		}
		s.invalid = batch.invalidCarry
		report.DeletedInvalid = batch.invalid
		report.DeletedExpired = batch.expired
		report.Evicted = batch.evicted
		deferred, err := s.compact(ctx)
		if err != nil {
			return Maintenance{}, err
		}
		report.Deferred = deferred
	}
	after, err := s.rowCounts(ctx)
	if err != nil {
		return Maintenance{}, err
	}
	usedAfter, err := s.PhysicalBytes()
	if err != nil {
		return Maintenance{}, err
	}
	report.EvidenceAfter = after.evidence
	report.PathsAfter = after.paths
	report.BytesAfter = usedAfter
	return report, nil
}

// batch is one cleanup selection and the budgets it has already spent.
type batch struct {
	ids []int64
	// invalidCarry holds the queued invalid keys this batch could not reach.
	// Invalid tracking lives only in this process, so a dropped key would lose
	// its first-place priority for good.
	invalidCarry []Key
	invalid      int
	expired      int
	evicted      int
	childRows    int
	pathBytes    int64
}

// candidate is one selectable record with the child rows and stored path bytes
// its deletion would spend.
type candidate struct {
	id        int64
	childRows int
	pathBytes int64
}

// admitCandidate adds one record while the record, child-row, and stored-path
// byte budgets of this batch allow it.
func (b *batch) admitCandidate(limits bounds, item candidate) bool {
	if len(b.ids) >= limits.evidenceBatch {
		return false
	}
	if b.childRows+item.childRows > limits.childBatch {
		return false
	}
	if b.pathBytes+item.pathBytes > limits.selectionPathBytes {
		return false
	}
	b.ids = append(b.ids, item.id)
	b.childRows += item.childRows
	b.pathBytes += item.pathBytes
	return true
}

// selectBatch builds the deterministic deletion order of one batch: queued
// invalid records, then expired records, then least-recently-used eviction that
// stops as soon as every retained bound is at or below its 75% target.
func (s *Store) selectBatch(ctx context.Context, before counts, used int64) (batch, error) {
	var selected batch
	for index, key := range s.invalid {
		item, found, err := s.candidateForKey(ctx, key)
		if err != nil {
			return batch{}, err
		}
		if !found {
			// The record is already gone, so nothing carries forward.
			continue
		}
		if !selected.admitCandidate(s.limits, item) {
			selected.invalidCarry = s.invalid[index:]
			return selected, nil
		}
		selected.invalid++
	}
	expired, err := s.candidates(ctx,
		`WHERE acquired_at <= ? ORDER BY acquired_at, repository_key, operation, base_sha, head_sha`,
		s.expiryCutoff())
	if err != nil {
		return batch{}, err
	}
	for _, item := range expired {
		if slices.Contains(selected.ids, item.id) {
			continue
		}
		if !selected.admitCandidate(s.limits, item) {
			return selected, nil
		}
		selected.expired++
	}
	evictable, err := s.candidates(ctx,
		`ORDER BY last_used_at, acquired_at, repository_key, operation, base_sha, head_sha`)
	if err != nil {
		return batch{}, err
	}
	remaining := before
	remaining.evidence -= len(selected.ids)
	remaining.paths -= selected.childRows
	for _, item := range evictable {
		if !s.overTarget(remaining, used) {
			break
		}
		if slices.Contains(selected.ids, item.id) {
			continue
		}
		if !selected.admitCandidate(s.limits, item) {
			break
		}
		selected.evicted++
		remaining.evidence--
		remaining.paths -= item.childRows
	}
	return selected, nil
}

// overTarget reports whether any retained bound is still above its 75% target.
func (s *Store) overTarget(remaining counts, used int64) bool {
	return remaining.evidence > s.limits.evidenceTarget() ||
		remaining.paths > s.limits.childTarget() ||
		used > s.limits.retainedTarget()
}

// candidates reads selectable records in one closed order, bounded by the batch
// record budget so a large store never loads an unbounded selection.
func (s *Store) candidates(ctx context.Context, clause string, args ...any) ([]candidate, error) {
	query := `SELECT e.id,
			 (SELECT count(*) FROM evidence_path p WHERE p.evidence_id = e.id),
			 COALESCE((SELECT sum(length(p.path)) FROM evidence_path p WHERE p.evidence_id = e.id), 0)
		 FROM evidence e ` + clause + ` LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, append(args, s.limits.evidenceBatch)...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrContended, err)
	}
	defer func() { _ = rows.Close() }()

	var items []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.childRows, &item.pathBytes); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrContended, err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrContended, err)
	}
	return items, nil
}

// candidateForKey reports the selectable record under one exact typed key.
func (s *Store) candidateForKey(ctx context.Context, key Key) (candidate, bool, error) {
	items, err := s.candidates(ctx,
		`WHERE repository_key = ? AND operation = ? AND base_sha = ? AND head_sha = ?`,
		key.Repository(), key.Operation(), key.Base(), key.Head())
	if err != nil {
		return candidate{}, false, err
	}
	if len(items) == 0 {
		return candidate{}, false, nil
	}
	return items[0], true, nil
}

// deleteBatch removes one selection in a single transaction. Cascading child
// rows go with their parent, so a reader never observes a partial record.
func (s *Store) deleteBatch(ctx context.Context, ids []int64) error {
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrContended, err)
	}
	defer func() { _ = transaction.Rollback() }()

	for _, id := range ids {
		if _, err := transaction.ExecContext(ctx, `DELETE FROM evidence WHERE id = ?`, id); err != nil {
			return fmt.Errorf("%w: %w", ErrContended, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("%w: %w", ErrContended, err)
	}
	return nil
}

// compact runs the bounded incremental vacuum and the one truncate checkpoint
// that follow a committed deletion batch. A busy or incomplete checkpoint is
// deferred only while the store stays at or below the retained ceiling;
// otherwise cache use is disabled pending a reset. OrgTop never runs a full
// VACUUM.
func (s *Store) compact(ctx context.Context) (bool, error) {
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf("PRAGMA incremental_vacuum(%d)", s.limits.vacuumPages)); err != nil {
		return false, fmt.Errorf("%w: %w", ErrContended, err)
	}
	var busy, log, checkpointed int
	if err := s.db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &log, &checkpointed); err != nil {
		return false, fmt.Errorf("%w: %w", ErrContended, err)
	}
	if busy == 0 && log == 0 {
		return false, nil
	}
	used, err := s.PhysicalBytes()
	if err != nil {
		return false, err
	}
	if used > s.limits.retainedBytes {
		s.disabled = true
		return false, fmt.Errorf("%w: an incomplete checkpoint left %d bytes above the %d byte ceiling, run --reset-cache",
			ErrOverCapacity, used, s.limits.retainedBytes)
	}
	return true, nil
}
