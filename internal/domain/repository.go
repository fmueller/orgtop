// Package domain contains OrgTop's source-independent activity model. It must not
// import GitHub adapter or TUI packages.
package domain

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidRepository reports a repository identifier that does not satisfy the
// lexical owner/repository grammar. GitHub remains authoritative for existence and
// access.
var ErrInvalidRepository = errors.New("invalid repository identifier")

const (
	maxOwnerLength      = 39
	maxRepositoryLength = 100
)

// Repository is a validated GitHub repository identity. It keeps the requested
// spelling for display and matches other identities case-insensitively.
type Repository struct {
	owner string
	name  string
}

// ParseRepository validates an exact owner/repository identifier and retains its
// requested spelling.
func ParseRepository(value string) (Repository, error) {
	owner, name, found := strings.Cut(value, "/")
	if !found || strings.Contains(name, "/") {
		return Repository{}, fmt.Errorf("%w %q: expected exactly one owner/repository separator", ErrInvalidRepository, value)
	}
	if err := validateOwner(owner); err != nil {
		return Repository{}, fmt.Errorf("%w %q: %w", ErrInvalidRepository, value, err)
	}
	if err := validateName(name); err != nil {
		return Repository{}, fmt.Errorf("%w %q: %w", ErrInvalidRepository, value, err)
	}
	return Repository{owner: owner, name: name}, nil
}

// Owner returns the owner component in its requested spelling.
func (r Repository) Owner() string { return r.owner }

// Name returns the repository component in its requested spelling.
func (r Repository) Name() string { return r.name }

// String returns the identifier in its requested spelling.
func (r Repository) String() string { return r.owner + "/" + r.name }

// Key returns the lowercase matching key used for case-insensitive comparison.
func (r Repository) Key() string { return strings.ToLower(r.String()) }

func validateOwner(owner string) error {
	if owner == "" {
		return errors.New("owner is empty")
	}
	if len(owner) > maxOwnerLength {
		return fmt.Errorf("owner exceeds %d characters", maxOwnerLength)
	}
	for i := 0; i < len(owner); i++ {
		c := owner[i]
		if isASCIIAlphanumeric(c) {
			continue
		}
		if c == '-' && i > 0 && i < len(owner)-1 {
			continue
		}
		return fmt.Errorf("owner contains an unsupported character %q", string(c))
	}
	return nil
}

func validateName(name string) error {
	if name == "" {
		return errors.New("repository is empty")
	}
	if len(name) > maxRepositoryLength {
		return fmt.Errorf("repository exceeds %d characters", maxRepositoryLength)
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if isASCIIAlphanumeric(c) || c == '.' || c == '_' || c == '-' {
			continue
		}
		return fmt.Errorf("repository contains an unsupported character %q", string(c))
	}
	return nil
}

func isASCIIAlphanumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
