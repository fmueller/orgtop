package cache

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Restrictive creation modes. A cache file is private to the effective user, so
// no other account can read evidence or corrupt the store.
const (
	directoryMode = fs.FileMode(0o700)
	fileMode      = fs.FileMode(0o600)
)

// openCacheRoot prepares the fixed cache directory and returns a
// directory-relative handle for every later inspection and mutation, so a check
// cannot be raced into following another object.
func openCacheRoot(location Location) (*os.Root, error) {
	if location.IsZero() {
		return nil, fmt.Errorf("%w: no cache directory", ErrUnavailable)
	}
	if err := os.MkdirAll(location.Directory(), directoryMode); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	info, err := os.Lstat(location.Directory())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: %q is not a directory", ErrUnavailable, location.Directory())
	}
	if err := checkDirectoryOwnership(location.Directory(), info); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(location.Directory())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return root, nil
}

// inspectOwnedPaths rejects a symlink or non-regular object at any path OrgTop
// owns and reports whether a broader existing mode still has to be narrowed. It
// mutates nothing, so it is safe under shared lifecycle access. A missing file
// is not a problem: it is the normal first-launch state.
func inspectOwnedPaths(root *os.Root, location Location) (bool, error) {
	broad, err := directoryNeedsNarrowing(location)
	if err != nil {
		return false, err
	}
	for _, name := range ownedNames() {
		info, err := root.Lstat(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("%w: %q is not a regular file", ErrUnavailable, name)
		}
		if err := checkFileOwnership(root, name, info); err != nil {
			return false, err
		}
		broad = broad || fileNeedsNarrowing(info)
	}
	return broad, nil
}

// repairOwnedPaths narrows the cache directory and every owned file to their
// restrictive modes. Every path mutation happens only under exclusive lifecycle
// access, so a check cannot be raced into following another object.
func repairOwnedPaths(root *os.Root, location Location) error {
	broad, err := directoryNeedsNarrowing(location)
	if err != nil {
		return err
	}
	if broad {
		if err := os.Chmod(location.Directory(), directoryMode); err != nil {
			return fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
	}
	for _, name := range ownedNames() {
		info, err := root.Lstat(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
		if !fileNeedsNarrowing(info) {
			continue
		}
		if err := root.Chmod(name, fileMode); err != nil {
			return fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
	}
	return nil
}

// directoryNeedsNarrowing reports whether the cache directory still grants
// access beyond the effective user. The decision is platform-specific: only a
// platform whose reported mode is the real access control may answer yes, or an
// ordinary launch would escalate to exclusive lifecycle access on every open
// and refuse every concurrent launch.
func directoryNeedsNarrowing(location Location) (bool, error) {
	info, err := os.Lstat(location.Directory())
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return directoryModeNeedsNarrowing(info), nil
}

// openOwnedFile opens or creates one cache file relative to the cache directory
// with restrictive permissions.
func openOwnedFile(root *os.Root, name string) (*os.File, error) {
	file, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE, fileMode)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return file, nil
}

// removeOwnedFile deletes one cache file relative to the cache directory. A
// missing file is already in the wanted state.
func removeOwnedFile(root *os.Root, name string) error {
	if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return nil
}
