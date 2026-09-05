package cli_test

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/fmueller/orgtop/internal/cli"
	"github.com/fmueller/orgtop/internal/domain"
)

// TestScopeCapacityOutranksSelectorCapacity keeps the closed RG-001 post-parse
// order at the capacity classes: the expanded repository/Scope count is decided,
// and reported, before the organization-selector bound of RG-010 is consulted.
// Every vector requests both an over-capacity expansion and an over-capacity
// selector list, so the reported class is the Scope one and the reported count is
// the number of Scopes the invocation actually requests (RG-009).
func TestScopeCapacityOutranksSelectorCapacity(t *testing.T) {
	barePatterns := 1 + domain.MaxScopes/domain.MaxSelectedRepositories

	tests := []struct {
		name string
		args []string
		want int
	}{
		{
			name: "bare patterns filter every exact repository",
			args: append(repositoryArgs(domain.MaxSelectedRepositories), patternArgs(barePatterns)...),
			want: domain.MaxSelectedRepositories * barePatterns,
		},
		{
			name: "qualified paths add to the repository scopes they stand beside",
			args: func() []string {
				args := repositoryArgs(domain.MaxSelectedRepositories)
				for i := range domain.MaxScopes - domain.MaxSelectedRepositories + 1 {
					args = append(args, "--path", fmt.Sprintf("acme/repo00:src%03d/**", i))
				}
				return args
			}(),
			want: domain.MaxScopes + 1,
		},
		{
			name: "a qualified path the bare product does not cover counts once more",
			args: append(append(repositoryArgs(domain.MaxSelectedRepositories),
				patternArgs(domain.MaxScopes/domain.MaxSelectedRepositories)...),
				"--path", "acme/repo00:docs/**"),
			want: domain.MaxScopes + 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append(slices.Clone(tt.args), selectorArgs(cli.MaxOrganizationSelectors+1)...)
			var output bytes.Buffer
			_, err := cli.ParseArgs("orgtop", args, &output)
			if !errors.Is(err, domain.ErrScopeCapacity) {
				t.Fatalf("error = %v, want %v", err, domain.ErrScopeCapacity)
			}
			if errors.Is(err, cli.ErrOrganizationCapacity) {
				t.Errorf("error = %q, want the scope capacity reported first", err)
			}
			want := fmt.Sprintf("%d scopes requested", tt.want)
			if got := err.Error(); !strings.Contains(got, want) {
				t.Errorf("error = %q, want it to report %q", got, want)
			}
			assertUsage(t, output.String())
		})
	}
}

// TestParseArgsRejectsALeadingNulByte keeps the RG-002 whole-value NUL
// precondition covering the first byte of a pattern. A NUL is never literal
// text, and the first byte is the one position an offset check can accept by
// mistake; TestParseArgsRejectsInvalidPathValues covers a NUL further in.
func TestParseArgsRejectsALeadingNulByte(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "leading nul byte",
			value: "\x00src",
			want:  "--path: invalid path pattern \"\\x00src\" at byte 0: value contains NUL",
		},
		{
			name:  "sole nul byte",
			value: "\x00",
			want:  "--path: invalid path pattern \"\\x00\" at byte 0: value contains NUL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			args := []string{"--repo", "acme/api", "--path", tt.value}
			_, err := cli.ParseArgs("orgtop", args, &output)
			if !errors.Is(err, domain.ErrInvalidMatcher) {
				t.Fatalf("ParseArgs(--path %q) error = %v, want %v", tt.value, err, domain.ErrInvalidMatcher)
			}
			if got := err.Error(); got != tt.want {
				t.Errorf("ParseArgs(--path %q) error = %q, want %q", tt.value, got, tt.want)
			}
			assertUsage(t, output.String())
		})
	}
}

// TestParseArgsAcceptsRecursiveSegmentsInAnyPosition keeps the RG-002 rule that a
// segment of exactly "**" is recursive wherever it occurs, not only as the last
// segment of a pattern.
func TestParseArgsAcceptsRecursiveSegmentsInAnyPosition(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "leading recursive segment", value: "**/api", want: "acme/api:**/api"},
		{name: "interior recursive segment", value: "src/**/api", want: "acme/api:src/**/api"},
		{name: "recursive segment before a wildcard", value: "**/*", want: "acme/api:**/*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := parseValid(t, []string{"--repo", "acme/api", "--path", tt.value})
			if got := scopeStrings(config.Scopes); !slices.Equal(got, []string{tt.want}) {
				t.Fatalf("scopes = %v, want [%s]", got, tt.want)
			}
		})
	}
}

// TestParseArgsAcceptsWildcardsBesideLiteralText keeps a segment that mixes the
// within-segment wildcard with literal text valid under RG-002: such a segment is
// neither empty nor a recursive one, whatever order its parts arrive in.
func TestParseArgsAcceptsWildcardsBesideLiteralText(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "literal text before a wildcard", value: "src/a*", want: "acme/api:src/a*"},
		{name: "wildcard before literal text", value: "src/*a", want: "acme/api:src/*a"},
		{name: "a wildcard as the whole segment", value: "*/a", want: "acme/api:*/a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := parseValid(t, []string{"--repo", "acme/api", "--path", tt.value})
			if got := scopeStrings(config.Scopes); !slices.Equal(got, []string{tt.want}) {
				t.Fatalf("scopes = %v, want [%s]", got, tt.want)
			}
			if got := scopeKinds(config.Scopes); !slices.Equal(got, []domain.ScopeKind{domain.ScopePath}) {
				t.Errorf("scope kinds = %v, want [%v]", got, domain.ScopePath)
			}
		})
	}
}
