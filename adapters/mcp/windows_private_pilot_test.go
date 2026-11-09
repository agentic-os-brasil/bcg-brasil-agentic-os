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

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"golang.org/x/sys/windows"
)

func TestWindowsWorkspaceInitializationMatchesElevation(t *testing.T) {
	base := physicalTempDir(t)
	before := treeSnapshot(t, base)
	got, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(base, "workspace"), DataRoot: filepath.Join(base, "data")})
	if windows.GetCurrentProcessToken().IsElevated() {
		if !errors.Is(err, userlevel.ErrElevatedProcess) || got.WorkspaceID != "" || !reflect.DeepEqual(before, treeSnapshot(t, base)) {
			t.Fatalf("elevated initialization did not refuse before effects: %+v %v", got, err)
		}
		t.Log("actual elevated host initialization refused without filesystem effects")
		return
	}
	if err != nil || got.State != "initialized" {
		t.Fatalf("ordinary initialization failed: %+v %v", got, err)
	}
}

func TestWindowsAdmissionRefused(t *testing.T) {
	base := physicalTempDir(t)
	user, err := winprivate.CurrentUser()
	if err != nil {
		t.Fatal(err)
	}
	// Elevated Windows temp directories may default to the Administrators owner.
	windowsFixtureDACL(t, base, "D:P(A;OICI;FA;;;"+user.String()+")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err := windows.SetNamedSecurityInfo(base, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION, user, nil, nil, nil); err != nil {
		t.Fatal("assign current owner to synthetic fixture base", err)
	}
	baseSecurity, err := windows.GetNamedSecurityInfo(base, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	baseOwner, _, err := baseSecurity.Owner()
	if err != nil || baseOwner == nil || !baseOwner.Equals(user) {
		t.Fatal("synthetic fixture base has the wrong native owner", err)
	}
	baseControl, _, err := baseSecurity.Control()
	if err != nil || baseControl&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("synthetic fixture base lacks a protected native DACL", err)
	}
	prod, home := filepath.Join(base, "product"), filepath.Join(base, "owner")
	for _, path := range []string{prod, home} {
		windowsPrivateFixtureDirectory(t, path)
	}
	windowsFixtureDACL(t, home, "D:P(A;OICI;FA;;;"+user.String()+")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;;FR;;;WD)")
	before := treeSnapshot(t, base)
	security, err := windows.GetNamedSecurityInfo(home, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := security.Owner()
	if err != nil || owner == nil || !owner.Equals(user) {
		t.Fatal("unsafe-DACL fixture has the wrong native owner", err)
	}
	control, _, err := security.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("unsafe-DACL fixture lacks native protection", err)
	}
	acl, _, err := security.DACL()
	if err != nil || acl == nil {
		t.Fatal("unsafe-DACL fixture is unavailable", err)
	}
	worldRead := false
	for index := uint16(0); index < acl.AceCount; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, uint32(index), &ace); err != nil || ace == nil {
			t.Fatal("inspect unsafe-DACL fixture ACE", err)
		}
		if ace.Header.AceType == windows.ACCESS_ALLOWED_ACE_TYPE && ace.Header.AceFlags&windows.INHERIT_ONLY_ACE == 0 && ace.Mask&windows.FILE_GENERIC_READ == windows.FILE_GENERIC_READ {
			principal := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			worldRead = worldRead || principal.IsWellKnown(windows.WinWorldSid)
		}
	}
	if !worldRead {
		t.Fatal("unsafe-DACL fixture omitted the unapproved world-read grant")
	}
	lookup := func(string) (string, bool) { return "", false }
	state, err := datahome.InitializeFresh(datahome.Options{ProductRoot: prod, UserHome: home, LookupEnv: lookup})
	if err == nil || state.Writable || state.Diagnostic != "user-home-invalid" {
		t.Fatalf("unsafe Windows home admitted: %+v %v", state, err)
	}
	if _, err := product.Open(product.Options{ProductRoot: prod, UserHome: home, WorkspacePath: filepath.Join(base, "workspace"), LookupEnv: lookup}); err == nil {
		t.Fatal("unsafe Windows product store admitted")
	}
	afterSecurity, err := windows.GetNamedSecurityInfo(home, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || security.String() != afterSecurity.String() || !reflect.DeepEqual(before, treeSnapshot(t, base)) {
		t.Fatal("refused Windows admission changed existing bytes, modes or native security", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".maestro")); !os.IsNotExist(err) {
		t.Fatal("refused bootstrap created state")
	}
}

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
				// Microsoft documents that OA without either GUID is converted to A.
				// Inspect the descriptor actually stored by Windows before classifying it.
				windowsFixtureDACL(t, path, "D:P(A;;FA;;;"+user.String()+")(OA;;GR;;;"+user.String()+")")
				for _, ace := range windowsNativeFixtureACEs(t, path) {
					if ace.kind != windows.ACCESS_ALLOWED_ACE_TYPE {
						t.Fatal("OA without GUID was not normalized to ordinary allow")
					}
				}
				normalized, e := scheduler.ReadPrivateFile(path, 128)
				if e != nil || string(normalized) != "preserve exact bytes" {
					t.Fatal("native-normalized owner allow was incorrectly rejected", e)
				}
				windowsFixtureDACL(t, path, "D:P(A;;FA;;;"+user.String()+")(OA;;GR;00000000-0000-0000-0000-000000000001;;"+user.String()+")")
				found := false
				for _, ace := range windowsNativeFixtureACEs(t, path) {
					if ace.kind == windowsAllowedObjectACEType && ace.flags&windows.INHERIT_ONLY_ACE == 0 && ace.mask != 0 {
						found = true
					}
				}
				if !found {
					t.Fatal("native fixture did not retain a genuine unsupported effective object ACE")
				}
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
		original, e := os.Stat(root)
		if e != nil {
			t.Fatal(e)
		}
		d, e := winprivate.Open(directory, false)
		if e != nil {
			t.Fatal(e)
		}
		defer d.Close()
		held, e := d.File.Stat()
		if e != nil {
			t.Fatal(e)
		}
		before := windowsCoreTree(t, outside)
		moved := root + "-moved"
		renameErr := os.Rename(root, moved)
		if renameErr == nil {
			windowsJunction(t, root, outside)
			if e = d.Publish("state", []byte("held object literal"), true); e != nil {
				t.Fatal(e)
			}
			t.Log("native_ancestor_custody mode=held_object_after_actual_swap")
		} else {
			if !errors.Is(renameErr, windows.ERROR_ACCESS_DENIED) && !errors.Is(renameErr, windows.ERROR_SHARING_VIOLATION) {
				t.Fatal("unexpected ancestor rename failure", renameErr)
			}
			after, e := os.Stat(root)
			if e != nil || !os.SameFile(original, after) {
				t.Fatal("denied ancestor rename changed root identity", e)
			}
			afterHeld, e := d.File.Stat()
			if e != nil || !os.SameFile(held, afterHeld) {
				t.Fatal("denied ancestor rename changed held directory identity", e)
			}
			if !reflect.DeepEqual(before, windowsCoreTree(t, outside)) {
				t.Fatal("denied ancestor rename changed outside bytes")
			}
			if e = d.Publish("state", []byte("held object literal"), true); e != nil {
				t.Fatal(e)
			}
			if e = d.Close(); e != nil {
				t.Fatal(e)
			}
			// Positive control: the same ordinary-user rename must work after release.
			if e = os.Rename(root, moved); e != nil {
				t.Fatal("ancestor rename still refused after held handle release", e)
			}
			t.Log("native_ancestor_custody mode=kernel_denial_while_held identity_unchanged=true rename_after_close=true")
		}
		body, e := os.ReadFile(filepath.Join(moved, "held", "state"))
		if e != nil || string(body) != "held object literal" {
			t.Fatal("held publication lost complete original bytes", e)
		}
		if !reflect.DeepEqual(before, windowsCoreTree(t, outside)) {
			t.Fatal("held publication changed outside bytes")
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
	t.Run("native_sharing_refusal_preserves_old_bytes_and_retry", func(t *testing.T) {
		root := windowsPrivateRoot(t)
		path := filepath.Join(root, "state")
		if e := scheduler.PublishNewPrivateFile(path, []byte("old complete bytes")); e != nil {
			t.Fatal(e)
		}
		pointer, e := windows.UTF16PtrFromString(path)
		if e != nil {
			t.Fatal(e)
		}
		// A real native reader deliberately withholds DELETE sharing. This forces
		// the actual kernel rename to refuse, independently of application guards.
		held, e := windows.CreateFile(pointer, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if e != nil {
			t.Fatal(e)
		}
		d, e := winprivate.Open(root, false)
		if e != nil {
			windows.CloseHandle(held)
			t.Fatal(e)
		}
		defer d.Close()
		publishErr := d.Publish("state", []byte("new complete bytes"), false)
		if e = windows.CloseHandle(held); e != nil {
			t.Fatal(e)
		}
		if publishErr == nil {
			t.Fatal("native reader without DELETE sharing did not refuse publication")
		}
		b, e := scheduler.ReadPrivateFile(path, 128)
		if e != nil || string(b) != "old complete bytes" {
			t.Fatal("failed native publication lost the old complete file", e)
		}
		entries, e := os.ReadDir(root)
		if e != nil || len(entries) != 1 {
			t.Fatal("refused native publication leaked a stage", e)
		}
		if e = d.Publish("state", []byte("new complete bytes"), false); e != nil {
			t.Fatal("native publication could not retry after sharing release", e)
		}
		b, e = scheduler.ReadPrivateFile(path, 128)
		if e != nil || string(b) != "new complete bytes" {
			t.Fatal("native retry lost complete readback", e)
		}
	})
	t.Run("live_lock_name_custody_prevents_replacement_writer", func(t *testing.T) {
		root := windowsPrivateRoot(t)
		path, moved := filepath.Join(root, "custody.lock"), filepath.Join(root, "custody-moved.lock")
		originalBody := []byte("original persistent lock bytes")
		replacementBody := []byte("replacement persistent lock bytes")
		if e := scheduler.PublishNewPrivateFile(path, originalBody); e != nil {
			t.Fatal(e)
		}
		original, e := windowsNativeFixtureIdentity(path)
		if e != nil {
			t.Fatal(e)
		}
		from, e := windows.UTF16PtrFromString(path)
		if e != nil {
			t.Fatal(e)
		}
		to, e := windows.UTF16PtrFromString(moved)
		if e != nil {
			t.Fatal(e)
		}
		var renamed, secondEntered bool
		var replacementID windowsFixtureFileIdentity
		var renameErr, secondErr error
		firstErr := scheduler.WithPrivateLock(path, func() error {
			// Independent Win32 namespace mutation, not the publication helper
			// under test. It must not split one logical lock into two objects.
			renameErr = windows.MoveFileEx(from, to, 0)
			renamed = renameErr == nil
			if renamed {
				actual, statErr := windowsNativeFixtureIdentity(moved)
				if statErr != nil || original != actual {
					return fmt.Errorf("live renamed lock identity changed: %v", statErr)
				}
				if publishErr := scheduler.PublishNewPrivateFile(path, replacementBody); publishErr != nil {
					return fmt.Errorf("replacement fixture creation: %w", publishErr)
				}
				replacement, statErr := windowsNativeFixtureIdentity(path)
				if statErr != nil || original == replacement {
					return fmt.Errorf("replacement fixture is not a distinct file: %v", statErr)
				}
				replacementID = replacement
			} else {
				if !errors.Is(renameErr, windows.ERROR_ACCESS_DENIED) && !errors.Is(renameErr, windows.ERROR_SHARING_VIOLATION) && !errors.Is(renameErr, windows.ERROR_LOCK_VIOLATION) {
					return fmt.Errorf("unexpected native live-lock rename error: %w", renameErr)
				}
				actual, statErr := windowsNativeFixtureIdentity(path)
				if statErr != nil || original != actual {
					return fmt.Errorf("refused live-lock rename changed identity: %v", statErr)
				}
			}
			secondErr = scheduler.WithPrivateLock(path, func() error {
				secondEntered = true
				return nil
			})
			return nil
		})
		// The first public call has now released its OS lock. Restore the
		// original namespace even on RED before failing the behavioral oracle.
		var cleanupObservationErr error
		if renamed {
			if replacement, readErr := os.ReadFile(path); readErr == nil {
				if !bytes.Equal(replacement, replacementBody) {
					cleanupObservationErr = errors.New("replacement lock bytes changed")
				}
				if removeErr := os.Remove(path); removeErr != nil {
					t.Fatal("cannot remove synthetic replacement after release", removeErr)
				}
			} else if !errors.Is(readErr, os.ErrNotExist) {
				t.Fatal("cannot inspect synthetic replacement after release", readErr)
			}
			if restoreErr := windows.MoveFileEx(to, from, 0); restoreErr != nil {
				t.Fatal("cannot restore original lock namespace after release", restoreErr)
			}
		}
		actual, statErr := windowsNativeFixtureIdentity(path)
		body, readErr := os.ReadFile(path)
		if statErr != nil || original != actual || readErr != nil || !bytes.Equal(body, originalBody) {
			t.Fatalf("original lock identity/bytes not preserved: stat=%v read=%v", statErr, readErr)
		}
		if e = windows.MoveFileEx(from, to, 0); e != nil {
			t.Fatal("same native lock rename refused after public release", e)
		}
		if e = windows.MoveFileEx(to, from, 0); e != nil {
			t.Fatal("cannot restore positive-control lock namespace", e)
		}
		t.Logf("native_live_lock_custody rename_succeeded_while_held=%v native_error=%v second_action_entered=%v second_error=%v original_identity_bytes_preserved=true same_rename_after_release=true original_native_id=%+v replacement_native_id=%+v", renamed, renameErr, secondEntered, secondErr, original, replacementID)
		if firstErr != nil || cleanupObservationErr != nil {
			t.Fatalf("public lock fixture failed after restoring original namespace: first=%v cleanup_observation=%v", firstErr, cleanupObservationErr)
		}
		if secondEntered || !errors.Is(secondErr, scheduler.ErrLeaseBusy) {
			t.Fatalf("live lock filename replacement defeated public serialization: rename_succeeded=%v second_entered=%v second_error=%v", renamed, secondEntered, secondErr)
		}
		if renamed {
			t.Fatal("live lock filename custody was surrendered despite second-writer refusal")
		}
		if e = scheduler.WithPrivateLock(path, func() error { return nil }); e != nil {
			t.Fatal("public lock could not reacquire after namespace positive control", e)
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
	t.Log("native_private_state exercised_cases=" + strings.Join([]string{"owner_acl", "unowned_read", "world_null_unknown_acl", "foreign_owner", "hardlink", "nonregular", "offline", "junction", "held_ancestor", "no_clobber", "cas", "absence", "native_sharing_refusal_retry", "live_lock_name_custody", "contention"}, ","))
}

const windowsAllowedObjectACEType = 0x05

type windowsFixtureACE struct {
	kind  uint8
	flags uint8
	mask  windows.ACCESS_MASK
}

func windowsNativeFixtureACEs(t *testing.T, path string) []windowsFixtureACE {
	t.Helper()
	sd, e := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		t.Fatal(e)
	}
	acl, _, e := sd.DACL()
	if e != nil || acl == nil {
		t.Fatal("native fixture DACL unavailable", e)
	}
	var result []windowsFixtureACE
	for i := uint16(0); i < acl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if e = windows.GetAce(acl, uint32(i), &ace); e != nil || ace == nil {
			t.Fatal("native fixture ACE unavailable", e)
		}
		result = append(result, windowsFixtureACE{kind: ace.Header.AceType, flags: ace.Header.AceFlags, mask: ace.Mask})
		t.Logf("native_fixture_ace type=%d mask=0x%x flags=0x%x", ace.Header.AceType, ace.Mask, ace.Header.AceFlags)
	}
	return result
}

// Capture identity eagerly: os.Stat/SameFile on Windows may lazily reopen
// the old pathname after a rename. Metadata handles are closed before mutation
// and request no DELETE access, so this oracle cannot protect the live name.
type windowsFixtureFileIdentity struct {
	Volume, IndexHigh, IndexLow uint32
}

func windowsNativeFixtureIdentity(path string) (windowsFixtureFileIdentity, error) {
	var identity windowsFixtureFileIdentity
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return identity, err
	}
	handle, err := windows.CreateFile(pointer, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return identity, err
	}
	var information windows.ByHandleFileInformation
	queryErr := windows.GetFileInformationByHandle(handle, &information)
	closeErr := windows.CloseHandle(handle)
	if err = errors.Join(queryErr, closeErr); err != nil {
		return identity, err
	}
	if information.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || information.NumberOfLinks != 1 {
		return identity, errors.New("native identity fixture is not a single-link regular file")
	}
	identity = windowsFixtureFileIdentity{Volume: information.VolumeSerialNumber, IndexHigh: information.FileIndexHigh, IndexLow: information.FileIndexLow}
	return identity, nil
}

func windowsPrivateFixtureDirectory(t *testing.T, path string) {
	t.Helper()
	if err := scheduler.EnsurePrivateDirectory(path); err != nil {
		t.Fatal(err)
	}
}
