package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/fmueller/orgtop/internal/domain"
)

// MaxOrganizationSelectors is the closed RG-010 bound on distinct organization
// selectors one invocation may request. It bounds expansion work, so it is
// checked before credential, cache, network, or TUI work happens.
const MaxOrganizationSelectors = 5

// ErrInvalidOrganization reports an organization selector value that does not
// satisfy the shipped repository-owner grammar. GitHub remains authoritative for
// existence and access, so an unknown organization is a runtime adapter outcome
// rather than a parse rejection (RG-010).
var ErrInvalidOrganization = errors.New("invalid organization")

// ErrOrganizationCapacity reports more distinct organization selectors than the
// closed expansion bound admits (RG-010).
var ErrOrganizationCapacity = errors.New("too many organization selectors requested")

// ErrInclusionWithoutSelector reports --include-archived or --include-forks
// without an organization selector to apply to. Both flags qualify expansion
// eligibility only and never alter exact repository or path Scopes (RG-010).
var ErrInclusionWithoutSelector = errors.New("--include-archived and --include-forks require at least one --org ORGANIZATION or --repo 'ORGANIZATION/*' selector")

// organizationWildcard is the repository component that turns an otherwise
// exact --repo value into the organization-selector alias. It must be the entire
// component and reach the process unchanged; the quoting a shell needs is
// guidance rather than parser-visible grammar (RG-010).
const organizationWildcard = "*"

// OrganizationSelector is one validated organization selection input. It is not
// a Scope: it carries no path matcher, has no Scope identity, and is expanded
// into ordinary repository Scopes at the adapter boundary (RG-010).
type OrganizationSelector struct {
	name  string
	alias bool
}

// parseOrganization validates an organization name and retains its requested
// spelling and form.
func parseOrganization(value string, alias bool) (OrganizationSelector, error) {
	if err := domain.ValidateOwner(value); err != nil {
		return OrganizationSelector{}, fmt.Errorf("%w %q: %w", ErrInvalidOrganization, value, err)
	}
	return OrganizationSelector{name: value, alias: alias}, nil
}

// Name returns the organization in its first requested spelling.
func (o OrganizationSelector) Name() string { return o.name }

// Key returns the lowercase matching key organization selectors deduplicate by.
func (o OrganizationSelector) Key() string { return strings.ToLower(o.name) }

// Alias reports whether the retained first occurrence used the
// --repo 'ORGANIZATION/*' alias rather than --org.
func (o OrganizationSelector) Alias() bool { return o.alias }

// String returns the retained selector in the form it was requested with, so a
// diagnostic names the argument the user actually supplied.
func (o OrganizationSelector) String() string {
	if o.alias {
		return "--" + repoFlag + " " + o.name + "/" + organizationWildcard
	}
	return "--" + orgFlag + " " + o.name
}

// organizationAlias reports whether an exact --repo value is the organization
// selector alias and returns the organization component it names.
func organizationAlias(value string) (string, bool) {
	organization, repository, found := strings.Cut(value, "/")
	return organization, found && repository == organizationWildcard
}

// checkOrganizationCapacity rejects an invocation requesting more distinct
// selectors than expansion admits, reporting the requested and allowed counts.
func checkOrganizationCapacity(selectors []OrganizationSelector) error {
	if len(selectors) > MaxOrganizationSelectors {
		return fmt.Errorf("%w: %d distinct organization selectors requested, at most %d are allowed", ErrOrganizationCapacity, len(selectors), MaxOrganizationSelectors)
	}
	return nil
}
