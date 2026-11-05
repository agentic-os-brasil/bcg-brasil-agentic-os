//go:build windows

package product_test

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

// This deliberately small native probe precedes broader backend qualification.
// A failure is a real unsupported native operation, never a no-op success.
func TestWindowsDirectoryDurabilityPilot(t *testing.T) {
	if os.Getenv("MAESTRO_WINDOWS_CORE_PROOF") != "1" {
		t.Skip("dedicated native ordinary-user driver required")
	}
	token := windows.GetCurrentProcessToken()
	admin, e := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if e != nil {
		t.Fatal(e)
	}
	member, e := windows.Token(0).IsMember(admin)
	if e != nil {
		t.Fatal(e)
	}
	var elevation, returned uint32
	if e = windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevation)), uint32(unsafe.Sizeof(elevation)), &returned); e != nil || returned != 4 || elevation != 0 || member {
		t.Fatalf("actual ordinary token required: elevation=%d admin=%v error=%v", elevation, member, e)
	}
	root, e := os.MkdirTemp(os.Getenv("MAESTRO_WINDOWS_CORE_PROOF_ROOT"), "durability-")
	if e != nil {
		t.Fatal(e)
	}
	d, e := winprivate.Open(root, false)
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	if e = d.Sync(); e != nil {
		t.Fatalf("NATIVE_DIRECTORY_DURABILITY_UNSUPPORTED: %v", e)
	}
	t.Log("native_directory_flush standard_user=true held_private_directory=true succeeded=true")
	child, e := d.Child("created", true, true)
	if e != nil {
		t.Fatal(e)
	}
	defer child.Close()
	if e = child.Publish("state.json", []byte("first"), true); e != nil {
		t.Fatal(e)
	}
	if e = child.Publish("state.json", []byte("second"), false); e != nil {
		t.Fatal(e)
	}
	b, e := winprivate.ReadFile(filepath.Join(root, "created", "state.json"), 64)
	if e != nil || string(b) != "second" {
		t.Fatalf("flushed native publication readback=%q error=%v", b, e)
	}
	if e = child.Remove("state.json", false); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Lstat(filepath.Join(root, "created", "state.json")); !os.IsNotExist(e) {
		t.Fatalf("flushed native absence not visible: %v", e)
	}
	t.Log("native_directory_publication exclusive_replace_absence=true durability_errors_preserved=true")
}
