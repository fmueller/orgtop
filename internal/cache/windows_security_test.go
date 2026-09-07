package cache

import (
	"errors"
	"strings"
	"testing"
)

// userSID stands in for the account OrgTop runs as. It is an ordinary domain
// user RID, so nothing about it is well-known or implicitly trusted.
const userSID = "S-1-5-21-1004336348-1177238915-682003330-1001"

// foreignSID is a second ordinary account on the same machine: the account
// RG-005 refuses to share a cache directory with.
const foreignSID = "S-1-5-21-1004336348-1177238915-682003330-1002"

// allowFullControl builds the ACE an inherited cache ACL carries for a
// principal Windows grants full control to.
func allowFullControl(sid string) windowsACE {
	return windowsACE{aceType: accessAllowedACEType, sid: sid, mask: windowsWriteAccess | 0x1}
}

// ownedSecurity is the security state a correctly provisioned Windows cache
// directory has: owned by the current user, with an inherited ACL granting full
// control to that user and to the two machine-local principals every
// %LOCALAPPDATA% ACL carries.
func ownedSecurity() windowsSecurity {
	return windowsSecurity{
		owner:   userSID,
		hasDACL: true,
		dacl: []windowsACE{
			allowFullControl(userSID),
			allowFullControl("S-1-5-18"),
			allowFullControl("S-1-5-32-544"),
		},
	}
}

// securityWithOwner is that same state with the owner replaced. Taking
// ownership on behalf of another account needs a privilege no test holds, so
// the owner recorded in the descriptor is varied instead.
func securityWithOwner(owner string) windowsSecurity {
	security := ownedSecurity()
	security.owner = owner
	return security
}

// securityGranting is that same state with one more entry in its discretionary
// ACL, so each case below states only the grant it puts under examination.
func securityGranting(entry windowsACE) windowsSecurity {
	security := ownedSecurity()
	security.dacl = append(security.dacl, entry)
	return security
}

// TestCheckWindowsSecurityAcceptsAnOwnedCacheDirectory proves the guard does
// not refuse an ordinary Windows launch. RG-005 degrades to no cache on an
// ownership failure, so a false positive here silently disables enrichment
// reuse on every Windows host rather than failing loudly.
func TestCheckWindowsSecurityAcceptsAnOwnedCacheDirectory(t *testing.T) {
	t.Parallel()

	if err := checkWindowsSecurity("orgtop", ownedSecurity(), userSID); err != nil {
		t.Errorf("checkWindowsSecurity() error = %v, want nil for an owned cache directory", err)
	}
}

// TestCheckWindowsSecurityRejectsForeignOwnershipAndWriteAccess covers the
// Windows half of RG-005's location, ownership, and permission contract: the
// object must belong to the current user and grant no other account write
// access, and every refusal is ErrUnavailable so the launch degrades rather
// than failing.
func TestCheckWindowsSecurityRejectsForeignOwnershipAndWriteAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		security windowsSecurity
		want     string
	}{
		{
			name:     "owned by another account",
			security: securityWithOwner(foreignSID),
			want:     "owned by another user",
		},
		{
			name:     "no discretionary access control list",
			security: windowsSecurity{owner: userSID},
			want:     "no discretionary access control list",
		},
		{
			name:     "inherited ACL grants another user write access",
			security: securityGranting(allowFullControl(foreignSID)),
			want:     "grants write access to another user",
		},
		{
			name:     "unattributed owner",
			security: securityWithOwner(""),
			want:     "owned by another user",
		},
		{
			name:     "unattributed grantee",
			security: securityGranting(allowFullControl("")),
			want:     "grants write access to another user",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := checkWindowsSecurity("orgtop", test.security, userSID)
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("checkWindowsSecurity() error = %v, want ErrUnavailable", err)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("checkWindowsSecurity() error = %q, want it to mention %q", err, test.want)
			}
			if !strings.Contains(err.Error(), "orgtop") {
				t.Errorf("checkWindowsSecurity() error = %q, want it to name the rejected path", err)
			}
		})
	}
}

// TestCheckWindowsSecurityIgnoresEntriesThatGrantNoWriteAccess keeps the guard
// from refusing the ordinary case for the wrong reason. An inherited ACL
// routinely carries entries that cannot change cache bytes, and refusing them
// would degrade every Windows launch.
func TestCheckWindowsSecurityIgnoresEntriesThatGrantNoWriteAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		entry windowsACE
	}{
		{
			// FILE_READ_DATA, FILE_READ_EA, FILE_EXECUTE and FILE_READ_ATTRIBUTES:
			// the rights an inherited ACL routinely grants that read the object
			// without being able to change a byte of it.
			name:  "read-only allow entry",
			entry: windowsACE{aceType: accessAllowedACEType, sid: foreignSID, mask: 0x1 | 0x8 | 0x20 | 0x80},
		},
		{
			name:  "deny entry",
			entry: windowsACE{aceType: accessDeniedACEType, sid: foreignSID, mask: windowsWriteAccess},
		},
		{
			name:  "inherit-only entry",
			entry: windowsACE{aceType: accessAllowedACEType, inheritOnly: true, sid: foreignSID, mask: windowsWriteAccess},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if err := checkWindowsSecurity("orgtop", securityGranting(test.entry), userSID); err != nil {
				t.Errorf("checkWindowsSecurity() error = %v, want nil", err)
			}
		})
	}
}

// TestCheckWindowsSecurityAcceptsTheMachineLocalPrincipals pins which SIDs are
// not "another user". LocalSystem and BUILTIN\Administrators appear in every
// inherited %LOCALAPPDATA% ACL and can already take ownership of any object, so
// treating them as foreign would refuse every Windows launch while protecting
// nothing. An elevated launch also stamps BUILTIN\Administrators as the owner
// of what it creates.
func TestCheckWindowsSecurityAcceptsTheMachineLocalPrincipals(t *testing.T) {
	t.Parallel()

	for _, sid := range []string{"S-1-5-18", "S-1-5-32-544"} {
		t.Run(sid, func(t *testing.T) {
			t.Parallel()

			if err := checkWindowsSecurity("orgtop", securityWithOwner(sid), userSID); err != nil {
				t.Errorf("checkWindowsSecurity() error = %v, want nil for owner %s", err, sid)
			}
		})
	}
}

// TestCheckWindowsSecurityRejectsEveryWriteRightIndividually proves the write
// mask is not narrower than the contract. Each right below lets another account
// change the cache bytes or take control of the object holding them, so any one
// of them alone must refuse.
func TestCheckWindowsSecurityRejectsEveryWriteRightIndividually(t *testing.T) {
	t.Parallel()

	rights := map[string]uint32{
		"FILE_WRITE_DATA":       fileWriteData,
		"FILE_APPEND_DATA":      fileAppendData,
		"FILE_WRITE_EA":         fileWriteEA,
		"FILE_WRITE_ATTRIBUTES": fileWriteAttributes,
		"FILE_DELETE_CHILD":     fileDeleteChild,
		"DELETE":                standardDelete,
		"WRITE_DAC":             standardWriteDAC,
		"WRITE_OWNER":           standardWriteOwner,
		"GENERIC_ALL":           genericAll,
		"GENERIC_WRITE":         genericWrite,
	}

	for name, mask := range rights {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			entry := windowsACE{aceType: accessAllowedACEType, sid: foreignSID, mask: mask}
			if err := checkWindowsSecurity("orgtop", securityGranting(entry), userSID); !errors.Is(err, ErrUnavailable) {
				t.Errorf("checkWindowsSecurity() error = %v, want ErrUnavailable for %s", err, name)
			}
		})
	}
}

// TestCheckWindowsSecurityRejectsAnUndecodableEntry keeps the guard from
// guessing about an entry it cannot read. Only ACCESS_ALLOWED_ACE and
// ACCESS_DENIED_ACE put the trustee SID directly after the access mask; an
// object ACE inserts a flag word and two GUIDs first, so decoding one as an
// allow entry reads the wrong bytes as the SID. An entry whose grant cannot be
// established is refused rather than assumed harmless.
func TestCheckWindowsSecurityRejectsAnUndecodableEntry(t *testing.T) {
	t.Parallel()

	// ACCESS_ALLOWED_OBJECT_ACE_TYPE.
	entry := windowsACE{aceType: 5, sid: userSID, mask: windowsWriteAccess}
	err := checkWindowsSecurity("orgtop", securityGranting(entry), userSID)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("checkWindowsSecurity() error = %v, want ErrUnavailable for an undecodable entry", err)
	}
	if !strings.Contains(err.Error(), "cannot be decoded") {
		t.Errorf("checkWindowsSecurity() error = %q, want it to say the entry cannot be decoded", err)
	}
}
