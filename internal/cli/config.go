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

const (
	repoFlag  = "repo"
	repoUsage = "select an exact GitHub owner/repository; repeat the flag to select several"
)

// Config is the validated launch configuration.
type Config struct {
	Scope domain.Scope
}

// ParseArgs parses args, which excludes the program name, into a Config. Every
// rejected configuration writes usage to output and returns an actionable error.
func ParseArgs(name string, args []string, output io.Writer) (Config, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(output)
	flags.Usage = func() { writeUsage(output, name) }

	var values repositoryValues
	flags.Var(&values, repoFlag, repoUsage)

	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}
	if flags.NArg() > 0 {
		flags.Usage()
		return Config{}, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}

	scope, err := domain.NewScope(values)
	if err != nil {
		flags.Usage()
		if errors.Is(err, domain.ErrEmptyScope) {
			return Config{}, ErrMissingRepository
		}
		return Config{}, fmt.Errorf("--%s: %w", repoFlag, err)
	}
	return Config{Scope: scope}, nil
}

func writeUsage(output io.Writer, name string) {
	_, _ = fmt.Fprintf(output, "Usage: %s --%s owner/repository [--%s owner/repository ...]\n\n", name, repoFlag, repoFlag)
	_, _ = fmt.Fprintf(output, "Flags:\n  --%s owner/repository\n        %s\n", repoFlag, repoUsage)
}

// repositoryValues collects repeated --repo values in request order.
type repositoryValues []string

func (v *repositoryValues) String() string { return strings.Join(*v, ", ") }

func (v *repositoryValues) Set(value string) error {
	*v = append(*v, value)
	return nil
}
