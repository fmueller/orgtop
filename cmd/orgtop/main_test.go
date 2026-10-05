package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/fmueller/orgtop/internal/auth"
	"github.com/fmueller/orgtop/internal/cache"
	"github.com/fmueller/orgtop/internal/cli"
	"github.com/fmueller/orgtop/internal/domain"
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
	// resetCache records that the administrative cache removal ran, and
	// resetErr fails it when non-nil.
	resetCache bool
	resetErr   error
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
			for _, repository := range config.Scopes.Repositories() {
				h.launched.repositories = append(h.launched.repositories, repository.String())
			}
			return h.launchErr
		},
		resetCache: func() error {
			h.resetCache = true
			return h.resetErr
		},
		output: &h.output,
		report: &h.version,
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
		{name: "true launch assignment", args: []string{"--no-cache=true", "--repo=acme/api"}, want: "--no-cache accepts only its bare form"},
		{name: "false launch assignment", args: []string{"--no-cache=false", "--repo=acme/api"}, want: "--no-cache accepts only its bare form"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := &harness{}
			code := harness.shell().run(context.Background(), "orgtop", test.args)

			if code != exitUsage {
				t.Errorf("run(%q) exit code = %d, want %d", test.args, code, exitUsage)
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

func TestExecutableBooleanAssignmentsPreserveCache(t *testing.T) {
	dir := t.TempDir()
	binary := buildCLI(t, dir)
	marker := credentialProbe(t, dir)
	root := isolateExecutable(t, t.TempDir())
	location := cache.LocationIn(root)
	store, err := cache.Open(location)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := domain.ParseRepository("acme/api")
	if err != nil {
		t.Fatal(err)
	}
	key, err := cache.CompareKey(repository, strings.Repeat("a", 40), strings.Repeat("b", 40))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	store.WithClock(func() time.Time { return now })
	if err := store.Save(context.Background(), cache.Entry{Key: key, AcquiredAt: now, LastUsedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(location.Database())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"reset-cache", "no-cache", "include-archived", "include-forks"} {
		for _, value := range []string{"true", "false"} {
			args := []string{"--reset-cache", "--" + name + "=" + value}
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				command := exec.Command(binary, args...)
				output, err := command.CombinedOutput()
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != exitUsage || !strings.Contains(string(output), "bare form") {
					t.Errorf("exit = %v, want usage 2; output = %s", err, output)
				}
				if _, err := os.Stat(marker); !os.IsNotExist(err) {
					t.Fatalf("credential hook ran: %v", err)
				}
				after, err := os.ReadFile(location.Database())
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("cache bytes changed or removed: %v", err)
				}
			})
		}
	}
}

// TestStartupFailureExitsDistinctlyFromARejectedConfiguration keeps the two
// non-zero outcomes apart: a rejected invocation exits 2 with usage (RG-001),
// while a failure after a complete configuration exits 1.
func TestStartupFailureExitsDistinctlyFromARejectedConfiguration(t *testing.T) {
	harness := &harness{resolveErr: errors.New("no credential")}
	code := harness.shell().run(context.Background(), "orgtop", []string{"--repo", "acme/backend"})

	if code != exitFailure {
		t.Errorf("exit code = %d, want %d", code, exitFailure)
	}
	if output := harness.output.String(); strings.Contains(output, "Usage:") {
		t.Errorf("a startup failure printed usage:\n%s", output)
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

// TestCacheResetExitsBeforeAnyAuthenticationWork keeps the standalone
// administrative reset off the launch path: it resolves no credential, makes no
// request, starts no terminal UI, runs the cache removal, and reports its
// outcome on stdout (A-019).
func TestCacheResetExitsBeforeAnyAuthenticationWork(t *testing.T) {
	harness := &harness{credential: sentinelCredential(t)}
	code := harness.shell().run(context.Background(), "orgtop", []string{"--reset-cache"})

	if code != exitSuccess {
		t.Errorf("exit code = %d, want %d", code, exitSuccess)
	}
	if harness.resolved {
		t.Error("a cache reset resolved a credential")
	}
	if harness.launched.called {
		t.Error("a cache reset started the terminal ui")
	}
	if !harness.resetCache {
		t.Error("a cache reset performed no cache removal")
	}
	if got := harness.version.String(); !strings.Contains(got, "cache") {
		t.Errorf("a cache reset reported %q, want it to report the cache outcome", got)
	}
	if harness.output.Len() != 0 {
		t.Errorf("a successful cache reset wrote %q to the failure stream, want none", harness.output.String())
	}
}

// TestFailedCacheResetIsReportedAndNonzero proves a reset that cannot complete
// exits nonzero with its sanitized cause on the failure stream, and reports no
// success a user could mistake for a removed cache (RG-005).
func TestFailedCacheResetIsReportedAndNonzero(t *testing.T) {
	harness := &harness{
		credential: sentinelCredential(t),
		resetErr:   errors.New("enrichment cache unavailable: cache file does not belong to orgtop"),
	}
	code := harness.shell().run(context.Background(), "orgtop", []string{"--reset-cache"})

	if code != exitFailure {
		t.Errorf("exit code = %d, want %d", code, exitFailure)
	}
	if got := harness.output.String(); !strings.Contains(got, "does not belong to orgtop") {
		t.Errorf("failure stream = %q, want the sanitized reset cause", got)
	}
	if harness.version.Len() != 0 {
		t.Errorf("a failed cache reset reported %q as a successful outcome", harness.version.String())
	}
}
