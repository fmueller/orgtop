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

func repositoryStrings(scope domain.Scope) []string {
	repositories := scope.Repositories()
	out := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		out = append(out, repository.String())
	}
	return out
}

func TestParseArgsCollectsRepeatedRepositories(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "single long flag", args: []string{"--repo", "acme/backend"}, want: []string{"acme/backend"}},
		{name: "single short flag", args: []string{"-repo", "acme/backend"}, want: []string{"acme/backend"}},
		{name: "equals form", args: []string{"--repo=acme/backend"}, want: []string{"acme/backend"}},
		{
			name: "repeated flags keep request order",
			args: []string{"--repo", "acme/backend", "--repo", "acme/frontend"},
			want: []string{"acme/backend", "acme/frontend"},
		},
		{
			name: "case insensitive duplicates collapse to first spelling",
			args: []string{"--repo", "Acme/Backend", "--repo", "acme/backend"},
			want: []string{"Acme/Backend"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			config, err := cli.ParseArgs("orgtop", tt.args, &output)
			if err != nil {
				t.Fatalf("ParseArgs(%v) returned error: %v", tt.args, err)
			}
			got := repositoryStrings(config.Scope)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("Scope repositories = %v, want %v", got, tt.want)
			}
			if output.Len() != 0 {
				t.Errorf("valid configuration wrote output %q, want none", output.String())
			}
		})
	}
}

func TestParseArgsRequiresAtLeastOneRepository(t *testing.T) {
	var output bytes.Buffer
	_, err := cli.ParseArgs("orgtop", nil, &output)
	if !errors.Is(err, cli.ErrMissingRepository) {
		t.Fatalf("ParseArgs(nil) error = %v, want %v", err, cli.ErrMissingRepository)
	}
	if got := err.Error(); !strings.Contains(got, "--repo") {
		t.Errorf("error %q does not mention the --repo flag", got)
	}
	if got := err.Error(); strings.Contains(got, domain.ErrEmptyScope.Error()) {
		t.Errorf("error %q surfaces the raw domain sentinel text", got)
	}
	assertUsage(t, output.String())
}

func TestParseArgsRejectsInvalidRepositoryValues(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		reason string
	}{
		{name: "missing owner separator", value: "backend", reason: "expected exactly one owner/repository separator"},
		{name: "empty owner", value: "/backend", reason: "owner is empty"},
		{name: "empty repository", value: "acme/", reason: "repository is empty"},
		{name: "extra separator", value: "acme/team/backend", reason: "expected exactly one owner/repository separator"},
		{name: "glob repository", value: "acme/*", reason: `repository contains an unsupported character "*"`},
		{name: "glob owner", value: "*/backend", reason: `owner contains an unsupported character "*"`},
		{name: "exclusion pattern", value: "!acme/backend", reason: `owner contains an unsupported character "!"`},
		{name: "url form", value: "https://github.com/acme/backend", reason: "expected exactly one owner/repository separator"},
		{name: "empty value", value: "", reason: "expected exactly one owner/repository separator"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			_, err := cli.ParseArgs("orgtop", []string{"--repo", tt.value}, &output)
			if !errors.Is(err, domain.ErrInvalidRepository) {
				t.Fatalf("ParseArgs(--repo %q) error = %v, want %v", tt.value, err, domain.ErrInvalidRepository)
			}
			want := fmt.Sprintf("--repo: invalid repository identifier %q: %s", tt.value, tt.reason)
			if got := err.Error(); got != want {
				t.Errorf("ParseArgs(--repo %q) error = %q, want %q", tt.value, got, want)
			}
			assertUsage(t, output.String())
		})
	}
}

func TestParseArgsRejectsFirstInvalidValueOfSeveral(t *testing.T) {
	var output bytes.Buffer
	_, err := cli.ParseArgs("orgtop", []string{"--repo", "acme/backend", "--repo", "acme/*"}, &output)
	if !errors.Is(err, domain.ErrInvalidRepository) {
		t.Fatalf("ParseArgs error = %v, want %v", err, domain.ErrInvalidRepository)
	}
	if got := err.Error(); !strings.Contains(got, "acme/*") {
		t.Errorf("error %q does not name the rejected value", got)
	}
}

func TestParseArgsRejectsUnknownFlagsAndPositionalArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "unknown flag", args: []string{"--repos", "acme/backend"}},
		{name: "positional argument", args: []string{"--repo", "acme/backend", "extra"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if _, err := cli.ParseArgs("orgtop", tt.args, &output); err == nil {
				t.Fatalf("ParseArgs(%v) returned no error", tt.args)
			}
			assertUsage(t, output.String())
		})
	}
}

func TestParseArgsReportsHelpRequestsDistinctly(t *testing.T) {
	var output bytes.Buffer
	_, err := cli.ParseArgs("orgtop", []string{"--help"}, &output)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("ParseArgs(--help) error = %v, want %v", err, flag.ErrHelp)
	}
	assertUsage(t, output.String())
}

func assertUsage(t *testing.T, output string) {
	t.Helper()
	if !strings.Contains(output, "orgtop") {
		t.Errorf("usage output %q does not name the binary", output)
	}
	if !strings.Contains(output, "--repo") {
		t.Errorf("usage output %q does not document the --repo flag", output)
	}
	if !strings.Contains(output, "owner/repository") {
		t.Errorf("usage output %q does not document the owner/repository form", output)
	}
}

// TestParseArgsReportsItsOwnRejectionsExactlyOnce keeps reporting in one place.
// The flag package reports the causes it raises itself, so ParseArgs reports the
// causes it raises itself and the caller only decides the exit code.
func TestParseArgsReportsItsOwnRejectionsExactlyOnce(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "no repository", args: nil},
		{name: "invalid repository", args: []string{"--repo", "acme/*"}},
		{name: "positional argument", args: []string{"--repo", "acme/backend", "extra"}},
		{name: "unknown flag", args: []string{"--repos", "acme/backend"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			_, err := cli.ParseArgs("orgtop", tt.args, &output)
			if err == nil {
				t.Fatalf("ParseArgs(%v) returned no error", tt.args)
			}
			if got := strings.Count(output.String(), err.Error()); got != 1 {
				t.Errorf("output reports %q %d times, want once:\n%s", err.Error(), got, output.String())
			}
		})
	}
}
