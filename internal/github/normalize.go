package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fmueller/orgtop/internal/domain"
)

// GitHub event types OrgTop describes explicitly. Matching is exact: GitHub's
// documented type spelling is the contract, and anything else is unsupported.
const (
	typePush                     = "PushEvent"
	typePullRequest              = "PullRequestEvent"
	typePullRequestReview        = "PullRequestReviewEvent"
	typePullRequestReviewComment = "PullRequestReviewCommentEvent"
	typeIssueComment             = "IssueCommentEvent"
)

// Entity nouns rendered in event descriptions.
const (
	entityPullRequest = "pull request"
	entityIssue       = "issue"
)

const branchRefPrefix = "refs/heads/"

var (
	// ErrInvalidPayload reports an events page that cannot be normalized because
	// the page itself or a required common event field is missing or unusable.
	// It carries no payload content beyond event identity.
	ErrInvalidPayload = errors.New("invalid github events payload")

	// ErrRepositoryMismatch reports a returned repository identity that does not
	// match the requested Scope entry case-insensitively. OrgTop fails the
	// repository refresh instead of silently changing Scope.
	ErrRepositoryMismatch = errors.New("github event repository mismatch")
)

// NormalizeEvents converts one GitHub repository events page into domain events
// for the requested repository, preserving the returned order. An empty page is
// valid. Any unusable event fails the whole page so that no partial data can be
// consumed.
func NormalizeEvents(requested domain.Repository, page []byte) ([]domain.Event, error) {
	var payloads []eventPayload
	if err := json.Unmarshal(page, &payloads); err != nil {
		return nil, fmt.Errorf("%w: %s did not return a github events page", ErrInvalidPayload, requested)
	}
	if len(payloads) == 0 {
		return nil, nil
	}

	events := make([]domain.Event, 0, len(payloads))
	for index, payload := range payloads {
		event, err := normalizeEvent(requested, index, payload)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func normalizeEvent(requested domain.Repository, index int, payload eventPayload) (domain.Event, error) {
	id := strings.TrimSpace(payload.ID)
	if id == "" {
		return domain.Event{}, fmt.Errorf("%w: event %d has no id", ErrInvalidPayload, index)
	}
	occurredAt, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.CreatedAt))
	if err != nil {
		return domain.Event{}, fmt.Errorf("%w: event %s has no parseable timestamp", ErrInvalidPayload, id)
	}
	repository, err := normalizeRepository(requested, id, payload.Repo)
	if err != nil {
		return domain.Event{}, err
	}

	category, kind, ref, description := classify(payload)
	return domain.Event{
		ID:          id,
		OccurredAt:  occurredAt,
		Repository:  repository,
		Actor:       actorLogin(payload.Actor),
		Category:    category,
		EntityKind:  kind,
		EntityRef:   ref,
		Description: description,
		Evidence:    changedFileEvidence(repository, payload),
	}, nil
}

// normalizeRepository returns the returned repository identity when it matches the
// requested Scope entry case-insensitively, keeping the returned spelling.
func normalizeRepository(requested domain.Repository, id string, repo *repoPayload) (domain.Repository, error) {
	name := ""
	if repo != nil {
		name = strings.TrimSpace(repo.Name)
	}
	if name == "" {
		return domain.Repository{}, fmt.Errorf("%w: event %s has no repository identity", ErrInvalidPayload, id)
	}
	returned, err := domain.ParseRepository(name)
	if err != nil {
		return domain.Repository{}, fmt.Errorf("%w: event %s has an unusable repository identity %q", ErrInvalidPayload, id, name)
	}
	if returned.Key() != requested.Key() {
		return domain.Repository{}, fmt.Errorf("%w: event %s reports %s, want %s", ErrRepositoryMismatch, id, returned, requested)
	}
	return returned, nil
}

func actorLogin(actor *actorPayload) string {
	if actor == nil {
		return ""
	}
	return strings.TrimSpace(actor.Login)
}

// classify maps a GitHub event type and its optional detail onto the domain
// category, entity kind, entity reference, and description.
func classify(payload eventPayload) (domain.Category, domain.EntityKind, string, string) {
	detail := payload.Payload
	if detail == nil {
		detail = &detailPayload{}
	}

	switch payload.Type {
	case typePush:
		return domain.CategoryPush, domain.EntityCommit, strings.TrimSpace(detail.Head), pushDescription(detail)
	case typePullRequest:
		ref := pullRequestRef(detail.PullRequest)
		return domain.CategoryPullRequest, domain.EntityPullRequest, ref, pullRequestDescription(detail, ref)
	case typePullRequestReview:
		ref := pullRequestRef(detail.PullRequest)
		return domain.CategoryReview, domain.EntityPullRequest, ref, reviewDescription(detail.Review, ref)
	case typePullRequestReviewComment:
		ref := pullRequestRef(detail.PullRequest)
		return domain.CategoryComment, domain.EntityPullRequest, ref, pullRequestCommentDescription(ref)
	case typeIssueComment:
		return classifyIssueComment(detail)
	default:
		return domain.CategoryOther, domain.EntityRepository, "", otherDescription(payload.Type)
	}
}

// classifyIssueComment keeps pull-request comments pull-request activity and
// normalizes issue-only comments as other.
func classifyIssueComment(detail *detailPayload) (domain.Category, domain.EntityKind, string, string) {
	ref := ""
	onPullRequest := false
	if issue := detail.Issue; issue != nil {
		ref = numberRef(issue.Number)
		onPullRequest = issue.PullRequest != nil
	}
	if onPullRequest {
		return domain.CategoryComment, domain.EntityPullRequest, ref, pullRequestCommentDescription(ref)
	}
	return domain.CategoryOther, domain.EntityOther, ref, issueCommentDescription(ref)
}

func pushDescription(detail *detailPayload) string {
	branch := strings.TrimPrefix(strings.TrimSpace(detail.Ref), branchRefPrefix)
	commits := "commits"
	if detail.Size != nil && *detail.Size >= 0 {
		commits = commitCount(*detail.Size)
	}
	if branch == "" {
		return "pushed " + commits
	}
	return "pushed " + commits + " to " + branch
}

func commitCount(size int) string {
	if size == 1 {
		return "1 commit"
	}
	return strconv.Itoa(size) + " commits"
}

func pullRequestDescription(detail *detailPayload, ref string) string {
	return subject(pullRequestVerb(detail), entityPullRequest, ref)
}

func pullRequestVerb(detail *detailPayload) string {
	switch strings.ToLower(strings.TrimSpace(detail.Action)) {
	case "opened":
		return "opened"
	case "reopened":
		return "reopened"
	case "closed":
		if detail.PullRequest != nil && detail.PullRequest.Merged {
			return "merged"
		}
		return "closed"
	default:
		return "updated"
	}
}

func reviewDescription(review *reviewPayload, ref string) string {
	return subject(reviewVerb(review), entityPullRequest, ref)
}

func reviewVerb(review *reviewPayload) string {
	if review == nil {
		return "reviewed"
	}
	switch strings.ToLower(strings.TrimSpace(review.State)) {
	case "approved":
		return "approved"
	case "changes_requested":
		return "requested changes on"
	default:
		return "reviewed"
	}
}

func pullRequestCommentDescription(ref string) string {
	return subject("commented on", entityPullRequest, ref)
}

func issueCommentDescription(ref string) string {
	return namedSubject("commented on", entityIssue, ref)
}

// subject renders "<verb> <ref>" for an event whose category already names the
// entity class, so a rendered row states that class once (FR-005). Without a
// reference it names the entity rather than degrading to a bare verb.
func subject(verb, entity, ref string) string {
	if ref == "" {
		return verb + " " + article(entity) + " " + entity
	}
	return verb + " " + ref
}

// namedSubject renders "<verb> <entity> <ref>" for an event whose category does
// not name the entity class. An issue-only comment normalizes as `other`, so
// only the noun keeps it distinguishable from a pull-request comment.
func namedSubject(verb, entity, ref string) string {
	if ref == "" {
		return verb + " " + article(entity) + " " + entity
	}
	return verb + " " + entity + " " + ref
}

func article(entity string) string {
	if entity == entityIssue {
		return "an"
	}
	return "a"
}

func otherDescription(eventType string) string {
	name := strings.TrimSpace(eventType)
	if name == "" {
		return "repository activity"
	}
	return strings.ToLower(strings.TrimSuffix(name, "Event")) + " activity"
}

func pullRequestRef(pullRequest *pullRequestPayload) string {
	if pullRequest == nil {
		return ""
	}
	return numberRef(pullRequest.Number)
}

func numberRef(number int) string {
	if number <= 0 {
		return ""
	}
	return "#" + strconv.Itoa(number)
}
