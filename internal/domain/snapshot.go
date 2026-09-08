package domain

// MaxSnapshotEvents bounds the in-memory snapshot after combination (FR-006).
const MaxSnapshotEvents = 500

// RepositoryActivity is the successful result for one Scope entry as the source
// returns it: the input a refresh retains through Retain before it enriches the
// retained set and publishes a ScopedSnapshot.
//
// It carries no repository display identity. v0.2 supersedes the v0.1 FR-002
// rule that promoted the first matching returned spelling to a snapshot display
// name: under RG-012 a published Scope row is labelled from the Scope's own
// retained requested spelling, so a returned spelling would be a display fact
// nothing reads. Each event keeps the returned identity the source validated
// case-insensitively against the request.
type RepositoryActivity struct {
	Events []Event
}

// isPullRequestActivity reports whether the event counts as pull-request
// activity under FR-006. Review and pull-request comment events count;
// unrelated issue comments do not.
func isPullRequestActivity(event Event) bool {
	switch event.Category {
	case CategoryPullRequest, CategoryReview:
		return true
	case CategoryComment:
		return event.EntityKind == EntityPullRequest
	default:
		return false
	}
}
