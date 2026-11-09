//go:build windows

package zipmigration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
)

func externalPlatformAccount(t *testing.T) {
	t.Helper()
	if os.Getenv("MAESTRO_WINDOWS_CORE_PROOF") != "1" {
		t.Skip("dedicated ordinary-account Windows driver required")
	}
	admin, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		t.Fatal(err)
	}
	member, err := windows.Token(0).IsMember(admin)
	if err != nil {
		t.Fatal(err)
	}
	var elevated, returned uint32
	err = windows.GetTokenInformation(windows.GetCurrentProcessToken(), windows.TokenElevation, (*byte)(unsafe.Pointer(&elevated)), 4, &returned)
	if err != nil || returned != 4 || elevated != 0 || member {
		t.Fatal("actual ordinary Windows account required", err, elevated, member)
	}
}

func externalDirectoryAlias(t *testing.T, alias, target string) {
	t.Helper()
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	if out, err := exec.Command(shell, "/c", "mklink", "/J", alias, target).CombinedOutput(); err != nil {
		t.Fatalf("required ordinary-user junction fixture unavailable: %s %v", out, err)
	}
}

func externalPublicControl(t *testing.T, _ string, name string) func() {
	t.Helper()
	private, err := winprivate.OpenPinnedTree(name)
	if err != nil {
		t.Fatal("private control positive fixture unavailable", err)
	}
	if err := private.Close(); err != nil {
		t.Fatal(err)
	}
	const security = windows.OWNER_SECURITY_INFORMATION | windows.GROUP_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION
	previous, err := windows.GetNamedSecurityInfo(name, windows.SE_FILE_OBJECT, security)
	if err != nil {
		t.Fatal(err)
	}
	oldACL, _, err := previous.DACL()
	if err != nil || oldACL == nil {
		t.Fatal("public fixture requires a finite original DACL", err)
	}
	control, _, err := previous.Control()
	if err != nil {
		t.Fatal(err)
	}
	restoreFlags := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION)
	if control&windows.SE_DACL_PROTECTED != 0 {
		restoreFlags = windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION
	}
	t.Cleanup(func() {
		if err := windows.SetNamedSecurityInfo(name, windows.SE_FILE_OBJECT, restoreFlags, nil, nil, oldACL, nil); err != nil {
			t.Error("restore synthetic public control DACL", err)
		}
	})
	user, err := winprivate.CurrentUser()
	if err != nil {
		t.Fatal(err)
	}
	public, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := public.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err = windows.SetNamedSecurityInfo(name, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	if directory, err := winprivate.OpenPinnedTree(name); err == nil {
		_ = directory.Close()
		t.Fatal("required public-DACL refusal fixture was not established")
	}
	expected, err := windows.GetNamedSecurityInfo(name, windows.SE_FILE_OBJECT, security)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	return func() {
		t.Helper()
		observed, err := windows.GetNamedSecurityInfo(name, windows.SE_FILE_OBJECT, security)
		after, statErr := os.Stat(name)
		if err != nil || statErr != nil || observed.String() != expected.String() || !os.SameFile(identity, after) {
			t.Fatal("unsafe native control was repaired or replaced", err, statErr)
		}
	}
}

func externalPlatformIdentity(t *testing.T, path string) [3]uint64 {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal("case capability requires actual directory handle", err)
	}
	var info windows.ByHandleFileInformation
	infoErr := windows.GetFileInformationByHandle(handle, &info)
	closeErr := windows.CloseHandle(handle)
	if infoErr != nil || closeErr != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		t.Fatal("case identity is unavailable or reparsed", infoErr, closeErr)
	}
	return [3]uint64{uint64(info.VolumeSerialNumber), uint64(info.FileIndexHigh), uint64(info.FileIndexLow)}
}
