package cache

import (
	"errors"
	"testing"

	"github.com/fmueller/orgtop/internal/domain"
)

const (
	baseSHA   = "1111111111111111111111111111111111111111"
	headSHA   = "2222222222222222222222222222222222222222"
	parentSHA = "3333333333333333333333333333333333333333"
)

func testRepository(t *testing.T, owner, name string) domain.Repository {
	t.Helper()

	repository, err := domain.ParseRepository(owner + "/" + name)
	if err != nil {
		t.Fatalf("ParseRepository(%q/%q) error = %v", owner, name, err)
	}
	return repository
}

// TestCompareKeyCarriesExactBaseAndHead pins the compare identity: exact
// normalized base and head with an empty verified parent.
func TestCompareKeyCarriesExactBaseAndHead(t *testing.T) {
	t.Parallel()

	key, err := CompareKey(testRepository(t, "Owner", "Repo"), baseSHA, headSHA)
	if err != nil {
		t.Fatalf("CompareKey() error = %v", err)
	}
	if got, want := key.Repository(), "owner/repo"; got != want {
		t.Errorf("Repository() = %q, want %q", got, want)
	}
	if got, want := key.Operation(), "compare"; got != want {
		t.Errorf("Operation() = %q, want %q", got, want)
	}
	if got, want := key.Base(), baseSHA; got != want {
		t.Errorf("Base() = %q, want %q", got, want)
	}
	if got, want := key.Head(), headSHA; got != want {
		t.Errorf("Head() = %q, want %q", got, want)
	}
}

// TestCommitKeyHasAnEmptyBase pins the commit identity: an empty base and the
// exact head. The proven sole parent lives on the record, not the key, so one
// commit read serves every event whose before SHA that parent matches.
func TestCommitKeyHasAnEmptyBase(t *testing.T) {
	t.Parallel()

	key, err := CommitKey(testRepository(t, "owner", "REPO"), headSHA)
	if err != nil {
		t.Fatalf("CommitKey() error = %v", err)
	}
	if key.Base() != "" {
		t.Errorf("Base() = %q, want an empty base", key.Base())
	}
	if got, want := key.Operation(), "commit"; got != want {
		t.Errorf("Operation() = %q, want %q", got, want)
	}
	if got, want := key.Repository(), "owner/repo"; got != want {
		t.Errorf("Repository() = %q, want %q", got, want)
	}
}

// TestKeyRejectsInvalidIdentity proves an unusable identity never becomes a
// cache key, so a malformed SHA cannot be written or looked up.
func TestKeyRejectsInvalidIdentity(t *testing.T) {
	t.Parallel()

	repository := testRepository(t, "owner", "repo")
	zero := "0000000000000000000000000000000000000000"

	if _, err := CommitKey(repository, "nope"); !errors.Is(err, domain.ErrInvalidEvidence) {
		t.Errorf("CommitKey(bad head) error = %v, want ErrInvalidEvidence", err)
	}
	if _, err := CommitKey(repository, zero); !errors.Is(err, domain.ErrInvalidEvidence) {
		t.Errorf("CommitKey(zero head) error = %v, want ErrInvalidEvidence", err)
	}
	if _, err := CompareKey(repository, baseSHA, baseSHA); !errors.Is(err, domain.ErrInvalidEvidence) {
		t.Errorf("CompareKey(equal) error = %v, want ErrInvalidEvidence", err)
	}
	if _, err := CompareKey(domain.Repository{}, baseSHA, headSHA); !errors.Is(err, domain.ErrInvalidEvidence) {
		t.Errorf("CompareKey(no repository) error = %v, want ErrInvalidEvidence", err)
	}
}

// TestKeyForPersistsOnlyImmutableChangedFileEvidence proves direct-event,
// unsupported, and pull request metadata descriptors bypass persistence.
func TestKeyForPersistsOnlyImmutableChangedFileEvidence(t *testing.T) {
	t.Parallel()

	repository := testRepository(t, "owner", "repo")
	commit, err := domain.NewCommitEvidence(repository, headSHA)
	if err != nil {
		t.Fatalf("NewCommitEvidence() error = %v", err)
	}
	compare, err := domain.NewCompareEvidence(repository, baseSHA, headSHA, domain.ProvenanceCurrentPR)
	if err != nil {
		t.Fatalf("NewCompareEvidence() error = %v", err)
	}
	pull, err := domain.NewPullRequestEvidence(repository, 7)
	if err != nil {
		t.Fatalf("NewPullRequestEvidence() error = %v", err)
	}

	for _, testCase := range []struct {
		name       string
		descriptor domain.EvidenceDescriptor
		want       bool
	}{
		{"commit", commit, true},
		{"compare", compare, true},
		{"pull request metadata", pull, false},
		{"direct event", domain.NewDirectEvidence(), false},
		{"unsupported", domain.NewUnsupportedEvidence("no evidence"), false},
		{"unchanged", domain.NewUnchangedEvidence(), false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, ok := KeyFor(testCase.descriptor)
			if ok != testCase.want {
				t.Errorf("KeyFor(%s) ok = %v, want %v", testCase.name, ok, testCase.want)
			}
		})
	}
}

// TestKeyForKeepsProvenanceOutOfIdentity proves one immutable compare has one
// identity whether it was read at event time or from a pull request's current
// refs, so provenance never splits or is stored with a record.
func TestKeyForKeepsProvenanceOutOfIdentity(t *testing.T) {
	t.Parallel()

	repository := testRepository(t, "owner", "repo")
	eventTime, err := domain.NewCompareEvidence(repository, baseSHA, headSHA, domain.ProvenanceEventTime)
	if err != nil {
		t.Fatalf("NewCompareEvidence(event-time) error = %v", err)
	}
	currentPR, err := domain.NewCompareEvidence(repository, baseSHA, headSHA, domain.ProvenanceCurrentPR)
	if err != nil {
		t.Fatalf("NewCompareEvidence(current-pr) error = %v", err)
	}

	first, _ := KeyFor(eventTime)
	second, _ := KeyFor(currentPR)
	if first != second {
		t.Errorf("KeyFor differs by provenance: %+v vs %+v", first, second)
	}
}
