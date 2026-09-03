package domain_test

import (
	"slices"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// pathScope builds one path Scope of a repository from already tokenized
// pattern segments, keeping the scoped snapshot tests readable.
func pathScope(t *testing.T, repo string, tokens ...domain.MatcherToken) domain.Scope {
	t.Helper()
	scope, err := domain.NewPathScope(mustParseRepository(t, repo), mustMatcher(t, tokens...))
	if err != nil {
		t.Fatalf("NewPathScope(%q) returned error: %v", repo, err)
	}
	return scope
}

// complete returns event-time evidence proving the changed paths.
func complete(t *testing.T, values ...string) domain.EvidenceOutcome {
	t.Helper()
	return completePaths(t, domain.ProvenanceEventTime, values...)
}

// currentPR returns qualified current-PR evidence proving the changed paths.
func currentPR(t *testing.T, values ...string) domain.EvidenceOutcome {
	t.Helper()
	return completePaths(t, domain.ProvenanceCurrentPR, values...)
}

// evidenceOf pairs one already built event with its settled evidence.
func evidenceOf(event domain.Event, outcome domain.EvidenceOutcome) domain.EventEvidence {
	return domain.EventEvidence{Event: event, Outcome: outcome}
}

// pushEvidence pairs one commit push event of a repository with its settled
// evidence, the form most scoped snapshot cases exercise.
func pushEvidence(t *testing.T, id string, minute int, repo string, outcome domain.EvidenceOutcome) domain.EventEvidence {
	t.Helper()
	return evidenceOf(testEvent(t, id, minute, repo, domain.CategoryPush, domain.EntityCommit), outcome)
}

// scopedActivity groups one repository's events with their settled evidence.
func scopedActivity(t *testing.T, repo string, evidence ...domain.EventEvidence) domain.ScopedActivity {
	t.Helper()
	return domain.ScopedActivity{Events: evidence}
}

// scopeLabels renders the aggregate rows in prepared order.
func scopeLabels(aggregates []domain.ScopeAggregate) []string {
	labels := make([]string, 0, len(aggregates))
	for _, aggregate := range aggregates {
		labels = append(labels, aggregate.Scope.String())
	}
	return labels
}

// findScopeAggregate returns the prepared row of one Scope identity.
func findScopeAggregate(t *testing.T, snapshot domain.ScopedSnapshot, scope domain.Scope) domain.ScopeAggregate {
	t.Helper()
	for _, aggregate := range snapshot.Aggregates() {
		if aggregate.Scope.Identity() == scope.Identity() {
			return aggregate
		}
	}
	t.Fatalf("no aggregate for %q in %v", scope, scopeLabels(snapshot.Aggregates()))
	return domain.ScopeAggregate{}
}

// TestScopedSnapshotCountsOnlyMembersForPathScopes guards RG-004: path Scope
// activity counts members only, while not-member and unknown outcomes are held
// as separate inspectable counts.
func TestScopedSnapshotCountsOnlyMembersForPathScopes(t *testing.T) {
	api := pathScope(t, "owner/repo", literal("services"), separator(), recursive())
	scope := mustScopeSet(t, api)

	snapshot := domain.NewScopedSnapshot(scope, []domain.ScopedActivity{scopedActivity(t, "owner/repo",
		pushEvidence(t, "1", 1, "owner/repo", complete(t, "services/api/main.go")),
		pushEvidence(t, "2", 2, "owner/repo", complete(t, "docs/readme.md")),
		pushEvidence(t, "3", 3, "owner/repo", domain.DeniedOutcome("forbidden")),
	)})

	aggregate := findScopeAggregate(t, snapshot, api)
	if aggregate.Activity != 1 {
		t.Errorf("activity = %d, want only the member event", aggregate.Activity)
	}
	if aggregate.NotMember != 1 {
		t.Errorf("not-member = %d, want 1", aggregate.NotMember)
	}
	if aggregate.Unknown != 1 {
		t.Errorf("unknown = %d, want 1", aggregate.Unknown)
	}
	if aggregate.Evaluated != 3 {
		t.Errorf("evaluated = %d, want 3", aggregate.Evaluated)
	}
	if got := aggregate.UnknownBy(domain.ReasonDenied); got != 1 {
		t.Errorf("denied unknowns = %d, want 1", got)
	}
	if got := aggregate.UnknownBy(domain.ReasonFailed); got != 0 {
		t.Errorf("failed unknowns = %d, want 0", got)
	}
}

// TestScopedSnapshotRepositoryScopesKeepDirectV01Semantics guards FR-006:
// repository Scope counts stay direct per-category counts of every retained
// event, independent of path evidence.
func TestScopedSnapshotRepositoryScopesKeepDirectV01Semantics(t *testing.T) {
	repository := domain.NewRepositoryScope(mustParseRepository(t, "owner/repo"))
	scope := mustScopeSet(t, repository)

	snapshot := domain.NewScopedSnapshot(scope, []domain.ScopedActivity{scopedActivity(t, "owner/repo",
		pushEvidence(t, "1", 1, "owner/repo", domain.FailedOutcome("timeout")),
		evidenceOf(testEvent(t, "2", 2, "owner/repo", domain.CategoryPullRequest, domain.EntityPullRequest), complete(t, "docs/readme.md")),
		evidenceOf(testEvent(t, "3", 3, "owner/repo", domain.CategoryReview, domain.EntityPullRequest), domain.UnsupportedOutcome("no evidence")),
		evidenceOf(testEvent(t, "4", 4, "owner/repo", domain.CategoryComment, domain.EntityPullRequest), domain.UnsupportedOutcome("no evidence")),
		evidenceOf(testEvent(t, "5", 5, "owner/repo", domain.CategoryComment, domain.EntityOther), domain.UnsupportedOutcome("no evidence")),
	)})

	aggregate := findScopeAggregate(t, snapshot, repository)
	if aggregate.Activity != 5 {
		t.Errorf("activity = %d, want every retained event", aggregate.Activity)
	}
	if aggregate.Unknown != 0 {
		t.Errorf("unknown = %d, want none for a repository Scope", aggregate.Unknown)
	}
	if aggregate.Pushes != 1 {
		t.Errorf("pushes = %d, want 1", aggregate.Pushes)
	}
	if aggregate.PullRequestActivity != 3 {
		t.Errorf("pull-request activity = %d, want the pull request, review, and pull-request comment", aggregate.PullRequestActivity)
	}
}

// TestScopedSnapshotRetainsOverlappingMembership guards FR-006: one event
// matching two Scopes contributes to both and the snapshot keeps the distinct
// event count separate from the overlapping sum.
func TestScopedSnapshotRetainsOverlappingMembership(t *testing.T) {
	services := pathScope(t, "owner/repo", literal("services"), separator(), recursive())
	api := pathScope(t, "owner/repo", literal("services"), separator(), literal("api"), separator(), recursive())
	scope := mustScopeSet(t, services, api)

	snapshot := domain.NewScopedSnapshot(scope, []domain.ScopedActivity{scopedActivity(t, "owner/repo",
		pushEvidence(t, "1", 1, "owner/repo", complete(t, "services/api/main.go")),
	)})

	if got := findScopeAggregate(t, snapshot, services).Activity; got != 1 {
		t.Errorf("services activity = %d, want 1", got)
	}
	if got := findScopeAggregate(t, snapshot, api).Activity; got != 1 {
		t.Errorf("api activity = %d, want 1", got)
	}
	if got := snapshot.DistinctActivity(); got != 1 {
		t.Errorf("distinct activity = %d, want the single deduplicated event", got)
	}
	if !snapshot.Overlapping() {
		t.Error("Overlapping() = false, want true so copy never presents the sum as a deduplicated total")
	}
}

// TestScopedSnapshotWithoutOverlapReportsNoOverlap guards the honesty signal's
// negative case: disjoint Scopes may be summed.
func TestScopedSnapshotWithoutOverlapReportsNoOverlap(t *testing.T) {
	docs := pathScope(t, "owner/repo", literal("docs"), separator(), recursive())
	api := pathScope(t, "owner/repo", literal("services"), separator(), recursive())
	scope := mustScopeSet(t, docs, api)

	snapshot := domain.NewScopedSnapshot(scope, []domain.ScopedActivity{scopedActivity(t, "owner/repo",
		pushEvidence(t, "1", 1, "owner/repo", complete(t, "docs/readme.md")),
		pushEvidence(t, "2", 2, "owner/repo", complete(t, "services/api/main.go")),
	)})

	if snapshot.Overlapping() {
		t.Error("Overlapping() = true for disjoint Scopes")
	}
	if got := snapshot.DistinctActivity(); got != 2 {
		t.Errorf("distinct activity = %d, want 2", got)
	}
}

// TestScopedSnapshotQualifiesCurrentPRMembers guards RG-004: only members
// decided from complete current-PR evidence are counted as qualified, and they
// are included in the activity total.
func TestScopedSnapshotQualifiesCurrentPRMembers(t *testing.T) {
	api := pathScope(t, "owner/repo", literal("services"), separator(), recursive())
	scope := mustScopeSet(t, api)

	snapshot := domain.NewScopedSnapshot(scope, []domain.ScopedActivity{scopedActivity(t, "owner/repo",
		pushEvidence(t, "1", 1, "owner/repo", complete(t, "services/api/main.go")),
		evidenceOf(testEvent(t, "2", 2, "owner/repo", domain.CategoryPullRequest, domain.EntityPullRequest), currentPR(t, "services/api/handler.go")),
		evidenceOf(testEvent(t, "3", 3, "owner/repo", domain.CategoryPullRequest, domain.EntityPullRequest), currentPR(t, "docs/readme.md")),
	)})

	aggregate := findScopeAggregate(t, snapshot, api)
	if aggregate.Activity != 2 {
		t.Errorf("activity = %d, want both members including the qualified one", aggregate.Activity)
	}
	if aggregate.CurrentPR != 1 {
		t.Errorf("current PR = %d, want only the qualified member", aggregate.CurrentPR)
	}
}

// TestScopedSnapshotStreamOmitsEventsThatAreOnlyNotMember guards RG-004
// inclusion: an
// event that is not-member for every active Scope is omitted, while an event
// unknown for any path Scope is retained as investigatory context.
func TestScopedSnapshotStreamOmitsEventsThatAreOnlyNotMember(t *testing.T) {
	api := pathScope(t, "owner/repo", literal("services"), separator(), recursive())
	scope := mustScopeSet(t, api)

	snapshot := domain.NewScopedSnapshot(scope, []domain.ScopedActivity{scopedActivity(t, "owner/repo",
		pushEvidence(t, "1", 1, "owner/repo", complete(t, "services/api/main.go")),
		pushEvidence(t, "2", 2, "owner/repo", complete(t, "docs/readme.md")),
		pushEvidence(t, "3", 3, "owner/repo", domain.RateLimitedOutcome(time.Date(2026, time.August, 22, 11, 0, 0, 0, time.UTC))),
	)})

	assertIDs(t, snapshot.StreamEvents(), []string{"3", "1"})
	assertIDs(t, snapshot.Events(), []string{"3", "2", "1"})
}

// TestScopedSnapshotDropsCanceledEvidence guards RG-004: canceled work is never
// synthesized into unknown, so the event contributes no membership at all.
func TestScopedSnapshotDropsCanceledEvidence(t *testing.T) {
	api := pathScope(t, "owner/repo", literal("services"), separator(), recursive())
	scope := mustScopeSet(t, api)

	snapshot := domain.NewScopedSnapshot(scope, []domain.ScopedActivity{scopedActivity(t, "owner/repo",
		pushEvidence(t, "1", 1, "owner/repo", domain.CanceledOutcome()),
	)})

	assertIDs(t, snapshot.Events(), nil)
	aggregate := findScopeAggregate(t, snapshot, api)
	if aggregate.Evaluated != 0 || aggregate.Unknown != 0 {
		t.Errorf("evaluated = %d and unknown = %d, want no published outcome for canceled evidence", aggregate.Evaluated, aggregate.Unknown)
	}
}

// TestScopedSnapshotOrdersRowsByActivityThenStableIdentity guards RG-012: rows
// order by confirmed activity descending and break ties with the stable Scope
// identity order, keeping zero-activity and all-unknown Scopes as rows.
func TestScopedSnapshotOrdersRowsByActivityThenStableIdentity(t *testing.T) {
	alphaRepository := domain.NewRepositoryScope(mustParseRepository(t, "owner/alpha"))
	alphaDocs := pathScope(t, "owner/alpha", literal("docs"), separator(), recursive())
	betaRepository := domain.NewRepositoryScope(mustParseRepository(t, "owner/beta"))
	betaUnknown := pathScope(t, "owner/beta", literal("services"), separator(), recursive())
	scope := mustScopeSet(t, betaUnknown, alphaDocs, betaRepository, alphaRepository)

	snapshot := domain.NewScopedSnapshot(scope, []domain.ScopedActivity{
		scopedActivity(t, "owner/alpha",
			pushEvidence(t, "1", 1, "owner/alpha", complete(t, "src/main.go")),
		),
		scopedActivity(t, "owner/beta",
			pushEvidence(t, "2", 2, "owner/beta", domain.IncompleteOutcome("hard cap")),
		),
	})

	want := []string{"owner/alpha", "owner/beta", "owner/alpha:docs/**", "owner/beta:services/**"}
	if got := scopeLabels(snapshot.Aggregates()); !slices.Equal(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
	if got := findScopeAggregate(t, snapshot, alphaDocs).Activity; got != 0 {
		t.Errorf("zero-activity Scope activity = %d, want 0", got)
	}
	if got := findScopeAggregate(t, snapshot, betaUnknown).Unknown; got != 1 {
		t.Errorf("all-unknown Scope unknown = %d, want 1", got)
	}
}

// TestScopedSnapshotKeepsEveryConfiguredScopeRow guards RG-004: a selection with
// no returned activity still prepares every configured row.
func TestScopedSnapshotKeepsEveryConfiguredScopeRow(t *testing.T) {
	repository := domain.NewRepositoryScope(mustParseRepository(t, "owner/repo"))
	docs := pathScope(t, "owner/repo", literal("docs"), separator(), recursive())
	scope := mustScopeSet(t, docs, repository)

	snapshot := domain.NewScopedSnapshot(scope, nil)

	want := []string{"owner/repo", "owner/repo:docs/**"}
	if got := scopeLabels(snapshot.Aggregates()); !slices.Equal(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
	if len(snapshot.Events()) != 0 {
		t.Errorf("events = %v, want none", eventIDs(snapshot.Events()))
	}
}

// TestScopedSnapshotBoundsEventsAndRecordsTruncation guards FR-006: the bounded
// snapshot keeps the newest MaxSnapshotEvents events and reports the cut.
func TestScopedSnapshotBoundsEventsAndRecordsTruncation(t *testing.T) {
	repository := domain.NewRepositoryScope(mustParseRepository(t, "owner/repo"))
	scope := mustScopeSet(t, repository)
	events := pushEvents(mustParseRepository(t, "owner/repo"), domain.MaxSnapshotEvents+1)
	evidence := make([]domain.EventEvidence, 0, len(events))
	for _, event := range events {
		evidence = append(evidence, evidenceOf(event, domain.UnsupportedOutcome("no evidence")))
	}

	snapshot := domain.NewScopedSnapshot(scope, []domain.ScopedActivity{scopedActivity(t, "owner/repo", evidence...)})

	if !snapshot.Truncated() {
		t.Error("Truncated() = false, want true past the bound")
	}
	if got := len(snapshot.Events()); got != domain.MaxSnapshotEvents {
		t.Errorf("events = %d, want %d", got, domain.MaxSnapshotEvents)
	}
	if got := findScopeAggregate(t, snapshot, repository).Activity; got != domain.MaxSnapshotEvents {
		t.Errorf("activity = %d, want the bounded event count", got)
	}
}

// TestScopedSnapshotDeduplicatesAndIgnoresUnselectedRepositories guards FR-006:
// repeated source IDs are retained once and activity outside the selection is
// ignored, whatever the reporting repository claimed.
func TestScopedSnapshotDeduplicatesAndIgnoresUnselectedRepositories(t *testing.T) {
	repository := domain.NewRepositoryScope(mustParseRepository(t, "owner/repo"))
	scope := mustScopeSet(t, repository)

	snapshot := domain.NewScopedSnapshot(scope, []domain.ScopedActivity{scopedActivity(t, "owner/repo",
		pushEvidence(t, "1", 1, "owner/repo", domain.UnsupportedOutcome("no evidence")),
		pushEvidence(t, "1", 5, "owner/repo", domain.UnsupportedOutcome("no evidence")),
		pushEvidence(t, "2", 2, "owner/other", domain.UnsupportedOutcome("no evidence")),
	)})

	assertIDs(t, snapshot.Events(), []string{"1"})
	if got := findScopeAggregate(t, snapshot, repository).Activity; got != 1 {
		t.Errorf("activity = %d, want the deduplicated event", got)
	}
}

// TestScopedSnapshotRetainsPerEventMemberships guards FR-006: memberships are
// prepared per event in the stable Scope identity order so Stream renders
// confirmed and unknown context without re-matching.
func TestScopedSnapshotRetainsPerEventMemberships(t *testing.T) {
	repository := domain.NewRepositoryScope(mustParseRepository(t, "owner/repo"))
	docs := pathScope(t, "owner/repo", literal("docs"), separator(), recursive())
	services := pathScope(t, "owner/repo", literal("services"), separator(), recursive())
	scope := mustScopeSet(t, services, docs, repository)

	snapshot := domain.NewScopedSnapshot(scope, []domain.ScopedActivity{scopedActivity(t, "owner/repo",
		pushEvidence(t, "1", 1, "owner/repo", complete(t, "docs/readme.md")),
	)})

	scoped := snapshot.ScopedEvents()
	if len(scoped) != 1 {
		t.Fatalf("scoped events = %d, want 1", len(scoped))
	}
	wantScopes := []string{"owner/repo", "owner/repo:docs/**", "owner/repo:services/**"}
	wantKinds := []domain.MembershipKind{domain.MembershipMember, domain.MembershipMember, domain.MembershipNotMember}
	if len(scoped[0].Memberships) != len(wantScopes) {
		t.Fatalf("memberships = %d, want %d", len(scoped[0].Memberships), len(wantScopes))
	}
	for index, membership := range scoped[0].Memberships {
		if got := membership.Scope.String(); got != wantScopes[index] {
			t.Errorf("membership %d scope = %q, want %q", index, got, wantScopes[index])
		}
		if got := membership.Membership.Kind(); got != wantKinds[index] {
			t.Errorf("membership %d kind = %v, want %v", index, got, wantKinds[index])
		}
	}
}

// TestScopedSnapshotIsRepeatableAndDoesNotMutateInputs guards determinism: two
// builds from one input agree and the caller's slices are untouched.
func TestScopedSnapshotIsRepeatableAndDoesNotMutateInputs(t *testing.T) {
	docs := pathScope(t, "owner/repo", literal("docs"), separator(), recursive())
	scope := mustScopeSet(t, docs)
	activities := []domain.ScopedActivity{scopedActivity(t, "owner/repo",
		pushEvidence(t, "2", 2, "owner/repo", complete(t, "docs/readme.md")),
		pushEvidence(t, "1", 1, "owner/repo", complete(t, "docs/guide.md")),
	)}
	order := []string{activities[0].Events[0].Event.ID, activities[0].Events[1].Event.ID}

	first := domain.NewScopedSnapshot(scope, activities)
	second := domain.NewScopedSnapshot(scope, activities)

	assertIDs(t, first.Events(), []string{"2", "1"})
	if !slices.Equal(eventIDs(first.Events()), eventIDs(second.Events())) {
		t.Error("repeated builds disagree on event order")
	}
	if got := []string{activities[0].Events[0].Event.ID, activities[0].Events[1].Event.ID}; !slices.Equal(got, order) {
		t.Errorf("input order = %v, want the untouched %v", got, order)
	}
	first.Events()[0] = domain.Event{}
	if len(second.Events()) == 0 || second.Events()[0].ID != "2" {
		t.Error("Events() exposed the snapshot's backing array")
	}
}

// TestZeroScopedSnapshotIsInert guards the zero value: it holds no events, no
// rows, no overlap, and was never cut by the bound.
func TestZeroScopedSnapshotIsInert(t *testing.T) {
	var snapshot domain.ScopedSnapshot
	if snapshot.Truncated() || snapshot.Overlapping() || snapshot.DistinctActivity() != 0 {
		t.Error("the zero scoped snapshot reports activity it never held")
	}
	if len(snapshot.Events()) != 0 || len(snapshot.StreamEvents()) != 0 || len(snapshot.Aggregates()) != 0 || len(snapshot.ScopedEvents()) != 0 {
		t.Error("the zero scoped snapshot exposes rows or events")
	}
}

// TestScopedSnapshotBreaksUnknownsDownByReason guards RG-004's quantitative
// unknown coverage: every primary reason group is counted separately for the
// Scope, so the deterministic breakdown is prepared rather than inferred.
func TestScopedSnapshotBreaksUnknownsDownByReason(t *testing.T) {
	outcomes := map[domain.UnknownReason]domain.EvidenceOutcome{
		domain.ReasonUnsupported: domain.UnsupportedOutcome("no evidence form"),
		domain.ReasonIncomplete:  domain.IncompleteOutcome("hard cap"),
		domain.ReasonUnavailable: domain.UnavailableOutcome("gone"),
		domain.ReasonDenied:      domain.DeniedOutcome("forbidden"),
		domain.ReasonRateLimited: domain.RateLimitedOutcome(time.Date(2026, time.August, 22, 11, 0, 0, 0, time.UTC)),
		domain.ReasonFailed:      domain.FailedOutcome("timeout"),
	}
	api := pathScope(t, "owner/repo", literal("services"), separator(), recursive())
	scope := mustScopeSet(t, api)

	for index, reason := range domain.UnknownReasons() {
		t.Run(reason.String(), func(t *testing.T) {
			snapshot := domain.NewScopedSnapshot(scope, []domain.ScopedActivity{scopedActivity(t, "owner/repo",
				pushEvidence(t, "1", index, "owner/repo", outcomes[reason]),
			)})

			aggregate := findScopeAggregate(t, snapshot, api)
			if aggregate.Unknown != 1 {
				t.Fatalf("unknown = %d, want 1", aggregate.Unknown)
			}
			for _, group := range domain.UnknownReasons() {
				want := 0
				if group == reason {
					want = 1
				}
				if got := aggregate.UnknownBy(group); got != want {
					t.Errorf("UnknownBy(%v) = %d, want %d", group, got, want)
				}
			}
		})
	}
}

// TestScopedSnapshotAccessorsDoNotExposeBackingState guards the snapshot's
// immutability: a caller mutating a returned row or membership cannot change
// what the snapshot reports next.
func TestScopedSnapshotAccessorsDoNotExposeBackingState(t *testing.T) {
	api := pathScope(t, "owner/repo", literal("services"), separator(), recursive())
	scope := mustScopeSet(t, api)
	snapshot := domain.NewScopedSnapshot(scope, []domain.ScopedActivity{scopedActivity(t, "owner/repo",
		pushEvidence(t, "1", 1, "owner/repo", complete(t, "services/api/main.go")),
		pushEvidence(t, "2", 2, "owner/repo", domain.DeniedOutcome("forbidden")),
	)})

	rows := snapshot.Aggregates()
	rows[0].Activity = 99
	rows[0].UnknownReasonCounts()[domain.ReasonDenied] = 99
	scoped := snapshot.ScopedEvents()
	scoped[0].Memberships[0] = domain.ScopeMembership{}

	aggregate := findScopeAggregate(t, snapshot, api)
	if aggregate.Activity != 1 {
		t.Errorf("activity = %d, want the unchanged 1", aggregate.Activity)
	}
	if got := aggregate.UnknownBy(domain.ReasonDenied); got != 1 {
		t.Errorf("denied unknowns = %d, want the unchanged 1", got)
	}
	if got := snapshot.ScopedEvents()[0].Memberships[0].Scope.Identity(); got != api.Identity() {
		t.Errorf("membership scope = %v, want the unchanged %v", got, api.Identity())
	}
}
