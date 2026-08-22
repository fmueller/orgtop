// Package auth resolves the existing local GitHub credential OrgTop launches with.
// It never logs, renders, or persists a token value and owns no credential store.
package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrNoCredential reports that no existing github.com credential is available.
var ErrNoCredential = errors.New("no github.com credential found: set GH_TOKEN or run gh auth login")

const (
	envGHToken     = "GH_TOKEN"
	envGitHubToken = "GITHUB_TOKEN"
	hostname       = "github.com"
	commandTimeout = 10 * time.Second
	redacted       = "[redacted]"
)

// Credential is a resolved GitHub token. It renders as a redaction marker so that
// no formatting verb can expose the token value.
type Credential struct {
	token string
}

// Token returns the credential value for authenticating GitHub requests.
func (c Credential) Token() string { return c.token }

// String implements fmt.Stringer without exposing the token.
func (c Credential) String() string { return redacted }

// GoString implements fmt.GoStringer so that %#v cannot expose the token.
func (c Credential) GoString() string { return redacted }

// CommandRunner runs a command bound to ctx and returns its standard output.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Resolver resolves the github.com credential from the environment or the GitHub
// CLI. Both fields are required; NewResolver binds the process defaults.
type Resolver struct {
	LookupEnv func(key string) string
	Run       CommandRunner
}

// NewResolver returns a Resolver bound to the process environment and the gh
// executable.
func NewResolver() Resolver {
	return Resolver{LookupEnv: os.Getenv, Run: runCommand}
}

// Resolve returns the first non-empty trimmed credential from GH_TOKEN,
// GITHUB_TOKEN, then gh auth token. Environment credentials prevent any gh
// invocation. A failed, timed-out, or empty gh result means authentication is
// unavailable and yields ErrNoCredential; a canceled ctx is reported as such.
func (r Resolver) Resolve(ctx context.Context) (Credential, error) {
	for _, key := range [...]string{envGHToken, envGitHubToken} {
		if token := strings.TrimSpace(r.LookupEnv(key)); token != "" {
			return Credential{token: token}, nil
		}
	}
	return r.resolveFromGitHubCLI(ctx)
}

func (r Resolver) resolveFromGitHubCLI(ctx context.Context) (Credential, error) {
	commandCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	stdout, err := r.Run(commandCtx, "gh", "auth", "token", "--hostname", hostname)
	if err != nil {
		// The command error is deliberately dropped: it can carry gh internals,
		// and FR-003 requires a remediation instead of command diagnostics. The
		// caller ctx, not commandCtx, decides whether this was a cancellation: an
		// expired commandTimeout means no credential is available.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Credential{}, fmt.Errorf("resolving the %s credential: %w", hostname, ctxErr)
		}
		return Credential{}, ErrNoCredential
	}

	token := strings.TrimSpace(string(stdout))
	if token == "" {
		return Credential{}, ErrNoCredential
	}
	return Credential{token: token}, nil
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}
