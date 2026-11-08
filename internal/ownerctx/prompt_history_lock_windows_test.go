//go:build windows

package ownerctx

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func nativePromptLockPath(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	directory := filepath.Join(root, "owner", "observations")
	if err := ensureOwnerDirectory(directory); err != nil {
		t.Fatal(err)
	}
	return root, filepath.Join(directory, promptHistoryLockName)
}

func TestWindowsPromptLockRetainsNameAcrossReaderHandle(t *testing.T) {
	_, path := nativePromptLockPath(t)
	unlock, err := acquirePromptHistoryLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unlock != nil {
			_ = unlock()
		}
	}()
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal("retained native reader fixture unavailable", err)
	}
	defer windows.CloseHandle(reader)
	err = unlock()
	unlock = nil
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() != 0 {
		t.Fatalf("release removed or replaced the persistent native guard: %+v %v", after, err)
	}
	// The old CREATE_NEW token protocol must refuse this persistent filename.
	legacy, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		_ = legacy.Close()
		t.Fatal("older token writer acquired the native guard name")
	}
	if !errors.Is(err, os.ErrExist) {
		t.Fatal("older writer did not fail closed on the retained name", err)
	}
	unlocked, err := acquirePromptHistoryLock(path)
	if err != nil {
		t.Fatal("native reacquisition failed while an independent reader retains the old handle", err)
	}
	if err := unlocked(); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsPromptLockExcludesEmptyLegacyWriter(t *testing.T) {
	_, path := nativePromptLockPath(t)
	legacy, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	unlock, err := acquirePromptHistoryLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unlock != nil {
			_ = unlock()
		}
	}()
	guard, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if written, err := legacy.WriteString(strings.Repeat("a", 32)); written != 0 || !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		t.Fatalf("native guard admitted an older writer's token: written=%d err=%v", written, err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err == nil {
		t.Fatal("older writer cleanup unlinked the active native guard")
	}
	observed, err := os.Stat(path)
	if err != nil || !os.SameFile(guard, observed) || observed.Size() != 0 {
		t.Fatalf("older cleanup changed active lock identity: %+v %v", observed, err)
	}
	if contender, err := acquirePromptHistoryLock(path); !errors.Is(err, ErrPromptHistoryLocked) {
		if contender != nil {
			_ = contender()
		}
		t.Fatal("a second native writer bypassed the active guard", err)
	}
	err = unlock()
	unlock = nil
	if err != nil {
		t.Fatal(err)
	}
	if next, err := acquirePromptHistoryLock(path); err != nil {
		t.Fatal("native guard could not be reacquired after the failed legacy cleanup", err)
	} else if err := next(); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsPromptLockRejectsUnsafeOrLegacyGuards(t *testing.T) {
	for _, kind := range []string{"legacy-token", "public-file-dacl", "denied-parent", "file-reparse", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			root, path := nativePromptLockPath(t)
			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "keep.txt"), []byte("preserve outside"), 0600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "legacy-token":
				if err := os.WriteFile(path, []byte(strings.Repeat("b", 32)), 0600); err != nil {
					t.Fatal(err)
				}
			case "public-file-dacl":
				unlock, err := acquirePromptHistoryLock(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := unlock(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(ownerPublicRegistry(t, path))
			case "denied-parent":
				user, err := windows.GetCurrentProcessToken().GetTokenUser()
				if err != nil {
					t.Fatal(err)
				}
				sid := user.User.Sid.String()
				sd, err := windows.SecurityDescriptorFromString("D:P(D;;0x2;;;" + sid + ")(A;;FA;;;" + sid + ")")
				if err != nil {
					t.Fatal(err)
				}
				acl, _, err := sd.DACL()
				if err != nil {
					t.Fatal(err)
				}
				if err := windows.SetNamedSecurityInfo(filepath.Dir(path), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
					t.Fatal(err)
				}
			case "file-reparse", "hardlink":
				target := filepath.Join(outside, "empty-lock")
				if err := os.WriteFile(target, nil, 0600); err != nil {
					t.Fatal(err)
				}
				if kind == "file-reparse" {
					if err := os.Symlink(target, path); err != nil {
						t.Fatal("required native file-reparse fixture unavailable", err)
					}
				} else if err := os.Link(target, path); err != nil {
					t.Fatal("required native hardlink fixture unavailable", err)
				}
			}
			before, external := ownerTreeSnapshot(t, root), ownerTreeSnapshot(t, outside)
			unlock, err := acquirePromptHistoryLock(path)
			if err == nil {
				_ = unlock()
				t.Fatal("unsafe or legacy guard acquired mutation authority")
			}
			if errors.Is(err, ErrPromptHistoryLocked) {
				t.Fatal("security or legacy refusal was relabelled as lock contention", err)
			}
			if kind == "denied-parent" && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
				t.Fatal("the genuine DACL-denied open did not preserve AccessDenied", err)
			}
			if kind == "legacy-token" && !strings.Contains(err.Error(), "legacy or unexpected data") {
				t.Fatal("legacy token was refused for an unrelated reason", err)
			}
			if !reflect.DeepEqual(before, ownerTreeSnapshot(t, root)) || !reflect.DeepEqual(external, ownerTreeSnapshot(t, outside)) {
				t.Fatal("guard refusal changed private or outside state")
			}
		})
	}
}
