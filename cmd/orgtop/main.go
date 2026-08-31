// Command orgtop renders recent GitHub activity for an explicitly selected set
// of repositories in a terminal UI. It validates the requested Scope before any
// authentication or network work happens and launches the Bubble Tea shell only
// for a complete launch configuration (FR-001).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/fmueller/orgtop/internal/auth"
	"github.com/fmueller/orgtop/internal/cli"
)

// The process exit codes the launch sequence reports.
const (
	exitSuccess = 0
	// exitFailure reports a failure after the invocation was accepted.
	exitFailure = 1
	// exitUsage reports a rejected invocation, which prints usage and its cause
	// instead of starting any launch work (RG-001).
	exitUsage = 2
)

// resolveFunc resolves the github.com credential the launch authenticates with.
type resolveFunc func(ctx context.Context) (auth.Credential, error)

// launchFunc runs the terminal UI for a validated configuration until it exits.
type launchFunc func(ctx context.Context, config cli.Config, credential auth.Credential) error

// shell is the launch sequence with its process seams injected, so startup is
// testable without a terminal, a credential, or GitHub.
type shell struct {
	resolve resolveFunc
	launch  launchFunc
	// output receives usage and every startup failure.
	output io.Writer
	// report receives the successful non-TUI output a caller reads: the version
	// line and the administrative cache-reset outcome. Keeping it apart from
	// output means a caller never has to filter usage or failure copy out of the
	// same stream (FR-001).
	report io.Writer
}

func main() {
	// The process interrupt cancels credential resolution and, once the shell
	// runs, the source request in flight behind it (NFR-001).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := shell{
		resolve: auth.NewResolver().Resolve,
		launch:  launch,
		output:  os.Stderr,
		report:  os.Stdout,
	}.run(ctx, filepath.Base(os.Args[0]), os.Args[1:])
	stop()
	os.Exit(code)
}

// run performs the launch sequence and returns the process exit code. Every
// rejected configuration and every startup failure is reported before the
// terminal UI starts.
func (s shell) run(ctx context.Context, name string, args []string) int {
	// ParseArgs writes usage plus the actionable cause, so a rejected
	// configuration is reported there and the shell only decides the exit code.
	config, err := cli.ParseArgs(name, args, s.output)
	if err != nil {
		// An explicit help request is a successful run rather than a rejected
		// configuration.
		if errors.Is(err, flag.ErrHelp) {
			return exitSuccess
		}
		// A version request is answered before any credential or network work,
		// on its own stream (A-012).
		if errors.Is(err, cli.ErrVersionRequested) {
			_, _ = fmt.Fprintln(s.report, cli.VersionLine())
			return exitSuccess
		}
		return exitUsage
	}

	// The administrative cache reset is answered before any credential, network,
	// or terminal work. OrgTop writes no enrichment cache yet, so there is no
	// state to remove and the reset reports that outcome and exits zero; the
	// removal itself is wired to the RG-005 store when that store lands.
	if config.ResetCache {
		_, _ = fmt.Fprintln(s.report, "orgtop: no cached enrichment state to remove")
		return exitSuccess
	}

	credential, err := s.resolve(ctx)
	if err != nil {
		return s.fail(err)
	}

	if err := s.launch(ctx, config, credential); err != nil {
		return s.fail(err)
	}
	return exitSuccess
}

// fail reports a startup failure. The reported causes carry remediation and no
// credential value, which Credential's redacting formatting keeps true even for
// a cause that formats one (NFR-003).
func (s shell) fail(err error) int {
	_, _ = fmt.Fprintln(s.output, err)
	return exitFailure
}
