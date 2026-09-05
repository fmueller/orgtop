package cli_test

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/fmueller/orgtop/internal/cli"
	"github.com/fmueller/orgtop/internal/domain"
)

// selectorNames returns the retained requested spelling of every organization
// selector in its shared first-occurrence order.
// selectorArgs requests count distinct organization selectors, the selection the
// RG-010 selector bound is measured against.
func selectorArgs(count int) []string {
	args := make([]string, 0, 2*count)
	for i := range count {
		args = append(args, "--org", fmt.Sprintf("acme%02d", i))
	}
	return args
}

func selectorNames(config cli.Config) []string {
	out := make([]string, 0, len(config.Organizations))
	for _, selector := range config.Organizations {
		out = append(out, selector.Name())
	}
	return out
}

// TestParseArgsCollectsOrganizationSelectors covers the closed RG-010 grammar:
// both spellings share one first-occurrence sequence in argument order and
// deduplicate case-insensitively by organization, retaining the first spelling
// and form (A-061).
func TestParseArgsCollectsOrganizationSelectors(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		want      []string
		wantAlias []bool
	}{
		{
			name:      "explicit flag",
			args:      []string{"--org", "Acme"},
			want:      []string{"Acme"},
			wantAlias: []bool{false},
		},
		{
			name:      "equals form",
			args:      []string{"--org=Acme"},
			want:      []string{"Acme"},
			wantAlias: []bool{false},
		},
		{
			name:      "repository alias",
			args:      []string{"--repo", "acme/*"},
			want:      []string{"acme"},
			wantAlias: []bool{true},
		},
		{
			name:      "alias equals form",
			args:      []string{"--repo=acme/*"},
			want:      []string{"acme"},
			wantAlias: []bool{true},
		},
		{
			name:      "repetition across both spellings collapses to the first",
			args:      []string{"--org", "Acme", "--repo", "acme/*", "--org", "ACME"},
			want:      []string{"Acme"},
			wantAlias: []bool{false},
		},
		{
			name:      "alias first retains the alias",
			args:      []string{"--repo=Acme/*", "--org", "acme"},
			want:      []string{"Acme"},
			wantAlias: []bool{true},
		},
		{
			name:      "interleaved case variants keep one shared order",
			args:      []string{"--org", "alpha", "--repo", "Beta/*", "--org", "ALPHA", "--repo", "beta/*"},
			want:      []string{"alpha", "Beta"},
			wantAlias: []bool{false, true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := parseValid(t, tt.args)
			if got := selectorNames(config); !slices.Equal(got, tt.want) {
				t.Fatalf("organization selectors = %v, want %v", got, tt.want)
			}
			for i, selector := range config.Organizations {
				if selector.Alias() != tt.wantAlias[i] {
					t.Errorf("selector %q alias = %v, want %v", selector.Name(), selector.Alias(), tt.wantAlias[i])
				}
				if got, want := selector.Key(), strings.ToLower(tt.want[i]); got != want {
					t.Errorf("selector key = %q, want %q", got, want)
				}
			}
		})
	}
}

// TestOrganizationOnlySelectionCreatesNoScopes keeps an organization selector a
// CLI selection input: it passes validation, creates no Scope, and leaves the
// downstream first-expansion state to the adapter (RG-010, A-061).
func TestOrganizationOnlySelectionCreatesNoScopes(t *testing.T) {
	config := parseValid(t, []string{"--org", "acme"})
	if got := config.Scopes.Len(); got != 0 {
		t.Fatalf("scopes = %d, want 0", got)
	}
	if got := len(config.Organizations); got != 1 {
		t.Fatalf("organization selectors = %d, want 1", got)
	}
}

// TestOrganizationSelectorsComposeWithExactSelections keeps expansion input and
// exact repository/path Scopes independent (A-061).
func TestOrganizationSelectorsComposeWithExactSelections(t *testing.T) {
	config := parseValid(t, []string{"--org", "acme", "--repo", "acme/api", "--path", "acme/web:src/**"})
	if got, want := len(config.Organizations), 1; got != want {
		t.Fatalf("organization selectors = %d, want %d", got, want)
	}
	got := make([]string, 0, config.Scopes.Len())
	for _, scope := range config.Scopes.Scopes() {
		got = append(got, scope.String())
	}
	want := []string{"acme/api", "acme/web:src/**"}
	if !slices.Equal(got, want) {
		t.Fatalf("scopes = %v, want %v", got, want)
	}
}

// TestOrganizationAliasNeverSatisfiesABarePath keeps bare patterns applicable
// only to exact repository arguments (RG-001, A-061).
func TestOrganizationAliasNeverSatisfiesABarePath(t *testing.T) {
	for _, args := range [][]string{
		{"--repo", "acme/*", "--path", "src/**"},
		{"--org", "acme", "--path", "src/**"},
	} {
		var output bytes.Buffer
		_, err := cli.ParseArgs("orgtop", args, &output)
		if !errors.Is(err, cli.ErrBarePathWithoutRepository) {
			t.Fatalf("ParseArgs(%v) error = %v, want %v", args, err, cli.ErrBarePathWithoutRepository)
		}
		assertUsage(t, output.String())
	}
}

// TestParseArgsRejectsInvalidOrganizationValues keeps the organization grammar
// the shipped repository-owner grammar (RG-010).
func TestParseArgsRejectsInvalidOrganizationValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "empty", args: []string{"--org", ""}},
		{name: "leading hyphen", args: []string{"--org", "-acme"}},
		{name: "trailing hyphen", args: []string{"--org", "acme-"}},
		{name: "unsupported character", args: []string{"--org", "acme.inc"}},
		{name: "slash", args: []string{"--org", "acme/api"}},
		{name: "too long", args: []string{"--org", strings.Repeat("a", 40)}},
		{name: "alias with a partial wildcard", args: []string{"--repo", "acme/ap*"}},
		{name: "alias with an empty organization", args: []string{"--repo", "/*"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			_, err := cli.ParseArgs("orgtop", tt.args, &output)
			if err == nil {
				t.Fatalf("ParseArgs(%v) accepted an invalid value", tt.args)
			}
			assertUsage(t, output.String())
		})
	}
}

// TestParseArgsEnforcesOrganizationSelectorCapacity keeps the RG-010 selector
// bound checked before credential, cache, network, or TUI work (A-061).
func TestParseArgsEnforcesOrganizationSelectorCapacity(t *testing.T) {
	t.Run("the maximum selector count is accepted", func(t *testing.T) {
		config := parseValid(t, selectorArgs(cli.MaxOrganizationSelectors))
		if got := len(config.Organizations); got != cli.MaxOrganizationSelectors {
			t.Errorf("organization selectors = %d, want %d", got, cli.MaxOrganizationSelectors)
		}
	})

	t.Run("a sixth selector is rejected with counts", func(t *testing.T) {
		var output bytes.Buffer
		_, err := cli.ParseArgs("orgtop", selectorArgs(cli.MaxOrganizationSelectors+1), &output)
		if !errors.Is(err, cli.ErrOrganizationCapacity) {
			t.Fatalf("error = %v, want %v", err, cli.ErrOrganizationCapacity)
		}
		if got := err.Error(); !strings.Contains(got, "6") || !strings.Contains(got, "5") {
			t.Errorf("error %q does not report the requested and allowed counts", got)
		}
		assertUsage(t, output.String())
	})

	t.Run("repository capacity outranks selector capacity", func(t *testing.T) {
		args := append(selectorArgs(cli.MaxOrganizationSelectors+1),
			repositoryArgs(domain.MaxSelectedRepositories+1)...)
		var output bytes.Buffer
		_, err := cli.ParseArgs("orgtop", args, &output)
		if !errors.Is(err, domain.ErrScopeCapacity) {
			t.Fatalf("error = %v, want %v", err, domain.ErrScopeCapacity)
		}
	})
}

// TestParseArgsParsesInclusionFlags keeps the process-global inclusion booleans
// selector-only and repetition-tolerant (RG-010).
func TestParseArgsParsesInclusionFlags(t *testing.T) {
	config := parseValid(t, []string{"--org", "acme", "--include-archived", "--include-forks", "--include-archived", "--no-cache"})
	if !config.IncludeArchived || !config.IncludeForks {
		t.Fatalf("inclusion flags = (%v, %v), want (true, true)", config.IncludeArchived, config.IncludeForks)
	}
	if !config.NoCache {
		t.Error("--no-cache composed with an organization selector was not retained")
	}
}

// TestInclusionFlagsRequireASelector keeps selector-only flag misuse its own
// rejection class, ahead of the missing-selection class (RG-001, A-061).
func TestInclusionFlagsRequireASelector(t *testing.T) {
	for _, args := range [][]string{
		{"--include-archived"},
		{"--include-forks"},
		{"--repo", "acme/api", "--include-archived"},
		{"--repo", "acme/api", "--include-forks"},
	} {
		var output bytes.Buffer
		_, err := cli.ParseArgs("orgtop", args, &output)
		if !errors.Is(err, cli.ErrInclusionWithoutSelector) {
			t.Fatalf("ParseArgs(%v) error = %v, want %v", args, err, cli.ErrInclusionWithoutSelector)
		}
		assertUsage(t, output.String())
	}
}

// TestResetCacheRejectsOrganizationInput keeps the administrative action
// standalone against every RG-010 flag (RG-001).
func TestResetCacheRejectsOrganizationInput(t *testing.T) {
	for _, args := range [][]string{
		{"--reset-cache", "--org", "acme"},
		{"--reset-cache", "--repo", "acme/*"},
		{"--reset-cache", "--include-archived"},
		{"--reset-cache", "--include-forks"},
	} {
		var output bytes.Buffer
		_, err := cli.ParseArgs("orgtop", args, &output)
		if !errors.Is(err, cli.ErrResetCacheNotStandalone) {
			t.Fatalf("ParseArgs(%v) error = %v, want %v", args, err, cli.ErrResetCacheNotStandalone)
		}
		assertUsage(t, output.String())
	}
}

// TestHelpAndVersionOutrankOrganizationSelection keeps RG-001 precedence: no
// expansion input is validated and nothing is rejected (A-061).
func TestHelpAndVersionOutrankOrganizationSelection(t *testing.T) {
	var version bytes.Buffer
	if _, err := cli.ParseArgs("orgtop", []string{"--org", "-invalid", "--version"}, &version); !errors.Is(err, cli.ErrVersionRequested) {
		t.Fatalf("error = %v, want %v", err, cli.ErrVersionRequested)
	}
	if version.Len() != 0 {
		t.Errorf("a version request wrote %q, want none", version.String())
	}

	var help bytes.Buffer
	if _, err := cli.ParseArgs("orgtop", []string{"--include-archived", "--help"}, &help); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("error = %v, want %v", err, flag.ErrHelp)
	}
}

// TestUsageDocumentsTheOrganizationForms keeps usage documenting every RG-010
// flag and form the parser accepts (RG-001).
func TestUsageDocumentsTheOrganizationForms(t *testing.T) {
	var output bytes.Buffer
	if _, err := cli.ParseArgs("orgtop", nil, &output); err == nil {
		t.Fatal("parsing an empty argument list must be rejected")
	}
	for _, want := range []string{"--org", "ORGANIZATION", "'ORGANIZATION/*'", "--include-archived", "--include-forks"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("usage does not document %q:\n%s", want, output.String())
		}
	}
}

// TestBarePatternsFilterOnlyExactRepositoriesBesideASelector keeps an
// organization selector out of bare-pattern filtering: the Cartesian product
// covers the exact repositories alone and expansion still emits no Scope
// (RG-001, A-061).
func TestBarePatternsFilterOnlyExactRepositoriesBesideASelector(t *testing.T) {
	config := parseValid(t, []string{"--org", "acme", "--repo", "other/api", "--path", "src/**"})
	got := make([]string, 0, config.Scopes.Len())
	for _, scope := range config.Scopes.Scopes() {
		got = append(got, scope.String())
	}
	if want := []string{"other/api:src/**"}; !slices.Equal(got, want) {
		t.Fatalf("scopes = %v, want %v", got, want)
	}
	if want := []string{"acme"}; !slices.Equal(selectorNames(config), want) {
		t.Errorf("organization selectors = %v, want %v", selectorNames(config), want)
	}
}

// TestMixedSelectorSpellingsShareOneCapacityBudget keeps the RG-010 selector
// bound counting distinct organizations across both spellings rather than per
// flag (A-061).
func TestMixedSelectorSpellingsShareOneCapacityBudget(t *testing.T) {
	mixed := func(count int) []string {
		args := make([]string, 0, 2*count)
		for i := range count {
			if i%2 == 0 {
				args = append(args, "--org", fmt.Sprintf("acme%02d", i))
				continue
			}
			args = append(args, "--repo", fmt.Sprintf("acme%02d/*", i))
		}
		return args
	}

	config := parseValid(t, mixed(cli.MaxOrganizationSelectors))
	if got := len(config.Organizations); got != cli.MaxOrganizationSelectors {
		t.Fatalf("organization selectors = %d, want %d", got, cli.MaxOrganizationSelectors)
	}

	var output bytes.Buffer
	_, err := cli.ParseArgs("orgtop", mixed(cli.MaxOrganizationSelectors+1), &output)
	if !errors.Is(err, cli.ErrOrganizationCapacity) {
		t.Fatalf("error = %v, want %v", err, cli.ErrOrganizationCapacity)
	}
	assertUsage(t, output.String())
}
