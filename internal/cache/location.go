// Package cache implements OrgTop's disposable local SQLite enrichment cache.
//
// The cache stores only immutable changed-file evidence and the proof facts that
// validate it. It never stores credentials, headers, raw payloads, rendered
// state, provenance, Scope identity, or speculative history, and removing it
// changes GitHub request volume rather than matching semantics.
package cache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrUnavailable reports a cache that cannot be located or used at all. It is
// never a failure of an otherwise valid launch: the caller bypasses the cache.
var ErrUnavailable = errors.New("enrichment cache unavailable")

// The fixed version-1 layout. There is no v0.2.0 cache-path override, so these
// names are the whole configuration surface.
const (
	directoryName = "orgtop"
	databaseName  = "enrichment-v1.db"
	lockName      = "enrichment-v1.lock"
	bootstrapName = "enrichment-v1.bootstrap"
	tombstoneName = "enrichment-v1.resetting"
)

// Location names the fixed cache directory and the files OrgTop owns inside it.
// A zero Location means the user cache directory was empty or unavailable.
type Location struct {
	directory string
}

// DefaultLocation resolves the production cache location beneath Go's user cache
// directory. An empty or failed lookup is an unavailable cache and never falls
// back to the working, home, or configuration directory.
func DefaultLocation() (Location, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return Location{}, fmt.Errorf("%w: no user cache directory", ErrUnavailable)
	}
	location := LocationIn(root)
	if location.IsZero() {
		return Location{}, fmt.Errorf("%w: no user cache directory", ErrUnavailable)
	}
	return location, nil
}

// LocationIn returns the cache location beneath an explicit user cache root. An
// empty root yields the zero Location so no relative path is ever derived.
func LocationIn(root string) Location {
	if root == "" {
		return Location{}
	}
	return Location{directory: filepath.Join(root, directoryName)}
}

// IsZero reports whether the location was never resolved.
func (l Location) IsZero() bool { return l.directory == "" }

// Directory reports the fixed cache directory.
func (l Location) Directory() string { return l.directory }

// Database reports the canonical version-1 database path.
func (l Location) Database() string { return l.file(databaseName) }

// Lock reports the private sidecar maintenance lock path.
func (l Location) Lock() string { return l.file(lockName) }

// Bootstrap reports the exclusively created initialization file path.
func (l Location) Bootstrap() string { return l.file(bootstrapName) }

// Tombstone reports the atomic reset rename target path.
func (l Location) Tombstone() string { return l.file(tombstoneName) }

// sidecarNames reports the SQLite sidecar file names of the canonical database.
func sidecarNames() []string {
	return []string{
		databaseName + "-wal",
		databaseName + "-shm",
		databaseName + "-journal",
	}
}

// ownedNames reports every file the cache accounts for physically: the canonical
// database, its sidecars, the maintenance lock, and any OrgTop-owned bootstrap
// or tombstone file. Every owned file lives directly in the cache directory, so
// the name is also the directory-relative handle every mutation uses.
func ownedNames() []string {
	names := append([]string{databaseName}, sidecarNames()...)
	return append(names, lockName, bootstrapName, tombstoneName)
}

// ownedFiles reports the absolute path of every owned file.
func (l Location) ownedFiles() []string {
	paths := make([]string, 0, len(ownedNames()))
	for _, name := range ownedNames() {
		paths = append(paths, l.file(name))
	}
	return paths
}

func (l Location) file(name string) string {
	if l.IsZero() {
		return ""
	}
	return filepath.Join(l.directory, name)
}
