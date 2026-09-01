package cache

import (
	"fmt"

	"github.com/fmueller/orgtop/internal/domain"
)

// The persisted operation spellings. They are part of the closed schema, so the
// CHECK constraint and every key share these exact literals.
const (
	operationCommit  = "commit"
	operationCompare = "compare"
)

// Key is the typed persistent identity of one immutable changed-file result. It
// is the repository plus the immutable object identity only: no Scope identity,
// event identity, pull request number, or provenance takes part, so adding a
// Scope or another commenting event never splits a record.
type Key struct {
	repository string
	operation  string
	base       string
	head       string
}

// CommitKey identifies the changed files of one exact commit. Its base is empty;
// the sole parent the adapter proved is a record field, not part of identity.
func CommitKey(repository domain.Repository, head string) (Key, error) {
	repositoryKey, err := repositoryKeyOf(repository)
	if err != nil {
		return Key{}, err
	}
	normalizedHead, ok := domain.NormalizeObjectSHA(head)
	if !ok {
		return Key{}, fmt.Errorf("%w: commit cache key needs a valid head object", domain.ErrInvalidEvidence)
	}
	return Key{repository: repositoryKey, operation: operationCommit, head: normalizedHead}, nil
}

// CompareKey identifies the changed files between two distinct exact commits.
func CompareKey(repository domain.Repository, base, head string) (Key, error) {
	repositoryKey, err := repositoryKeyOf(repository)
	if err != nil {
		return Key{}, err
	}
	normalizedBase, baseOK := domain.NormalizeObjectSHA(base)
	normalizedHead, headOK := domain.NormalizeObjectSHA(head)
	if !baseOK || !headOK {
		return Key{}, fmt.Errorf("%w: compare cache key needs valid base and head objects", domain.ErrInvalidEvidence)
	}
	if normalizedBase == normalizedHead {
		return Key{}, fmt.Errorf("%w: compare cache key needs distinct base and head objects", domain.ErrInvalidEvidence)
	}
	return Key{repository: repositoryKey, operation: operationCompare, base: normalizedBase, head: normalizedHead}, nil
}

// KeyFor reports the persistent identity of an evidence descriptor and whether
// the descriptor is persistable at all. Settled evidence — direct paths, an
// unchanged pair, malformed direct data, an unsupported form — and pull request
// metadata carry no reusable changed-file record and bypass persistence.
func KeyFor(descriptor domain.EvidenceDescriptor) (Key, bool) {
	var (
		key Key
		err error
	)
	switch descriptor.Operation() {
	case domain.EvidenceCommit:
		key, err = CommitKey(descriptor.Repository(), descriptor.Head())
	case domain.EvidenceCompare:
		key, err = CompareKey(descriptor.Repository(), descriptor.Base(), descriptor.Head())
	default:
		return Key{}, false
	}
	if err != nil {
		return Key{}, false
	}
	return key, true
}

// Repository reports the canonical case-folded owner/name the record belongs to.
func (k Key) Repository() string { return k.repository }

// Operation reports the persisted operation spelling.
func (k Key) Operation() string { return k.operation }

// Base reports the exact base object of a compare key, empty for a commit key.
func (k Key) Base() string { return k.base }

// Head reports the exact head object.
func (k Key) Head() string { return k.head }

// IsZero reports whether the key was never derived.
func (k Key) IsZero() bool { return k.operation == "" }

func repositoryKeyOf(repository domain.Repository) (string, error) {
	if repository.Owner() == "" || repository.Name() == "" {
		return "", fmt.Errorf("%w: cache key needs a validated repository", domain.ErrInvalidEvidence)
	}
	return repository.Key(), nil
}
