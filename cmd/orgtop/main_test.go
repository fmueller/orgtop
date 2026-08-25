package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/fmueller/orgtop/internal/auth"
	"github.com/fmueller/orgtop/internal/cli"
)

// sentinelToken is the credential value no captured output may ever contain
// (NFR-003).
const sentinelToken = "orgtop-sentinel-token-value"

// launchRecord is what one launch attempt observed.
type launchRecord struct {
	called       bool
	repositories []string
	token        string
}

// harness builds a shell whose process seams are recorded rather than executed.
type harness struct {
	output bytes.Buffer
	// version receives the version line, which the process writes to stdout so
	// a caller can read it apart from usage and failure copy.
	version  bytes.Buffer
	resolved bool
	launched launchRecord
	// credential is handed to launch when resolution is asked to succeed.
	credential auth.Credential
	// resolveErr fails credential resolution when non-nil.
	resolveErr error
	// launchErr fails the launch when non-nil.
	launchErr error
	// resolveCtxErr records the cancellation state of the context credential
	// resolution ran under.
	resolveCtxErr error
}

func (h *harness) shell() shell {
	return shell{
		resolve: func(ctx context.Context) (auth.Credential, error) {
			h.resolved = true
			h.resolveCtxErr = ctx.Err()
			if h.resolveErr != nil {
				return auth.Credential{}, h.resolveErr
			}
			return h.credential, nil
		},
		launch: func(_ context.Context, config cli.Config, credential auth.Credential) error {
			h.launched.called = true
			h.launched.token = credential.Token()
			for _, repository := range config.Scope.Repositories() {
				h.launched.repositories = append(h.launched.repositories, repository.String())
			}
			return h.launchErr
		},
		output:  &h.output,
		version: &h.version,
	}
}

// realResolver returns the production credential resolver with only its process
// seams stubbed: token is what GH_TOKEN holds, and gh always reports nothing. An
// environment credential must short-circuit resolution, so a gh invocation
// alongside one fails the test that asked for it.
func realResolver(t *testing.T, token string) auth.Resolver {
	t.Helper()

	return auth.Resolver{
		LookupEnv: func(key string) string {
			if key == "GH_TOKEN" {
				return token
			}
			return ""
		},
		Run: func(context.Context, string, ...string) ([]byte, error) {
			if token != "" {
				t.Error("gh was invoked although an environment credential is set")
			}
			return nil, errors.New("gh reported no credential")
		},
	}
}

// sentinelCredential resolves a credential carrying sentinelToken through the
// documented GH_TOKEN precedence, which is the only constructor auth exposes.
func sentinelCredential(t *testing.T) auth.Credential {
	t.Helper()

	credential, err := realResolver(t, sentinelToken).Resolve(context.Background())
	if err != nil {
		t.Fatalf("resolving the sentinel credential failed: %v", err)
	}
	return credential
}

func TestRejectedConfigurationReportsUsageBeforeAnyAuthenticationWork(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no repository", args: nil, want: cli.ErrMissingRepository.Error()},
		{name: "invalid repository", args: []string{"--repo", "acme/back end"}, want: `--repo: invalid repository identifier "acme/back end": repository contains an unsupported character " "`},
		{name: "positional argument", args: []string{"--repo", "acme/backend", "stray"}, want: `unexpected argument "stray"`},
		{name: "malformed flag", args: []string{"--bogus"}, want: "flag provided but not defined: -bogus"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := &harness{}
			code := harness.shell().run(context.Background(), "orgtop", test.args)

			if code != exitFailure {
				t.Errorf("run(%q) exit code = %d, want %d", test.args, code, exitFailure)
			}
			if harness.resolved {
				t.Error("a rejected configuration resolved a credential")
			}
			if harness.launched.called {
				t.Error("a rejected configuration launched the terminal ui")
			}
			output := harness.output.String()
			if !strings.Contains(output, "Usage:") {
				t.Errorf("output does not print usage:\n%s", output)
			}
			if got := strings.Count(output, test.want); got != 1 {
				t.Errorf("output reports %q %d times, want once:\n%s", test.want, got, output)
			}
		})
	}
}

func TestHelpRequestExitsSuccessfullyWithoutLaunching(t *testing.T) {
	harness := &harness{}
	code := harness.shell().run(context.Background(), "orgtop", []string{"--help"})

	if code != exitSuccess {
		t.Errorf("run(--help) exit code = %d, want %d", code, exitSuccess)
	}
	if harness.resolved || harness.launched.called {
		t.Error("a help request resolved a credential or launched the terminal ui")
	}
	if output := harness.output.String(); !strings.Contains(output, "Usage:") {
		t.Errorf("a help request does not print usage:\n%s", output)
	}
}

// TestVersionRequestExitsSuccessfullyWithoutLaunching keeps the version path
// ahead of every other decision: neither spelling needs a --repo selection, a
// credential, or the terminal UI (FR-001, A-012).
func TestVersionRequestExitsSuccessfullyWithoutLaunching(t *testing.T) {
	tests := []struct {
		name   string
		binary string
		args   []string
	}{
		{name: "long flag", binary: "orgtop", args: []string{"--version"}},
		{name: "short flag", binary: "orgtop", args: []string{"-v"}},
		// The Windows archive ships orgtop.exe and a user may rename or symlink
		// any copy, but the reported line stays the documented one (A-012).
		{name: "windows executable name", binary: "orgtop.exe", args: []string{"--version"}},
		{name: "renamed binary", binary: "orgtop-nightly", args: []string{"--version"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := &harness{}
			code := harness.shell().run(context.Background(), test.binary, test.args)

			if code != exitSuccess {
				t.Errorf("run(%q) exit code = %d, want %d", test.args, code, exitSuccess)
			}
			if harness.resolved || harness.launched.called {
				t.Error("a version request resolved a credential or launched the terminal ui")
			}
			want := "orgtop " + cli.Version + "\n"
			if got := harness.version.String(); got != want {
				t.Errorf("version output = %q, want %q", got, want)
			}
			if got := harness.output.String(); got != "" {
				t.Errorf("a version request wrote %q to the error stream, want nothing", got)
			}
		})
	}
}

// TestHelpAndFailuresStayOffTheVersionStream keeps the two streams separable:
// only the version line reaches stdout, so a caller reading it never has to
// filter usage or a startup failure out of it.
func TestHelpAndFailuresStayOffTheVersionStream(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "help request", args: []string{"--help"}},
		{name: "rejected configuration", args: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := &harness{}
			harness.shell().run(context.Background(), "orgtop", test.args)

			if got := harness.version.String(); got != "" {
				t.Errorf("wrote %q to the version stream, want nothing", got)
			}
		})
	}
}

func TestMissingAuthenticationReportsRemediationWithoutLaunching(t *testing.T) {
	harness := &harness{resolveErr: auth.ErrNoCredential}
	code := harness.shell().run(context.Background(), "orgtop", []string{"--repo", "acme/backend"})

	if code != exitFailure {
		t.Errorf("run exit code = %d, want %d", code, exitFailure)
	}
	if harness.launched.called {
		t.Error("a failed credential resolution launched the terminal ui")
	}
	output := harness.output.String()
	for _, want := range []string{"GH_TOKEN", "gh auth login"} {
		if !strings.Contains(output, want) {
			t.Errorf("remediation does not mention %q:\n%s", want, output)
		}
	}
}

func TestValidConfigurationLaunchesWithTheResolvedDependencies(t *testing.T) {
	harness := &harness{credential: sentinelCredential(t)}
	code := harness.shell().run(context.Background(), "orgtop", []string{"--repo", "acme/backend", "--repo", "acme/frontend"})

	if code != exitSuccess {
		t.Errorf("run exit code = %d, want %d, output:\n%s", code, exitSuccess, harness.output.String())
	}
	if !harness.resolved {
		t.Error("a valid configuration did not resolve a credential")
	}
	if !harness.launched.called {
		t.Fatal("a valid configuration did not launch the terminal ui")
	}
	want := []string{"acme/backend", "acme/frontend"}
	if got := harness.launched.repositories; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("launched scope = %v, want %v", got, want)
	}
	if harness.launched.token != sentinelToken {
		t.Error("the launch did not receive the resolved credential")
	}
	if output := harness.output.String(); output != "" {
		t.Errorf("a successful launch wrote to the error stream:\n%s", output)
	}
}

func TestProcessInterruptCancelsCredentialResolution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	harness := &harness{resolveErr: fmt.Errorf("resolving the github.com credential: %w", context.Canceled)}
	code := harness.shell().run(ctx, "orgtop", []string{"--repo", "acme/backend"})

	if code != exitFailure {
		t.Errorf("run exit code = %d, want %d", code, exitFailure)
	}
	if !errors.Is(harness.resolveCtxErr, context.Canceled) {
		t.Errorf("credential resolution context error = %v, want %v", harness.resolveCtxErr, context.Canceled)
	}
	if harness.launched.called {
		t.Error("an interrupted startup launched the terminal ui")
	}
}

func TestStartupFailuresNeverReportACredentialValue(t *testing.T) {
	credential := sentinelCredential(t)
	harness := &harness{
		credential: credential,
		// A launch failure that formats the credential is the worst realistic
		// leak path: the reported cause must stay redacted (NFR-003).
		launchErr: fmt.Errorf("launching failed for %v/%#v", credential, credential),
	}
	code := harness.shell().run(context.Background(), "orgtop", []string{"--repo", "acme/backend"})

	if code != exitFailure {
		t.Errorf("run exit code = %d, want %d", code, exitFailure)
	}
	output := harness.output.String()
	if strings.Contains(output, sentinelToken) {
		t.Errorf("startup output contains the credential value:\n%s", output)
	}
	if !strings.Contains(output, "[redacted]") {
		t.Errorf("the reported cause does not redact the credential:\n%s", output)
	}
}
