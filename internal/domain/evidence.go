package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrInvalidEvidence reports an evidence descriptor that does not carry the
// immutable identity its GitHub operation requires. A descriptor is never
// repaired: the event producing it stays unsupported or incomplete instead.
var ErrInvalidEvidence = errors.New("invalid changed-file evidence")

// objectSHALength is the exact length of a normalized object SHA.
const objectSHALength = 40

// Closed per-evidence-identity capacities. A limit is part of correctness: an
// identity that exceeds one is incomplete rather than truncated, so a bound can
// never be mistaken for a complete changed-file set.
const (
	// MaxEvidencePages bounds the response pages one evidence identity may span.
	MaxEvidencePages = 10
	// MaxEvidencePaths bounds the normalized changed paths of one identity.
	MaxEvidencePaths = 1000
	// MaxChangedPathBytes bounds the UTF-8 bytes of one normalized changed path.
	MaxChangedPathBytes = 4096
	// MaxEvidenceBytes bounds the UTF-8 bytes across one evidence identity.
	MaxEvidenceBytes = 1 << 20
)

// EvidenceProvenance records which evidence form produced a changed-path set. It
// is retained with every outcome, membership, and aggregate so current-PR
// evidence is never described as the files at event time.
type EvidenceProvenance uint8

const (
	// ProvenanceNone marks evidence that produced no changed-path set.
	ProvenanceNone EvidenceProvenance = iota
	// ProvenanceDirectEvent marks a path the event payload itself named.
	ProvenanceDirectEvent
	// ProvenanceEventTime marks evidence read from immutable event-time objects.
	ProvenanceEventTime
	// ProvenanceCurrentPR marks evidence read from a pull request's current
	// base/head because the event carried no event-time object pair.
	ProvenanceCurrentPR
)

// String names the provenance using its contract spelling.
func (p EvidenceProvenance) String() string {
	switch p {
	case ProvenanceNone:
		return "none"
	case ProvenanceDirectEvent:
		return "direct-event"
	case ProvenanceEventTime:
		return "event-time"
	case ProvenanceCurrentPR:
		return "current-pr"
	default:
		return fmt.Sprintf("provenance %d", uint8(p))
	}
}

// EvidenceOperation is the bounded GitHub work one evidence descriptor needs.
type EvidenceOperation uint8

const (
	// EvidenceSettled marks evidence that is already terminal without any
	// GitHub request: direct paths, an empty push, malformed direct data, or an
	// unsupported event form.
	EvidenceSettled EvidenceOperation = iota
	// EvidenceCommit reads the changed files of one exact commit.
	EvidenceCommit
	// EvidenceCompare reads the changed files between two exact commits.
	EvidenceCompare
	// EvidencePullRequest reads one pull request's current base and head.
	EvidencePullRequest
)

// String names the operation.
func (o EvidenceOperation) String() string {
	switch o {
	case EvidenceSettled:
		return "settled"
	case EvidenceCommit:
		return "commit"
	case EvidenceCompare:
		return "compare"
	case EvidencePullRequest:
		return "pr-metadata"
	default:
		return fmt.Sprintf("evidence operation %d", uint8(o))
	}
}

// EvidenceDescriptor names the immutable evidence one retained event needs. Its
// identity is the repository key plus immutable object identity, never a Scope
// identity or a pull request number alone, so adding a Scope duplicates no work.
type EvidenceDescriptor struct {
	operation  EvidenceOperation
	provenance EvidenceProvenance
	repository Repository
	before     string
	base       string
	head       string
	number     int
	settled    EvidenceOutcome
}

// NewSettledEvidence describes evidence that is already terminal, so no GitHub
// work and no cache entry can change it.
func NewSettledEvidence(outcome EvidenceOutcome) EvidenceDescriptor {
	return EvidenceDescriptor{
		operation:  EvidenceSettled,
		provenance: outcome.Provenance(),
		settled:    outcome,
	}
}

// NewUnsupportedEvidence describes an event that carries no supported evidence
// form. Its path membership is unknown and it consumes no work.
func NewUnsupportedEvidence(reason string) EvidenceDescriptor {
	return NewSettledEvidence(UnsupportedOutcome(reason))
}

// NewIncompleteEvidence describes malformed direct evidence. It never falls back
// to another evidence form, because a discarded direct path is not an absent one.
func NewIncompleteEvidence(reason string) EvidenceDescriptor {
	return NewSettledEvidence(IncompleteOutcome(reason))
}

// NewDirectEvidence describes paths the event payload named itself. It is
// complete without a request and needs no persistent cache entry.
func NewDirectEvidence(paths ...ChangedPath) EvidenceDescriptor {
	return NewSettledEvidence(CompleteOutcome(ProvenanceDirectEvent, paths))
}

// NewUnchangedEvidence describes an event whose valid before/base and head
// objects are equal. The comparison they name is empty, so the changed-file set
// is complete and empty without a request, regardless of any contradictory size
// the payload reported.
func NewUnchangedEvidence() EvidenceDescriptor {
	return NewSettledEvidence(CompleteOutcome(ProvenanceEventTime, nil))
}

// NewCommitEvidence describes the changed files of one exact commit for an
// event whose before object must match the commit's verified sole parent.
func NewCommitEvidence(repository Repository, before, head string) (EvidenceDescriptor, error) {
	if err := requireRepository(repository); err != nil {
		return EvidenceDescriptor{}, err
	}
	normalizedBefore, beforeOK := NormalizeObjectSHA(before)
	normalizedHead, headOK := NormalizeObjectSHA(head)
	if !beforeOK || !headOK {
		return EvidenceDescriptor{}, fmt.Errorf("%w: commit evidence needs valid before and head objects", ErrInvalidEvidence)
	}
	return EvidenceDescriptor{
		operation:  EvidenceCommit,
		provenance: ProvenanceEventTime,
		repository: repository,
		before:     normalizedBefore,
		head:       normalizedHead,
	}, nil
}

// NewCompareEvidence describes the changed files between two distinct exact
// commits. Provenance stays with the descriptor because a comparison derived
// from a pull request's current refs is never event-time evidence.
func NewCompareEvidence(repository Repository, base, head string, provenance EvidenceProvenance) (EvidenceDescriptor, error) {
	if err := requireRepository(repository); err != nil {
		return EvidenceDescriptor{}, err
	}
	normalizedBase, baseOK := NormalizeObjectSHA(base)
	normalizedHead, headOK := NormalizeObjectSHA(head)
	if !baseOK || !headOK {
		return EvidenceDescriptor{}, fmt.Errorf("%w: compare evidence needs valid base and head objects", ErrInvalidEvidence)
	}
	if normalizedBase == normalizedHead {
		return EvidenceDescriptor{}, fmt.Errorf("%w: compare evidence needs distinct base and head objects", ErrInvalidEvidence)
	}
	return EvidenceDescriptor{
		operation:  EvidenceCompare,
		provenance: provenance,
		repository: repository,
		base:       normalizedBase,
		head:       normalizedHead,
	}, nil
}

// NewPullRequestEvidence describes the one metadata read that captures a pull
// request's current base and head for a refresh. It yields SHAs, never paths.
func NewPullRequestEvidence(repository Repository, number int) (EvidenceDescriptor, error) {
	if err := requireRepository(repository); err != nil {
		return EvidenceDescriptor{}, err
	}
	if number <= 0 {
		return EvidenceDescriptor{}, fmt.Errorf("%w: pull request evidence needs a positive number", ErrInvalidEvidence)
	}
	return EvidenceDescriptor{
		operation:  EvidencePullRequest,
		provenance: ProvenanceCurrentPR,
		repository: repository,
		number:     number,
	}, nil
}

func requireRepository(repository Repository) error {
	if repository.Owner() == "" || repository.Name() == "" {
		return fmt.Errorf("%w: evidence needs a validated repository", ErrInvalidEvidence)
	}
	return nil
}

// Operation reports the GitHub work this descriptor needs.
func (d EvidenceDescriptor) Operation() EvidenceOperation { return d.operation }

// Provenance reports which evidence form the descriptor produces.
func (d EvidenceDescriptor) Provenance() EvidenceProvenance { return d.provenance }

// Repository reports the repository the evidence belongs to.
func (d EvidenceDescriptor) Repository() Repository { return d.repository }

// Before reports the normalized before object of commit evidence.
func (d EvidenceDescriptor) Before() string { return d.before }

// Base reports the normalized base object of compare evidence.
func (d EvidenceDescriptor) Base() string { return d.base }

// Head reports the normalized head object of commit or compare evidence.
func (d EvidenceDescriptor) Head() string { return d.head }

// Number reports the pull request number of metadata evidence.
func (d EvidenceDescriptor) Number() int { return d.number }

// Settled reports the terminal outcome of evidence that needs no GitHub work.
func (d EvidenceDescriptor) Settled() (EvidenceOutcome, bool) {
	return d.settled, d.operation == EvidenceSettled
}

// Key returns the deterministic work key that coalesces equal evidence across
// retained events and Scopes. Evidence needing no request has no work key.
func (d EvidenceDescriptor) Key() string {
	switch d.operation {
	case EvidenceCommit:
		return "commit(" + d.repository.Key() + "," + d.head + ")"
	case EvidenceCompare:
		return "compare(" + d.repository.Key() + "," + d.base + "," + d.head + ")"
	case EvidencePullRequest:
		return "pr-metadata(" + d.repository.Key() + "," + strconv.Itoa(d.number) + ")"
	default:
		return ""
	}
}

// NormalizeObjectSHA reports the lowercase form of an object SHA and whether it
// is valid. A valid SHA is exactly 40 ASCII hexadecimal digits; the all-zero SHA
// names no object and is invalid, so a creation or deletion side never infers
// branch contents.
func NormalizeObjectSHA(value string) (string, bool) {
	if len(value) != objectSHALength {
		return "", false
	}
	var builder strings.Builder
	builder.Grow(objectSHALength)
	zero := true
	for index := 0; index < len(value); index++ {
		digit := value[index]
		switch {
		case digit >= '0' && digit <= '9':
		case digit >= 'a' && digit <= 'f':
		case digit >= 'A' && digit <= 'F':
			digit += 'a' - 'A'
		default:
			return "", false
		}
		if digit != '0' {
			zero = false
		}
		builder.WriteByte(digit)
	}
	if zero {
		return "", false
	}
	return builder.String(), true
}
