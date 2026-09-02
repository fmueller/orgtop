package domain_test

import (
	"slices"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

func eventIn(t *testing.T, repository string) domain.Event {
	t.Helper()
	return domain.Event{
		ID:         "1",
		OccurredAt: time.Unix(0, 0).UTC(),
		Repository: mustParseRepository(t, repository),
		Category:   domain.CategoryPush,
		EntityKind: domain.EntityCommit,
	}
}

func mustScopeSet(t *testing.T, scopes ...domain.Scope) domain.ScopeSet {
	t.Helper()
	set, err := domain.NewScopeSet(scopes)
	if err != nil {
		t.Fatalf("NewScopeSet(%v) returned error: %v", scopes, err)
	}
	return set
}

func completePaths(t *testing.T, provenance domain.EvidenceProvenance, values ...string) domain.EvidenceOutcome {
	t.Helper()
	paths := make([]domain.ChangedPath, 0, len(values))
	for _, value := range values {
		paths = append(paths, mustChangedPath(t, value))
	}
	return domain.CompleteOutcome(provenance, paths)
}

// TestUnknownReasonOrderAndSpelling pins the closed RG-004 reason groups, their
// deterministic breakdown order, and their contract spelling.
func TestUnknownReasonOrderAndSpelling(t *testing.T) {
	tests := []struct {
		reason domain.UnknownReason
		want   string
	}{
		{domain.ReasonUnsupported, "unsupported"},
		{domain.ReasonIncomplete, "incomplete"},
		{domain.ReasonUnavailable, "unavailable"},
		{domain.ReasonDenied, "denied"},
		{domain.ReasonRateLimited, "rate-limited"},
		{domain.ReasonFailed, "failed"},
	}
	ordered := make([]domain.UnknownReason, len(tests))
	for i, test := range tests {
		ordered[i] = test.reason
	}

	if got := domain.UnknownReasons(); !slices.Equal(got, ordered) {
		t.Fatalf("UnknownReasons() = %v, want %v", got, ordered)
	}
	for i, test := range tests {
		if i > 0 && ordered[i-1] >= test.reason {
			t.Errorf("reason %v does not sort after %v", test.reason, ordered[i-1])
		}
		if got := test.reason.String(); got != test.want {
			t.Errorf("%d.String() = %q, want %q", i, got, test.want)
		}
	}
	if got := domain.ReasonNone.String(); got != "none" {
		t.Errorf("ReasonNone.String() = %q, want %q", got, "none")
	}
}

// TestMembershipKindSpelling pins the closed three-value outcome vocabulary.
func TestMembershipKindSpelling(t *testing.T) {
	tests := []struct {
		kind domain.MembershipKind
		want string
	}{
		{domain.MembershipMember, "member"},
		{domain.MembershipNotMember, "not-member"},
		{domain.MembershipUnknown, "unknown"},
	}
	for _, test := range tests {
		if got := test.kind.String(); got != test.want {
			t.Errorf("%d.String() = %q, want %q", int(test.kind), got, test.want)
		}
	}
}

// TestRepositoryScopeEvaluatesFromIdentity guards FR-004: a repository Scope is
// decided by repository identity alone and never becomes unknown, whatever the
// changed-file evidence did.
func TestRepositoryScopeEvaluatesFromIdentity(t *testing.T) {
	scope := domain.NewRepositoryScope(mustParseRepository(t, "acme/api"))
	outcomes := []domain.EvidenceOutcome{
		completePaths(t, domain.ProvenanceEventTime, "services/api/main.go"),
		domain.UnsupportedOutcome("issue event"),
		domain.IncompleteOutcome("hard cap"),
		domain.UnavailableOutcome("deleted"),
		domain.DeniedOutcome("forbidden"),
		domain.RateLimitedOutcome(time.Unix(100, 0).UTC()),
		domain.FailedOutcome("timeout"),
		domain.CanceledOutcome(),
	}

	for _, outcome := range outcomes {
		membership, ok := scope.Evaluate(eventIn(t, "Acme/API"), outcome)
		if !ok {
			t.Fatalf("Evaluate with %v outcome reported no publishable membership", outcome.Kind())
		}
		if membership.Kind() != domain.MembershipMember {
			t.Errorf("Evaluate with %v outcome = %v, want member", outcome.Kind(), membership.Kind())
		}
		if membership.Reason() != domain.ReasonNone {
			t.Errorf("Evaluate with %v outcome kept reason %v, want none", outcome.Kind(), membership.Reason())
		}
	}
}

// TestScopeEvaluationRejectsOtherRepositories guards FR-002: a Scope of another
// repository is never a member and never unknown, so missing evidence in one
// repository cannot degrade another repository's coverage.
func TestScopeEvaluationRejectsOtherRepositories(t *testing.T) {
	scopes := []domain.Scope{
		domain.NewRepositoryScope(mustParseRepository(t, "acme/api")),
		mustPathScope(t, "acme/api", literal("services"), separator(), recursive()),
	}
	for _, scope := range scopes {
		membership, ok := scope.Evaluate(eventIn(t, "acme/web"), domain.CanceledOutcome())
		if !ok {
			t.Fatalf("%v: Evaluate reported no publishable membership", scope)
		}
		if membership.Kind() != domain.MembershipNotMember {
			t.Errorf("%v: Evaluate = %v, want not-member", scope, membership.Kind())
		}
	}
}

// TestPathScopeOutcomeMatrix records the full closed outcome matrix: only
// complete evidence decides member or not-member, and every other evidence kind
// stays unknown under its single primary RG-004 reason group.
func TestPathScopeOutcomeMatrix(t *testing.T) {
	retryAt := time.Unix(1_700_000_000, 0).UTC()
	tests := []struct {
		name           string
		outcome        domain.EvidenceOutcome
		wantKind       domain.MembershipKind
		wantReason     domain.UnknownReason
		wantProvenance domain.EvidenceProvenance
		wantRetryAt    time.Time
	}{
		{
			name:           "complete evidence matching the matcher is a member",
			outcome:        completePaths(t, domain.ProvenanceEventTime, "docs/readme.md", "services/api/main.go"),
			wantKind:       domain.MembershipMember,
			wantProvenance: domain.ProvenanceEventTime,
		},
		{
			name:           "complete evidence missing the matcher is not a member",
			outcome:        completePaths(t, domain.ProvenanceEventTime, "docs/readme.md"),
			wantKind:       domain.MembershipNotMember,
			wantProvenance: domain.ProvenanceEventTime,
		},
		{
			name:           "a complete empty set decides not-member",
			outcome:        completePaths(t, domain.ProvenanceDirectEvent),
			wantKind:       domain.MembershipNotMember,
			wantProvenance: domain.ProvenanceDirectEvent,
		},
		{
			name:           "complete current-PR evidence may decide a qualified member",
			outcome:        completePaths(t, domain.ProvenanceCurrentPR, "services/api/main.go"),
			wantKind:       domain.MembershipMember,
			wantProvenance: domain.ProvenanceCurrentPR,
		},
		{
			name:       "unsupported evidence is unknown",
			outcome:    domain.UnsupportedOutcome("issue only event"),
			wantKind:   domain.MembershipUnknown,
			wantReason: domain.ReasonUnsupported,
		},
		{
			name:       "incomplete evidence is unknown",
			outcome:    domain.IncompleteOutcome("malformed path"),
			wantKind:   domain.MembershipUnknown,
			wantReason: domain.ReasonIncomplete,
		},
		{
			name:       "unavailable evidence is unknown",
			outcome:    domain.UnavailableOutcome("commit no longer resolves"),
			wantKind:   domain.MembershipUnknown,
			wantReason: domain.ReasonUnavailable,
		},
		{
			name:       "denied evidence is unknown",
			outcome:    domain.DeniedOutcome("forbidden"),
			wantKind:   domain.MembershipUnknown,
			wantReason: domain.ReasonDenied,
		},
		{
			name:        "rate limited evidence is unknown and retains its retry instant",
			outcome:     domain.RateLimitedOutcome(retryAt),
			wantKind:    domain.MembershipUnknown,
			wantReason:  domain.ReasonRateLimited,
			wantRetryAt: retryAt,
		},
		{
			name:       "failed evidence is unknown",
			outcome:    domain.FailedOutcome("request timeout"),
			wantKind:   domain.MembershipUnknown,
			wantReason: domain.ReasonFailed,
		},
	}

	scope := mustPathScope(t, "acme/api", literal("services"), separator(), literal("api"))
	event := eventIn(t, "acme/api")
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			membership, ok := scope.Evaluate(event, test.outcome)
			if !ok {
				t.Fatalf("Evaluate reported no publishable membership")
			}
			if membership.Kind() != test.wantKind {
				t.Errorf("Kind() = %v, want %v", membership.Kind(), test.wantKind)
			}
			if membership.Reason() != test.wantReason {
				t.Errorf("Reason() = %v, want %v", membership.Reason(), test.wantReason)
			}
			if membership.Provenance() != test.wantProvenance {
				t.Errorf("Provenance() = %v, want %v", membership.Provenance(), test.wantProvenance)
			}
			if !membership.RetryAt().Equal(test.wantRetryAt) {
				t.Errorf("RetryAt() = %v, want %v", membership.RetryAt(), test.wantRetryAt)
			}
			if got, want := membership.IsUnknown(), test.wantKind == domain.MembershipUnknown; got != want {
				t.Errorf("IsUnknown() = %t, want %t", got, want)
			}
			if got, want := membership.IsMember(), test.wantKind == domain.MembershipMember; got != want {
				t.Errorf("IsMember() = %t, want %t", got, want)
			}
		})
	}
}

// TestQualifiedCurrentPRCountsOnlyCompleteMembers guards RG-004: the qualified
// current-PR count admits member outcomes decided from complete current-PR
// evidence and never not-member outcomes or failed attempts.
func TestQualifiedCurrentPRCountsOnlyCompleteMembers(t *testing.T) {
	scope := mustPathScope(t, "acme/api", literal("services"), separator(), literal("api"))
	event := eventIn(t, "acme/api")
	tests := []struct {
		name    string
		outcome domain.EvidenceOutcome
		want    bool
	}{
		{"member from current PR evidence", completePaths(t, domain.ProvenanceCurrentPR, "services/api/main.go"), true},
		{"not-member from current PR evidence", completePaths(t, domain.ProvenanceCurrentPR, "docs/readme.md"), false},
		{"failed current PR attempt", domain.FailedOutcome("timeout"), false},
		{"member from event-time evidence", completePaths(t, domain.ProvenanceEventTime, "services/api/main.go"), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			membership, ok := scope.Evaluate(event, test.outcome)
			if !ok {
				t.Fatalf("Evaluate reported no publishable membership")
			}
			if got := membership.QualifiedCurrentPR(); got != test.want {
				t.Errorf("QualifiedCurrentPR() = %t, want %t", got, test.want)
			}
		})
	}
}

// TestCanceledEvidencePublishesNoPathMembership guards RG-004: canceled work is
// never synthesized into unknown, so a path Scope reports no publishable
// membership at all.
func TestCanceledEvidencePublishesNoPathMembership(t *testing.T) {
	scope := mustPathScope(t, "acme/api", literal("services"), separator(), recursive())
	if membership, ok := scope.Evaluate(eventIn(t, "acme/api"), domain.CanceledOutcome()); ok {
		t.Fatalf("Evaluate returned %v for canceled evidence, want no publishable membership", membership.Kind())
	}
}

// TestRenameEvidenceRetainsOneMembershipPerScope guards A-024: a rename offers
// both normalized paths and each Scope keeps exactly one membership.
func TestRenameEvidenceRetainsOneMembershipPerScope(t *testing.T) {
	outcome := completePaths(t, domain.ProvenanceEventTime, "services/old/item.go", "services/new/item.go")
	event := eventIn(t, "acme/api")
	scopes := []domain.Scope{
		mustPathScope(t, "acme/api", literal("services"), separator(), literal("old")),
		mustPathScope(t, "acme/api", literal("services"), separator(), literal("new")),
		mustPathScope(t, "acme/api", literal("services")),
	}

	for _, scope := range scopes {
		membership, ok := scope.Evaluate(event, outcome)
		if !ok {
			t.Fatalf("%v: Evaluate reported no publishable membership", scope)
		}
		if membership.Kind() != domain.MembershipMember {
			t.Errorf("%v: Evaluate = %v, want member", scope, membership.Kind())
		}
	}

	memberships, ok := mustScopeSet(t, scopes...).Evaluate(event, outcome)
	if !ok {
		t.Fatalf("ScopeSet.Evaluate reported no publishable memberships")
	}
	if len(memberships) != len(scopes) {
		t.Fatalf("ScopeSet.Evaluate returned %d memberships, want %d", len(memberships), len(scopes))
	}
	seen := make(map[domain.ScopeIdentity]int, len(memberships))
	for _, membership := range memberships {
		seen[membership.Scope.Identity()]++
	}
	for identity, count := range seen {
		if count != 1 {
			t.Errorf("scope %v retained %d memberships, want exactly one", identity, count)
		}
	}
}

// TestScopeSetEvaluationRetainsIndependentOverlap guards A-038: one event may be
// a member of one Scope, unknown in another, and not-member in a third, and the
// three outcomes are retained independently in stable Scope order.
func TestScopeSetEvaluationRetainsIndependentOverlap(t *testing.T) {
	member := mustPathScope(t, "acme/api", literal("services"), separator(), literal("api"))
	other := mustPathScope(t, "acme/api", literal("docs"))
	elsewhere := mustPathScope(t, "acme/web", literal("services"))
	repository := domain.NewRepositoryScope(mustParseRepository(t, "acme/api"))

	set := mustScopeSet(t, other, elsewhere, member, repository)
	event := eventIn(t, "acme/api")
	memberships, ok := set.Evaluate(event, completePaths(t, domain.ProvenanceEventTime, "services/api/main.go"))
	if !ok {
		t.Fatalf("ScopeSet.Evaluate reported no publishable memberships")
	}

	want := []struct {
		scope string
		kind  domain.MembershipKind
	}{
		{"acme/api", domain.MembershipMember},
		{"acme/api:docs", domain.MembershipNotMember},
		{"acme/api:services/api", domain.MembershipMember},
	}
	if len(memberships) != len(want) {
		t.Fatalf("ScopeSet.Evaluate returned %d memberships, want %d", len(memberships), len(want))
	}
	for i, membership := range memberships {
		if got := membership.Scope.String(); got != want[i].scope {
			t.Errorf("membership %d scope = %q, want %q", i, got, want[i].scope)
		}
		if got := membership.Membership.Kind(); got != want[i].kind {
			t.Errorf("membership %d kind = %v, want %v", i, got, want[i].kind)
		}
	}
}

// TestScopeSetEvaluationKeepsUnknownPerScope guards RG-004: an unknown outcome
// applies to every path Scope of the repository while the repository Scope stays
// known from identity.
func TestScopeSetEvaluationKeepsUnknownPerScope(t *testing.T) {
	set := mustScopeSet(t,
		domain.NewRepositoryScope(mustParseRepository(t, "acme/api")),
		mustPathScope(t, "acme/api", literal("services"), separator(), literal("api")),
		mustPathScope(t, "acme/api", literal("docs")),
	)

	memberships, ok := set.Evaluate(eventIn(t, "acme/api"), domain.DeniedOutcome("forbidden"))
	if !ok {
		t.Fatalf("ScopeSet.Evaluate reported no publishable memberships")
	}
	want := []struct {
		scope  string
		kind   domain.MembershipKind
		reason domain.UnknownReason
	}{
		{"acme/api", domain.MembershipMember, domain.ReasonNone},
		{"acme/api:docs", domain.MembershipUnknown, domain.ReasonDenied},
		{"acme/api:services/api", domain.MembershipUnknown, domain.ReasonDenied},
	}
	if len(memberships) != len(want) {
		t.Fatalf("ScopeSet.Evaluate returned %d memberships, want %d", len(memberships), len(want))
	}
	for i, membership := range memberships {
		if got := membership.Scope.String(); got != want[i].scope {
			t.Fatalf("membership %d scope = %q, want %q", i, got, want[i].scope)
		}
		if got := membership.Membership.Kind(); got != want[i].kind {
			t.Errorf("%s kind = %v, want %v", want[i].scope, got, want[i].kind)
		}
		if got := membership.Membership.Reason(); got != want[i].reason {
			t.Errorf("%s reason = %v, want %v", want[i].scope, got, want[i].reason)
		}
	}
}

// TestScopeSetEvaluationCancelsWholeEvent guards RG-004: canceled evidence
// publishes nothing for the event, not a partial mix of known repository
// memberships and synthesized unknowns.
func TestScopeSetEvaluationCancelsWholeEvent(t *testing.T) {
	set := mustScopeSet(t,
		domain.NewRepositoryScope(mustParseRepository(t, "acme/api")),
		mustPathScope(t, "acme/api", literal("services")),
	)
	if memberships, ok := set.Evaluate(eventIn(t, "acme/api"), domain.CanceledOutcome()); ok {
		t.Fatalf("ScopeSet.Evaluate returned %d memberships for canceled evidence, want none", len(memberships))
	}
}

// TestScopeSetEvaluationSkipsOtherRepositories guards RG-004: an event receives
// one outcome per Scope of its own repository only.
func TestScopeSetEvaluationSkipsOtherRepositories(t *testing.T) {
	set := mustScopeSet(t,
		mustPathScope(t, "acme/web", literal("services")),
		domain.NewRepositoryScope(mustParseRepository(t, "acme/web")),
	)
	memberships, ok := set.Evaluate(eventIn(t, "acme/api"), domain.FailedOutcome("timeout"))
	if !ok {
		t.Fatalf("ScopeSet.Evaluate reported no publishable memberships")
	}
	if len(memberships) != 0 {
		t.Fatalf("ScopeSet.Evaluate returned %d memberships for another repository, want none", len(memberships))
	}
}

// TestUnknownMembershipRecoversOnLaterEvidence guards FR-004 recovery: the same
// Scope and event decide member once the retried evidence completes, and the
// unknown reason does not survive.
func TestUnknownMembershipRecoversOnLaterEvidence(t *testing.T) {
	scope := mustPathScope(t, "acme/api", literal("services"), separator(), recursive())
	event := eventIn(t, "acme/api")

	unknown, ok := scope.Evaluate(event, domain.RateLimitedOutcome(time.Unix(10, 0).UTC()))
	if !ok || unknown.Kind() != domain.MembershipUnknown {
		t.Fatalf("first Evaluate = (%v, %t), want unknown", unknown.Kind(), ok)
	}

	recovered, ok := scope.Evaluate(event, completePaths(t, domain.ProvenanceEventTime, "services/api/main.go"))
	if !ok {
		t.Fatalf("retried Evaluate reported no publishable membership")
	}
	if recovered.Kind() != domain.MembershipMember {
		t.Errorf("retried Evaluate = %v, want member", recovered.Kind())
	}
	if recovered.Reason() != domain.ReasonNone {
		t.Errorf("retried Evaluate kept reason %v, want none", recovered.Reason())
	}
	if !recovered.RetryAt().IsZero() {
		t.Errorf("retried Evaluate kept retry instant %v, want zero", recovered.RetryAt())
	}
	if unknown.Reason() != domain.ReasonRateLimited {
		t.Errorf("the earlier membership changed to %v, want rate-limited", unknown.Reason())
	}
}

// TestZeroMembershipIsUnknown guards the closed FR-004 invariant against an
// accidentally zero-valued Membership: an outcome nothing decided must fail safe
// as unknown rather than report a confirmed member with no evidence behind it.
func TestZeroMembershipIsUnknown(t *testing.T) {
	var membership domain.Membership

	if membership.Kind() != domain.MembershipUnknown {
		t.Errorf("zero Membership Kind() = %v, want unknown", membership.Kind())
	}
	if membership.IsMember() {
		t.Error("zero Membership reports IsMember() = true, want false")
	}
	if !membership.IsUnknown() {
		t.Error("zero Membership reports IsUnknown() = false, want true")
	}
	if membership.QualifiedCurrentPR() {
		t.Error("zero Membership reports QualifiedCurrentPR() = true, want false")
	}
	if zero := (domain.ScopeMembership{}); !zero.Membership.IsUnknown() {
		t.Error("zero ScopeMembership carries a decided membership, want unknown")
	}
}
