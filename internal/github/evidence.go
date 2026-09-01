package github

import (
	"strings"

	"github.com/fmueller/orgtop/internal/domain"
)

// Sanitized classification reasons. They name the missing evidence form only and
// never quote a payload value.
const (
	reasonUnsupportedType   = "event type carries no changed-file evidence"
	reasonNoObjects         = "event carries no valid immutable objects"
	reasonNoPullRequest     = "comment is not on a pull request"
	reasonNoCommentPath     = "review comment carries no usable path"
	reasonUnusableEvidence  = "event evidence identity is unusable"
	reasonIssueOnlyActivity = "issue-only activity has no changed files"
)

// changedFileEvidence maps one normalized event fact onto the evidence the RG-003
// eligibility table selects for it. Classification is ordered per event type and
// never guesses: an event whose immutable identity is missing, zero, or malformed
// receives settled evidence rather than a request that could return the wrong
// files.
func changedFileEvidence(repository domain.Repository, payload eventPayload) domain.EvidenceDescriptor {
	detail := payload.Payload
	if detail == nil {
		return domain.NewUnsupportedEvidence(reasonNoObjects)
	}
	switch payload.Type {
	case typePush:
		return pushEvidence(repository, detail)
	case typePullRequestReviewComment:
		return reviewCommentEvidence(detail)
	case typePullRequest, typePullRequestReview:
		return eventTimeEvidence(repository, detail.PullRequest)
	case typeIssueComment:
		return issueCommentEvidence(repository, detail.Issue)
	default:
		return domain.NewUnsupportedEvidence(reasonUnsupportedType)
	}
}

// pushEvidence classifies a push in the closed order: SHA syntax first, then
// equality, then the one-commit optimization, then immutable compare. Equality
// wins over a contradictory size, so an empty push costs no request.
func pushEvidence(repository domain.Repository, detail *detailPayload) domain.EvidenceDescriptor {
	before, beforeOK := domain.NormalizeObjectSHA(strings.TrimSpace(detail.Before))
	head, headOK := domain.NormalizeObjectSHA(strings.TrimSpace(detail.Head))
	if !beforeOK || !headOK {
		return domain.NewUnsupportedEvidence(reasonNoObjects)
	}
	if before == head {
		return domain.NewUnchangedEvidence()
	}
	if detail.Size != nil && *detail.Size == 1 {
		return descriptorOrUnsupported(domain.NewCommitEvidence(repository, head))
	}
	return descriptorOrUnsupported(domain.NewCompareEvidence(repository, before, head, domain.ProvenanceEventTime))
}

// reviewCommentEvidence classifies a file-specific review comment. It is
// direct-only: a present valid path is complete singleton evidence, and a
// missing, empty, or malformed path is incomplete rather than a fallback to the
// pull request's objects, because that comment describes one file and not the
// whole pull request.
func reviewCommentEvidence(detail *detailPayload) domain.EvidenceDescriptor {
	if detail.Comment == nil {
		return domain.NewIncompleteEvidence(reasonNoCommentPath)
	}
	path, err := domain.NewChangedPath(detail.Comment.Path)
	if err != nil {
		return domain.NewIncompleteEvidence(reasonNoCommentPath)
	}
	return domain.NewDirectEvidence(path)
}

// eventTimeEvidence classifies a pull-request-family event from the base and head
// objects the event itself carried, so a later force-push cannot change it. Equal
// valid objects are decidable without a request for exactly the reason an equal
// push is: the comparison they name is empty. Reporting that as unsupported would
// turn a knowable not-member into an unknown.
func eventTimeEvidence(repository domain.Repository, pullRequest *pullRequestPayload) domain.EvidenceDescriptor {
	if pullRequest == nil || pullRequest.Base == nil || pullRequest.Head == nil {
		return domain.NewUnsupportedEvidence(reasonNoObjects)
	}
	base, baseValid := domain.NormalizeObjectSHA(strings.TrimSpace(pullRequest.Base.SHA))
	head, headValid := domain.NormalizeObjectSHA(strings.TrimSpace(pullRequest.Head.SHA))
	if !baseValid || !headValid {
		return domain.NewUnsupportedEvidence(reasonNoObjects)
	}
	if base == head {
		return domain.NewUnchangedEvidence()
	}
	return descriptorOrUnsupported(domain.NewCompareEvidence(repository, base, head, domain.ProvenanceEventTime))
}

// issueCommentEvidence classifies an ordinary comment. Only a comment carrying
// the pull request marker and a number qualifies for the current-PR exception;
// issue-only activity has no changed files at all.
func issueCommentEvidence(repository domain.Repository, issue *issuePayload) domain.EvidenceDescriptor {
	if issue == nil || issue.PullRequest == nil {
		return domain.NewUnsupportedEvidence(reasonIssueOnlyActivity)
	}
	if issue.Number <= 0 {
		return domain.NewUnsupportedEvidence(reasonNoPullRequest)
	}
	return descriptorOrUnsupported(domain.NewPullRequestEvidence(repository, issue.Number))
}

// descriptorOrUnsupported keeps a rejected identity out of the pipeline instead
// of surfacing a construction error the caller could only turn into the same
// unknown membership.
func descriptorOrUnsupported(descriptor domain.EvidenceDescriptor, err error) domain.EvidenceDescriptor {
	if err != nil {
		return domain.NewUnsupportedEvidence(reasonUnusableEvidence)
	}
	return descriptor
}
