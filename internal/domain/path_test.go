package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/fmueller/orgtop/internal/domain"
)

func mustChangedPath(t *testing.T, value string) domain.ChangedPath {
	t.Helper()
	path, err := domain.NewChangedPath(value)
	if err != nil {
		t.Fatalf("NewChangedPath(%q) returned error: %v", value, err)
	}
	return path
}

// TestNewChangedPathAcceptsStrictRepositoryRelativePaths pins the closed RG-002
// changed-path representation: valid UTF-8, non-empty slash separated segments,
// no cleanup, no case folding, and no separator conversion.
func TestNewChangedPathAcceptsStrictRepositoryRelativePaths(t *testing.T) {
	valid := []string{
		"README.md",
		".github/workflows/ci.yml",
		"services/api/main.go",
		"Services/web/main.go",
		`services\api.go`,
		"services/a*b.go",
		"services/..hidden/main.go",
		"services/…/main.go",
	}
	for _, value := range valid {
		t.Run(value, func(t *testing.T) {
			path, err := domain.NewChangedPath(value)
			if err != nil {
				t.Fatalf("NewChangedPath(%q) returned error: %v", value, err)
			}
			if path.String() != value {
				t.Errorf("NewChangedPath(%q).String() = %q, want the unmodified path", value, path.String())
			}
			if path.IsZero() {
				t.Errorf("NewChangedPath(%q).IsZero() = true, want false", value)
			}
		})
	}
}

// TestNewChangedPathRejectsMalformedPaths covers every malformed changed-path
// vector A-024 names. No host filesystem cleanup repairs any of them.
func TestNewChangedPathRejectsMalformedPaths(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "absolute", value: "/services/api/main.go"},
		{name: "trailing separator", value: "services/api/"},
		{name: "repeated separator", value: "services//api/main.go"},
		{name: "only separator", value: "/"},
		{name: "dot segment", value: "services/./main.go"},
		{name: "traversal segment", value: "services/../main.go"},
		{name: "leading traversal", value: "../services/main.go"},
		{name: "sole dot", value: "."},
		{name: "invalid UTF-8", value: "services/\xffapi.go"},
		{name: "NUL", value: "services/api\x00.go"},
		{name: "backslash path with a trailing separator", value: `services\api\main.go` + "/"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path, err := domain.NewChangedPath(test.value)
			if !errors.Is(err, domain.ErrInvalidPath) {
				t.Fatalf("NewChangedPath(%q) error = %v, want ErrInvalidPath", test.value, err)
			}
			if !path.IsZero() {
				t.Errorf("NewChangedPath(%q) returned a usable path for a rejected value", test.value)
			}
		})
	}
}

// TestZeroChangedPathNeverMatches guards that an unvalidated path cannot become
// evidence of membership.
func TestZeroChangedPathNeverMatches(t *testing.T) {
	matcher := mustMatcher(t, recursive())
	if matcher.Matches(domain.ChangedPath{}) {
		t.Error("the all-path matcher matched the zero changed path, want no match")
	}
}

// TestPathMatcherMatchesClosedVectors translates the normative RG-002 matching
// vectors, including A-021 directory semantics, case sensitivity, dotfiles,
// recursive segments, and literal star and colon tokens.
func TestPathMatcherMatchesClosedVectors(t *testing.T) {
	tests := []struct {
		name    string
		tokens  []domain.MatcherToken
		matches []string
		misses  []string
	}{
		{
			name:   "sole wildcard matches every root entry and descendant",
			tokens: []domain.MatcherToken{wildcard()},
			matches: []string{
				".github/workflows/ci.yml",
				"services/api/main.go",
				"Services/web/main.go",
				"README.md",
			},
		},
		{
			name:   "literal component matches the path and its descendants",
			tokens: []domain.MatcherToken{literal("services"), separator(), literal("api")},
			matches: []string{
				"services/api",
				"services/api/main.go",
				"services/api/internal/deep/file.go",
			},
			misses: []string{
				"services",
				"service/payments/main.go",
				"services-payments/main.go",
				"services/apix/main.go",
				"Services/api/main.go",
			},
		},
		{
			name:   "single wildcard segment matches immediate children and below",
			tokens: []domain.MatcherToken{literal("services"), separator(), wildcard()},
			matches: []string{
				"services/api/main.go",
				"services/region/api/main.go",
				"services/.hidden/main.go",
			},
			misses: []string{
				"services",
				"Services/web/main.go",
				".github/workflows/ci.yml",
			},
		},
		{
			name:    "case sensitive wildcard segment",
			tokens:  []domain.MatcherToken{literal("Services"), separator(), wildcard()},
			matches: []string{"Services/web/main.go"},
			misses:  []string{"services/api/main.go"},
		},
		{
			name:   "recursive segment matches zero or more segments",
			tokens: []domain.MatcherToken{literal("services"), separator(), recursive(), separator(), literal("api")},
			matches: []string{
				"services/api",
				"services/api/main.go",
				"services/region/api/main.go",
				"services/a/b/c/api/main.go",
			},
			misses: []string{
				"services/region/apis/main.go",
				"api/main.go",
			},
		},
		{
			name:   "recursive matcher matches every valid path",
			tokens: []domain.MatcherToken{recursive()},
			matches: []string{
				"README.md",
				".github/workflows/ci.yml",
				"services/api/main.go",
			},
		},
		{
			name:   "wildcard does not cross a separator",
			tokens: []domain.MatcherToken{literal("services"), separator(), wildcard(), separator(), literal("main.go")},
			matches: []string{
				"services/api/main.go",
				"services/api/main.go/generated",
			},
			misses: []string{
				"services/api/internal/main.go",
				"services/main.go",
			},
		},
		{
			name:   "wildcard matches zero code points within a segment",
			tokens: []domain.MatcherToken{literal("services"), separator(), literal("api"), wildcard()},
			matches: []string{
				"services/api",
				"services/apix/main.go",
			},
			misses: []string{"services/ap"},
		},
		{
			name:   "wildcards surround literal text within one segment",
			tokens: []domain.MatcherToken{wildcard(), literal("api"), wildcard()},
			matches: []string{
				"my-api-service/main.go",
				"api",
			},
			misses: []string{"services/api/main.go"},
		},
		{
			name:   "escaped star is literal path text",
			tokens: []domain.MatcherToken{literal("services"), separator(), literal("*")},
			matches: []string{
				"services/*",
				"services/*/main.go",
			},
			misses: []string{"services/api/main.go"},
		},
		{
			name:    "escaped colon is literal path text",
			tokens:  []domain.MatcherToken{literal("docs:api")},
			matches: []string{"docs:api/index.md"},
			misses:  []string{"docs/api/index.md"},
		},
		{
			name:    "bang is ordinary literal text and never negates",
			tokens:  []domain.MatcherToken{literal("!src"), separator(), recursive()},
			matches: []string{"!src/main.go"},
			misses:  []string{"src/main.go"},
		},
		{
			name:   "terminal recursive segment keeps descendant semantics",
			tokens: []domain.MatcherToken{literal("services"), separator(), literal("api"), separator(), recursive()},
			matches: []string{
				"services/api",
				"services/api/main.go",
			},
			misses: []string{"services/apix/main.go"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matcher := mustMatcher(t, test.tokens...)
			for _, value := range test.matches {
				if !matcher.Matches(mustChangedPath(t, value)) {
					t.Errorf("matcher %q did not match %q, want a match", matcher, value)
				}
			}
			for _, value := range test.misses {
				if matcher.Matches(mustChangedPath(t, value)) {
					t.Errorf("matcher %q matched %q, want no match", matcher, value)
				}
			}
		})
	}
}

// TestPathMatcherMatchingIsIndependentOfRequestedSpelling guards that
// canonicalization agrees with matching: identical canonical identities decide
// membership identically.
func TestPathMatcherMatchingIsIndependentOfRequestedSpelling(t *testing.T) {
	spellings := [][]domain.MatcherToken{
		{literal("services"), separator(), literal("api")},
		{literal("services"), separator(), literal("api"), separator(), recursive()},
		{literal("services"), separator(), literal("api"), separator(), recursive(), separator(), recursive()},
		{literal("service"), literal("s"), separator(), literal("a"), literal("pi")},
	}
	paths := []string{"services/api", "services/api/main.go", "services/apix/main.go", "services"}

	want := mustMatcher(t, spellings[0]...)
	for _, tokens := range spellings[1:] {
		matcher := mustMatcher(t, tokens...)
		if matcher.Identity() != want.Identity() {
			t.Fatalf("matcher %q identity = %q, want %q", matcher, matcher.Identity(), want.Identity())
		}
		for _, value := range paths {
			path := mustChangedPath(t, value)
			if got := matcher.Matches(path); got != want.Matches(path) {
				t.Errorf("matcher %q matched %q = %t, want %t", matcher, value, got, want.Matches(path))
			}
		}
	}
}

// TestPathMatcherMatchesAnyCoversRenamePaths pins the rename rule: a rename
// contributes both normalized names and matching either or both still reports one
// membership for the Scope.
func TestPathMatcherMatchesAnyCoversRenamePaths(t *testing.T) {
	oldPath := mustChangedPath(t, "services/old/item.go")
	newPath := mustChangedPath(t, "services/new/item.go")

	tests := []struct {
		name   string
		tokens []domain.MatcherToken
		want   bool
	}{
		{
			name:   "old name only",
			tokens: []domain.MatcherToken{literal("services"), separator(), literal("old")},
			want:   true,
		},
		{
			name:   "new name only",
			tokens: []domain.MatcherToken{literal("services"), separator(), literal("new")},
			want:   true,
		},
		{
			name:   "both names",
			tokens: []domain.MatcherToken{literal("services"), separator(), wildcard()},
			want:   true,
		},
		{
			name:   "neither name",
			tokens: []domain.MatcherToken{literal("docs"), separator(), recursive()},
			want:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matcher := mustMatcher(t, test.tokens...)
			if got := matcher.MatchesAny(oldPath, newPath); got != test.want {
				t.Errorf("MatchesAny(old, new) = %t, want %t", got, test.want)
			}
		})
	}

	if mustMatcher(t, literal("docs")).MatchesAny() {
		t.Error("MatchesAny() with no paths reported a match, want no match")
	}
}

// TestZeroPathMatcherNeverMatches guards that a repository Scope's zero matcher
// cannot silently behave like the all-path matcher.
func TestZeroPathMatcherNeverMatches(t *testing.T) {
	var matcher domain.PathMatcher
	if matcher.Matches(mustChangedPath(t, "services/api/main.go")) {
		t.Error("the zero matcher matched a path, want no match")
	}
}

// TestPathMatcherMatchesLongPathsWithoutBlowup keeps the recursive-segment
// evaluation bounded for adversarially repetitive input.
func TestPathMatcherMatchesLongPathsWithoutBlowup(t *testing.T) {
	var tokens []domain.MatcherToken
	for range 12 {
		tokens = append(tokens, recursive(), separator(), literal("a"), separator())
	}
	tokens = append(tokens, literal("zzz"))

	matcher := mustMatcher(t, tokens...)
	path := mustChangedPath(t, strings.TrimSuffix(strings.Repeat("a/", 60), "/"))
	if matcher.Matches(path) {
		t.Error("matcher matched a path without the terminal literal segment, want no match")
	}
}
