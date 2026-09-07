//go:build windows

package cache

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// checkWindowsAccess applies RG-005's Windows ownership and permission contract
// to one already-open cache object. The caller opened it, so the state checked
// is the state of that object rather than whatever its path resolves to next.
func checkWindowsAccess(name string, file *os.File) error {
	user, err := currentWindowsUser()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	security, err := describeWindowsSecurity(file)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return checkWindowsSecurity(name, security, user)
}

// currentWindowsUser reports the SID of the account this process runs as. The
// process token is a pseudo-handle, so it is never closed.
func currentWindowsUser() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

// openDirectoryNoFollow opens the cache directory itself, without following a
// reparse point and without asking for more than the right to read its security
// state. Go's os.Open follows a junction, so a check reading the resulting
// handle would describe the target rather than the object at the path.
func openDirectoryNoFollow(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		name,
		windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}

// describeHandleAttributes reports one open object's file attributes, so the
// type checks read the object the handle already refers to rather than what the
// path resolved to at some earlier moment.
func describeHandleAttributes(file *os.File) (uint32, error) {
	var information windows.ByHandleFileInformation
	err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &information)
	runtime.KeepAlive(file)
	if err != nil {
		return 0, err
	}
	return information.FileAttributes, nil
}

// describeWindowsSecurity reads one open object's owner and discretionary ACL
// through its handle and decodes them into the platform-independent form
// checkWindowsSecurity reads. The caller must keep file reachable — an open
// *os.File that a deferred Close still refers to — for the whole call, because
// a finalized one closes the handle read here.
func describeWindowsSecurity(file *os.File) (windowsSecurity, error) {
	descriptor, err := windows.GetSecurityInfo(
		windows.Handle(file.Fd()),
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	runtime.KeepAlive(file)
	if err != nil {
		return windowsSecurity{}, err
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return windowsSecurity{}, err
	}
	security := windowsSecurity{owner: owner.String()}

	// An absent DACL and a present NULL one both mean unrestricted access, and
	// both leave hasDACL false so the guard refuses them the same way.
	dacl, _, err := descriptor.DACL()
	if errors.Is(err, windows.ERROR_OBJECT_NOT_FOUND) {
		return security, nil
	}
	if err != nil {
		return windowsSecurity{}, err
	}
	if dacl == nil {
		return security, nil
	}

	security.hasDACL = true
	security.dacl = make([]windowsACE, 0, dacl.AceCount)
	for index := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			return windowsSecurity{}, err
		}
		// Only an allowance or a denial places the trustee SID directly after
		// the mask. checkWindowsSecurity refuses any other type, so no SID is
		// decoded out of an entry whose layout is unknown.
		entry := windowsACE{
			aceType:     ace.Header.AceType,
			inheritOnly: ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0,
		}
		if entry.aceType == accessAllowedACEType || entry.aceType == accessDeniedACEType {
			entry.sid = (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
			entry.mask = uint32(ace.Mask)
		}
		security.dacl = append(security.dacl, entry)
	}
	return security, nil
}
