//go:build windows

package scheduler_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unsafe"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/sys/windows"
)

// The actual native positive serialization/CAS test runs on Windows too. This
// separate refusal witness supplies an independently verified unsafe root.
func TestPrivateAtomicUpdateWindowsUnsafeRootPreservesExistingState(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "state.json")
	before := []byte("existing-private-state")
	if err := scheduler.WriteNewPrivateFile(path, before); err != nil {
		t.Fatal(err)
	}
	control, err := scheduler.ReadPrivateStateFile(path, 128)
	if err != nil || !bytes.Equal(control, before) {
		t.Fatal("safe native fixture was not admitted before its unsafe DACL", err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		t.Fatal("actual native user unavailable", err)
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;;FR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	security, err := windows.GetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := security.Owner()
	if err != nil || owner == nil || !owner.Equals(user.User.Sid) {
		t.Fatal("unsafe root does not belong to the actual user", err)
	}
	protection, _, err := security.Control()
	if err != nil || protection&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("unsafe native root lacks its protected fixture DACL", err)
	}
	nativeACL, _, err := security.DACL()
	if err != nil || nativeACL == nil {
		t.Fatal("unsafe native DACL unavailable", err)
	}
	worldRead := false
	for index := uint16(0); index < nativeACL.AceCount; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(nativeACL, uint32(index), &ace); err != nil || ace == nil {
			t.Fatal(err)
		}
		principal := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		worldRead = worldRead || (ace.Header.AceType == windows.ACCESS_ALLOWED_ACE_TYPE && ace.Header.AceFlags&windows.INHERIT_ONLY_ACE == 0 && ace.Mask&windows.FILE_GENERIC_READ == windows.FILE_GENERIC_READ && principal.IsWellKnown(windows.WinWorldSid))
	}
	if !worldRead {
		t.Fatal("unsafe fixture omitted its effective world-read grant")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	called := false
	err = scheduler.UpdatePrivateFile(path, 128, func([]byte) ([]byte, error) { called = true; return []byte("must-not-replace"), nil })
	if err == nil || called {
		t.Fatalf("unsafe native root entered mutation: callback=%t error=%v", called, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("unsafe refusal changed source bytes: %q %v", after, err)
	}
	afterEntries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	afterNames := make([]string, 0, len(afterEntries))
	for _, entry := range afterEntries {
		afterNames = append(afterNames, entry.Name())
	}
	afterSecurity, err := windows.GetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || afterSecurity.String() != security.String() || !reflect.DeepEqual(names, afterNames) {
		t.Fatal("unsafe refusal repaired security or created a lock/file", err)
	}
}
