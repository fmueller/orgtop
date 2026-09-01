package github_test

import (
	"fmt"
	"testing"

	"github.com/fmueller/orgtop/internal/domain"
	"github.com/fmueller/orgtop/internal/github"
)

const (
	evidenceRepository = "Acme/Web"
	evidenceKey        = "acme/web"
	shaBefore          = "1111111111111111111111111111111111111111"
	shaAfter           = "2222222222222222222222222222222222222222"
	shaZero            = "0000000000000000000000000000000000000000"
)

// evidenceEvent builds a one-event page carrying the given type and payload so a
// classification case reads next to the exact GitHub facts it depends on.
func evidenceEvent(t *testing.T, eventType, payload string) domain.Event {
	t.Helper()
	page := fmt.Sprintf(`[{"id":"1","type":%q,"created_at":"2026-03-10T09:00:00Z",`+
		`"repo":{"name":%q},"actor":{"login":"dev"},"payload":%s}]`, eventType, evidenceRepository, payload)
	events, err := github.NormalizeEvents(mustParseRepository(t, evidenceRepository), []byte(page))
	if err != nil {
		t.Fatalf("normalizing the %s page failed: %v", eventType, err)
	}
	if len(events) != 1 {
		t.Fatalf("normalizing returned %d events, want 1", len(events))
	}
	return events[0]
}

// TestChangedFileEvidenceClassification guards the RG-003 evidence eligibility
// table: ordered event classification, immutable object identity, and the work
// key each supported form needs.
func TestChangedFileEvidenceClassification(t *testing.T) {
	tests := []struct {
		name       string
		eventType  string
		payload    string
		operation  domain.EvidenceOperation
		key        string
		provenance domain.EvidenceProvenance
	}{
		{
			name:       "one-commit push uses commit work",
			eventType:  "PushEvent",
			payload:    fmt.Sprintf(`{"before":%q,"head":%q,"size":1}`, shaBefore, shaAfter),
			operation:  domain.EvidenceCommit,
			key:        "commit(" + evidenceKey + "," + shaAfter + ")",
			provenance: domain.ProvenanceEventTime,
		},
		{
			name:       "multi-commit push uses immutable compare",
			eventType:  "PushEvent",
			payload:    fmt.Sprintf(`{"before":%q,"head":%q,"size":3}`, shaBefore, shaAfter),
			operation:  domain.EvidenceCompare,
			key:        "compare(" + evidenceKey + "," + shaBefore + "," + shaAfter + ")",
			provenance: domain.ProvenanceEventTime,
		},
		{
			name:       "push without a size cannot take the one-commit optimization",
			eventType:  "PushEvent",
			payload:    fmt.Sprintf(`{"before":%q,"head":%q}`, shaBefore, shaAfter),
			operation:  domain.EvidenceCompare,
			key:        "compare(" + evidenceKey + "," + shaBefore + "," + shaAfter + ")",
			provenance: domain.ProvenanceEventTime,
		},
		{
			name:       "uppercase push objects normalize to one lowercase key",
			eventType:  "PushEvent",
			payload:    `{"before":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","head":"BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB","size":2}`,
			operation:  domain.EvidenceCompare,
			key:        "compare(" + evidenceKey + ",aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa,bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb)",
			provenance: domain.ProvenanceEventTime,
		},
		{
			name:       "pull request events use event-time base and head",
			eventType:  "PullRequestEvent",
			payload:    fmt.Sprintf(`{"pull_request":{"number":7,"base":{"sha":%q},"head":{"sha":%q}}}`, shaBefore, shaAfter),
			operation:  domain.EvidenceCompare,
			key:        "compare(" + evidenceKey + "," + shaBefore + "," + shaAfter + ")",
			provenance: domain.ProvenanceEventTime,
		},
		{
			name:       "pull request review events use event-time base and head",
			eventType:  "PullRequestReviewEvent",
			payload:    fmt.Sprintf(`{"pull_request":{"number":7,"base":{"sha":%q},"head":{"sha":%q}}}`, shaBefore, shaAfter),
			operation:  domain.EvidenceCompare,
			key:        "compare(" + evidenceKey + "," + shaBefore + "," + shaAfter + ")",
			provenance: domain.ProvenanceEventTime,
		},
		{
			name:       "pull request issue comments capture current pull request refs",
			eventType:  "IssueCommentEvent",
			payload:    `{"issue":{"number":12,"pull_request":{"url":"https://api.github.com/repos/acme/web/pulls/12"}}}`,
			operation:  domain.EvidencePullRequest,
			key:        "pr-metadata(" + evidenceKey + ",12)",
			provenance: domain.ProvenanceCurrentPR,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := evidenceEvent(t, test.eventType, test.payload).Evidence
			if evidence.Operation() != test.operation {
				t.Errorf("operation = %v, want %v", evidence.Operation(), test.operation)
			}
			if evidence.Key() != test.key {
				t.Errorf("key = %q, want %q", evidence.Key(), test.key)
			}
			if evidence.Provenance() != test.provenance {
				t.Errorf("provenance = %v, want %v", evidence.Provenance(), test.provenance)
			}
			if _, settled := evidence.Settled(); settled {
				t.Error("evidence needing GitHub work must not be settled")
			}
		})
	}
}

// TestSettledChangedFileEvidence guards the classifications that need no request:
// direct review-comment paths, the empty push, malformed direct evidence, and
// every unsupported event form.
func TestSettledChangedFileEvidence(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		payload   string
		kind      domain.OutcomeKind
		paths     []string
	}{
		{
			name:      "a file-specific review comment is complete singleton direct evidence",
			eventType: "PullRequestReviewCommentEvent",
			payload:   `{"pull_request":{"number":7,"base":{"sha":"1111111111111111111111111111111111111111"},"head":{"sha":"2222222222222222222222222222222222222222"}},"comment":{"path":"services/payments/api.go"}}`,
			kind:      domain.OutcomeComplete,
			paths:     []string{"services/payments/api.go"},
		},
		{
			name:      "a review comment without a path is incomplete and never falls back to pull request objects",
			eventType: "PullRequestReviewCommentEvent",
			payload:   `{"pull_request":{"number":7,"base":{"sha":"1111111111111111111111111111111111111111"},"head":{"sha":"2222222222222222222222222222222222222222"}}}`,
			kind:      domain.OutcomeIncomplete,
		},
		{
			name:      "a review comment with an empty path is incomplete",
			eventType: "PullRequestReviewCommentEvent",
			payload:   `{"comment":{"path":""}}`,
			kind:      domain.OutcomeIncomplete,
		},
		{
			name:      "a review comment with a malformed path is incomplete",
			eventType: "PullRequestReviewCommentEvent",
			payload:   `{"comment":{"path":"services//api.go"}}`,
			kind:      domain.OutcomeIncomplete,
		},
		{
			name:      "an equal-object push is complete and empty regardless of a contradictory size",
			eventType: "PushEvent",
			payload:   fmt.Sprintf(`{"before":%q,"head":%q,"size":1}`, shaBefore, shaBefore),
			kind:      domain.OutcomeComplete,
		},
		{
			name:      "a push with an all-zero side infers no branch contents",
			eventType: "PushEvent",
			payload:   fmt.Sprintf(`{"before":%q,"head":%q,"size":1}`, shaZero, shaAfter),
			kind:      domain.OutcomeUnsupported,
		},
		{
			name:      "a push with a malformed object is unsupported",
			eventType: "PushEvent",
			payload:   fmt.Sprintf(`{"before":"nope","head":%q,"size":1}`, shaAfter),
			kind:      domain.OutcomeUnsupported,
		},
		{
			name:      "a pull request event without event-time objects is unsupported",
			eventType: "PullRequestEvent",
			payload:   `{"pull_request":{"number":7}}`,
			kind:      domain.OutcomeUnsupported,
		},
		{
			name:      "a pull request event with equal event-time objects is complete and empty",
			eventType: "PullRequestEvent",
			payload:   fmt.Sprintf(`{"pull_request":{"number":7,"base":{"sha":%q},"head":{"sha":%q}}}`, shaAfter, shaAfter),
			kind:      domain.OutcomeComplete,
		},
		{
			name:      "a pull request review event with equal event-time objects is complete and empty",
			eventType: "PullRequestReviewEvent",
			payload:   fmt.Sprintf(`{"pull_request":{"number":7,"base":{"sha":%q},"head":{"sha":%q}}}`, shaAfter, shaAfter),
			kind:      domain.OutcomeComplete,
		},
		{
			name:      "an issue-only comment is unsupported",
			eventType: "IssueCommentEvent",
			payload:   `{"issue":{"number":12}}`,
			kind:      domain.OutcomeUnsupported,
		},
		{
			name:      "a pull request comment without a number is unsupported",
			eventType: "IssueCommentEvent",
			payload:   `{"issue":{"pull_request":{"url":"https://api.github.com/repos/acme/web/pulls/12"}}}`,
			kind:      domain.OutcomeUnsupported,
		},
		{
			name:      "a repository-level event is unsupported",
			eventType: "WatchEvent",
			payload:   `{"action":"started"}`,
			kind:      domain.OutcomeUnsupported,
		},
		{
			name:      "an event without a payload is unsupported",
			eventType: "PushEvent",
			payload:   `null`,
			kind:      domain.OutcomeUnsupported,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := evidenceEvent(t, test.eventType, test.payload).Evidence
			if evidence.Operation() != domain.EvidenceSettled {
				t.Fatalf("operation = %v, want settled", evidence.Operation())
			}
			if evidence.Key() != "" {
				t.Errorf("settled evidence has work key %q", evidence.Key())
			}
			outcome, settled := evidence.Settled()
			if !settled {
				t.Fatal("settled evidence reported no outcome")
			}
			if outcome.Kind() != test.kind {
				t.Fatalf("outcome = %v, want %v", outcome.Kind(), test.kind)
			}
			got := make([]string, 0, len(outcome.Paths()))
			for _, path := range outcome.Paths() {
				got = append(got, path.String())
			}
			if fmt.Sprint(got) != fmt.Sprint(test.paths) {
				t.Errorf("paths = %v, want %v", got, test.paths)
			}
			if test.kind == domain.OutcomeComplete && len(test.paths) > 0 && outcome.Provenance() != domain.ProvenanceDirectEvent {
				t.Errorf("direct provenance = %v", outcome.Provenance())
			}
		})
	}
}
