//go:build windows

package product_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
)

func windowsPrivateOrdinaryUser(t *testing.T) *windows.SID {
	t.Helper()
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
	user, e := token.GetTokenUser()
	if e != nil || user == nil || user.User.Sid == nil {
		t.Fatal("actual user SID unavailable", e)
	}
	return user.User.Sid
}

func windowsPrivateRoot(t *testing.T) string {
	t.Helper()
	root, e := os.MkdirTemp(os.Getenv("MAESTRO_WINDOWS_CORE_PROOF_ROOT"), "private-")
	if e != nil {
		t.Fatal(e)
	}
	return root
}
func windowsFixtureDACL(t *testing.T, path, sddl string) {
	t.Helper()
	sd, e := windows.SecurityDescriptorFromString(sddl)
	if e != nil {
		t.Fatal(e)
	}
	acl, _, e := sd.DACL()
	if e != nil {
		t.Fatal(e)
	}
	if e = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); e != nil {
		t.Fatal(e)
	}
}
func windowsJunction(t *testing.T, alias, target string) {
	t.Helper()
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	out, e := exec.Command(shell, "/c", "mklink", "/J", alias, target).CombinedOutput()
	if e != nil {
		t.Fatalf("actual ordinary-user junction fixture unavailable: %s %v", out, e)
	}
}

// Public private-file operations are checked against independent native ACL and
// filesystem mutations. All fixtures are synthetic children of the driver root.
func TestWindowsPrivateStatePilot(t *testing.T) {
	user := windowsPrivateOrdinaryUser(t)
	t.Run("creation_has_actual_protected_owner_acl", func(t *testing.T) {
		root := windowsPrivateRoot(t)
		d, e := winprivate.Open(root, false)
		if e != nil {
			t.Fatal(e)
		}
		defer d.Close()
		child, e := d.Child("private", true, true)
		if e != nil {
			t.Fatal(e)
		}
		defer child.Close()
		if e = child.Publish("state", []byte("private literal"), true); e != nil {
			t.Fatal(e)
		}
		for _, path := range []string{filepath.Join(root, "private"), filepath.Join(root, "private", "state")} {
			sd, e := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
			if e != nil {
				t.Fatal(e)
			}
			owner, _, e := sd.Owner()
			if e != nil || owner == nil || !owner.Equals(user) {
				t.Fatal("created object does not belong to the actual kernel user", e)
			}
			control, _, e := sd.Control()
			if e != nil || control&windows.SE_DACL_PROTECTED == 0 {
				t.Fatal("creation inherited an unprotected descriptor", e)
			}
			acl, _, e := sd.DACL()
			if e != nil || acl == nil {
				t.Fatal("creation has no actual non-null DACL", e)
			}
			for index := uint16(0); index < acl.AceCount; index++ {
				var ace *windows.ACCESS_ALLOWED_ACE
				if e = windows.GetAce(acl, uint32(index), &ace); e != nil || ace == nil {
					t.Fatal("created ACE unavailable", e)
				}
				if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
					t.Fatal("unexpected created effective ACE type")
				}
				principal := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
				if !principal.Equals(user) && !principal.IsWellKnown(windows.WinLocalSystemSid) && !principal.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
					t.Fatal("created object grants a foreign principal")
				}
			}
		}
	})
	t.Run("unc_and_device_names_refuse_before_native_open", func(t *testing.T) {
		for _, path := range []string{`\\127.0.0.1\never-enrolled\state`, `\\?\C:\state`, `\\.\pipe\never-enrolled`} {
			if _, e := winprivate.Open(path, false); e == nil || !strings.Contains(e.Error(), "local drive") {
				t.Fatalf("nonlocal/device path reached private admission: %q %v", path, e)
			}
		}
	})
	t.Run("unowned_read_seam_does_not_grant_private_admission", func(t *testing.T) {
		path := os.Getenv("MAESTRO_WINDOWS_CORE_PROOF_ROOT")
		sd, e := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
		if e != nil {
			t.Fatal(e)
		}
		owner, _, e := sd.Owner()
		if e != nil || owner == nil || owner.Equals(user) {
			t.Fatal("driver foreign-owner fixture missing", e)
		}
		if e = scheduler.ValidateDirectory(path); e != nil {
			t.Fatal("ordinary unowned directory read was rejected", e)
		}
		if e = scheduler.ValidatePrivateDirectory(path); e == nil {
			t.Fatal("foreign owner acquired private admission")
		}
		f, e := scheduler.OpenRegularFile(os.Getenv("MAESTRO_WINDOWS_CORE_SERVER"))
		if e != nil {
			t.Fatal("unowned ordinary binary read was rejected", e)
		}
		f.Close()
	})
	for _, kind := range []string{"world-read", "null", "unknown-effective-ace"} {
		t.Run(kind, func(t *testing.T) {
			root := windowsPrivateRoot(t)
			path := filepath.Join(root, "state")
			if e := scheduler.PublishNewPrivateFile(path, []byte("preserve exact bytes")); e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "world-read":
				windowsFixtureDACL(t, path, "D:P(A;;FA;;;"+user.String()+")(A;;GR;;;WD)")
			case "null":
				if e := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, nil, nil); e != nil {
					t.Fatal(e)
				}
			case "unknown-effective-ace":
				windowsFixtureDACL(t, path, "D:P(A;;FA;;;"+user.String()+")(OA;;GR;;;"+user.String()+")")
			}
			if _, e := scheduler.ReadPrivateFile(path, 128); e == nil {
				t.Fatal("unsafe descriptor was accepted for private read")
			}
			if e := scheduler.ReplacePrivateFile(path, []byte("must not truncate")); e == nil {
				t.Fatal("unsafe descriptor was accepted for replacement")
			}
			b, e := os.ReadFile(path)
			if e != nil || string(b) != "preserve exact bytes" {
				t.Fatalf("refusal changed private bytes: %q %v", b, e)
			}
		})
	}
	t.Run("world_read_directory_is_not_repaired", func(t *testing.T) {
		root := windowsPrivateRoot(t)
		windowsFixtureDACL(t, root, "D:P(A;OICI;FA;;;"+user.String()+")(A;OICI;GR;;;WD)")
		before, e := windows.GetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if e != nil {
			t.Fatal(e)
		}
		if e = scheduler.EnsurePrivateDirectory(root); e == nil {
			t.Fatal("unsafe existing directory was adopted")
		}
		after, e := windows.GetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if e != nil || after.String() != before.String() {
			t.Fatal("unsafe existing directory was silently repaired", e)
		}
	})
	t.Run("hardlink_and_nonregular_refuse_before_truncation", func(t *testing.T) {
		root := windowsPrivateRoot(t)
		source, target := filepath.Join(root, "source"), filepath.Join(windowsPrivateRoot(t), "outside-hardlink")
		if e := scheduler.PublishNewPrivateFile(source, []byte("outside link exact literal")); e != nil {
			t.Fatal(e)
		}
		if e := os.Link(source, target); e != nil {
			t.Fatal("native hardlink fixture unavailable", e)
		}
		if e := scheduler.ReplacePrivateFile(source, []byte("must not truncate")); e == nil {
			t.Fatal("hardlink admitted")
		}
		b, e := os.ReadFile(target)
		if e != nil || string(b) != "outside link exact literal" {
			t.Fatal("hardlink refusal damaged original", e)
		}
		dir := filepath.Join(root, "directory-leaf")
		if e = os.Mkdir(dir, 0700); e != nil {
			t.Fatal(e)
		}
		if _, e = scheduler.ReadPrivateFile(dir, 128); e == nil {
			t.Fatal("directory admitted as regular leaf")
		}
		if _, e = scheduler.ReadPrivateFile(source+":stream", 128); e == nil {
			t.Fatal("alternate stream admitted as a private leaf")
		}
	})
	t.Run("offline_leaf_refuses_without_recall", func(t *testing.T) {
		root := windowsPrivateRoot(t)
		path := filepath.Join(root, "offline")
		if e := scheduler.PublishNewPrivateFile(path, []byte("offline exact literal")); e != nil {
			t.Fatal(e)
		}
		pointer, e := windows.UTF16PtrFromString(path)
		if e != nil {
			t.Fatal(e)
		}
		if e = windows.SetFileAttributes(pointer, windows.FILE_ATTRIBUTE_OFFLINE); e != nil {
			t.Fatal("native offline fixture unavailable", e)
		}
		if _, e = scheduler.ReadPrivateFile(path, 128); e == nil {
			t.Fatal("offline file admitted")
		}
		if e = windows.SetFileAttributes(pointer, windows.FILE_ATTRIBUTE_NORMAL); e != nil {
			t.Fatal(e)
		}
		b, e := os.ReadFile(path)
		if e != nil || string(b) != "offline exact literal" {
			t.Fatal("offline refusal damaged bytes", e)
		}
	})
	t.Run("junction_never_redirects_private_read_or_creation", func(t *testing.T) {
		root, outside := windowsPrivateRoot(t), windowsPrivateRoot(t)
		victim := filepath.Join(outside, "victim")
		if e := os.WriteFile(victim, []byte("outside immutable literal"), 0600); e != nil {
			t.Fatal(e)
		}
		alias := filepath.Join(root, "alias")
		windowsJunction(t, alias, outside)
		before := windowsCoreTree(t, outside)
		if _, e := scheduler.ReadPrivateFile(filepath.Join(alias, "victim"), 128); e == nil {
			t.Fatal("junction read admitted")
		}
		if e := scheduler.EnsurePrivateDirectory(filepath.Join(alias, "created")); e == nil {
			t.Fatal("junction creation admitted")
		}
		if !reflect.DeepEqual(before, windowsCoreTree(t, outside)) {
			t.Fatal("junction refusal changed outside bytes")
		}
	})
	t.Run("renamed_ancestor_cannot_redirect_held_publication", func(t *testing.T) {
		root, outside := windowsPrivateRoot(t), windowsPrivateRoot(t)
		directory := filepath.Join(root, "held")
		if e := winprivate.Mkdir(directory); e != nil {
			t.Fatal(e)
		}
		d, e := winprivate.Open(directory, false)
		if e != nil {
			t.Fatal(e)
		}
		defer d.Close()
		moved := root + "-moved"
		if e = os.Rename(root, moved); e != nil {
			t.Fatal("native ancestor-swap fixture unavailable", e)
		}
		windowsJunction(t, root, outside)
		before := windowsCoreTree(t, outside)
		if e = d.Publish("state", []byte("held object literal"), true); e != nil {
			t.Fatal(e)
		}
		b, e := os.ReadFile(filepath.Join(moved, "held", "state"))
		if e != nil || string(b) != "held object literal" {
			t.Fatal("publication lost its original held directory", e)
		}
		if !reflect.DeepEqual(before, windowsCoreTree(t, outside)) {
			t.Fatal("held publication wrote through replacement ancestor")
		}
	})
	t.Run("exclusive_cas_and_exact_absence_compensation", func(t *testing.T) {
		root := windowsPrivateRoot(t)
		path := filepath.Join(root, "state")
		if e := scheduler.PublishNewPrivateFile(path, []byte("first")); e != nil {
			t.Fatal(e)
		}
		if e := scheduler.PublishNewPrivateFile(path, []byte("clobber")); e == nil {
			t.Fatal("exclusive publication clobbered existing leaf")
		}
		veto := errors.New("synthetic publication veto")
		if e := scheduler.UpdatePrivateFileGuarded(path, 128, func(func() error) error { return veto }, func([]byte) ([]byte, error) { return []byte("unapproved"), nil }); !errors.Is(e, veto) {
			t.Fatal("publication guard veto not preserved", e)
		}
		guard := func(action func() error) error {
			if e := scheduler.WritePrivateFileAtomic(path, []byte("newer")); e != nil {
				return e
			}
			return action()
		}
		if e := scheduler.UpdatePrivateFileGuarded(path, 128, guard, func([]byte) ([]byte, error) { return []byte("stale"), nil }); e == nil {
			t.Fatal("stale publisher overwrote intervening bytes")
		}
		b, e := scheduler.ReadPrivateFile(path, 128)
		if e != nil || !bytes.Equal(b, []byte("newer")) {
			t.Fatal("later bytes were not preserved", e)
		}
		live := func(action func() error) error { return action() }
		if e = scheduler.RestorePrivateFileGuarded(path, 128, []byte("stale"), nil, live); e == nil {
			t.Fatal("compensation erased different parent")
		}
		if e = scheduler.RestorePrivateFileGuarded(path, 128, []byte("newer"), nil, live); e != nil {
			t.Fatal(e)
		}
		if _, e = os.Lstat(path); !os.IsNotExist(e) {
			t.Fatal("exact compensation did not restore absence", e)
		}
	})
	t.Run("native_lock_contention_preserves_one_writer", func(t *testing.T) {
		root := windowsPrivateRoot(t)
		path := filepath.Join(root, "transition.lock")
		e := scheduler.WithPrivateLock(path, func() error {
			ch := make(chan error, 1)
			go func() {
				ch <- scheduler.WithPrivateLock(path, func() error { return fmt.Errorf("second writer entered") })
			}()
			second := <-ch
			if !errors.Is(second, scheduler.ErrLeaseBusy) {
				return fmt.Errorf("native contention did not refuse: %w", second)
			}
			return nil
		})
		if e != nil {
			t.Fatal(e)
		}
		if e = scheduler.WithPrivateLock(path, func() error { return nil }); e != nil {
			t.Fatal("native lock was not released", e)
		}
	})
	t.Log("native_private_state exercised_cases=" + strings.Join([]string{"owner_acl", "unowned_read", "world_null_unknown_acl", "foreign_owner", "hardlink", "nonregular", "offline", "junction", "held_ancestor", "no_clobber", "cas", "absence", "contention"}, ","))
}
