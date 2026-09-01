//go:build unix

package cache

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// lockRegion takes one byte of the maintenance lock through a POSIX record
// lock, retrying without spinning until the wait bound expires.
func lockRegion(file *os.File, region int, exclusive bool, wait time.Duration) error {
	kind := int16(syscall.F_RDLCK)
	if exclusive {
		kind = syscall.F_WRLCK
	}
	deadline := time.Now().Add(wait)
	for {
		lock := syscall.Flock_t{
			Type:   kind,
			Whence: 0,
			Start:  int64(region),
			Len:    1,
		}
		err := syscall.FcntlFlock(file.Fd(), syscall.F_SETLK, &lock)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EACCES) && !errors.Is(err, syscall.EINTR) {
			return fmt.Errorf("%w: cache lock region %d: %w", ErrUnavailable, region, err)
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%w: cache lock region %d", ErrContended, region)
		}
		time.Sleep(retryInterval)
	}
}

// unlockRegion drops one byte of the maintenance lock.
func unlockRegion(file *os.File, region int) error {
	lock := syscall.Flock_t{
		Type:   syscall.F_UNLCK,
		Whence: 0,
		Start:  int64(region),
		Len:    1,
	}
	return syscall.FcntlFlock(file.Fd(), syscall.F_SETLK, &lock)
}
