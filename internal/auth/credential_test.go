package auth_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/auth"
)

// sentinelToken must never appear in output, errors, or test diagnostics that the
// product produces (FR-003, NFR-003).
const sentinelToken = "sentinel-token-must-not-leak"

type recordedCommand struct {
	name        string
	args        []string
	deadline    time.Time
	hasDeadline bool
}

type fakeRunner struct {
	calls  []recordedCommand
	stdout string
	err    error
}

func (f *fakeRunner) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	deadline, hasDeadline := ctx.Deadline()
	f.calls = append(f.calls, recordedCommand{
		name:        name,
		args:        slices.Clone(args),
		deadline:    deadline,
		hasDeadline: hasDeadline,
	})
	if ctx.Err() != nil {
		return nil, fmt.Errorf("gh: %w", ctx.Err())
	}
	return []byte(f.stdout), f.err
}

func resolverWith(values map[string]string, runner *fakeRunner) auth.Resolver {
	return auth.Resolver{
		LookupEnv: func(key string) string { return values[key] },
		Run:       runner.run,
	}
}

func TestResolvePrefersEnvironmentCredentialsWithoutRunningGh(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
	}{
		{name: "gh token", values: map[string]string{"GH_TOKEN": sentinelToken}},
		{
			name:   "gh token wins over github token",
			values: map[string]string{"GH_TOKEN": sentinelToken, "GITHUB_TOKEN": "other-token"},
		},
		{
			name:   "github token when gh token is unset",
			values: map[string]string{"GITHUB_TOKEN": sentinelToken},
		},
		{
			name:   "github token when gh token is blank",
			values: map[string]string{"GH_TOKEN": "   \t\n", "GITHUB_TOKEN": sentinelToken},
		},
		{
			name:   "surrounding whitespace is trimmed",
			values: map[string]string{"GH_TOKEN": "\n  " + sentinelToken + " \t\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{stdout: "unused-token"}
			credential, err := resolverWith(tt.values, runner).Resolve(t.Context())
			if err != nil {
				t.Fatalf("Resolve() returned error: %v", err)
			}
			if credential.Token() != sentinelToken {
				t.Errorf("Token() did not return the expected environment credential")
			}
			if len(runner.calls) != 0 {
				t.Errorf("gh was invoked %d times, want 0", len(runner.calls))
			}
		})
	}
}

func TestResolveFallsBackToGhAuthToken(t *testing.T) {
	runner := &fakeRunner{stdout: "  " + sentinelToken + "\n"}
	values := map[string]string{"GH_TOKEN": " ", "GITHUB_TOKEN": "\n"}

	credential, err := resolverWith(values, runner).Resolve(t.Context())
	if err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}
	if credential.Token() != sentinelToken {
		t.Error("Token() did not return the trimmed gh credential")
	}
	if len(runner.calls) != 1 {
		t.Fatalf("gh was invoked %d times, want 1", len(runner.calls))
	}
	call := runner.calls[0]
	if call.name != "gh" {
		t.Errorf("command = %q, want %q", call.name, "gh")
	}
	wantArgs := []string{"auth", "token", "--hostname", "github.com"}
	if !slices.Equal(call.args, wantArgs) {
		t.Errorf("args = %v, want %v", call.args, wantArgs)
	}
}

func TestResolveBoundsGhWithATenSecondTimeout(t *testing.T) {
	runner := &fakeRunner{stdout: sentinelToken}
	start := time.Now()

	if _, err := resolverWith(nil, runner).Resolve(t.Context()); err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}
	end := time.Now()

	if len(runner.calls) != 1 {
		t.Fatalf("gh was invoked %d times, want 1", len(runner.calls))
	}
	call := runner.calls[0]
	if !call.hasDeadline {
		t.Fatal("the gh context carried no deadline")
	}
	if upper := call.deadline.Sub(end); upper > 10*time.Second {
		t.Errorf("gh deadline budget = %v, want at most 10s", upper)
	}
	if lower := call.deadline.Sub(start); lower < 9*time.Second {
		t.Errorf("gh deadline budget = %v, want about 10s", lower)
	}
}

func TestResolveReportsMissingAuthenticationWithRemediation(t *testing.T) {
	tests := []struct {
		name   string
		runner *fakeRunner
	}{
		{name: "gh fails", runner: &fakeRunner{err: errors.New("exit status 1: gh: not logged in " + sentinelToken)}},
		{name: "gh returns nothing", runner: &fakeRunner{stdout: ""}},
		{name: "gh returns whitespace", runner: &fakeRunner{stdout: " \n\t "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credential, err := resolverWith(nil, tt.runner).Resolve(t.Context())
			if !errors.Is(err, auth.ErrNoCredential) {
				t.Fatalf("Resolve() error = %v, want %v", err, auth.ErrNoCredential)
			}
			if credential.Token() != "" {
				t.Error("Resolve() returned a credential together with an error")
			}
			message := err.Error()
			for _, want := range []string{"GH_TOKEN", "gh auth login"} {
				if !strings.Contains(message, want) {
					t.Errorf("error %q does not mention %q", message, want)
				}
			}
			if strings.Contains(message, sentinelToken) {
				t.Error("error text leaked the underlying command output")
			}
		})
	}
}

func TestResolveReportsCancellationRatherThanMissingAuthentication(t *testing.T) {
	tests := []struct {
		name    string
		context func(t *testing.T) context.Context
		want    error
	}{
		{
			name: "canceled",
			context: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx
			},
			want: context.Canceled,
		},
		{
			name: "deadline exceeded",
			context: func(t *testing.T) context.Context {
				ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
				t.Cleanup(cancel)
				return ctx
			},
			want: context.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{stdout: sentinelToken}
			_, err := resolverWith(nil, runner).Resolve(tt.context(t))
			if !errors.Is(err, tt.want) {
				t.Fatalf("Resolve() error = %v, want %v", err, tt.want)
			}
			if errors.Is(err, auth.ErrNoCredential) {
				t.Error("a canceled resolution reported missing authentication")
			}
		})
	}
}

func TestResolveUsesEnvironmentCredentialNamesExactly(t *testing.T) {
	var requested []string
	resolver := auth.Resolver{
		LookupEnv: func(key string) string {
			requested = append(requested, key)
			return ""
		},
		Run: (&fakeRunner{stdout: sentinelToken}).run,
	}

	if _, err := resolver.Resolve(t.Context()); err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}
	want := []string{"GH_TOKEN", "GITHUB_TOKEN"}
	if !slices.Equal(requested, want) {
		t.Errorf("environment lookups = %v, want %v", requested, want)
	}
}

func TestCredentialNeverRendersItsToken(t *testing.T) {
	runner := &fakeRunner{stdout: sentinelToken}
	credential, err := resolverWith(nil, runner).Resolve(t.Context())
	if err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}

	rendered := []string{
		credential.String(),
		fmt.Sprint(credential),
		fmt.Sprintf("%v", credential),
		fmt.Sprintf("%q", credential),
		fmt.Sprintf("%+v", credential),
		fmt.Sprintf("%#v", credential),
		fmt.Sprintf("%v", []auth.Credential{credential}),
		fmt.Sprintf("%v", struct{ Credential auth.Credential }{credential}),
		fmt.Errorf("wrapped: %v", credential).Error(),
	}
	for _, got := range rendered {
		if strings.Contains(got, sentinelToken) {
			t.Errorf("rendering %q exposed the credential", got)
		}
		if got == "" {
			t.Error("rendering produced an empty string, want a redaction marker")
		}
	}
}

func TestNewResolverUsesProcessEnvironmentAndCommandExecution(t *testing.T) {
	resolver := auth.NewResolver()
	if resolver.LookupEnv == nil {
		t.Error("NewResolver() left LookupEnv nil")
	}
	if resolver.Run == nil {
		t.Error("NewResolver() left Run nil")
	}

	t.Setenv("GH_TOKEN", sentinelToken)
	credential, err := resolver.Resolve(t.Context())
	if err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}
	if credential.Token() != sentinelToken {
		t.Error("NewResolver() did not read the process environment")
	}
}
