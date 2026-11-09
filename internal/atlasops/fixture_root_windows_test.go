//go:build windows

package atlasops

import (
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
)

func atlasTestDataRoot(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	user, err := winprivate.CurrentUser()
	if err != nil {
		t.Fatal(err)
	}
	// Elevated hosted temp roots can default to the Administrators owner.
	// Initialize only this new synthetic subtree with the actual caller SID.
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	flags := windows.SECURITY_INFORMATION(windows.OWNER_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
	if err := windows.SetNamedSecurityInfo(base, windows.SE_FILE_OBJECT, flags, user, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "local", "BCGOS")
	directory, err := winprivate.Fresh(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := directory.Close(); err != nil {
		t.Fatal(err)
	}
	for _, selected := range []string{base, path} {
		actual, err := windows.GetNamedSecurityInfo(selected, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		owner, _, err := actual.Owner()
		if err != nil || !owner.Equals(user) {
			t.Fatal("fixture does not retain the actual caller owner", err)
		}
		control, _, err := actual.Control()
		if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
			t.Fatal("fixture lacks independently observed protected DACL", err)
		}
	}
	return path
}
