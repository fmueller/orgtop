package github_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
	"github.com/fmueller/orgtop/internal/github"
)

func mustParseRepository(t *testing.T, value string) domain.Repository {
	t.Helper()
	repository, err := domain.ParseRepository(value)
	if err != nil {
		t.Fatalf("parsing repository %q failed: %v", value, err)
	}
	return repository
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %s failed: %v", name, err)
	}
	return body
}

func normalizeOne(t *testing.T, requested, fixture string) domain.Event {
	t.Helper()
	events, err := github.NormalizeEvents(mustParseRepository(t, requested), loadFixture(t, fixture))
	if err != nil {
		t.Fatalf("normalizing %s failed: %v", fixture, err)
	}
	if len(events) != 1 {
		t.Fatalf("normalizing %s returned %d events, want 1", fixture, len(events))
	}
	return events[0]
}

// normalizeExpectingError normalizes a fixture that must fail and asserts that no
// partial data is returned.
func normalizeExpectingError(t *testing.T, requested, fixture string) error {
	t.Helper()
	events, err := github.NormalizeEvents(mustParseRepository(t, requested), loadFixture(t, fixture))
	if err == nil {
		t.Fatalf("normalizing %s succeeded with %d events, want an error", fixture, len(events))
	}
	if events != nil {
		t.Errorf("normalizing %s returned %d events, want no consumable partial data", fixture, len(events))
	}
	return err
}

func TestNormalizeEventsByCategory(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		id          string
		occurredAt  string
		actor       string
		category    domain.Category
		entityKind  domain.EntityKind
		entityRef   string
		description string
	}{
		{
			name:        "push",
			fixture:     "push.json",
			id:          "1001",
			occurredAt:  "2026-08-22T10:30:00Z",
			actor:       "octocat",
			category:    domain.CategoryPush,
			entityKind:  domain.EntityCommit,
			entityRef:   "9c1f0a1b2c3d4e5f60718293a4b5c6d7e8f90123",
			description: "pushed 3 commits to main",
		},
		{
			name:        "push with a single commit",
			fixture:     "push_single.json",
			id:          "1002",
			occurredAt:  "2026-08-22T10:31:00Z",
			actor:       "octocat",
			category:    domain.CategoryPush,
			entityKind:  domain.EntityCommit,
			entityRef:   "aaaabbbbccccdddd",
			description: "pushed 1 commit to release/1.0",
		},
		{
			name:        "pull request opened",
			fixture:     "pull_request_opened.json",
			id:          "2001",
			occurredAt:  "2026-08-22T09:00:00Z",
			actor:       "hubot",
			category:    domain.CategoryPullRequest,
			entityKind:  domain.EntityPullRequest,
			entityRef:   "#42",
			description: "opened #42",
		},
		{
			name:        "pull request merged",
			fixture:     "pull_request_merged.json",
			id:          "2002",
			occurredAt:  "2026-08-22T09:01:00Z",
			actor:       "hubot",
			category:    domain.CategoryPullRequest,
			entityKind:  domain.EntityPullRequest,
			entityRef:   "#42",
			description: "merged #42",
		},
		{
			name:        "pull request reopened",
			fixture:     "pull_request_reopened.json",
			id:          "2004",
			occurredAt:  "2026-08-22T09:03:00Z",
			actor:       "hubot",
			category:    domain.CategoryPullRequest,
			entityKind:  domain.EntityPullRequest,
			entityRef:   "#42",
			description: "reopened #42",
		},
		{
			name:        "pull request closed without a merge",
			fixture:     "pull_request_closed.json",
			id:          "2003",
			occurredAt:  "2026-08-22T09:02:00Z",
			actor:       "hubot",
			category:    domain.CategoryPullRequest,
			entityKind:  domain.EntityPullRequest,
			entityRef:   "#42",
			description: "closed #42",
		},
		{
			name:        "pull request updated by an unmapped action",
			fixture:     "pull_request_edited.json",
			id:          "2005",
			occurredAt:  "2026-08-22T09:04:00Z",
			actor:       "hubot",
			category:    domain.CategoryPullRequest,
			entityKind:  domain.EntityPullRequest,
			entityRef:   "#42",
			description: "updated #42",
		},
		{
			name:        "review approved",
			fixture:     "review_approved.json",
			id:          "3001",
			occurredAt:  "2026-08-22T08:00:00Z",
			actor:       "reviewer",
			category:    domain.CategoryReview,
			entityKind:  domain.EntityPullRequest,
			entityRef:   "#42",
			description: "approved #42",
		},
		{
			name:        "review requesting changes",
			fixture:     "review_changes_requested.json",
			id:          "3002",
			occurredAt:  "2026-08-22T08:01:00Z",
			actor:       "reviewer",
			category:    domain.CategoryReview,
			entityKind:  domain.EntityPullRequest,
			entityRef:   "#42",
			description: "requested changes on #42",
		},
		{
			name:        "review with a dismissed state",
			fixture:     "review_dismissed.json",
			id:          "3003",
			occurredAt:  "2026-08-22T08:02:00Z",
			actor:       "reviewer",
			category:    domain.CategoryReview,
			entityKind:  domain.EntityPullRequest,
			entityRef:   "#42",
			description: "reviewed #42",
		},
		{
			name:        "review with a commented state",
			fixture:     "review_commented.json",
			id:          "3004",
			occurredAt:  "2026-08-22T08:03:00Z",
			actor:       "reviewer",
			category:    domain.CategoryReview,
			entityKind:  domain.EntityPullRequest,
			entityRef:   "#42",
			description: "reviewed #42",
		},
		{
			name:        "pull request review comment",
			fixture:     "review_comment.json",
			id:          "4001",
			occurredAt:  "2026-08-22T07:00:00Z",
			actor:       "reviewer",
			category:    domain.CategoryComment,
			entityKind:  domain.EntityPullRequest,
			entityRef:   "#42",
			description: "commented on #42",
		},
		{
			name:        "issue comment on a pull request",
			fixture:     "issue_comment_pull_request.json",
			id:          "5001",
			occurredAt:  "2026-08-22T06:00:00Z",
			actor:       "commenter",
			category:    domain.CategoryComment,
			entityKind:  domain.EntityPullRequest,
			entityRef:   "#42",
			description: "commented on #42",
		},
		{
			name:        "issue only comment",
			fixture:     "issue_comment_issue.json",
			id:          "5002",
			occurredAt:  "2026-08-22T06:01:00Z",
			actor:       "commenter",
			category:    domain.CategoryOther,
			entityKind:  domain.EntityOther,
			entityRef:   "#7",
			description: "commented on issue #7",
		},
		{
			name:        "unsupported but valid type",
			fixture:     "unsupported.json",
			id:          "6001",
			occurredAt:  "2026-08-22T05:00:00Z",
			actor:       "stargazer",
			category:    domain.CategoryOther,
			entityKind:  domain.EntityRepository,
			entityRef:   "",
			description: "watch activity",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := normalizeOne(t, "acme/backend", test.fixture)

			occurredAt, err := time.Parse(time.RFC3339, test.occurredAt)
			if err != nil {
				t.Fatalf("parsing the expected timestamp failed: %v", err)
			}
			if event.ID != test.id {
				t.Errorf("ID = %q, want %q", event.ID, test.id)
			}
			if !event.OccurredAt.Equal(occurredAt) {
				t.Errorf("OccurredAt = %s, want %s", event.OccurredAt, occurredAt)
			}
			if event.Repository.Key() != "acme/backend" {
				t.Errorf("Repository key = %q, want %q", event.Repository.Key(), "acme/backend")
			}
			if event.Actor != test.actor {
				t.Errorf("Actor = %q, want %q", event.Actor, test.actor)
			}
			if event.Category != test.category {
				t.Errorf("Category = %q, want %q", event.Category, test.category)
			}
			if event.EntityKind != test.entityKind {
				t.Errorf("EntityKind = %q, want %q", event.EntityKind, test.entityKind)
			}
			if event.EntityRef != test.entityRef {
				t.Errorf("EntityRef = %q, want %q", event.EntityRef, test.entityRef)
			}
			if event.Description != test.description {
				t.Errorf("Description = %q, want %q", event.Description, test.description)
			}
		})
	}
}

func TestNormalizeEventsUsesGenericDescriptionsForMissingOptionalDetail(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		category    domain.Category
		entityKind  domain.EntityKind
		description string
	}{
		{
			name:        "push without a ref or size",
			fixture:     "push_minimal.json",
			category:    domain.CategoryPush,
			entityKind:  domain.EntityCommit,
			description: "pushed commits",
		},
		{
			name:        "pull request without an action or number",
			fixture:     "pull_request_minimal.json",
			category:    domain.CategoryPullRequest,
			entityKind:  domain.EntityPullRequest,
			description: "updated a pull request",
		},
		{
			name:        "review without a state or number",
			fixture:     "review_minimal.json",
			category:    domain.CategoryReview,
			entityKind:  domain.EntityPullRequest,
			description: "reviewed a pull request",
		},
		{
			name:        "pull request review comment without a pull request",
			fixture:     "review_comment_minimal.json",
			category:    domain.CategoryComment,
			entityKind:  domain.EntityPullRequest,
			description: "commented on a pull request",
		},
		{
			name:        "issue comment without an issue",
			fixture:     "issue_comment_minimal.json",
			category:    domain.CategoryOther,
			entityKind:  domain.EntityOther,
			description: "commented on an issue",
		},
		{
			name:        "unsupported type without a type",
			fixture:     "unsupported_without_type.json",
			category:    domain.CategoryOther,
			entityKind:  domain.EntityRepository,
			description: "repository activity",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := normalizeOne(t, "acme/backend", test.fixture)
			if event.Actor != "" {
				t.Errorf("Actor = %q, want an empty actor", event.Actor)
			}
			if event.EntityRef != "" {
				t.Errorf("EntityRef = %q, want an empty reference", event.EntityRef)
			}
			if event.Category != test.category {
				t.Errorf("Category = %q, want %q", event.Category, test.category)
			}
			if event.EntityKind != test.entityKind {
				t.Errorf("EntityKind = %q, want %q", event.EntityKind, test.entityKind)
			}
			if event.Description != test.description {
				t.Errorf("Description = %q, want %q", event.Description, test.description)
			}
		})
	}
}

// TestNormalizeEventsIgnoresNonPositiveEntityNumbers covers a structurally valid
// payload that reports a non-positive entity number: OrgTop must fall back to the
// generic description instead of rendering a meaningless "#0" reference.
func TestNormalizeEventsIgnoresNonPositiveEntityNumbers(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		category    domain.Category
		entityKind  domain.EntityKind
		description string
	}{
		{
			name:        "pull request numbered zero",
			fixture:     "pull_request_zero_number.json",
			category:    domain.CategoryPullRequest,
			entityKind:  domain.EntityPullRequest,
			description: "opened a pull request",
		},
		{
			name:        "pull request review comment numbered zero",
			fixture:     "review_comment_zero_number.json",
			category:    domain.CategoryComment,
			entityKind:  domain.EntityPullRequest,
			description: "commented on a pull request",
		},
		{
			name:        "issue numbered zero",
			fixture:     "issue_comment_zero_number.json",
			category:    domain.CategoryOther,
			entityKind:  domain.EntityOther,
			description: "commented on an issue",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := normalizeOne(t, "acme/backend", test.fixture)
			if event.EntityRef != "" {
				t.Errorf("EntityRef = %q, want an empty reference", event.EntityRef)
			}
			if event.Category != test.category {
				t.Errorf("Category = %q, want %q", event.Category, test.category)
			}
			if event.EntityKind != test.entityKind {
				t.Errorf("EntityKind = %q, want %q", event.EntityKind, test.entityKind)
			}
			if event.Description != test.description {
				t.Errorf("Description = %q, want %q", event.Description, test.description)
			}
		})
	}
}

func TestNormalizeEventsTreatsUnknownTypeCaseVariationAsOther(t *testing.T) {
	event := normalizeOne(t, "acme/backend", "unsupported_case_variant_type.json")
	if event.Category != domain.CategoryOther {
		t.Errorf("Category = %q, want %q", event.Category, domain.CategoryOther)
	}
	if event.EntityKind != domain.EntityRepository {
		t.Errorf("EntityKind = %q, want %q", event.EntityKind, domain.EntityRepository)
	}
}

func TestNormalizeEventsAcceptsRepositoryCaseVariation(t *testing.T) {
	event := normalizeOne(t, "acme/backend", "repository_case_variation.json")
	if got := event.Repository.String(); got != "Acme/Backend" {
		t.Errorf("Repository = %q, want the returned spelling %q", got, "Acme/Backend")
	}
	if got := event.Repository.Key(); got != "acme/backend" {
		t.Errorf("Repository key = %q, want %q", got, "acme/backend")
	}
}

func TestNormalizeEventsKeepsPayloadOrder(t *testing.T) {
	events, err := github.NormalizeEvents(mustParseRepository(t, "acme/backend"), loadFixture(t, "page.json"))
	if err != nil {
		t.Fatalf("normalizing page.json failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].ID != "8002" || events[1].ID != "8001" {
		t.Errorf("got IDs %q, %q, want the payload order %q, %q", events[0].ID, events[1].ID, "8002", "8001")
	}
}

func TestNormalizeEventsAcceptsAnEmptyPage(t *testing.T) {
	events, err := github.NormalizeEvents(mustParseRepository(t, "acme/backend"), loadFixture(t, "empty.json"))
	if err != nil {
		t.Fatalf("normalizing empty.json failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("got %d events, want none", len(events))
	}
}

func TestNormalizeEventsRejectsMalformedCommonFields(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
	}{
		{name: "malformed top level json", fixture: "malformed.json"},
		{name: "top level object instead of a page", fixture: "not_an_array.json"},
		{name: "missing id", fixture: "missing_id.json"},
		{name: "missing timestamp", fixture: "missing_created_at.json"},
		{name: "unparseable timestamp", fixture: "invalid_created_at.json"},
		{name: "missing repository identity", fixture: "missing_repository.json"},
		{name: "invalid repository identity", fixture: "invalid_repository.json"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := normalizeExpectingError(t, "acme/backend", test.fixture)
			if !errors.Is(err, github.ErrInvalidPayload) {
				t.Errorf("error %v does not match ErrInvalidPayload", err)
			}
		})
	}
}

func TestNormalizeEventsRejectsRepositoryIdentityMismatch(t *testing.T) {
	err := normalizeExpectingError(t, "acme/backend", "repository_mismatch.json")
	if !errors.Is(err, github.ErrRepositoryMismatch) {
		t.Errorf("error %v does not match ErrRepositoryMismatch", err)
	}
}

// forbiddenSecretMarkers guards NFR-003: fixtures and errors must never carry a
// credential value or an authenticated request header.
var forbiddenSecretMarkers = []string{
	"ghp_",
	"gho_",
	"ghs_",
	"github_pat_",
	"authorization",
	"bearer ",
	"x-ratelimit",
}

func TestFixturesAndErrorsCarryNoCredentials(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("reading testdata failed: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no fixtures found in testdata")
	}
	for _, entry := range entries {
		body := loadFixture(t, entry.Name())
		lowered := strings.ToLower(string(body))
		for _, marker := range forbiddenSecretMarkers {
			if strings.Contains(lowered, marker) {
				t.Errorf("fixture %s contains the forbidden marker %q", entry.Name(), marker)
			}
		}
		if strings.Contains(lowered, "api.github.com/repos/") && !strings.Contains(lowered, "acme/") {
			t.Errorf("fixture %s references a repository outside the synthetic acme organization", entry.Name())
		}
	}

	for _, fixture := range []string{"malformed.json", "missing_id.json", "invalid_created_at.json", "repository_mismatch.json"} {
		err := normalizeExpectingError(t, "acme/backend", fixture)
		lowered := strings.ToLower(err.Error())
		for _, marker := range forbiddenSecretMarkers {
			if strings.Contains(lowered, marker) {
				t.Errorf("the %s error contains the forbidden marker %q: %v", fixture, marker, err)
			}
		}
	}
}
