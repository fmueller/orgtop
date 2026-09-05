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

// scopeStrings renders the selected Scopes in request order using the retained
// requested spelling, which is what the CLI expansion vectors are written in.
func scopeStrings(scopes domain.ScopeSet) []string {
	selected := scopes.Scopes()
	out := make([]string, 0, len(selected))
	for _, scope := range selected {
		out = append(out, scope.String())
	}
	return out
}

func scopeKinds(scopes domain.ScopeSet) []domain.ScopeKind {
	selected := scopes.Scopes()
	out := make([]domain.ScopeKind, 0, len(selected))
	for _, scope := range selected {
		out = append(out, scope.Kind())
	}
	return out
}

// repositoryArgs requests count distinct exact repositories, the selection the
// RG-009 capacity vectors are built on.
func repositoryArgs(count int) []string {
	args := make([]string, 0, 2*count)
	for i := range count {
		args = append(args, "--repo", fmt.Sprintf("acme/repo%02d", i))
	}
	return args
}

// patternArgs requests count bare path patterns, each of which filters every
// selected repository.
func patternArgs(count int) []string {
	args := make([]string, 0, 2*count)
	for i := range count {
		args = append(args, "--path", fmt.Sprintf("src%02d/**", i))
	}
	return args
}

func parseValid(t *testing.T, args []string) cli.Config {
	t.Helper()
	var output bytes.Buffer
	config, err := cli.ParseArgs("orgtop", args, &output)
	if err != nil {
		t.Fatalf("ParseArgs(%v) returned error: %v", args, err)
	}
	if output.Len() != 0 {
		t.Errorf("valid configuration wrote output %q, want none", output.String())
	}
	return config
}

// TestParseArgsExpandsUnifiedScopes covers the closed RG-001 composition
// vectors: bare filtering (A-017), qualified and mixed paths (A-018), accepted
// flag forms, escaping, and repository-only compatibility (A-016).
func TestParseArgsExpandsUnifiedScopes(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		want  []string
		kinds []domain.ScopeKind
	}{
		{
			name:  "repository only selection keeps v0.1 meaning",
			args:  []string{"--repo", "acme/api", "--repo", "acme/web"},
			want:  []string{"acme/api", "acme/web"},
			kinds: []domain.ScopeKind{domain.ScopeRepository, domain.ScopeRepository},
		},
		{
			name: "bare paths filter every repository in cartesian request order",
			args: []string{"--repo", "acme/api", "--repo", "acme/web", "--path", "src/**", "--path", "docs/**"},
			want: []string{
				"acme/api:src/**",
				"acme/api:docs/**",
				"acme/web:src/**",
				"acme/web:docs/**",
			},
			kinds: []domain.ScopeKind{domain.ScopePath, domain.ScopePath, domain.ScopePath, domain.ScopePath},
		},
		{
			name:  "a qualified path needs no repository flag",
			args:  []string{"--path", "acme/api:src/**"},
			want:  []string{"acme/api:src/**"},
			kinds: []domain.ScopeKind{domain.ScopePath},
		},
		{
			name:  "a qualified path composes with its repository scope",
			args:  []string{"--repo", "acme/api", "--path", "acme/api:src/**"},
			want:  []string{"acme/api", "acme/api:src/**"},
			kinds: []domain.ScopeKind{domain.ScopeRepository, domain.ScopePath},
		},
		{
			name: "bare and qualified paths mix",
			args: []string{"--repo", "acme/api", "--path", "src/**", "--path", "acme/web:docs/**"},
			want: []string{
				"acme/api:src/**",
				"acme/web:docs/**",
			},
			kinds: []domain.ScopeKind{domain.ScopePath, domain.ScopePath},
		},
		{
			name:  "equals and single dash forms are accepted",
			args:  []string{"-repo=acme/api", "--path=src/**"},
			want:  []string{"acme/api:src/**"},
			kinds: []domain.ScopeKind{domain.ScopePath},
		},
		{
			name:  "an escaped colon is literal bare pattern text and needs no re-escaping",
			args:  []string{"--repo", "acme/api", "--path", `docs\:api`},
			want:  []string{"acme/api:docs:api"},
			kinds: []domain.ScopeKind{domain.ScopePath},
		},
		{
			name:  "a leading bang is inclusive literal text",
			args:  []string{"--repo", "acme/api", "--path", "!src/**"},
			want:  []string{"acme/api:!src/**"},
			kinds: []domain.ScopeKind{domain.ScopePath},
		},
		{
			name:  "a sole recursive segment stays a path scope",
			args:  []string{"--repo", "acme/api", "--path", "**"},
			want:  []string{"acme/api:**"},
			kinds: []domain.ScopeKind{domain.ScopePath},
		},
		{
			name:  "equivalent scopes deduplicate to the first spelling",
			args:  []string{"--repo", "acme/api", "--path", "src/**", "--path", "Acme/API:src", "--path", "acme/api:src/**"},
			want:  []string{"acme/api:src/**"},
			kinds: []domain.ScopeKind{domain.ScopePath},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := parseValid(t, tt.args)
			if got := scopeStrings(config.Scopes); !slices.Equal(got, tt.want) {
				t.Fatalf("scopes = %v, want %v", got, tt.want)
			}
			if got := scopeKinds(config.Scopes); !slices.Equal(got, tt.kinds) {
				t.Errorf("scope kinds = %v, want %v", got, tt.kinds)
			}
		})
	}
}

// TestParseArgsParsesCacheControls covers the closed RG-001 cache controls of
// A-019: --no-cache is a launch flag beside a selection, --reset-cache is a
// standalone administrative action.
func TestParseArgsParsesCacheControls(t *testing.T) {
	t.Run("no cache defaults to off", func(t *testing.T) {
		config := parseValid(t, []string{"--repo", "acme/api"})
		if config.NoCache {
			t.Error("NoCache = true without --no-cache")
		}
		if config.ResetCache {
			t.Error("ResetCache = true without --reset-cache")
		}
	})

	t.Run("no cache composes with a selection", func(t *testing.T) {
		config := parseValid(t, []string{"--repo", "acme/api", "--no-cache"})
		if !config.NoCache {
			t.Error("NoCache = false with --no-cache")
		}
		if got := scopeStrings(config.Scopes); !slices.Equal(got, []string{"acme/api"}) {
			t.Errorf("scopes = %v, want [acme/api]", got)
		}
	})

	t.Run("reset cache is standalone and selects nothing", func(t *testing.T) {
		config := parseValid(t, []string{"--reset-cache"})
		if !config.ResetCache {
			t.Error("ResetCache = false with --reset-cache")
		}
		if config.Scopes.Len() != 0 {
			t.Errorf("reset selected %d scopes, want none", config.Scopes.Len())
		}
	})
}

// TestParseArgsRejectsIncompatibleCacheControls keeps --reset-cache standalone:
// combining it with any selection or cache flag is rejected before any side
// effect (A-019).
func TestParseArgsRejectsIncompatibleCacheControls(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "with a repository", args: []string{"--reset-cache", "--repo", "acme/api"}},
		{name: "with a path", args: []string{"--reset-cache", "--path", "acme/api:src/**"}},
		{name: "with no cache", args: []string{"--reset-cache", "--no-cache"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			_, err := cli.ParseArgs("orgtop", tt.args, &output)
			if !errors.Is(err, cli.ErrResetCacheNotStandalone) {
				t.Fatalf("ParseArgs(%v) error = %v, want %v", tt.args, err, cli.ErrResetCacheNotStandalone)
			}
			assertUsage(t, output.String())
		})
	}
}

// TestParseArgsRejectsABarePathWithoutARepository keeps a bare pattern from
// implying a repository or an organization (RG-001, A-020).
func TestParseArgsRejectsABarePathWithoutARepository(t *testing.T) {
	var output bytes.Buffer
	_, err := cli.ParseArgs("orgtop", []string{"--path", "src/**"}, &output)
	if !errors.Is(err, cli.ErrBarePathWithoutRepository) {
		t.Fatalf("error = %v, want %v", err, cli.ErrBarePathWithoutRepository)
	}
	assertUsage(t, output.String())
}

// TestParseArgsRejectsInvalidPathValues covers the RG-002 pattern diagnostics
// the CLI owns: qualified-prefix validation precedes tokenization, and
// tokenization reports the first cause with its zero-based byte offset.
func TestParseArgsRejectsInvalidPathValues(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr error
		want    string
	}{
		{
			name:    "unescaped colon requires a valid repository prefix",
			value:   "docs:api",
			wantErr: domain.ErrInvalidRepository,
			want:    `--path: invalid repository identifier "docs": expected exactly one owner/repository separator`,
		},
		{
			name:    "qualified prefix is validated before the pattern",
			value:   "acme/*:src//x",
			wantErr: domain.ErrInvalidRepository,
			want:    `--path: invalid repository identifier "acme/*": repository contains an unsupported character "*"`,
		},
		{
			name:    "empty value",
			value:   "",
			wantErr: domain.ErrInvalidMatcher,
			want:    `--path: invalid path pattern "" at byte 0: empty pattern`,
		},
		{
			name:    "leading separator",
			value:   "/src",
			wantErr: domain.ErrInvalidMatcher,
			want:    `--path: invalid path pattern "/src" at byte 0: empty segment`,
		},
		{
			name:    "repeated separator",
			value:   "src//api",
			wantErr: domain.ErrInvalidMatcher,
			want:    `--path: invalid path pattern "src//api" at byte 4: empty segment`,
		},
		{
			name:    "trailing separator",
			value:   "src/",
			wantErr: domain.ErrInvalidMatcher,
			want:    `--path: invalid path pattern "src/" at byte 3: empty segment`,
		},
		{
			name:    "dot segment",
			value:   "src/./api",
			wantErr: domain.ErrInvalidMatcher,
			want:    `--path: invalid path pattern "src/./api" at byte 4: "." is not a path segment`,
		},
		{
			name:    "dot dot segment",
			value:   "../src",
			wantErr: domain.ErrInvalidMatcher,
			want:    `--path: invalid path pattern "../src" at byte 0: ".." is not a path segment`,
		},
		{
			name:    "recursive stars inside a larger segment",
			value:   "src/**api",
			wantErr: domain.ErrInvalidMatcher,
			want:    `--path: invalid path pattern "src/**api" at byte 4: "**" must be a complete segment`,
		},
		{
			name:    "three stars",
			value:   "src/***",
			wantErr: domain.ErrInvalidMatcher,
			want:    `--path: invalid path pattern "src/***" at byte 4: "**" must be a complete segment`,
		},
		{
			name:    "invalid escape",
			value:   `src/\q`,
			wantErr: domain.ErrInvalidMatcher,
			want:    `--path: invalid path pattern "src/\\q" at byte 4: invalid escape "\\q"`,
		},
		{
			name:    "trailing backslash",
			value:   `src/api\`,
			wantErr: domain.ErrInvalidMatcher,
			want:    `--path: invalid path pattern "src/api\\" at byte 7: trailing backslash`,
		},
		{
			name:    "second unescaped colon",
			value:   "acme/api:src:api",
			wantErr: domain.ErrInvalidMatcher,
			want:    `--path: invalid path pattern "src:api" at byte 3: more than one unescaped colon`,
		},
		{
			name:    "nul byte",
			value:   "src/\x00api",
			wantErr: domain.ErrInvalidMatcher,
			want:    "--path: invalid path pattern \"src/\\x00api\" at byte 4: value contains NUL",
		},
		{
			name:    "invalid utf-8 outranks a NUL that occurs first",
			value:   "\x00\xff",
			wantErr: domain.ErrInvalidMatcher,
			want:    "--path: invalid path pattern \"\\x00\\xff\" at byte 1: value is not valid UTF-8",
		},
		{
			name:    "invalid utf-8",
			value:   "src/\xff",
			wantErr: domain.ErrInvalidMatcher,
			want:    "--path: invalid path pattern \"src/\\xff\" at byte 4: value is not valid UTF-8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			args := []string{"--repo", "acme/api", "--path", tt.value}
			_, err := cli.ParseArgs("orgtop", args, &output)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ParseArgs(--path %q) error = %v, want %v", tt.value, err, tt.wantErr)
			}
			if got := err.Error(); got != tt.want {
				t.Errorf("ParseArgs(--path %q) error = %q, want %q", tt.value, got, tt.want)
			}
			assertUsage(t, output.String())
			if got := strings.Count(output.String(), err.Error()); got != 1 {
				t.Errorf("output reports the cause %d times, want once:\n%s", got, output.String())
			}
		})
	}
}

// TestParseArgsReportsTheFirstMalformedArgument keeps left-to-right argument
// scanning: the first malformed value wins regardless of which flag carries it
// (RG-001).
func TestParseArgsReportsTheFirstMalformedArgument(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "a malformed path precedes a malformed repository",
			args: []string{"--repo", "acme/api", "--path", "src//api", "--repo", "acme/ap*"},
			want: "invalid path pattern",
		},
		{
			name: "a malformed repository precedes a malformed path",
			args: []string{"--repo", "acme/ap*", "--path", "src//api"},
			want: "invalid repository identifier",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			_, err := cli.ParseArgs("orgtop", tt.args, &output)
			if err == nil {
				t.Fatalf("ParseArgs(%v) returned no error", tt.args)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to report %q first", err.Error(), tt.want)
			}
		})
	}
}

// TestParseArgsEnforcesSelectionCapacity keeps the closed RG-009 selection
// capacities deterministic and pre-startup: explicit intent is never truncated.
func TestParseArgsEnforcesSelectionCapacity(t *testing.T) {
	t.Run("the maximum product is accepted", func(t *testing.T) {
		args := append(repositoryArgs(domain.MaxSelectedRepositories), patternArgs(domain.MaxScopes/domain.MaxSelectedRepositories)...)
		config := parseValid(t, args)
		if got := config.Scopes.Len(); got != domain.MaxScopes {
			t.Errorf("scopes = %d, want %d", got, domain.MaxScopes)
		}
	})

	t.Run("too many repositories are rejected", func(t *testing.T) {
		var output bytes.Buffer
		args := repositoryArgs(domain.MaxSelectedRepositories + 1)
		_, err := cli.ParseArgs("orgtop", args, &output)
		if !errors.Is(err, domain.ErrScopeCapacity) {
			t.Fatalf("error = %v, want %v", err, domain.ErrScopeCapacity)
		}
		if got := err.Error(); !strings.Contains(got, "21") || !strings.Contains(got, "20") {
			t.Errorf("error %q does not report the requested and allowed counts", got)
		}
		assertUsage(t, output.String())
	})

	t.Run("too many expanded scopes are rejected", func(t *testing.T) {
		var output bytes.Buffer
		args := append(repositoryArgs(domain.MaxSelectedRepositories), patternArgs(6)...)
		_, err := cli.ParseArgs("orgtop", args, &output)
		if !errors.Is(err, domain.ErrScopeCapacity) {
			t.Fatalf("error = %v, want %v", err, domain.ErrScopeCapacity)
		}
		if got := err.Error(); !strings.Contains(got, "120") || !strings.Contains(got, "100") {
			t.Errorf("error %q does not report the requested and allowed counts", got)
		}
	})
}

// TestCapacityDiagnosticsReportTheExpandedCount keeps the RG-009 diagnostic
// honest when the pre-expansion guard is the check that fires: the reported
// count is the number of Scopes the invocation actually requests, not the
// number of patterns it repeats.
func TestCapacityDiagnosticsReportTheExpandedCount(t *testing.T) {
	args := []string{"--repo", "acme/api", "--repo", "acme/web"}
	for i := range domain.MaxScopes + 1 {
		args = append(args, "--path", fmt.Sprintf("src%03d/**", i))
	}

	var output bytes.Buffer
	_, err := cli.ParseArgs("orgtop", args, &output)
	if !errors.Is(err, domain.ErrScopeCapacity) {
		t.Fatalf("error = %v, want %v", err, domain.ErrScopeCapacity)
	}
	want := fmt.Sprintf("%d scopes requested", 2*(domain.MaxScopes+1))
	if got := err.Error(); !strings.Contains(got, want) {
		t.Errorf("error = %q, want it to report %q", got, want)
	}
}

// TestCapacityDiagnosticsCountQualifiedScopesOnce keeps the expanded count from
// double counting a qualified Scope the bare expansion already produces, so a
// selection at the capacity boundary is still accepted (RG-001, RG-009).
func TestCapacityDiagnosticsCountQualifiedScopesOnce(t *testing.T) {
	args := append(repositoryArgs(domain.MaxSelectedRepositories),
		patternArgs(domain.MaxScopes/domain.MaxSelectedRepositories)...)
	args = append(args, "--path", "acme/repo00:src00/**")

	config := parseValid(t, args)
	if got := config.Scopes.Len(); got != domain.MaxScopes {
		t.Errorf("scopes = %d, want %d", got, domain.MaxScopes)
	}
}

// TestMalformedValuesOutrankIncompatibleCacheControls keeps the closed
// diagnostic order: argument scanning reports a malformed value before the
// post-parse administrative-incompatibility class (RG-001).
func TestMalformedValuesOutrankIncompatibleCacheControls(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr error
	}{
		{
			name:    "malformed repository",
			args:    []string{"--reset-cache", "--repo", "not-a-repository"},
			wantErr: domain.ErrInvalidRepository,
		},
		{
			name:    "malformed path",
			args:    []string{"--reset-cache", "--path", "acme/api:src//x"},
			wantErr: domain.ErrInvalidMatcher,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			_, err := cli.ParseArgs("orgtop", tt.args, &output)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ParseArgs(%v) error = %v, want %v", tt.args, err, tt.wantErr)
			}
			if errors.Is(err, cli.ErrResetCacheNotStandalone) {
				t.Errorf("error = %q, want the malformed value reported first", err)
			}
		})
	}
}

// TestHelpAndVersionOutrankCacheControls keeps the documented precedence: help
// and version answer before any administrative or selection decision (A-019).
func TestHelpAndVersionOutrankCacheControls(t *testing.T) {
	var output bytes.Buffer
	if _, err := cli.ParseArgs("orgtop", []string{"--reset-cache", "--version"}, &output); !errors.Is(err, cli.ErrVersionRequested) {
		t.Fatalf("error = %v, want %v", err, cli.ErrVersionRequested)
	}
	if output.Len() != 0 {
		t.Errorf("a version request wrote %q, want none", output.String())
	}
}

// TestUsageDocumentsTheUnifiedForms keeps usage documenting every flag and form
// the parser accepts (RG-001).
func TestUsageDocumentsTheUnifiedForms(t *testing.T) {
	var output bytes.Buffer
	if _, err := cli.ParseArgs("orgtop", nil, &output); err == nil {
		t.Fatal("parsing an empty argument list must be rejected")
	}
	for _, want := range []string{"--path", "PATTERN", "OWNER/REPOSITORY:PATTERN", "--no-cache", "--reset-cache"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("usage does not document %q:\n%s", want, output.String())
		}
	}
}
