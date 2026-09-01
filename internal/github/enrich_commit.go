package github

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fmueller/orgtop/internal/domain"
)

// commitEvidence walks the paginated files of one exact commit. Completeness is
// proven, never assumed: every page must repeat the requested commit identity
// and one consistent sole parent, pagination must advance safely within the
// closed page limit, and only a final page without a next link proves the set is
// whole. Any violation discards every path already collected, because a
// valid-looking subset would be indistinguishable from complete evidence.
func (e Enricher) commitEvidence(ctx context.Context, descriptor domain.EvidenceDescriptor) domain.EvidenceOutcome {
	paths := newPathSet()
	parent := ""
	endpoint := e.commitURL(descriptor)
	visited := map[string]struct{}{}

	for range domain.MaxEvidencePages {
		if _, repeated := visited[endpoint]; repeated {
			return domain.IncompleteOutcome(reasonMalformedBody)
		}
		visited[endpoint] = struct{}{}

		body, header, outcome, ok := e.read(ctx, endpoint)
		if !ok {
			return outcome
		}
		var response commitResponse
		if !decode(body, &response) {
			return domain.IncompleteOutcome(reasonMalformedBody)
		}
		soleParent, identical := commitPageIdentity(descriptor, response, parent)
		if !identical || !paths.addRecords(response.Files) {
			return domain.IncompleteOutcome(reasonMalformedBody)
		}
		parent = soleParent

		next, present := e.nextCommitPage(header, descriptor)
		if !present {
			// A final page without a next link proves completeness, the empty
			// set included. Per-event applicability is then decided from the
			// verified sole parent rather than from another request.
			return domain.CompleteOutcome(domain.ProvenanceEventTime, paths.collected()).WithSoleParent(parent)
		}
		if next == "" {
			// A next link exists but is not safe to follow, so the remaining
			// pages are unreachable and the set is not whole.
			return domain.IncompleteOutcome(reasonMalformedBody)
		}
		endpoint = next
	}
	// A next link still present at the page bound proves incompleteness.
	return domain.IncompleteOutcome(reasonMalformedBody)
}

// commitPageIdentity verifies that a page describes the requested commit and one
// consistent sole parent. Commit pages cannot mutate by contract, so a changed
// identity or parent list is malformed rather than mixed across pages, and a
// commit without exactly one parent can prove no event's before object.
func commitPageIdentity(descriptor domain.EvidenceDescriptor, response commitResponse, seenParent string) (string, bool) {
	sha, ok := domain.NormalizeObjectSHA(strings.TrimSpace(response.SHA))
	if !ok || sha != descriptor.Head() {
		return "", false
	}
	if len(response.Parents) != 1 {
		return "", false
	}
	parent, ok := domain.NormalizeObjectSHA(strings.TrimSpace(response.Parents[0].SHA))
	if !ok || (seenParent != "" && parent != seenParent) {
		return "", false
	}
	return parent, true
}

// nextCommitPage reports the response's rel="next" target and whether one was
// offered at all. An offered target that is unsafe to follow comes back empty:
// it must be the same scheme and host as the configured API root, name the same
// canonical repository and exact commit, and carry only the page and per-page
// query OrgTop itself requested, advancing to an unseen page.
func (e Enricher) nextCommitPage(header http.Header, descriptor domain.EvidenceDescriptor) (string, bool) {
	raw, present := linkRelation(header, "next")
	if !present {
		return "", false
	}
	parsed, components, ok := parseAPIURL(e.baseURL(), raw)
	if !ok || !matchesEntityPath(components, descriptor.Repository(), "commits", descriptor.Head()) {
		return "", true
	}
	if !expectedPageQuery(parsed.Query()) {
		return "", true
	}
	return raw, true
}

// expectedPageQuery reports whether a next link carries only the page and
// per-page members OrgTop itself requested, with a page that advances.
func expectedPageQuery(query url.Values) bool {
	if len(query) != 2 || query.Get("per_page") != perPage {
		return false
	}
	page, err := strconv.Atoi(query.Get("page"))
	return err == nil && page > 1
}

// linkRelation returns the target of one Link relation.
func linkRelation(header http.Header, relation string) (string, bool) {
	for _, value := range header.Values("Link") {
		for _, entry := range strings.Split(value, ",") {
			parts := strings.Split(entry, ";")
			if len(parts) < 2 {
				continue
			}
			target := strings.TrimSpace(parts[0])
			if !strings.HasPrefix(target, "<") || !strings.HasSuffix(target, ">") {
				continue
			}
			for _, parameter := range parts[1:] {
				if strings.EqualFold(strings.TrimSpace(parameter), `rel="`+relation+`"`) {
					return target[1 : len(target)-1], true
				}
			}
		}
	}
	return "", false
}
