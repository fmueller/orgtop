package cache

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// ErrInvalidRecord reports evidence that may not be persisted or that a stored
// row failed to prove on read. It is never repaired into a partial hit: the
// whole record is a miss and the reader falls back to GitHub.
var ErrInvalidRecord = errors.New("invalid cache record")

// futureSkew bounds how far ahead of the refresh clock a stored timestamp may
// be. A larger forward skew invalidates the record rather than aging it.
const futureSkew = 5 * time.Minute

// Entry is one complete changed-file record. It carries no provenance, Scope,
// event identity, credential, header, payload, or error: provenance belongs to
// the requesting descriptor, so one immutable compare may serve either
// event-time or qualified current-PR use.
type Entry struct {
	// Key is the typed persistent identity of the record.
	Key Key
	// VerifiedParent is the sole parent a commit response proved. It is empty
	// for compare evidence and required for commit evidence.
	VerifiedParent string
	// Paths is the complete normalized changed-path set. An empty set is
	// complete negative membership evidence, not an absent one.
	Paths []domain.ChangedPath
	// AcquiredAt is when the evidence was read from GitHub.
	AcquiredAt time.Time
	// LastUsedAt is when the evidence last satisfied a lookup.
	LastUsedAt time.Time
}

// Outcome renders the record as a complete domain outcome under the requesting
// descriptor's provenance. A commit record also carries its verified sole
// parent, so the caller can reject an event whose before SHA it does not match.
func (e Entry) Outcome(provenance domain.EvidenceProvenance) domain.EvidenceOutcome {
	outcome := domain.CompleteOutcome(provenance, e.Paths)
	if e.Key.Operation() == operationCommit {
		return outcome.WithSoleParent(e.VerifiedParent)
	}
	return outcome
}

// validate proves every closed per-entity invariant of one record: the typed
// key, the proof fields the operation requires, path count and byte bounds,
// unique ascending normalized paths, and usable timestamps.
func (e Entry) validate(reference time.Time) error {
	if e.Key.IsZero() {
		return fmt.Errorf("%w: no typed key", ErrInvalidRecord)
	}
	if err := e.validateProof(); err != nil {
		return err
	}
	if err := validatePaths(e.Paths); err != nil {
		return err
	}
	if err := validateTimestamp(e.AcquiredAt, reference, "acquired_at"); err != nil {
		return err
	}
	return validateTimestamp(e.LastUsedAt, reference, "last_used_at")
}

// validateProof checks the fields that make a typed key usable. A compare key
// proves itself with exact base and head; a commit key needs the sole parent
// the adapter verified, because a hit is usable only when that parent equals
// the requesting event's before SHA.
func (e Entry) validateProof() error {
	switch e.Key.Operation() {
	case operationCompare:
		if e.VerifiedParent != "" {
			return fmt.Errorf("%w: compare evidence stores no verified parent", ErrInvalidRecord)
		}
	case operationCommit:
		if _, ok := domain.NormalizeObjectSHA(e.VerifiedParent); !ok {
			return fmt.Errorf("%w: commit evidence needs a verified sole parent", ErrInvalidRecord)
		}
	default:
		return fmt.Errorf("%w: unknown operation %q", ErrInvalidRecord, e.Key.Operation())
	}
	return nil
}

// validatePaths proves a changed-path set is storable: within the closed count
// and byte bounds, validated, unique, and in ascending normalized byte order.
func validatePaths(paths []domain.ChangedPath) error {
	if len(paths) > domain.MaxEvidencePaths {
		return fmt.Errorf("%w: %d paths exceed the %d bound", ErrInvalidRecord, len(paths), domain.MaxEvidencePaths)
	}
	total := 0
	previous := ""
	for index, path := range paths {
		value := path.String()
		if path.IsZero() {
			return fmt.Errorf("%w: path %d is not validated", ErrInvalidRecord, index)
		}
		if len(value) > domain.MaxChangedPathBytes {
			return fmt.Errorf("%w: path %d exceeds %d bytes", ErrInvalidRecord, index, domain.MaxChangedPathBytes)
		}
		if index > 0 && value <= previous {
			return fmt.Errorf("%w: paths are not unique in ascending order at %d", ErrInvalidRecord, index)
		}
		previous = value
		total += len(value)
		if total > domain.MaxEvidenceBytes {
			return fmt.Errorf("%w: paths exceed %d bytes", ErrInvalidRecord, domain.MaxEvidenceBytes)
		}
	}
	return nil
}

// validateTimestamp rejects a negative, overflowing, or too-far-future time.
// Smaller forward skew is age zero rather than an invalid record, and a
// timestamp never decides evidence semantics.
func validateTimestamp(value, reference time.Time, field string) error {
	if value.IsZero() {
		return fmt.Errorf("%w: %s is unset", ErrInvalidRecord, field)
	}
	if value.Unix() < 0 {
		return fmt.Errorf("%w: %s predates the epoch", ErrInvalidRecord, field)
	}
	if value.After(reference.Add(futureSkew)) {
		return fmt.Errorf("%w: %s is more than %s in the future", ErrInvalidRecord, field, futureSkew)
	}
	return nil
}

// sortedPaths reports the normalized ascending storage order of a changed-path
// set. Paths are stored once, so a duplicate collapses before validation.
func sortedPaths(paths []domain.ChangedPath) []domain.ChangedPath {
	unique := make(map[string]domain.ChangedPath, len(paths))
	for _, path := range paths {
		if path.IsZero() {
			continue
		}
		unique[path.String()] = path
	}
	ordered := make([]domain.ChangedPath, 0, len(unique))
	for _, path := range unique {
		ordered = append(ordered, path)
	}
	slices.SortFunc(ordered, func(a, b domain.ChangedPath) int {
		return strings.Compare(a.String(), b.String())
	})
	return ordered
}
