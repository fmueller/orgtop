package github

import (
	"context"
	"strings"

	"github.com/fmueller/orgtop/internal/domain"
)

// maxCompareFiles is the file count GitHub documents as the comparison cap. The
// response describes the whole comparison but stops listing at this many files,
// so exactly this many records can never be proven complete, with or without a
// pagination link.
const maxCompareFiles = 300

// compareEvidence reads the changed files between two exact immutable objects.
// The request carries no pagination parameters, so the single response either
// proves the whole comparison or proves nothing.
func (e Enricher) compareEvidence(ctx context.Context, descriptor domain.EvidenceDescriptor) domain.EvidenceOutcome {
	body, _, outcome, ok := e.read(ctx, e.compareURL(descriptor))
	if !ok {
		return outcome
	}
	var response compareResponse
	if !decode(body, &response) {
		return domain.IncompleteOutcome(reasonMalformedBody)
	}
	if !matchesCompareURL(e.baseURL(), response.URL, descriptor) {
		return domain.IncompleteOutcome(reasonMalformedBody)
	}
	if response.BaseCommit == nil {
		return domain.IncompleteOutcome(reasonMalformedBody)
	}
	base, valid := domain.NormalizeObjectSHA(strings.TrimSpace(response.BaseCommit.SHA))
	if !valid || base != descriptor.Base() {
		return domain.IncompleteOutcome(reasonMalformedBody)
	}
	if len(response.Files) >= maxCompareFiles {
		return domain.IncompleteOutcome(reasonMalformedBody)
	}
	paths := newPathSet()
	if !paths.addRecords(response.Files) {
		return domain.IncompleteOutcome(reasonMalformedBody)
	}
	// The comparison is bound to its immutable base and head, so its provenance
	// is whatever selected that pair: event-time facts, or a captured current
	// pull request pair that must never be described as the files at event time.
	return domain.CompleteOutcome(descriptor.Provenance(), paths.collected())
}

// currentPullRequestRefs verifies a pull request metadata response and reports
// the current base and head it froze for this refresh.
func currentPullRequestRefs(root string, descriptor domain.EvidenceDescriptor, body []byte) (string, string, bool) {
	var response pullRequestResponse
	if !decode(body, &response) {
		return "", "", false
	}
	if response.Number != descriptor.Number() || !matchesPullRequestURL(root, response.URL, descriptor) {
		return "", "", false
	}
	if response.Base == nil || response.Head == nil || response.Base.Repository == nil {
		return "", "", false
	}
	if !matchesCanonicalRepository(response.Base.Repository.FullName, descriptor.Repository()) {
		return "", "", false
	}
	base, baseValid := domain.NormalizeObjectSHA(strings.TrimSpace(response.Base.SHA))
	head, headValid := domain.NormalizeObjectSHA(strings.TrimSpace(response.Head.SHA))
	if !baseValid || !headValid {
		return "", "", false
	}
	return base, head, true
}
