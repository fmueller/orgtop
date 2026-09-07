//go:build windows

package cache

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// setDACL replaces one object's discretionary ACL with the SDDL below, marking
// it protected so no inherited entry is merged back in. It is how a test puts a
// cache path into a security state Windows would otherwise only reach through
// an administrator or a second account.
func setDACL(t *testing.T, path, sddl string) {
	t.Helper()

	descriptor, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		t.Fatalf("SecurityDescriptorFromString(%q) error = %v", sddl, err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatalf("DACL() error = %v", err)
	}
	object, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	defer func() { _ = object.Close() }()

	err = windows.SetSecurityInfo(
		windows.Handle(object.Fd()),
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
	if err != nil {
		t.Fatalf("SetSecurityInfo(%q) error = %v", path, err)
	}
}

// describePath reads one cache path's real owner and ACL the way the guard
// does, through a handle to the object itself.
func describePath(t *testing.T, path string) windowsSecurity {
	t.Helper()

	object, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	defer func() { _ = object.Close() }()

	security, err := describeWindowsSecurity(object)
	if err != nil {
		t.Fatalf("describeWindowsSecurity(%q) error = %v", path, err)
	}
	return security
}

// mustOpen opens one cache path for a check that needs the handle itself.
func mustOpen(t *testing.T, path string) *os.File {
	t.Helper()

	object, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	t.Cleanup(func() { _ = object.Close() })
	return object
}

// TestWindowsAcceptsACacheDirectoryThisAccountCreated proves the decoding and
// the decision agree on the ordinary case. RG-005 degrades to no cache on an
// ownership failure, so a guard that misreads a real inherited ACL would
// silently disable enrichment reuse on every Windows host.
func TestWindowsAcceptsACacheDirectoryThisAccountCreated(t *testing.T) {
	t.Parallel()

	directory := stagedLocation(t).Directory()
	if err := checkWindowsAccess(directory, mustOpen(t, directory)); err != nil {
		t.Errorf("checkWindowsAccess() error = %v, want nil for a directory this account created", err)
	}
}

// TestWindowsRejectsACacheDirectoryOwnedByAnotherAccount covers the owner half
// of the contract against a real security descriptor. Taking ownership on
// behalf of another account needs a privilege no test holds, so the account
// OrgTop runs as is varied instead: the directory is unchanged, and the
// question asked of it is the one a foreign-owned directory would answer.
func TestWindowsRejectsACacheDirectoryOwnedByAnotherAccount(t *testing.T) {
	t.Parallel()

	directory := stagedLocation(t).Directory()
	security := describePath(t, directory)
	if security.owner == "" {
		t.Fatal("describeWindowsSecurity() read no owner from a directory this account created")
	}
	if security.owner == foreignSID {
		t.Fatalf("the stand-in foreign SID %s is this account", foreignSID)
	}

	err := checkWindowsSecurity(directory, security, foreignSID)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("checkWindowsSecurity() error = %v, want ErrUnavailable for a foreign-owned directory", err)
	}
	if _, statErr := os.Lstat(directory); statErr != nil {
		t.Errorf("a rejected cache directory must be preserved, not repaired, lstat error = %v", statErr)
	}
}

// TestWindowsRejectsACacheDirectoryAnotherUserMayWrite covers the ACL half for
// the directory against a real object and a real handle. BUILTIN\Users is
// granted full control of the cache directory, which includes deleting the
// files inside it, so the launch must degrade rather than open a store another
// account can empty.
func TestWindowsRejectsACacheDirectoryAnotherUserMayWrite(t *testing.T) {
	t.Parallel()

	location := stagedLocation(t)
	user, err := currentWindowsUser()
	if err != nil {
		t.Fatalf("currentWindowsUser() error = %v", err)
	}
	// The current account keeps full control, so the refusal below is the
	// guard's decision and not an access denial from opening the directory.
	setDACL(t, location.Directory(), "D:(A;;FA;;;"+user+")(A;;FA;;;BU)")

	if _, err := Open(location); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Open() error = %v, want ErrUnavailable for a cache directory another user may write", err)
	}
}

// TestWindowsRejectsACacheFileAnotherUserMayWrite covers the ACL half against a
// real object: BUILTIN\Users is granted full control of the canonical database,
// which is exactly the inherited state RG-005 refuses. Open must degrade to
// ErrUnavailable rather than trust or repair it.
func TestWindowsRejectsACacheFileAnotherUserMayWrite(t *testing.T) {
	t.Parallel()

	store, location := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	user, err := currentWindowsUser()
	if err != nil {
		t.Fatalf("currentWindowsUser() error = %v", err)
	}
	// The current account keeps full control, so the refusal below is the
	// guard's decision and not an access denial from opening the file.
	setDACL(t, location.Database(), "D:(A;;FA;;;"+user+")(A;;FA;;;BU)")

	if _, err := Open(location); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Open() error = %v, want ErrUnavailable for a cache file another user may write", err)
	}
}
