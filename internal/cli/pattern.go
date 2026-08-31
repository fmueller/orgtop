package cli

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/fmueller/orgtop/internal/domain"
)

// escapable lists the only characters a backslash may escape. Backslash is never
// a path separator (RG-002).
const escapable = `*:\`

// patternError is one deterministic pattern diagnostic. It names the offending
// pattern text, the zero-based UTF-8 byte offset of the cause, and the cause
// itself, and it reports the domain matcher sentinel so callers can classify it
// without matching copy.
type patternError struct {
	pattern string
	offset  int
	cause   string
}

func (e *patternError) Error() string {
	return fmt.Sprintf("invalid path pattern %q at byte %d: %s", e.pattern, e.offset, e.cause)
}

func (e *patternError) Unwrap() error { return domain.ErrInvalidMatcher }

func patternFault(pattern string, offset int, cause string) error {
	return &patternError{pattern: pattern, offset: offset, cause: cause}
}

// pathSelection is one parsed --path value: a matcher plus, for the qualified
// form, the repository named before the delimiting colon.
type pathSelection struct {
	repository domain.Repository
	qualified  bool
	matcher    domain.PathMatcher
}

// parsePathValue splits a --path value into its optional qualified repository
// prefix and its pattern. Repository-prefix validation precedes pattern
// tokenization, so a malformed qualified repository is reported even when the
// remaining pattern is also invalid (RG-002).
func parsePathValue(value string) (pathSelection, error) {
	prefix, pattern, qualified := splitQualifiedPath(value)
	if !qualified {
		matcher, err := parsePattern(value)
		if err != nil {
			return pathSelection{}, err
		}
		return pathSelection{matcher: matcher}, nil
	}
	repository, err := domain.ParseRepository(prefix)
	if err != nil {
		return pathSelection{}, err
	}
	matcher, err := parsePattern(pattern)
	if err != nil {
		return pathSelection{}, err
	}
	return pathSelection{repository: repository, qualified: true, matcher: matcher}, nil
}

// splitQualifiedPath reports the text before and after the first unescaped
// colon. An escaped colon belongs to the pattern, so the scan steps over every
// escape pair without judging it: tokenization reports invalid escapes.
func splitQualifiedPath(value string) (prefix, pattern string, qualified bool) {
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\\':
			i++
		case ':':
			return value[:i], value[i+1:], true
		}
	}
	return "", value, false
}

// parsePattern tokenizes one pattern into validated matcher tokens. Invalid
// UTF-8 is checked first, then NUL, then a single left-to-right scan reports the
// first invalid escape, second unescaped colon, empty segment, "."/".." segment,
// or invalid star construction (RG-002).
func parsePattern(pattern string) (domain.PathMatcher, error) {
	if err := checkPatternBytes(pattern); err != nil {
		return domain.PathMatcher{}, err
	}
	if pattern == "" {
		return domain.PathMatcher{}, patternFault(pattern, 0, "empty pattern")
	}

	scan := patternScan{pattern: pattern}
	if err := scan.run(); err != nil {
		return domain.PathMatcher{}, err
	}
	matcher, err := domain.NewPathMatcher(scan.tokens)
	if err != nil {
		return domain.PathMatcher{}, fmt.Errorf("invalid path pattern %q: %w", pattern, err)
	}
	return matcher, nil
}

// checkPatternBytes enforces the two whole-value preconditions of the grammar in
// their closed order.
func checkPatternBytes(pattern string) error {
	for i := 0; i < len(pattern); {
		// A RuneError of width one is an undecodable byte; a wider one is the
		// caller's own U+FFFD, which is valid text.
		r, width := utf8.DecodeRuneInString(pattern[i:])
		if r == utf8.RuneError && width == 1 {
			return patternFault(pattern, i, "value is not valid UTF-8")
		}
		i += width
	}
	if i := strings.IndexByte(pattern, 0); i >= 0 {
		return patternFault(pattern, i, "value contains NUL")
	}
	return nil
}

// patternScan is the left-to-right tokenizer state. It tracks the current
// segment so segment-scoped rules report the offset the contract names.
type patternScan struct {
	pattern string
	tokens  []domain.MatcherToken

	// segmentStart is the byte offset the current segment begins at.
	segmentStart int
	// segmentTokens counts the tokens the current segment has emitted.
	segmentTokens int
	// segmentText collects the segment's literal text so a "." or ".." segment
	// is recognized only when nothing else contributed to it.
	segmentText strings.Builder
	// segmentLiteralOnly reports whether the segment consists of literal text.
	segmentLiteralOnly bool
	// separatorOffset is the byte offset of the separator that opened the
	// current segment, which a trailing separator reports.
	separatorOffset int
}

func (s *patternScan) run() error {
	s.segmentLiteralOnly = true
	for i := 0; i < len(s.pattern); {
		switch s.pattern[i] {
		case '\\':
			width, err := s.escape(i)
			if err != nil {
				return err
			}
			i += width
		case ':':
			return patternFault(s.pattern, i, "more than one unescaped colon")
		case '/':
			if err := s.closeSegment(i); err != nil {
				return err
			}
			s.tokens = append(s.tokens, domain.SeparatorToken())
			s.openSegment(i)
			i++
		case '*':
			width, err := s.stars(i)
			if err != nil {
				return err
			}
			i += width
		default:
			_, width := utf8.DecodeRuneInString(s.pattern[i:])
			s.literal(s.pattern[i : i+width])
			i += width
		}
	}
	return s.closeSegment(s.separatorOffset)
}

// escape consumes one escape pair and returns its width in bytes.
func (s *patternScan) escape(i int) (int, error) {
	if i+1 >= len(s.pattern) {
		return 0, patternFault(s.pattern, i, "trailing backslash")
	}
	escaped := s.pattern[i+1]
	if !strings.ContainsRune(escapable, rune(escaped)) {
		return 0, patternFault(s.pattern, i, fmt.Sprintf("invalid escape %q", s.pattern[i:i+2]))
	}
	s.literal(string(escaped))
	return 2, nil
}

// stars consumes one run of unescaped stars and returns its width in bytes. A
// single star is the within-segment wildcard; exactly two stars are recursive
// only as a complete segment.
func (s *patternScan) stars(i int) (int, error) {
	width := 0
	for i+width < len(s.pattern) && s.pattern[i+width] == '*' {
		width++
	}
	if width == 1 {
		s.emit(domain.WildcardToken())
		return width, nil
	}
	completeSegment := width == 2 && s.segmentTokens == 0 && (i+width == len(s.pattern) || s.pattern[i+width] == '/')
	if !completeSegment {
		return 0, patternFault(s.pattern, i, `"**" must be a complete segment`)
	}
	s.emit(domain.RecursiveToken())
	return width, nil
}

// emit appends one wildcard token to the current segment, which the token's
// presence makes non-literal.
func (s *patternScan) emit(token domain.MatcherToken) {
	s.tokens = append(s.tokens, token)
	s.segmentTokens++
	s.segmentLiteralOnly = false
}

func (s *patternScan) literal(text string) {
	s.tokens = append(s.tokens, domain.LiteralToken(text))
	s.segmentTokens++
	s.segmentText.WriteString(text)
}

// closeSegment validates the segment that ends at offset. An empty segment is
// reported at the separator position the contract names; a "." or ".." segment
// is reported at the segment's own start.
func (s *patternScan) closeSegment(offset int) error {
	if s.segmentTokens == 0 {
		return patternFault(s.pattern, offset, "empty segment")
	}
	if text := s.segmentText.String(); s.segmentLiteralOnly && (text == "." || text == "..") {
		return patternFault(s.pattern, s.segmentStart, fmt.Sprintf("%q is not a path segment", text))
	}
	return nil
}

func (s *patternScan) openSegment(separatorOffset int) {
	s.separatorOffset = separatorOffset
	s.segmentStart = separatorOffset + 1
	s.segmentTokens = 0
	s.segmentText.Reset()
	s.segmentLiteralOnly = true
}
