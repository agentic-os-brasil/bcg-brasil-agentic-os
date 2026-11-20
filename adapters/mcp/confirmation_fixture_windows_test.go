//go:build windows

package product

import (
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
)

func prepareLocalReviewFixtureRoot(t *testing.T, root string) {
	t.Helper()
	user, err := winprivate.CurrentUser()
	if err != nil {
		t.Fatal(err)
	}
	// Elevated hosted temp roots can default to the Administrators owner.
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	flags := windows.SECURITY_INFORMATION(windows.OWNER_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
	if err := windows.SetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, flags, user, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
}
