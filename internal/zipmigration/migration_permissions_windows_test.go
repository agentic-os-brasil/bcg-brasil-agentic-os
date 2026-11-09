//go:build windows

package zipmigration

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

func migrationDenyRead(t *testing.T, path string, directory bool) func() {
	t.Helper()
	restore := migrationFixtureDACL(t, path, windows.FILE_READ_DATA)
	var err error
	if directory {
		_, err = os.ReadDir(path)
	} else {
		_, err = os.ReadFile(path)
	}
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatal("required native read/list denial was not established", err)
	}
	return restore
}

func migrationDenyWrite(t *testing.T, path string) func() {
	t.Helper()
	// Directory add-file/add-subdirectory rights share the write/append bits.
	restore := migrationFixtureDACL(t, path, windows.FILE_WRITE_DATA|windows.FILE_APPEND_DATA)
	probe := filepath.Join(path, ".native-denied-write-probe")
	file, err := os.OpenFile(probe, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		_ = file.Close()
		_ = os.Remove(probe)
	}
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatal("required native create/write denial was not established", err)
	}
	return restore
}

func migrationFixtureDACL(t *testing.T, path string, denied uint32) func() {
	t.Helper()
	externalNativeAccount(t)
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.READ_CONTROL|windows.WRITE_DAC|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal("open actual permission fixture", err)
	}
	const security = windows.OWNER_SECURITY_INFORMATION | windows.GROUP_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION
	original, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, security)
	if err != nil {
		_ = windows.CloseHandle(handle)
		t.Fatal(err)
	}
	originalText := original.String()
	oldACL, _, err := original.DACL()
	if err != nil || oldACL == nil {
		_ = windows.CloseHandle(handle)
		t.Fatal("permission fixture requires a finite original DACL", err)
	}
	control, _, err := original.Control()
	if err != nil {
		_ = windows.CloseHandle(handle)
		t.Fatal(err)
	}
	restoreFlags := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION)
	if control&windows.SE_DACL_PROTECTED != 0 {
		restoreFlags = windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		defer func() {
			if err := windows.CloseHandle(handle); err != nil {
				t.Error("close restored permission fixture", err)
			}
		}()
		if err := windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, restoreFlags, nil, nil, oldACL, nil); err != nil {
			t.Fatal("restore same-handle original DACL", err)
		}
		after, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, security)
		if err != nil || after.String() != originalText {
			t.Fatal("permission fixture failed exact security restoration", err)
		}
	}
	t.Cleanup(restore)
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	var pin runtime.Pinner
	pin.Pin(user.User.Sid)
	defer pin.Unpin()
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{AccessPermissions: windows.ACCESS_MASK(denied), AccessMode: windows.DENY_ACCESS, Inheritance: windows.NO_INHERITANCE, Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_USER, TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid)}}}, oldACL)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal("set actual deny DACL", err)
	}
	return restore
}
