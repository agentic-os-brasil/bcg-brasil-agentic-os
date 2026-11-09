//go:build windows

package ownerctx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func ownerPublicRegistry(t *testing.T, path string) func() {
	t.Helper()
	previous, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	original, _, err := previous.DACL()
	if err != nil {
		t.Fatal(err)
	}
	public, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := public.DACL()
	if err != nil {
		t.Fatal(err)
	}
	set := func(acl *windows.ACL) {
		t.Helper()
		if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
			t.Fatal(err)
		}
	}
	set(acl)
	return func() { set(original) }
}

func ownerDirectoryAlias(t *testing.T, alias, target string) {
	t.Helper()
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	if out, err := exec.Command(shell, "/c", "mklink", "/J", alias, target).CombinedOutput(); err != nil {
		t.Fatalf("required ordinary-user junction fixture unavailable: %s %v", out, err)
	}
}

func ownerSecuritySnapshot(t *testing.T, path string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return sd.String()
}
