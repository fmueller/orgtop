//go:build windows

package cache

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

// Windows byte-range locking. The syscall package exposes no LockFileEx binding,
// and golang.org/x/sys is not an approved direct requirement, so the two
// kernel32 entry points are resolved lazily here.
var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	lockFileEx   = kernel32.NewProc("LockFileEx")
	unlockFileEx = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
)

// lockRegion takes one byte of the maintenance lock through LockFileEx,
// retrying without spinning until the wait bound expires.
func lockRegion(file *os.File, region int, exclusive bool, wait time.Duration) error {
	flags := uintptr(lockfileFailImmediately)
	if exclusive {
		flags |= lockfileExclusiveLock
	}
	deadline := time.Now().Add(wait)
	for {
		overlapped := syscall.Overlapped{Offset: uint32(region)}
		result, _, err := lockFileEx.Call(
			file.Fd(),
			flags,
			0,
			1,
			0,
			uintptr(unsafe.Pointer(&overlapped)),
		)
		if result != 0 {
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
	overlapped := syscall.Overlapped{Offset: uint32(region)}
	result, _, err := unlockFileEx.Call(
		file.Fd(),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if result == 0 {
		return fmt.Errorf("cache lock region %d could not be released: %w", region, err)
	}
	return nil
}
