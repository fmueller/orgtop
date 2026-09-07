package cache

import "fmt"

// The Windows access rights that let a principal change the bytes of a cache
// object, delete a cache file out of the directory holding it, or take enough
// control of either to do so afterwards. They are the
// winnt.h values; this file carries no build constraint so the decision below
// stays testable on every host, and golang.org/x/sys/windows — which defines
// them — builds only on Windows.
const (
	fileWriteData       = 0x00000002
	fileAppendData      = 0x00000004
	fileWriteEA         = 0x00000010
	fileDeleteChild     = 0x00000040
	fileWriteAttributes = 0x00000100
	standardDelete      = 0x00010000
	standardWriteDAC    = 0x00040000
	standardWriteOwner  = 0x00080000
	genericAll          = 0x10000000
	genericWrite        = 0x40000000
)

// windowsWriteAccess is the whole of that set. An access mask sharing any bit
// with it grants write access in RG-005's sense.
const windowsWriteAccess = fileWriteData | fileAppendData | fileWriteEA |
	fileDeleteChild | fileWriteAttributes | standardDelete | standardWriteDAC |
	standardWriteOwner | genericAll | genericWrite

// The two ACE types that place the trustee SID directly after the access mask,
// which is the layout the decoding this file reads depends on. An object ACE
// inserts a flag word and two GUIDs first; it belongs to directory-service
// objects rather than files, and the guard refuses one instead of decoding the
// wrong bytes as a SID.
const (
	accessAllowedACEType = 0
	accessDeniedACEType  = 1
)

// trustedWindowsSIDs are the machine-local principals that are not "another
// user". LocalSystem (S-1-5-18) and BUILTIN\Administrators (S-1-5-32-544)
// appear in every inherited %LOCALAPPDATA% ACL and can already take ownership
// of any object on the machine, so refusing them would degrade every Windows
// launch while protecting nothing. An elevated launch also stamps
// BUILTIN\Administrators as the owner of what it creates.
var trustedWindowsSIDs = map[string]struct{}{
	"S-1-5-18":     {},
	"S-1-5-32-544": {},
}

// windowsACE is one decoded discretionary access control entry.
type windowsACE struct {
	// aceType is the raw ACE_HEADER type. A denial only ever removes access and
	// so can never make an object foreign-writable; anything that is neither an
	// allowance nor a denial is not decodable as this entry.
	aceType uint8
	// inheritOnly marks an entry that propagates to children without applying
	// to this object. A residual CREATOR OWNER template arrives this way.
	inheritOnly bool
	sid         string
	mask        uint32
}

// windowsSecurity is the decoded owner and discretionary ACL of one open
// object, in the platform-independent form the decision below reads.
type windowsSecurity struct {
	owner string
	// hasDACL is false both when the object carries no DACL and when it carries
	// a NULL one. Windows treats both as unrestricted access for everyone, so
	// the guard refuses them identically.
	hasDACL bool
	dacl    []windowsACE
}

// checkWindowsSecurity is the Windows half of RG-005's ownership and permission
// contract: a cache object must belong to the account OrgTop runs as and must
// grant no other account write access under its inherited ACL. Every refusal is
// ErrUnavailable, so a foreign cache directory degrades the launch to no cache
// rather than being repaired or failing it.
func checkWindowsSecurity(path string, security windowsSecurity, userSID string) error {
	if !windowsSelfOrTrusted(security.owner, userSID) {
		return fmt.Errorf("%w: %q is owned by another user (SID %q)", ErrUnavailable, path, security.owner)
	}
	if !security.hasDACL {
		return fmt.Errorf("%w: %q has no discretionary access control list, so every account may write to it", ErrUnavailable, path)
	}
	for _, entry := range security.dacl {
		if entry.aceType == accessDeniedACEType || entry.inheritOnly {
			continue
		}
		if entry.aceType != accessAllowedACEType {
			return fmt.Errorf("%w: an access control entry of %q cannot be decoded (ACE type %d)", ErrUnavailable, path, entry.aceType)
		}
		if entry.mask&windowsWriteAccess == 0 {
			continue
		}
		if windowsSelfOrTrusted(entry.sid, userSID) {
			continue
		}
		return fmt.Errorf("%w: %q grants write access to another user (SID %q)", ErrUnavailable, path, entry.sid)
	}
	return nil
}

// windowsSelfOrTrusted reports whether one SID is the current account or a
// machine-local principal. An empty SID is never either: it means the object
// carried no attributable owner or grantee, which is not evidence of ownership.
func windowsSelfOrTrusted(sid, userSID string) bool {
	if sid == "" {
		return false
	}
	if sid == userSID {
		return true
	}
	_, trusted := trustedWindowsSIDs[sid]
	return trusted
}
