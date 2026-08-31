package domain_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/fmueller/orgtop/internal/domain"
)

func literal(text string) domain.MatcherToken { return domain.LiteralToken(text) }

func wildcard() domain.MatcherToken { return domain.WildcardToken() }

func recursive() domain.MatcherToken { return domain.RecursiveToken() }

func separator() domain.MatcherToken { return domain.SeparatorToken() }

// pattern builds the token sequence for a slash separated pattern whose segments
// are already tokenized, keeping the tests readable.
func mustMatcher(t *testing.T, tokens ...domain.MatcherToken) domain.PathMatcher {
	t.Helper()
	matcher, err := domain.NewPathMatcher(tokens)
	if err != nil {
		t.Fatalf("NewPathMatcher(%v) returned error: %v", tokens, err)
	}
	return matcher
}

// TestNewPathMatcherCanonicalizesTokens pins the closed RG-002 canonical
// rewrites: literals coalesce, consecutive recursive segments collapse, and a
// terminal recursive segment following another retained segment is redundant.
func TestNewPathMatcherCanonicalizesTokens(t *testing.T) {
	tests := []struct {
		name   string
		tokens []domain.MatcherToken
		want   []domain.MatcherToken
	}{
		{
			name:   "literal segments",
			tokens: []domain.MatcherToken{literal("services"), separator(), literal("api")},
			want:   []domain.MatcherToken{literal("services"), separator(), literal("api")},
		},
		{
			name:   "adjacent literals coalesce",
			tokens: []domain.MatcherToken{literal("doc"), literal("s"), separator(), literal("a"), literal("pi")},
			want:   []domain.MatcherToken{literal("docs"), separator(), literal("api")},
		},
		{
			name:   "terminal recursive segment is removed",
			tokens: []domain.MatcherToken{literal("services"), separator(), literal("api"), separator(), recursive()},
			want:   []domain.MatcherToken{literal("services"), separator(), literal("api")},
		},
		{
			name:   "repeated terminal recursive segments collapse and are removed",
			tokens: []domain.MatcherToken{literal("services"), separator(), literal("api"), separator(), recursive(), separator(), recursive()},
			want:   []domain.MatcherToken{literal("services"), separator(), literal("api")},
		},
		{
			name:   "sole recursive segment remains the all path matcher",
			tokens: []domain.MatcherToken{recursive()},
			want:   []domain.MatcherToken{recursive()},
		},
		{
			name:   "consecutive sole recursive segments collapse to one",
			tokens: []domain.MatcherToken{recursive(), separator(), recursive(), separator(), recursive()},
			want:   []domain.MatcherToken{recursive()},
		},
		{
			name:   "interior recursive segments collapse",
			tokens: []domain.MatcherToken{literal("services"), separator(), recursive(), separator(), recursive(), separator(), literal("api")},
			want:   []domain.MatcherToken{literal("services"), separator(), recursive(), separator(), literal("api")},
		},
		{
			name:   "leading recursive segment is retained",
			tokens: []domain.MatcherToken{recursive(), separator(), literal("api"), separator(), recursive()},
			want:   []domain.MatcherToken{recursive(), separator(), literal("api")},
		},
		{
			name:   "wildcards are retained",
			tokens: []domain.MatcherToken{literal("services"), separator(), wildcard()},
			want:   []domain.MatcherToken{literal("services"), separator(), wildcard()},
		},
		{
			name:   "wildcard and literal share a segment",
			tokens: []domain.MatcherToken{literal("src"), separator(), wildcard(), literal(".go")},
			want:   []domain.MatcherToken{literal("src"), separator(), wildcard(), literal(".go")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := mustMatcher(t, tt.tokens...)
			if got := matcher.CanonicalTokens(); !slices.Equal(got, tt.want) {
				t.Fatalf("CanonicalTokens() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNewPathMatcherRetainsRequestedTokens guards the RG-002 rule that the
// requested spelling survives canonicalization for presentation.
func TestNewPathMatcherRetainsRequestedTokens(t *testing.T) {
	matcher := mustMatcher(t, literal("services"), separator(), literal("api"), separator(), recursive())

	want := []domain.MatcherToken{literal("services"), separator(), literal("api"), separator(), recursive()}
	if got := matcher.Tokens(); !slices.Equal(got, want) {
		t.Fatalf("Tokens() = %v, want %v", got, want)
	}
	if got, want := matcher.String(), "services/api/**"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestPathMatcherTokensReturnCopies(t *testing.T) {
	matcher := mustMatcher(t, literal("services"), separator(), literal("api"))

	matcher.Tokens()[0] = literal("replaced")
	matcher.CanonicalTokens()[0] = literal("replaced")

	if got := matcher.Tokens()[0]; got != literal("services") {
		t.Errorf("Tokens()[0] = %v after mutating the returned slice, want %v", got, literal("services"))
	}
	if got := matcher.CanonicalTokens()[0]; got != literal("services") {
		t.Errorf("CanonicalTokens()[0] = %v after mutating the returned slice, want %v", got, literal("services"))
	}
}

// TestPathMatcherIdentityIsCanonicalAndCaseSensitive pins that identity follows
// canonical tokens only and that no case folding occurs.
func TestPathMatcherIdentityIsCanonicalAndCaseSensitive(t *testing.T) {
	base := mustMatcher(t, literal("services"), separator(), literal("api"))

	equal := []struct {
		name   string
		tokens []domain.MatcherToken
	}{
		{name: "terminal recursive", tokens: []domain.MatcherToken{literal("services"), separator(), literal("api"), separator(), recursive()}},
		{name: "repeated terminal recursive", tokens: []domain.MatcherToken{literal("services"), separator(), literal("api"), separator(), recursive(), separator(), recursive()}},
		{name: "split literals", tokens: []domain.MatcherToken{literal("serv"), literal("ices"), separator(), literal("api")}},
	}
	for _, tt := range equal {
		t.Run("equal/"+tt.name, func(t *testing.T) {
			if got := mustMatcher(t, tt.tokens...).Identity(); got != base.Identity() {
				t.Errorf("Identity() = %q, want %q", got, base.Identity())
			}
		})
	}

	distinct := []struct {
		name   string
		tokens []domain.MatcherToken
	}{
		{name: "wildcard segment", tokens: []domain.MatcherToken{literal("services"), separator(), wildcard()}},
		{name: "different case", tokens: []domain.MatcherToken{literal("Services"), separator(), literal("api")}},
		{name: "all paths", tokens: []domain.MatcherToken{recursive()}},
		{name: "interior recursive", tokens: []domain.MatcherToken{literal("services"), separator(), recursive(), separator(), literal("api")}},
	}
	for _, tt := range distinct {
		t.Run("distinct/"+tt.name, func(t *testing.T) {
			if got := mustMatcher(t, tt.tokens...).Identity(); got == base.Identity() {
				t.Errorf("Identity() = %q, want a different identity than %q", got, base.Identity())
			}
		})
	}
}

func TestNewPathMatcherRejectsInvalidConstruction(t *testing.T) {
	tests := []struct {
		name   string
		tokens []domain.MatcherToken
	}{
		{name: "no tokens", tokens: nil},
		{name: "empty literal", tokens: []domain.MatcherToken{literal("")}},
		{name: "literal separator", tokens: []domain.MatcherToken{literal("services/api")}},
		{name: "literal NUL", tokens: []domain.MatcherToken{literal("api\x00")}},
		{name: "invalid UTF-8 literal", tokens: []domain.MatcherToken{literal("\xff")}},
		{name: "leading separator", tokens: []domain.MatcherToken{separator(), literal("api")}},
		{name: "trailing separator", tokens: []domain.MatcherToken{literal("api"), separator()}},
		{name: "repeated separator", tokens: []domain.MatcherToken{literal("services"), separator(), separator(), literal("api")}},
		{name: "dot segment", tokens: []domain.MatcherToken{literal("services"), separator(), literal("."), separator(), literal("api")}},
		{name: "dot dot segment", tokens: []domain.MatcherToken{literal("services"), separator(), literal(".."), separator(), literal("api")}},
		{name: "adjacent wildcards", tokens: []domain.MatcherToken{literal("services"), separator(), wildcard(), wildcard()}},
		{name: "recursive shares a segment", tokens: []domain.MatcherToken{literal("services"), recursive()}},
		{name: "unknown token kind", tokens: []domain.MatcherToken{{Kind: domain.MatcherTokenKind(42)}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewPathMatcher(tt.tokens)
			if err == nil {
				t.Fatal("NewPathMatcher returned no error, want error")
			}
			if !errors.Is(err, domain.ErrInvalidMatcher) {
				t.Errorf("error %v does not match ErrInvalidMatcher", err)
			}
		})
	}
}

func TestZeroPathMatcherIsNotUsable(t *testing.T) {
	var matcher domain.PathMatcher
	if !matcher.IsZero() {
		t.Fatal("IsZero() = false for the zero PathMatcher, want true")
	}
	if got := mustMatcher(t, recursive()); got.IsZero() {
		t.Error("IsZero() = true for a constructed PathMatcher, want false")
	}
}

// TestPathMatcherIdentityDistinguishesEscapedStarsFromWildcards guards the
// closed RG-002 rule that an escaped star is literal path text: it must not
// share identity with the single-segment wildcard.
func TestPathMatcherIdentityDistinguishesEscapedStarsFromWildcards(t *testing.T) {
	escaped := mustMatcher(t, literal("services"), separator(), literal("*"))
	wild := mustMatcher(t, literal("services"), separator(), wildcard())

	if escaped.Identity() == wild.Identity() {
		t.Errorf("Identity() = %q for both a literal star and a wildcard segment, want distinct identities", escaped.Identity())
	}
	if got, want := escaped.String(), "services/*"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
