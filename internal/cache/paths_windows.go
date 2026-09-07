//go:build windows

package cache

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

// checkDirectoryOwnership performs the Windows type, owner, and ACL checks. It
// opens the directory itself without following a reparse point, then answers
// every question from that one handle: the caller's stat described the path at
// an earlier moment, and a junction swapped in since would make a decision
// taken from it describe the wrong object.
func checkDirectoryOwnership(path string, _ fs.FileInfo) error {
	directory, err := openDirectoryNoFollow(path)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() { _ = directory.Close() }()

	attributes, err := describeHandleAttributes(directory)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("%w: %q is a reparse point", ErrUnavailable, path)
	}
	if attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return fmt.Errorf("%w: %q is not a directory", ErrUnavailable, path)
	}
	return checkWindowsAccess(path, directory)
}

// checkFileOwnership performs the Windows handle-based type, link, owner, and
// ACL checks. It rejects a reparse point, any file another name aliases, a file
// belonging to another account, and one whose inherited ACL lets another
// account write to it.
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
	return checkWindowsAccess(name, file)
}

// directoryModeNeedsNarrowing reports no POSIX narrowing on Windows. Go derives
// the reported mode from the read-only attribute alone, so a correctly secured
// cache directory still reads as 0777 and a mode comparison would demand a
// repair that can never succeed. Escalating to exclusive lifecycle access on
// every open would then refuse every concurrent launch. Windows access is
// governed by ownership and inherited ACLs, which checkWindowsAccess enforces
// above and which no repair may widen back.
func directoryModeNeedsNarrowing(fs.FileInfo) bool { return false }

// fileNeedsNarrowing reports no POSIX narrowing on Windows, for the same reason
// as the directory: a mutable cache file reads as 0666 however it is secured.
func fileNeedsNarrowing(fs.FileInfo) bool { return false }
