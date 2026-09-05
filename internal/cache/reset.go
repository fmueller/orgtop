package cache

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Reset performs RG-005's administrative `--reset-cache` procedure: it
// validates the fixed location, takes the lifecycle region exclusively, renames
// the main database to the tombstone, deletes the original-name sidecars before
// the tombstone itself, and syncs the directory where the platform supports it.
// An absent cache succeeds once its orphaned sidecars are gone, and a database
// OrgTop does not own is preserved with a sanitized ownership cause.
//
// Reset is a standalone process step, so it opens no database handle at all:
// nothing of this process can hold the file it is about to rename.
func Reset(location Location) error {
	root, err := openCacheRoot(location)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	lock, err := openMaintenanceLock(root, location.Lock())
	if err != nil {
		return err
	}
	defer func() { _ = lock.close() }()

	// An explicit reset waits longer than ordinary work, because the user asked
	// for it and a launch that is still shutting down may still hold the region.
	if err := lock.acquire(lifecycleRegion, true, lockWaits.reset); err != nil {
		return err
	}
	// Path validation runs under the exclusive region, so a symlink or a
	// non-regular object at an owned path cannot be raced in after the check.
	if _, err := inspectOwnedPaths(root, location); err != nil {
		return err
	}
	// A reset creates the maintenance lock inside the cache directory, so it
	// uses that directory and narrows a broader inherited mode like any other
	// use. Repair is a path mutation and is allowed here because the exclusive
	// lifecycle region is already held.
	if err := repairOwnedPaths(root, location); err != nil {
		return err
	}
	return discardDatabase(root, location)
}

// discardDatabase is the tombstone procedure shared by `--reset-cache` and the
// one structural rebuild attempt. It must hold exclusive lifecycle access.
// The rename is atomic, so a crash at any point leaves either the original
// database or a tombstone the next launch finishes removing, never a canonical
// database beside sidecars that no longer describe it.
func discardDatabase(root *os.Root, location Location) error {
	owned, err := canonicalIsOwned(root)
	if err != nil {
		return err
	}
	if owned {
		if err := root.Rename(databaseName, tombstoneName); err != nil {
			return fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
	}
	if err := clearInterruptedReset(root); err != nil {
		return err
	}
	if err := removeOwnedFile(root, bootstrapName); err != nil {
		return err
	}
	syncDirectory(location)
	return nil
}

// clearInterruptedReset removes the original-name sidecars and only then the
// tombstone, so a crash mid-procedure is still recognizable as an unfinished
// reset. Every restart runs it before any canonical create or open, which is
// what keeps a fresh main database from opening beside a stale write-ahead log.
func clearInterruptedReset(root *os.Root) error {
	for _, name := range sidecarNames() {
		if err := removeOwnedFile(root, name); err != nil {
			return err
		}
	}
	return removeOwnedFile(root, tombstoneName)
}

// canonicalIsOwned reports whether the canonical path holds a database this
// process may remove. Only OrgTop's exact application ID qualifies: a foreign
// nonzero ID and an ambiguous pre-existing zero-ID database are preserved, and
// so is any file whose header does not read as a database at all.
func canonicalIsOwned(root *os.Root) (bool, error) {
	identity, err := readIdentity(root)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if identity.application != applicationID {
		return false, fmt.Errorf("%w: application id %#08x", ErrForeignDatabase, identity.application)
	}
	return true, nil
}

// syncDirectory flushes the directory entry changes the procedure made. It is
// best effort: a platform that does not support syncing a directory reports an
// error the procedure has no way to act on, and the rename ordering is what the
// recovery path actually depends on.
func syncDirectory(location Location) {
	directory, err := os.Open(location.Directory())
	if err != nil {
		return
	}
	defer func() { _ = directory.Close() }()
	_ = directory.Sync()
}
