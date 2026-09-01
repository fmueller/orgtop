//go:build unix

package cache

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// checkDirectoryOwnership requires the cache directory to belong to the
// effective user. A directory another account owns is never repaired or used.
func checkDirectoryOwnership(path string, info fs.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("%w: %q is owned by another user", ErrUnavailable, path)
	}
	return nil
}

// checkFileOwnership requires a mutable cache file to belong to the effective
// user and to have exactly one link, so no other name aliases the same inode.
func checkFileOwnership(_ *os.Root, name string, info fs.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("%w: %q is owned by another user", ErrUnavailable, name)
	}
	if stat.Nlink != 1 {
		return fmt.Errorf("%w: %q has %d links", ErrUnavailable, name, stat.Nlink)
	}
	return nil
}
