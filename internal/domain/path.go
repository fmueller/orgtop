package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ErrInvalidPath reports a changed path outside the strict repository-relative
// representation. A malformed path is never cleaned up into a valid one, because
// the whole evidence set it belongs to is incomplete instead.
var ErrInvalidPath = errors.New("invalid changed path")

// ChangedPath is a validated repository-relative changed path. It is stored
// exactly as observed: no host filesystem cleanup, case folding, symlink
// resolution, or separator conversion occurs.
type ChangedPath struct {
	value string
}

// NewChangedPath validates one changed path against the closed representation:
// valid UTF-8 without NUL, and non-empty "/" separated segments with no leading,
// trailing, or repeated separator and no "." or ".." segment.
func NewChangedPath(value string) (ChangedPath, error) {
	if value == "" {
		return ChangedPath{}, fmt.Errorf("%w: empty path", ErrInvalidPath)
	}
	if !utf8.ValidString(value) {
		return ChangedPath{}, fmt.Errorf("%w: path is not valid UTF-8", ErrInvalidPath)
	}
	if strings.ContainsRune(value, 0) {
		return ChangedPath{}, fmt.Errorf("%w: path %q contains NUL", ErrInvalidPath, value)
	}
	for _, segment := range strings.Split(value, "/") {
		switch segment {
		case "":
			return ChangedPath{}, fmt.Errorf("%w: path %q has an empty segment", ErrInvalidPath, value)
		case ".", "..":
			return ChangedPath{}, fmt.Errorf("%w: path %q has a %q segment", ErrInvalidPath, value, segment)
		}
	}
	return ChangedPath{value: value}, nil
}

// String returns the unmodified changed path.
func (p ChangedPath) String() string { return p.value }

// IsZero reports whether the path was never validated.
func (p ChangedPath) IsZero() bool { return p.value == "" }

// Segments returns the path's segments in order.
func (p ChangedPath) Segments() []string {
	if p.IsZero() {
		return nil
	}
	return strings.Split(p.value, "/")
}

// Matches reports whether the matcher selects the changed path. The canonical
// tokens match either the complete path or a segment-boundary prefix of it, so a
// selected component also selects everything below it. Matching is case-sensitive
// and code-point exact, and an unvalidated path never matches.
func (m PathMatcher) Matches(path ChangedPath) bool {
	if m.IsZero() || path.IsZero() {
		return false
	}
	return matchSegmentSequences(splitMatcherSegments(m.canonical), path.Segments())
}

// MatchesAny reports whether the matcher selects any of the paths. A rename
// offers both its old and its new name, and matching either or both is still one
// membership for the matcher's Scope.
func (m PathMatcher) MatchesAny(paths ...ChangedPath) bool {
	for _, path := range paths {
		if m.Matches(path) {
			return true
		}
	}
	return false
}

// matchSegmentSequences decides segment-level matching. It fills the suffix table
// matched[i][j], "pattern segments from i match path segments from j", so a
// recursive segment costs no backtracking and repetitive input stays bounded.
// An exhausted pattern matches because every matcher includes descendants.
func matchSegmentSequences(pattern [][]MatcherToken, path []string) bool {
	matched := make([][]bool, len(pattern)+1)
	for i := range matched {
		matched[i] = make([]bool, len(path)+1)
	}
	exhausted := matched[len(pattern)]
	for j := range exhausted {
		exhausted[j] = true
	}

	for i := len(pattern) - 1; i >= 0; i-- {
		recursive := isRecursiveSegment(pattern[i])
		for j := len(path); j >= 0; j-- {
			if recursive {
				// Zero segments here, or one more segment consumed.
				matched[i][j] = matched[i+1][j] || (j < len(path) && matched[i][j+1])
				continue
			}
			// Any other segment consumes exactly one path segment.
			matched[i][j] = j < len(path) && matchSegment(pattern[i], path[j]) && matched[i+1][j+1]
		}
	}
	return matched[0][0]
}

// matchSegment matches one non-recursive segment's tokens against one path
// segment. Wildcards match zero or more code points and never cross a separator,
// which the caller guarantees by matching segment by segment.
func matchSegment(tokens []MatcherToken, segment string) bool {
	tokenIndex, offset := 0, 0
	starToken, starOffset := -1, 0
	for offset < len(segment) {
		switch {
		case tokenIndex < len(tokens) && tokens[tokenIndex].Kind == MatcherTokenWildcard:
			starToken, starOffset = tokenIndex, offset
			tokenIndex++
		case tokenIndex < len(tokens) && strings.HasPrefix(segment[offset:], tokens[tokenIndex].Literal):
			offset += len(tokens[tokenIndex].Literal)
			tokenIndex++
		case starToken >= 0:
			// Let the last wildcard consume one more code point and retry.
			_, width := utf8.DecodeRuneInString(segment[starOffset:])
			starOffset += width
			tokenIndex, offset = starToken+1, starOffset
		default:
			return false
		}
	}
	for tokenIndex < len(tokens) && tokens[tokenIndex].Kind == MatcherTokenWildcard {
		tokenIndex++
	}
	return tokenIndex == len(tokens)
}
