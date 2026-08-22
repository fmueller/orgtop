// Package github adapts GitHub's repository Events API to OrgTop's domain model.
// The transport payload types below are owned by this package: they are never
// returned, embedded in domain events, or rendered by the TUI.
package github

// eventPayload is one entry of a GET /repos/{owner}/{repo}/events page.
type eventPayload struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	CreatedAt string         `json:"created_at"`
	Actor     *actorPayload  `json:"actor"`
	Repo      *repoPayload   `json:"repo"`
	Payload   *detailPayload `json:"payload"`
}

// actorPayload is the optional originator of an event.
type actorPayload struct {
	Login string `json:"login"`
}

// repoPayload carries the returned owner/repository identity of an event.
type repoPayload struct {
	Name string `json:"name"`
}

// detailPayload holds the category-specific detail OrgTop describes. Fields that
// v0.1.0 does not describe are deliberately not modelled.
type detailPayload struct {
	Action      string              `json:"action"`
	Ref         string              `json:"ref"`
	Head        string              `json:"head"`
	Size        *int                `json:"size"`
	PullRequest *pullRequestPayload `json:"pull_request"`
	Issue       *issuePayload       `json:"issue"`
	Review      *reviewPayload      `json:"review"`
}

// pullRequestPayload is the pull request a pull-request-scoped event refers to.
type pullRequestPayload struct {
	Number int  `json:"number"`
	Merged bool `json:"merged"`
}

// issuePayload is the issue an IssueCommentEvent refers to. A non-nil PullRequest
// marks the issue as a pull request.
type issuePayload struct {
	Number      int                      `json:"number"`
	PullRequest *issuePullRequestPayload `json:"pull_request"`
}

// issuePullRequestPayload marks an issue as a pull request.
type issuePullRequestPayload struct {
	URL string `json:"url"`
}

// reviewPayload is the review a PullRequestReviewEvent refers to.
type reviewPayload struct {
	State string `json:"state"`
}
