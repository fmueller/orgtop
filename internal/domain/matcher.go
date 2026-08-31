package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// ErrInvalidMatcher reports a path matcher that does not satisfy the closed path
// grammar. It is returned for structurally impossible token sequences; pattern
// text diagnostics remain the parser's responsibility.
var ErrInvalidMatcher = errors.New("invalid path matcher")

// MatcherTokenKind names the four matcher token kinds of the closed grammar.
type MatcherTokenKind int

const (
	// MatcherTokenLiteral is case-sensitive literal path text within one segment.
	MatcherTokenLiteral MatcherTokenKind = iota
	// MatcherTokenWildcard matches zero or more code points other than the separator.
	MatcherTokenWildcard
	// MatcherTokenRecursive is a complete segment matching zero or more segments.
	MatcherTokenRecursive
	// MatcherTokenSeparator separates two retained segments.
	MatcherTokenSeparator
)

// MatcherToken is one token of a path matcher. Literal carries text only for
// MatcherTokenLiteral, which keeps the token comparable and map-key safe.
type MatcherToken struct {
	Kind    MatcherTokenKind
	Literal string
}

// LiteralToken returns a literal token holding already unescaped path text.
func LiteralToken(text string) MatcherToken {
	return MatcherToken{Kind: MatcherTokenLiteral, Literal: text}
}

// WildcardToken returns the single-segment wildcard token.
func WildcardToken() MatcherToken { return MatcherToken{Kind: MatcherTokenWildcard} }

// RecursiveToken returns the recursive segment token.
func RecursiveToken() MatcherToken { return MatcherToken{Kind: MatcherTokenRecursive} }

// SeparatorToken returns the segment separator token.
func SeparatorToken() MatcherToken { return MatcherToken{Kind: MatcherTokenSeparator} }

// String renders one token in pattern syntax.
func (t MatcherToken) String() string {
	switch t.Kind {
	case MatcherTokenLiteral:
		return t.Literal
	case MatcherTokenWildcard:
		return "*"
	case MatcherTokenRecursive:
		return "**"
	case MatcherTokenSeparator:
		return "/"
	default:
		return fmt.Sprintf("matcher token kind %d", int(t.Kind))
	}
}

// PathMatcher is a validated inclusive path matcher. It retains the requested
// token spelling for presentation and derives a canonical token sequence that
// closes matcher identity.
type PathMatcher struct {
	requested []MatcherToken
	canonical []MatcherToken
	identity  string
}

// NewPathMatcher validates the requested tokens and canonicalizes them. Adjacent
// literals coalesce, consecutive recursive segments collapse into one, and a
// terminal recursive segment following another retained segment is dropped
// because every matcher already includes descendants.
func NewPathMatcher(tokens []MatcherToken) (PathMatcher, error) {
	requested, err := coalesceLiterals(tokens)
	if err != nil {
		return PathMatcher{}, err
	}
	if err := validateMatcherStructure(requested); err != nil {
		return PathMatcher{}, err
	}
	canonical := canonicalizeMatcher(requested)
	return PathMatcher{
		requested: requested,
		canonical: canonical,
		identity:  encodeMatcherTokens(canonical),
	}, nil
}

// Tokens returns the requested token spelling.
func (m PathMatcher) Tokens() []MatcherToken { return slices.Clone(m.requested) }

// CanonicalTokens returns the canonical token sequence closing matcher identity.
func (m PathMatcher) CanonicalTokens() []MatcherToken { return slices.Clone(m.canonical) }

// Identity returns the canonical matcher-token encoding used by Scope identity.
func (m PathMatcher) Identity() string { return m.identity }

// IsZero reports whether the matcher was never constructed.
func (m PathMatcher) IsZero() bool { return len(m.canonical) == 0 }

// String renders the requested pattern spelling.
func (m PathMatcher) String() string { return renderMatcherTokens(m.requested) }

// coalesceLiterals validates every token kind and merges adjacent literal code
// points into one maximal literal token.
func coalesceLiterals(tokens []MatcherToken) ([]MatcherToken, error) {
	if len(tokens) == 0 {
		return nil, fmt.Errorf("%w: no tokens", ErrInvalidMatcher)
	}
	merged := make([]MatcherToken, 0, len(tokens))
	for _, token := range tokens {
		switch token.Kind {
		case MatcherTokenLiteral:
			if err := validateLiteral(token.Literal); err != nil {
				return nil, err
			}
			if last := len(merged) - 1; last >= 0 && merged[last].Kind == MatcherTokenLiteral {
				merged[last] = LiteralToken(merged[last].Literal + token.Literal)
				continue
			}
			merged = append(merged, token)
		case MatcherTokenWildcard, MatcherTokenRecursive, MatcherTokenSeparator:
			merged = append(merged, MatcherToken{Kind: token.Kind})
		default:
			return nil, fmt.Errorf("%w: unknown token kind %d", ErrInvalidMatcher, int(token.Kind))
		}
	}
	return merged, nil
}

func validateLiteral(text string) error {
	if text == "" {
		return fmt.Errorf("%w: empty literal token", ErrInvalidMatcher)
	}
	if !utf8.ValidString(text) {
		return fmt.Errorf("%w: literal token is not valid UTF-8", ErrInvalidMatcher)
	}
	if strings.ContainsRune(text, 0) {
		return fmt.Errorf("%w: literal token contains NUL", ErrInvalidMatcher)
	}
	if strings.Contains(text, "/") {
		return fmt.Errorf("%w: literal token %q contains a separator", ErrInvalidMatcher, text)
	}
	return nil
}

// validateMatcherStructure enforces non-empty segments separated by exactly one
// separator, recursive segments standing alone, and no adjacent wildcards.
func validateMatcherStructure(tokens []MatcherToken) error {
	for _, segment := range splitMatcherSegments(tokens) {
		if len(segment) == 0 {
			return fmt.Errorf("%w: empty segment", ErrInvalidMatcher)
		}
		if isRecursiveSegment(segment) {
			continue
		}
		if err := validateOrdinarySegment(segment); err != nil {
			return err
		}
	}
	return nil
}

func validateOrdinarySegment(segment []MatcherToken) error {
	for i, token := range segment {
		switch token.Kind {
		case MatcherTokenLiteral:
			if len(segment) == 1 && (token.Literal == "." || token.Literal == "..") {
				return fmt.Errorf("%w: %q is not a path segment", ErrInvalidMatcher, token.Literal)
			}
		case MatcherTokenWildcard:
			if i > 0 && segment[i-1].Kind == MatcherTokenWildcard {
				return fmt.Errorf("%w: adjacent wildcards in one segment", ErrInvalidMatcher)
			}
		case MatcherTokenRecursive:
			return fmt.Errorf("%w: a recursive segment cannot share a segment", ErrInvalidMatcher)
		default:
			return fmt.Errorf("%w: unexpected token %v in a segment", ErrInvalidMatcher, token)
		}
	}
	return nil
}

// splitMatcherSegments splits the token sequence on separators. A leading,
// trailing, or repeated separator produces an empty segment, which the caller
// rejects.
func splitMatcherSegments(tokens []MatcherToken) [][]MatcherToken {
	segments := make([][]MatcherToken, 1)
	for _, token := range tokens {
		if token.Kind == MatcherTokenSeparator {
			segments = append(segments, nil)
			continue
		}
		last := len(segments) - 1
		segments[last] = append(segments[last], token)
	}
	return segments
}

// canonicalizeMatcher applies the closed identity rewrites to validated segments.
func canonicalizeMatcher(tokens []MatcherToken) []MatcherToken {
	segments := splitMatcherSegments(tokens)

	retained := make([][]MatcherToken, 0, len(segments))
	for _, segment := range segments {
		if isRecursiveSegment(segment) && len(retained) > 0 && isRecursiveSegment(retained[len(retained)-1]) {
			continue
		}
		retained = append(retained, segment)
	}
	if len(retained) > 1 && isRecursiveSegment(retained[len(retained)-1]) {
		retained = retained[:len(retained)-1]
	}

	canonical := make([]MatcherToken, 0, len(tokens))
	for i, segment := range retained {
		if i > 0 {
			canonical = append(canonical, SeparatorToken())
		}
		canonical = append(canonical, segment...)
	}
	return canonical
}

func isRecursiveSegment(segment []MatcherToken) bool {
	return len(segment) == 1 && segment[0].Kind == MatcherTokenRecursive
}

// encodeMatcherTokens renders the canonical identity encoding. Tokens are joined
// by NUL, which the grammar excludes from literal text, so no literal spelling can
// collide with another token sequence.
func encodeMatcherTokens(tokens []MatcherToken) string {
	var encoded strings.Builder
	for i, token := range tokens {
		if i > 0 {
			encoded.WriteByte(0)
		}
		switch token.Kind {
		case MatcherTokenLiteral:
			encoded.WriteString("L")
			encoded.WriteString(token.Literal)
		case MatcherTokenWildcard:
			encoded.WriteString("S")
		case MatcherTokenRecursive:
			encoded.WriteString("D")
		case MatcherTokenSeparator:
			encoded.WriteString("X")
		}
	}
	return encoded.String()
}

func renderMatcherTokens(tokens []MatcherToken) string {
	var rendered strings.Builder
	for _, token := range tokens {
		rendered.WriteString(token.String())
	}
	return rendered.String()
}
