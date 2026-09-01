package domain

import (
	"fmt"
	"slices"
	"time"
)

// OutcomeKind is the closed set of results one evidence descriptor can produce.
// Only OutcomeComplete may feed the path matcher; no other kind is ever
// converted into an empty changed-file set.
type OutcomeKind uint8

const (
	// OutcomeComplete is a fully proven valid changed-path set, empty included.
	OutcomeComplete OutcomeKind = iota
	// OutcomeUnsupported reports an event with no supported evidence form.
	OutcomeUnsupported
	// OutcomeIncomplete reports a hard cap, budget, malformed data, or a failed
	// completeness proof.
	OutcomeIncomplete
	// OutcomeUnavailable reports an exact entity that can no longer be resolved.
	OutcomeUnavailable
	// OutcomeDenied reports authentication or authorization refusing the lookup.
	OutcomeDenied
	// OutcomeRateLimited reports work GitHub instructed OrgTop to delay.
	OutcomeRateLimited
	// OutcomeFailed reports a timeout, transport, or temporary server failure.
	OutcomeFailed
	// OutcomeCanceled reports refresh or shutdown cancellation; nothing partial
	// is published.
	OutcomeCanceled
)

// String names the outcome kind using its contract spelling.
func (k OutcomeKind) String() string {
	switch k {
	case OutcomeComplete:
		return "complete"
	case OutcomeUnsupported:
		return "unsupported"
	case OutcomeIncomplete:
		return "incomplete"
	case OutcomeUnavailable:
		return "unavailable"
	case OutcomeDenied:
		return "denied"
	case OutcomeRateLimited:
		return "rate-limited"
	case OutcomeFailed:
		return "failed"
	case OutcomeCanceled:
		return "canceled"
	default:
		return fmt.Sprintf("outcome %d", uint8(k))
	}
}

// EvidenceOutcome is the normalized result of one evidence descriptor. It never
// carries an HTTP status, a transport payload, a pagination link, or a response
// header, so downstream code cannot re-derive discarded evidence.
type EvidenceOutcome struct {
	kind       OutcomeKind
	provenance EvidenceProvenance
	paths      []ChangedPath
	reason     string
	retryAt    time.Time
	soleParent string
}

// CompleteOutcome reports a fully proven changed-path set. An empty set is a
// complete result and may decide not-member.
func CompleteOutcome(provenance EvidenceProvenance, paths []ChangedPath) EvidenceOutcome {
	return EvidenceOutcome{kind: OutcomeComplete, provenance: provenance, paths: slices.Clone(paths)}
}

// UnsupportedOutcome reports an event form that carries no changed-file evidence.
func UnsupportedOutcome(reason string) EvidenceOutcome {
	return EvidenceOutcome{kind: OutcomeUnsupported, reason: reason}
}

// IncompleteOutcome reports evidence that failed its completeness proof or hit a
// closed capacity. No valid-looking subset of it is admitted.
func IncompleteOutcome(reason string) EvidenceOutcome {
	return EvidenceOutcome{kind: OutcomeIncomplete, reason: reason}
}

// UnavailableOutcome reports an exact entity GitHub no longer serves.
func UnavailableOutcome(reason string) EvidenceOutcome {
	return EvidenceOutcome{kind: OutcomeUnavailable, reason: reason}
}

// DeniedOutcome reports a credential that may not read the entity.
func DeniedOutcome(reason string) EvidenceOutcome {
	return EvidenceOutcome{kind: OutcomeDenied, reason: reason}
}

// RateLimitedOutcome reports delayed work and the earliest instructed retry.
func RateLimitedOutcome(retryAt time.Time) EvidenceOutcome {
	return EvidenceOutcome{kind: OutcomeRateLimited, retryAt: retryAt}
}

// FailedOutcome reports a timeout, transport, or temporary server failure.
func FailedOutcome(reason string) EvidenceOutcome {
	return EvidenceOutcome{kind: OutcomeFailed, reason: reason}
}

// CanceledOutcome reports canceled work that publishes nothing.
func CanceledOutcome() EvidenceOutcome {
	return EvidenceOutcome{kind: OutcomeCanceled}
}

// Kind reports the outcome kind.
func (o EvidenceOutcome) Kind() OutcomeKind { return o.kind }

// IsComplete reports whether the outcome may feed the path matcher.
func (o EvidenceOutcome) IsComplete() bool { return o.kind == OutcomeComplete }

// Provenance reports which evidence form produced a complete set.
func (o EvidenceOutcome) Provenance() EvidenceProvenance { return o.provenance }

// Paths returns a copy of the proven changed paths. Only a complete outcome has
// any, so an unknown result can never be read as an empty change set.
func (o EvidenceOutcome) Paths() []ChangedPath { return slices.Clone(o.paths) }

// Reason reports the sanitized cause of a non-complete outcome.
func (o EvidenceOutcome) Reason() string { return o.reason }

// RetryAt reports the earliest instructed retry of a rate-limited outcome.
func (o EvidenceOutcome) RetryAt() time.Time { return o.retryAt }

// SoleParent reports the verified single parent of a complete commit result.
func (o EvidenceOutcome) SoleParent() string { return o.soleParent }

// WithSoleParent records the verified sole parent a commit response reported, so
// one coalesced result can be checked against each event's own before SHA.
func (o EvidenceOutcome) WithSoleParent(sha string) EvidenceOutcome {
	normalized, ok := NormalizeObjectSHA(sha)
	if !ok {
		return IncompleteOutcome("commit parent verification failed")
	}
	o.soleParent = normalized
	return o
}

// ForSoleParent applies one coalesced commit result to a single event. The event
// receives the complete set only when its own before SHA is the verified sole
// parent; every other event sharing that head is incomplete rather than guessed.
func (o EvidenceOutcome) ForSoleParent(before string) EvidenceOutcome {
	if !o.IsComplete() {
		return o
	}
	normalized, ok := NormalizeObjectSHA(before)
	if !ok || normalized != o.soleParent {
		return IncompleteOutcome("commit parent does not match the event's before object")
	}
	return o
}
