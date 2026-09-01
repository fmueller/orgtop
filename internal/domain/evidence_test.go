package domain_test

import (
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

const (
	shaBase = "0123456789ABCDEF0123456789abcdef01234567"
	shaHead = "89abcdef0123456789abcdef0123456789abcdef"
	shaZero = "0000000000000000000000000000000000000000"
)

// TestObjectSHANormalization guards the RG-003 object identity rule: exactly 40
// ASCII hexadecimal digits lowercased, and all-zero is never a valid object.
func TestObjectSHANormalization(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
		valid bool
	}{
		{name: "lowercases a valid sha", value: shaBase, want: "0123456789abcdef0123456789abcdef01234567", valid: true},
		{name: "accepts an already lowercase sha", value: shaHead, want: shaHead, valid: true},
		{name: "rejects the all-zero sha", value: shaZero},
		{name: "rejects a short sha", value: shaHead[:39]},
		{name: "rejects a long sha", value: shaHead + "0"},
		{name: "rejects a non-hexadecimal sha", value: "g" + shaHead[1:]},
		{name: "rejects surrounding space", value: " " + shaHead[1:] + " "},
		{name: "rejects an empty sha", value: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := domain.NormalizeObjectSHA(test.value)
			if ok != test.valid {
				t.Fatalf("NormalizeObjectSHA(%q) validity = %t, want %t", test.value, ok, test.valid)
			}
			if got != test.want {
				t.Errorf("NormalizeObjectSHA(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

// TestEvidenceDescriptorWorkKeys guards the RG-009 work keys: identity is the
// repository key plus immutable object identity, never a Scope or PR number
// alone for changed paths.
func TestEvidenceDescriptorWorkKeys(t *testing.T) {
	repository := mustParseRepository(t, "Acme/Web")
	other := mustParseRepository(t, "acme/web")

	commit, err := domain.NewCommitEvidence(repository, shaHead)
	if err != nil {
		t.Fatalf("NewCommitEvidence failed: %v", err)
	}
	sameCommit, err := domain.NewCommitEvidence(other, shaHead)
	if err != nil {
		t.Fatalf("NewCommitEvidence failed: %v", err)
	}
	compare, err := domain.NewCompareEvidence(repository, shaBase, shaHead, domain.ProvenanceEventTime)
	if err != nil {
		t.Fatalf("NewCompareEvidence failed: %v", err)
	}
	metadata, err := domain.NewPullRequestEvidence(repository, 42)
	if err != nil {
		t.Fatalf("NewPullRequestEvidence failed: %v", err)
	}

	if commit.Key() != "commit(acme/web,"+shaHead+")" {
		t.Errorf("commit key = %q", commit.Key())
	}
	if commit.Key() != sameCommit.Key() {
		t.Errorf("commit keys differ across repository spellings: %q vs %q", commit.Key(), sameCommit.Key())
	}
	if want := "compare(acme/web,0123456789abcdef0123456789abcdef01234567," + shaHead + ")"; compare.Key() != want {
		t.Errorf("compare key = %q, want %q", compare.Key(), want)
	}
	if want := "pr-metadata(acme/web,42)"; metadata.Key() != want {
		t.Errorf("pull request key = %q, want %q", metadata.Key(), want)
	}
	if commit.Key() == compare.Key() {
		t.Error("commit and compare share one work key")
	}
	for _, descriptor := range []domain.EvidenceDescriptor{
		domain.NewDirectEvidence(mustChangedPath(t, "a/b.go")),
		domain.NewUnchangedEvidence(),
		domain.NewUnsupportedEvidence("issue-only comment"),
	} {
		if descriptor.Key() != "" {
			t.Errorf("%v requires no queued work but has key %q", descriptor.Operation(), descriptor.Key())
		}
	}
}

// TestEvidenceDescriptorConstruction guards descriptor validation and the
// provenance retained with each evidence form.
func TestEvidenceDescriptorConstruction(t *testing.T) {
	repository := mustParseRepository(t, "acme/web")

	if _, err := domain.NewCommitEvidence(repository, shaZero); err == nil {
		t.Error("NewCommitEvidence accepted the all-zero sha")
	}
	if _, err := domain.NewCompareEvidence(repository, shaHead, shaHead, domain.ProvenanceEventTime); err == nil {
		t.Error("NewCompareEvidence accepted an equal base and head")
	}
	if _, err := domain.NewCompareEvidence(repository, "nope", shaHead, domain.ProvenanceEventTime); err == nil {
		t.Error("NewCompareEvidence accepted a malformed base")
	}
	if _, err := domain.NewPullRequestEvidence(repository, 0); err == nil {
		t.Error("NewPullRequestEvidence accepted a non-positive number")
	}
	if _, err := domain.NewCommitEvidence(domain.Repository{}, shaHead); err == nil {
		t.Error("NewCommitEvidence accepted an unvalidated repository")
	}

	direct := domain.NewDirectEvidence(mustChangedPath(t, "docs/readme.md"))
	if _, settled := direct.Settled(); !settled {
		t.Error("direct evidence must be settled without a request")
	}
	if direct.Provenance() != domain.ProvenanceDirectEvent {
		t.Errorf("direct provenance = %v", direct.Provenance())
	}
	if direct.Operation() != domain.EvidenceSettled {
		t.Errorf("direct operation = %v, want settled", direct.Operation())
	}
	empty, _ := domain.NewUnchangedEvidence().Settled()
	if !empty.IsComplete() || empty.Provenance() != domain.ProvenanceEventTime || len(empty.Paths()) != 0 {
		t.Errorf("empty push evidence = %v/%v with %d paths", empty.Kind(), empty.Provenance(), len(empty.Paths()))
	}
	current, err := domain.NewCompareEvidence(repository, shaBase, shaHead, domain.ProvenanceCurrentPR)
	if err != nil {
		t.Fatalf("NewCompareEvidence failed: %v", err)
	}
	if current.Provenance() != domain.ProvenanceCurrentPR {
		t.Errorf("current-PR compare provenance = %v", current.Provenance())
	}
}

// TestDirectEvidencePathsAreCopied guards that a descriptor cannot be mutated
// through the slice a caller supplied or received.
func TestDirectEvidencePathsAreCopied(t *testing.T) {
	paths := []domain.ChangedPath{mustChangedPath(t, "a/b.go")}
	descriptor := domain.NewDirectEvidence(paths...)
	paths[0] = mustChangedPath(t, "c/d.go")
	settled, _ := descriptor.Settled()
	if got := settled.Paths(); got[0].String() != "a/b.go" {
		t.Errorf("descriptor path = %q, want a/b.go", got[0])
	}
	returned := settled.Paths()
	returned[0] = mustChangedPath(t, "e/f.go")
	if got := settled.Paths(); got[0].String() != "a/b.go" {
		t.Errorf("descriptor path = %q after mutating a returned slice", got[0])
	}
}

// TestEvidenceOutcomes guards the closed RG-003 outcome set: only complete
// carries paths, and every other outcome keeps membership unknown.
func TestEvidenceOutcomes(t *testing.T) {
	retryAt := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	paths := []domain.ChangedPath{mustChangedPath(t, "a/b.go")}

	tests := []struct {
		name     string
		outcome  domain.EvidenceOutcome
		kind     domain.OutcomeKind
		complete bool
	}{
		{name: "complete", outcome: domain.CompleteOutcome(domain.ProvenanceEventTime, paths), kind: domain.OutcomeComplete, complete: true},
		{name: "unsupported", outcome: domain.UnsupportedOutcome("issue-only comment"), kind: domain.OutcomeUnsupported},
		{name: "incomplete", outcome: domain.IncompleteOutcome("pagination bound"), kind: domain.OutcomeIncomplete},
		{name: "unavailable", outcome: domain.UnavailableOutcome("commit no longer served"), kind: domain.OutcomeUnavailable},
		{name: "denied", outcome: domain.DeniedOutcome("contents read access"), kind: domain.OutcomeDenied},
		{name: "rate limited", outcome: domain.RateLimitedOutcome(retryAt), kind: domain.OutcomeRateLimited},
		{name: "failed", outcome: domain.FailedOutcome("timeout"), kind: domain.OutcomeFailed},
		{name: "canceled", outcome: domain.CanceledOutcome(), kind: domain.OutcomeCanceled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.outcome.Kind() != test.kind {
				t.Errorf("kind = %v, want %v", test.outcome.Kind(), test.kind)
			}
			if test.outcome.IsComplete() != test.complete {
				t.Errorf("IsComplete = %t, want %t", test.outcome.IsComplete(), test.complete)
			}
			if !test.complete && len(test.outcome.Paths()) != 0 {
				t.Errorf("%v carries %d paths; only complete evidence may", test.kind, len(test.outcome.Paths()))
			}
		})
	}

	if got := domain.RateLimitedOutcome(retryAt).RetryAt(); !got.Equal(retryAt) {
		t.Errorf("RetryAt = %v, want %v", got, retryAt)
	}
	if got := domain.CompleteOutcome(domain.ProvenanceEventTime, nil); !got.IsComplete() || len(got.Paths()) != 0 {
		t.Errorf("a complete empty set must stay complete, got %v with %d paths", got.Kind(), len(got.Paths()))
	}
}

// TestCompleteOutcomePathsAreCopied guards outcome immutability across callers.
func TestCompleteOutcomePathsAreCopied(t *testing.T) {
	paths := []domain.ChangedPath{mustChangedPath(t, "a/b.go")}
	outcome := domain.CompleteOutcome(domain.ProvenanceEventTime, paths)
	paths[0] = mustChangedPath(t, "c/d.go")
	if got := outcome.Paths(); got[0].String() != "a/b.go" {
		t.Errorf("outcome path = %q, want a/b.go", got[0])
	}
	outcome.Paths()[0] = mustChangedPath(t, "e/f.go")
	if got := outcome.Paths(); got[0].String() != "a/b.go" {
		t.Errorf("outcome path = %q after mutating a returned slice", got[0])
	}
}

// TestCommitOutcomeParentApplicability guards A-082: one coalesced commit result
// serves several events, and each event is complete only when its own before SHA
// is the verified sole parent.
func TestCommitOutcomeParentApplicability(t *testing.T) {
	paths := []domain.ChangedPath{mustChangedPath(t, "a/b.go")}
	outcome := domain.CompleteOutcome(domain.ProvenanceEventTime, paths).WithSoleParent(shaBase)

	if got := outcome.SoleParent(); got != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("SoleParent = %q", got)
	}
	applicable := outcome.ForSoleParent(shaBase)
	if !applicable.IsComplete() {
		t.Errorf("matching before sha = %v, want complete", applicable.Kind())
	}
	mismatched := outcome.ForSoleParent(shaHead)
	if mismatched.Kind() != domain.OutcomeIncomplete {
		t.Errorf("mismatched before sha = %v, want incomplete", mismatched.Kind())
	}
	if len(mismatched.Paths()) != 0 {
		t.Errorf("mismatched applicability leaked %d paths", len(mismatched.Paths()))
	}
	if got := domain.IncompleteOutcome("bound").ForSoleParent(shaBase); got.Kind() != domain.OutcomeIncomplete {
		t.Errorf("non-complete outcome applicability = %v, want incomplete", got.Kind())
	}
}
