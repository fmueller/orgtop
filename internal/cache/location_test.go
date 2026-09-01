package cache

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestLocationInDerivesTheFixedV1Names pins RG-005's fixed cache layout: one
// `orgtop` directory beneath the user cache directory holding exactly the
// version-1 database and its sidecar maintenance lock.
func TestLocationInDerivesTheFixedV1Names(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	location := LocationIn(root)

	if got, want := location.Directory(), filepath.Join(root, "orgtop"); got != want {
		t.Errorf("Directory() = %q, want %q", got, want)
	}
	if got, want := location.Database(), filepath.Join(root, "orgtop", "enrichment-v1.db"); got != want {
		t.Errorf("Database() = %q, want %q", got, want)
	}
	if got, want := location.Lock(), filepath.Join(root, "orgtop", "enrichment-v1.lock"); got != want {
		t.Errorf("Lock() = %q, want %q", got, want)
	}
	if got, want := location.Bootstrap(), filepath.Join(root, "orgtop", "enrichment-v1.bootstrap"); got != want {
		t.Errorf("Bootstrap() = %q, want %q", got, want)
	}
	if got, want := location.Tombstone(), filepath.Join(root, "orgtop", "enrichment-v1.resetting"); got != want {
		t.Errorf("Tombstone() = %q, want %q", got, want)
	}
}

// TestLocationInRejectsAnEmptyRoot keeps an empty or failed user-cache lookup
// cache-unavailable. RG-005 forbids falling back to the working, home, or
// configuration directory, so an empty root must not produce a relative path.
func TestLocationInRejectsAnEmptyRoot(t *testing.T) {
	t.Parallel()

	if location := LocationIn(""); !location.IsZero() {
		t.Errorf("LocationIn(\"\") = %+v, want the zero location", location)
	}
}

// TestDefaultLocationFollowsTheUserCacheDirectory proves the production
// resolution uses Go's user cache directory and no override.
func TestDefaultLocationFollowsTheUserCacheDirectory(t *testing.T) {
	root := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LocalAppData", root)
	case "darwin":
		t.Setenv("HOME", root)
		root = filepath.Join(root, "Library", "Caches")
	default:
		t.Setenv("XDG_CACHE_HOME", root)
	}

	location, err := DefaultLocation()
	if err != nil {
		t.Fatalf("DefaultLocation() error = %v", err)
	}
	if got, want := location.Directory(), filepath.Join(root, "orgtop"); got != want {
		t.Errorf("Directory() = %q, want %q", got, want)
	}
}

// TestDefaultLocationReportsAnUnavailableUserCacheDirectory proves a failed
// lookup is a sanitized unavailable cache rather than a working-directory
// fallback.
func TestDefaultLocationReportsAnUnavailableUserCacheDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Windows user cache directory is not environment-clearable")
	}
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")
	if _, ok := os.LookupEnv("HOME"); ok {
		_ = os.Unsetenv("HOME")
	}

	if _, err := DefaultLocation(); err == nil {
		t.Fatal("DefaultLocation() error = nil, want an unavailable cache error")
	}
}
