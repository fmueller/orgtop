package domain

// MaxSnapshotEvents bounds the in-memory snapshot after combination (FR-006).
const MaxSnapshotEvents = 500

// RepositoryActivity is the successful result for one Scope entry as the source
// returns it: the input a refresh retains through Retain before it enriches the
// retained set and publishes a ScopedSnapshot. Only Events are retained.
//
// Repository is the FR-002 returned display identity the source records for
// this refresh: the first matching returned spelling, or the requested spelling
// when the page is empty. Nothing downstream consumes it for display today,
// because a published Scope row is labelled from the Scope's own retained
// requested spelling; the field carries the source-side fact so a later change
// can render it without re-deriving one.
type RepositoryActivity struct {
	Repository Repository
	Events     []Event
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
