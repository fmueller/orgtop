package cache

import (
	"context"
	"errors"
	"testing"
	"time"
)

// storedEntry saves one record with an explicit acquisition time so freshness
// boundaries are proven against a controlled clock rather than wall time.
func storedEntry(t *testing.T, store *Store, entry Entry, acquired time.Time) Entry {
	t.Helper()

	entry.AcquiredAt = acquired
	entry.LastUsedAt = acquired
	if err := store.Save(context.Background(), entry); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	return entry
}

// TestLookupExpiresEvidenceAtTheThirtyDayBoundary proves complete evidence is
// fresh for exactly 30 days from acquisition and expired at the boundary.
func TestLookupExpiresEvidenceAtTheThirtyDayBoundary(t *testing.T) {
	t.Parallel()

	for name, age := range map[string]time.Duration{
		"one second inside the boundary": evidenceTTL - time.Second,
		"at the boundary":                evidenceTTL,
		"past the boundary":              evidenceTTL + time.Second,
	} {
		wantHit := age < evidenceTTL
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store, _ := fixedClockStore(t)
			entry := storedEntry(t, store, compareEntry(t, "src/main.go"), referenceTime.Add(-age))

			_, ok, err := store.Lookup(context.Background(), entry.Key)
			if ok != wantHit {
				t.Fatalf("Lookup() hit = %v, want %v (error = %v)", ok, wantHit, err)
			}
			if wantHit {
				if err != nil {
					t.Fatalf("Lookup() error = %v, want a hit", err)
				}
				return
			}
			if !errors.Is(err, ErrExpired) {
				t.Fatalf("Lookup() error = %v, want ErrExpired", err)
			}
		})
	}
}

// TestExpiredEvidenceRemainsStoredUntilCleanup proves an expired read is a miss
// that opens no invalidation transaction: the row stays until bounded cleanup.
func TestExpiredEvidenceRemainsStoredUntilCleanup(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	entry := storedEntry(t, store, compareEntry(t, "src/main.go"), referenceTime.Add(-evidenceTTL))

	if _, ok, _ := store.Lookup(context.Background(), entry.Key); ok {
		t.Fatalf("Lookup() hit expired evidence")
	}
	if got := queryScalar[int](t, location, "SELECT count(*) FROM evidence"); got != 1 {
		t.Errorf("evidence rows = %d, want the expired record to remain stored", got)
	}
}

// TestTouchUpdatesOnlyLastUsedAt proves reuse never changes acquired_at and so
// never extends the 30-day TTL.
func TestTouchUpdatesOnlyLastUsedAt(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	acquired := referenceTime.Add(-24 * time.Hour)
	entry := storedEntry(t, store, compareEntry(t, "src/main.go"), acquired)

	if _, ok, err := store.Lookup(context.Background(), entry.Key); !ok || err != nil {
		t.Fatalf("Lookup() ok = %v, error = %v, want a hit", ok, err)
	}
	if err := store.Touch(context.Background()); err != nil {
		t.Fatalf("Touch() error = %v", err)
	}

	if got, want := queryScalar[int64](t, location, "SELECT acquired_at FROM evidence"), acquired.Unix(); got != want {
		t.Errorf("acquired_at = %d, want %d unchanged by reuse", got, want)
	}
	if got, want := queryScalar[int64](t, location, "SELECT last_used_at FROM evidence"), referenceTime.Unix(); got != want {
		t.Errorf("last_used_at = %d, want the refresh time %d", got, want)
	}
}

// TestTouchIsOneBatchedTransactionForEveryHit proves the touch of a refresh is
// a single batched transaction covering every hit, and that it is drained.
func TestTouchIsOneBatchedTransactionForEveryHit(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	first := storedEntry(t, store, compareEntry(t, "a.go"), referenceTime.Add(-48*time.Hour))
	second := storedEntry(t, store, commitEntry(t, "b.go"), referenceTime.Add(-48*time.Hour))

	for _, key := range []Key{first.Key, second.Key} {
		if _, ok, err := store.Lookup(context.Background(), key); !ok || err != nil {
			t.Fatalf("Lookup() ok = %v, error = %v, want a hit", ok, err)
		}
	}
	if err := store.Touch(context.Background()); err != nil {
		t.Fatalf("Touch() error = %v", err)
	}

	touched := queryScalar[int](t, location, "SELECT count(*) FROM evidence WHERE last_used_at = "+unixLiteral(referenceTime))
	if touched != 2 {
		t.Errorf("touched rows = %d, want 2", touched)
	}
	// A drained queue keeps a later refresh from rewriting rows it never read.
	if err := store.Touch(context.Background()); err != nil {
		t.Fatalf("second Touch() error = %v", err)
	}
	if got := store.pendingTouches(); got != 0 {
		t.Errorf("pending touches = %d, want the queue drained", got)
	}
}

// TestTouchDoesNotInvalidateAProvenHit proves a failed touch is reported as
// cache degradation and never turns a proven hit into a miss.
func TestTouchDoesNotInvalidateAProvenHit(t *testing.T) {
	t.Parallel()

	store, _ := fixedClockStore(t)
	entry := storedEntry(t, store, compareEntry(t, "src/main.go"), referenceTime.Add(-time.Hour))
	if _, ok, err := store.Lookup(context.Background(), entry.Key); !ok || err != nil {
		t.Fatalf("Lookup() ok = %v, error = %v, want a hit", ok, err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := store.Touch(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Touch() error = %v, want ErrUnavailable", err)
	}
}
