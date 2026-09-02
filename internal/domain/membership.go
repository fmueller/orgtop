package domain

import (
	"fmt"
	"slices"
	"time"
)

// MembershipKind is the closed three-value result of one Scope membership
// evaluation. Nothing outside this vocabulary is ever published, and unknown is
// never collapsed into member or not-member.
type MembershipKind uint8

const (
	// MembershipUnknown reports evidence that decides neither. It is the zero
	// value, so a membership nothing decided fails safe as undecided rather than
	// as a confirmed member with no evidence behind it.
	MembershipUnknown MembershipKind = iota
	// MembershipMember reports sufficient evidence and a Scope match.
	MembershipMember
	// MembershipNotMember reports sufficient evidence and no Scope match.
	MembershipNotMember
)

// String names the membership kind using its contract spelling.
func (k MembershipKind) String() string {
	switch k {
	case MembershipMember:
		return "member"
	case MembershipNotMember:
		return "not-member"
	case MembershipUnknown:
		return "unknown"
	default:
		return fmt.Sprintf("membership kind %d", uint8(k))
	}
}

// UnknownReason is the single primary reason group of one unknown membership.
// The declaration order is the closed RG-004 breakdown order.
type UnknownReason uint8

const (
	// ReasonNone marks a decided membership, which carries no reason.
	ReasonNone UnknownReason = iota
	// ReasonUnsupported reports an event form carrying no changed-file evidence.
	ReasonUnsupported
	// ReasonIncomplete reports malformed data or a spent capacity, so no subset
	// of the evidence is admitted.
	ReasonIncomplete
	// ReasonUnavailable reports an entity that can no longer be resolved.
	ReasonUnavailable
	// ReasonDenied reports authentication or authorization refusing the lookup.
	ReasonDenied
	// ReasonRateLimited reports work GitHub instructed OrgTop to delay.
	ReasonRateLimited
	// ReasonFailed reports a timeout, transport, or temporary server failure.
	ReasonFailed
)

// unknownReasonOrder is the deterministic RG-004 breakdown order.
var unknownReasonOrder = []UnknownReason{
	ReasonUnsupported,
	ReasonIncomplete,
	ReasonUnavailable,
	ReasonDenied,
	ReasonRateLimited,
	ReasonFailed,
}

// UnknownReasons returns the unknown reason groups in the deterministic
// breakdown order presentation and aggregation must follow.
func UnknownReasons() []UnknownReason { return slices.Clone(unknownReasonOrder) }

// String names the reason group using its contract spelling.
func (r UnknownReason) String() string {
	switch r {
	case ReasonNone:
		return "none"
	case ReasonUnsupported:
		return "unsupported"
	case ReasonIncomplete:
		return "incomplete"
	case ReasonUnavailable:
		return "unavailable"
	case ReasonDenied:
		return "denied"
	case ReasonRateLimited:
		return "rate-limited"
	case ReasonFailed:
		return "failed"
	default:
		return fmt.Sprintf("unknown reason %d", uint8(r))
	}
}

// Membership is one event's explicit outcome in one Scope. A decided outcome
// retains the provenance it was read from, so current-PR membership can be
// qualified and is never described as event-time evidence. An unknown outcome
// retains its reason group and any instructed retry instant, so both stay
// inspectable.
type Membership struct {
	kind       MembershipKind
	reason     UnknownReason
	provenance EvidenceProvenance
	retryAt    time.Time
}

// Kind reports the membership kind.
func (m Membership) Kind() MembershipKind { return m.kind }

// IsMember reports whether the event is a confirmed member of the Scope.
func (m Membership) IsMember() bool { return m.kind == MembershipMember }

// IsUnknown reports whether the membership is undecided.
func (m Membership) IsUnknown() bool { return m.kind == MembershipUnknown }

// Reason reports the primary reason group of an unknown membership and
// ReasonNone for a decided one.
func (m Membership) Reason() UnknownReason { return m.reason }

// Provenance reports the evidence form a decided membership was read from.
func (m Membership) Provenance() EvidenceProvenance { return m.provenance }

// RetryAt reports the earliest instructed retry of a rate-limited unknown.
func (m Membership) RetryAt() time.Time { return m.retryAt }

// QualifiedCurrentPR reports whether the membership is the qualified current-PR
// kind RG-004 counts: a member decided from complete current-PR evidence. A
// not-member outcome and an incomplete attempt never qualify.
func (m Membership) QualifiedCurrentPR() bool {
	return m.kind == MembershipMember && m.provenance == ProvenanceCurrentPR
}

// String renders the membership using its contract spelling, naming the reason
// group of an unknown outcome.
func (m Membership) String() string {
	if m.kind == MembershipUnknown {
		return fmt.Sprintf("unknown(%s)", m.reason)
	}
	return m.kind.String()
}

// unknownMembership returns the undecided membership for one reason group.
func unknownMembership(reason UnknownReason) Membership {
	return Membership{kind: MembershipUnknown, reason: reason}
}

// Evaluate decides this Scope's membership for one event from its settled
// changed-file evidence outcome. The second result is false when nothing may be
// published: canceled evidence is never synthesized into unknown.
//
// A repository Scope is decided by repository identity alone and stays evaluable
// whatever the evidence did. A path Scope of another repository is not a member
// without consulting evidence, so one repository's missing evidence never
// degrades another's coverage. Only a complete outcome reaches the matcher.
func (s Scope) Evaluate(event Event, outcome EvidenceOutcome) (Membership, bool) {
	if s.repository.Key() != event.Repository.Key() {
		return Membership{kind: MembershipNotMember}, true
	}
	if s.kind == ScopeRepository {
		return Membership{kind: MembershipMember}, true
	}

	switch outcome.Kind() {
	case OutcomeComplete:
		kind := MembershipNotMember
		if s.matcher.MatchesAny(outcome.Paths()...) {
			kind = MembershipMember
		}
		return Membership{kind: kind, provenance: outcome.Provenance()}, true
	case OutcomeUnsupported:
		return unknownMembership(ReasonUnsupported), true
	case OutcomeIncomplete:
		return unknownMembership(ReasonIncomplete), true
	case OutcomeUnavailable:
		return unknownMembership(ReasonUnavailable), true
	case OutcomeDenied:
		return unknownMembership(ReasonDenied), true
	case OutcomeRateLimited:
		return Membership{kind: MembershipUnknown, reason: ReasonRateLimited, retryAt: outcome.RetryAt()}, true
	case OutcomeFailed:
		return unknownMembership(ReasonFailed), true
	case OutcomeCanceled:
		return Membership{}, false
	default:
		return unknownMembership(ReasonIncomplete), true
	}
}

// ScopeMembership pairs one Scope with the event outcome it retained.
type ScopeMembership struct {
	// Scope is the selected Scope the outcome belongs to.
	Scope Scope
	// Membership is that Scope's explicit outcome for the event.
	Membership Membership
}

// Evaluate decides one event's membership in every selected Scope of the event's
// repository, in the stable Scope identity order. Overlapping path Scopes retain
// independent outcomes rather than collapsing into an arbitrary single owner.
//
// The second result is false when the evidence was canceled: the event publishes
// no membership at all rather than a partial mix of known repository outcomes and
// synthesized unknowns.
func (s ScopeSet) Evaluate(event Event, outcome EvidenceOutcome) ([]ScopeMembership, bool) {
	if !s.Contains(event.Repository) {
		return nil, true
	}
	if outcome.Kind() == OutcomeCanceled {
		return nil, false
	}
	ordered := s.Ordered()
	memberships := make([]ScopeMembership, 0, len(ordered))
	for _, scope := range ordered {
		if scope.repository.Key() != event.Repository.Key() {
			continue
		}
		membership, ok := scope.Evaluate(event, outcome)
		if !ok {
			return nil, false
		}
		memberships = append(memberships, ScopeMembership{Scope: scope, Membership: membership})
	}
	return memberships, true
}
