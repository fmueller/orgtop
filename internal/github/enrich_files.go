package github

import (
	"github.com/fmueller/orgtop/internal/domain"
)

// Documented changed-file statuses. Any other status makes the whole evidence
// result incomplete: an unknown status could mean a path form OrgTop does not
// understand, and no valid-looking subset is admitted.
const (
	statusAdded     = "added"
	statusRemoved   = "removed"
	statusModified  = "modified"
	statusRenamed   = "renamed"
	statusCopied    = "copied"
	statusChanged   = "changed"
	statusUnchanged = "unchanged"
)

// pathSet accumulates the normalized changed paths of one evidence identity. It
// enforces the closed per-identity capacities while collecting, so an over-bound
// identity is rejected instead of silently truncated.
type pathSet struct {
	paths []domain.ChangedPath
	seen  map[string]struct{}
	bytes int
}

func newPathSet() *pathSet {
	return &pathSet{seen: map[string]struct{}{}}
}

// addRecords normalizes one response's file records into the set. It reports
// false as soon as any record is invalid, duplicated, or over a capacity.
func (s *pathSet) addRecords(records []filePayload) bool {
	for _, record := range records {
		if !s.addRecord(record) {
			return false
		}
	}
	return true
}

// addRecord admits one file record. A rename contributes both its old and its
// new normalized path; every other documented status contributes its filename.
// A repeated file record is malformed rather than idempotent, because one
// response describing a file twice cannot be proven consistent.
func (s *pathSet) addRecord(record filePayload) bool {
	if !isDocumentedStatus(record.Status) {
		return false
	}
	if !s.add(record.Filename) {
		return false
	}
	if record.Status != statusRenamed {
		return true
	}
	return s.add(record.PreviousFilename)
}

func (s *pathSet) add(value string) bool {
	if len(value) > domain.MaxChangedPathBytes {
		return false
	}
	path, err := domain.NewChangedPath(value)
	if err != nil {
		return false
	}
	if _, duplicate := s.seen[path.String()]; duplicate {
		return false
	}
	if len(s.paths) >= domain.MaxEvidencePaths || s.bytes+len(value) > domain.MaxEvidenceBytes {
		return false
	}
	s.seen[path.String()] = struct{}{}
	s.bytes += len(value)
	s.paths = append(s.paths, path)
	return true
}

// collected returns the accumulated paths in the order GitHub reported them.
func (s *pathSet) collected() []domain.ChangedPath { return s.paths }

func isDocumentedStatus(status string) bool {
	switch status {
	case statusAdded, statusRemoved, statusModified, statusRenamed,
		statusCopied, statusChanged, statusUnchanged:
		return true
	default:
		return false
	}
}
