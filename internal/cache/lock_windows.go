//go:build windows

package cache

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// Windows byte-range locking. The syscall package exposes no LockFileEx
// binding, but golang.org/x/sys/windows — an approved direct requirement since
// T-078 admitted it for the cache's owner and ACL checks — wraps both entry
// points with typed arguments, so the lock takes them from there rather than
// re-resolving kernel32 and passing an OVERLAPPED across a uintptr.

// lockRegion takes one byte of the maintenance lock through LockFileEx,
// retrying without spinning until the wait bound expires.
func lockRegion(file *os.File, region int, exclusive bool, wait time.Duration) error {
	flags := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if exclusive {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	deadline := time.Now().Add(wait)
	for {
		overlapped := windows.Overlapped{Offset: uint32(region)}
		err := windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, 1, 0, &overlapped)
		if err == nil {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%w: cache lock region %d: %v", ErrContended, region, err)
		}
		time.Sleep(retryInterval)
	}
}

// unlockRegion drops one byte of the maintenance lock.
func unlockRegion(file *os.File, region int) error {
	overlapped := windows.Overlapped{Offset: uint32(region)}
	if err := windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped); err != nil {
		return fmt.Errorf("cache lock region %d could not be released: %w", region, err)
	}
	return nil
}
