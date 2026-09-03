// Package cli parses OrgTop's launch configuration from command-line arguments.
// It validates the requested repository Scopes before any authentication or
// network work happens.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/fmueller/orgtop/internal/domain"
)

// ErrMissingRepository reports a launch without any repository or path
// selection. It replaces domain.ErrEmptyScope, which is a domain invariant
// rather than user-facing copy.
var ErrMissingRepository = errors.New("at least one --repo owner/repository, --path owner/repository:pattern, or --org organization selection is required")

// ErrBarePathWithoutRepository reports a bare --path pattern with nothing to
// filter. A bare pattern never implies a repository (RG-001).
var ErrBarePathWithoutRepository = errors.New("a bare --path pattern requires at least one --repo owner/repository")

// ErrResetCacheNotStandalone reports --reset-cache combined with another launch
// selection or cache control. The administrative action stands alone (RG-001).
var ErrResetCacheNotStandalone = errors.New("--reset-cache does not accept --repo, --path, --org, --include-archived, --include-forks, --no-cache, or any other launch selection")

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
	repoFlag             = "repo"
	repoUsage            = "select an exact GitHub owner/repository; repeat the flag to select several"
	orgFlag              = "org"
	orgUsage             = "select every eligible repository of a GitHub organization; repeat the flag to select several"
	includeArchivedFlag  = "include-archived"
	includeArchivedUsage = "include archived repositories when expanding organization selectors"
	includeForksFlag     = "include-forks"
	includeForksUsage    = "include forked repositories when expanding organization selectors"
	pathFlag             = "path"
	pathUsage            = "select a path pattern; a bare PATTERN filters every --repo selection, a qualified OWNER/REPOSITORY:PATTERN stands alone; repeat the flag to select several"
	noCacheFlag          = "no-cache"
	noCacheUsage         = "run without opening, reading, or writing the enrichment cache"
	resetCacheFlag       = "reset-cache"
	resetCacheUsage      = "remove OrgTop's cached enrichment state and exit; accepts no other flag"
	versionFlag          = "version"
	versionShort         = "v"
	versionUsage         = "print the release version and exit"
)

// Config is the validated launch configuration.
type Config struct {
	// Scopes is the expanded, deduplicated selection the launch renders. It is
	// empty for the standalone administrative cache reset and for an
	// organization-only invocation, whose Scopes only exist after expansion.
	Scopes domain.ScopeSet
	// Organizations is the deduplicated organization-selector input, in its
	// shared first-occurrence argument order. It is expansion input for the
	// adapter boundary and never a Scope (RG-010).
	Organizations []OrganizationSelector
	// IncludeArchived and IncludeForks are the process-global expansion
	// eligibility flags. They qualify organization expansion only and never
	// alter exact repository or path Scopes.
	IncludeArchived bool
	IncludeForks    bool
	// NoCache reports the process-local request to perform no cache operation.
	NoCache bool
	// ResetCache reports the standalone administrative cache reset, which
	// selects nothing and launches no terminal UI.
	ResetCache bool
}

// ParseArgs parses args, which excludes the program name, into a Config. Every
// rejected configuration writes usage plus one actionable cause to output and
// returns that cause; a help request writes usage and reports flag.ErrHelp; a
// version request writes nothing and reports ErrVersionRequested.
func ParseArgs(name string, args []string, output io.Writer) (Config, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(output)
	flags.Usage = func() { writeUsage(output, name) }

	var values selections
	flags.Var(selectionFlag{kind: repositorySelection, collected: &values}, repoFlag, repoUsage)
	flags.Var(selectionFlag{kind: pathSelectionKind, collected: &values}, pathFlag, pathUsage)
	flags.Var(selectionFlag{kind: organizationSelection, collected: &values}, orgFlag, orgUsage)
	includeArchived := flags.Bool(includeArchivedFlag, false, includeArchivedUsage)
	includeForks := flags.Bool(includeForksFlag, false, includeForksUsage)
	noCache := flags.Bool(noCacheFlag, false, noCacheUsage)
	resetCache := flags.Bool(resetCacheFlag, false, resetCacheUsage)
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
	// Argument scanning reports a malformed value before any post-parse class,
	// so a reset request beside one names that value first (RG-001).
	requested, err := validateSelections(values)
	if err != nil {
		return Config{}, reject(output, flags, err)
	}
	// Incompatible administrative and cache controls are the first post-parse
	// class, so a reset request is answered before any selection is expanded.
	if *resetCache {
		if len(values) > 0 || *noCache || *includeArchived || *includeForks {
			return Config{}, reject(output, flags, ErrResetCacheNotStandalone)
		}
		return Config{ResetCache: true}, nil
	}
	// Selector-only flag misuse is the next class, then a missing selection;
	// both outrank composition and capacity rejections (RG-001).
	if (*includeArchived || *includeForks) && len(requested.organizations) == 0 {
		return Config{}, reject(output, flags, ErrInclusionWithoutSelector)
	}
	if len(values) == 0 {
		return Config{}, reject(output, flags, ErrMissingRepository)
	}

	scopes, err := expandSelections(requested)
	if err != nil {
		return Config{}, reject(output, flags, err)
	}
	return Config{
		Scopes:          scopes,
		Organizations:   requested.organizations,
		NoCache:         *noCache,
		IncludeArchived: *includeArchived,
		IncludeForks:    *includeForks,
	}, nil
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
	_, _ = fmt.Fprintf(output, "Usage: %s --%s OWNER/REPOSITORY [--%s OWNER/REPOSITORY ...] [--%s (PATTERN | OWNER/REPOSITORY:PATTERN) ...] [--%s]\n", name, repoFlag, repoFlag, pathFlag, noCacheFlag)
	_, _ = fmt.Fprintf(output, "       %s --%s OWNER/REPOSITORY:PATTERN [--%s OWNER/REPOSITORY:PATTERN ...] [--%s OWNER/REPOSITORY ...] [--%s]\n", name, pathFlag, pathFlag, repoFlag, noCacheFlag)
	_, _ = fmt.Fprintf(output, "       %s (--%s ORGANIZATION | --%s 'ORGANIZATION/*') [...] [--%s OWNER/REPOSITORY ...] [--%s OWNER/REPOSITORY:PATTERN ...] [--%s] [--%s] [--%s]\n", name, orgFlag, repoFlag, repoFlag, pathFlag, includeArchivedFlag, includeForksFlag, noCacheFlag)
	_, _ = fmt.Fprintf(output, "       %s (--%s ORGANIZATION | --%s 'ORGANIZATION/*') [...] --%s OWNER/REPOSITORY [--%s OWNER/REPOSITORY ...] --%s PATTERN [--%s PATTERN ...] [--%s OWNER/REPOSITORY:PATTERN ...] [--%s] [--%s] [--%s]\n", name, orgFlag, repoFlag, repoFlag, repoFlag, pathFlag, pathFlag, pathFlag, includeArchivedFlag, includeForksFlag, noCacheFlag)
	_, _ = fmt.Fprintf(output, "       %s --%s\n", name, resetCacheFlag)
	_, _ = fmt.Fprintf(output, "       %s --%s\n\n", name, versionFlag)
	_, _ = fmt.Fprintf(output, "Flags:\n  --%s owner/repository\n        %s\n", repoFlag, repoUsage)
	_, _ = fmt.Fprintf(output, "  --%s (PATTERN | OWNER/REPOSITORY:PATTERN)\n        %s\n", pathFlag, pathUsage)
	_, _ = fmt.Fprintf(output, "  --%s (ORGANIZATION), --%s 'ORGANIZATION/*'\n        %s\n", orgFlag, repoFlag, orgUsage)
	_, _ = fmt.Fprintf(output, "  --%s\n        %s\n", includeArchivedFlag, includeArchivedUsage)
	_, _ = fmt.Fprintf(output, "  --%s\n        %s\n", includeForksFlag, includeForksUsage)
	_, _ = fmt.Fprintf(output, "  --%s\n        %s\n", noCacheFlag, noCacheUsage)
	_, _ = fmt.Fprintf(output, "  --%s\n        %s\n", resetCacheFlag, resetCacheUsage)
	_, _ = fmt.Fprintf(output, "  --%s, -%s\n        %s\n", versionFlag, versionShort, versionUsage)
}
