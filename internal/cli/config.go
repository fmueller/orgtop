// Package cli parses OrgTop's launch configuration from command-line arguments.
// It validates the requested repository Scope before any authentication or
// network work happens.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/fmueller/orgtop/internal/domain"
)

// ErrMissingRepository reports a launch without any --repo selection. It replaces
// domain.ErrEmptyScope, which is a domain invariant rather than user-facing copy.
var ErrMissingRepository = errors.New("at least one --repo owner/repository is required")

// ErrVersionRequested reports an explicit --version or -v request. It mirrors
// flag.ErrHelp: parsing stops, nothing is rejected, and the caller decides the
// writer and the exit code.
var ErrVersionRequested = errors.New("version requested")

// Version is the release version the binary reports. A release build stamps it
// with `-X github.com/fmueller/orgtop/internal/cli.Version=<tag>`; a build
// without that stamp reports dev rather than an empty value (FR-001).
var Version = "dev"

// programName is the released binary's own name. The version line reports it
// literally rather than the invoked name, so a Windows `orgtop.exe`, a renamed
// copy, or a symlink still reports the documented `orgtop <version>` form a
// caller parses (FR-001, A-012).
const programName = "orgtop"

// VersionLine is the single line a version request reports.
func VersionLine() string { return programName + " " + Version }

const (
	repoFlag     = "repo"
	repoUsage    = "select an exact GitHub owner/repository; repeat the flag to select several"
	versionFlag  = "version"
	versionShort = "v"
	versionUsage = "print the release version and exit"
)

// Config is the validated launch configuration.
type Config struct {
	Scope domain.Scope
}

// ParseArgs parses args, which excludes the program name, into a Config. Every
// rejected configuration writes usage plus one actionable cause to output and
// returns that cause; a help request writes usage and reports flag.ErrHelp; a
// version request writes nothing and reports ErrVersionRequested.
func ParseArgs(name string, args []string, output io.Writer) (Config, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(output)
	flags.Usage = func() { writeUsage(output, name) }

	var values repositoryValues
	flags.Var(&values, repoFlag, repoUsage)
	// -v is not special to the flag package the way -h is, so both spellings of
	// the version request are declared here.
	version := flags.Bool(versionFlag, false, versionUsage)
	short := flags.Bool(versionShort, false, versionUsage)

	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}
	// A version request outranks every other decision, so it needs neither a
	// --repo selection nor a well-formed remainder (A-012).
	if *version || *short {
		return Config{}, ErrVersionRequested
	}
	if flags.NArg() > 0 {
		return Config{}, reject(output, flags, fmt.Errorf("unexpected argument %q", flags.Arg(0)))
	}

	scope, err := domain.NewScope(values)
	if err != nil {
		if errors.Is(err, domain.ErrEmptyScope) {
			return Config{}, reject(output, flags, ErrMissingRepository)
		}
		return Config{}, reject(output, flags, fmt.Errorf("--%s: %w", repoFlag, err))
	}
	return Config{Scope: scope}, nil
}

// reject writes usage and the actionable cause, then returns that cause. The
// flag package already reports the causes it raises itself, so every rejection
// reaches output exactly once and the caller only decides the exit code.
func reject(output io.Writer, flags *flag.FlagSet, err error) error {
	flags.Usage()
	_, _ = fmt.Fprintln(output, err)
	return err
}

func writeUsage(output io.Writer, name string) {
	_, _ = fmt.Fprintf(output, "Usage: %s --%s owner/repository [--%s owner/repository ...]\n\n", name, repoFlag, repoFlag)
	_, _ = fmt.Fprintf(output, "Flags:\n  --%s owner/repository\n        %s\n", repoFlag, repoUsage)
	_, _ = fmt.Fprintf(output, "  --%s, -%s\n        %s\n", versionFlag, versionShort, versionUsage)
}

// repositoryValues collects repeated --repo values in request order.
type repositoryValues []string

func (v *repositoryValues) String() string { return strings.Join(*v, ", ") }

func (v *repositoryValues) Set(value string) error {
	*v = append(*v, value)
	return nil
}
