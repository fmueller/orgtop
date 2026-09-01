//go:build windows

package cache

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// checkDirectoryOwnership performs the Windows type check. Go marks a reparse
// point irregular, and the caller has already rejected a non-directory, so a
// junction or symbolic link never becomes the cache directory.
func checkDirectoryOwnership(path string, info fs.FileInfo) error {
	if info.Mode()&fs.ModeIrregular != 0 {
		return fmt.Errorf("%w: %q is a reparse point", ErrUnavailable, path)
	}
	return nil
}

// checkFileOwnership performs the Windows handle-based type and link checks. It
// rejects a reparse point and any file another name aliases.
func checkFileOwnership(root *os.Root, name string, info fs.FileInfo) error {
	if info.Mode()&fs.ModeIrregular != 0 {
		return fmt.Errorf("%w: %q is a reparse point", ErrUnavailable, name)
	}
	file, err := root.Open(name)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() { _ = file.Close() }()

	var information syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &information); err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if information.NumberOfLinks != 1 {
		return fmt.Errorf("%w: %q has %d links", ErrUnavailable, name, information.NumberOfLinks)
	}
	return nil
}
