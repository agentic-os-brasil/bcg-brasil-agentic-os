//go:build windows

package winprivate

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func migrationAccount(t *testing.T) {
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

func migrationPrivateParent(t *testing.T) *Directory {
	t.Helper()
	migrationAccount(t)
	d, err := OpenPinnedTree(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestWindowsMigrationSourceTreeIsReadOnlyAndPinned(t *testing.T) {
	migrationAccount(t)
	base := t.TempDir()
	root := filepath.Join(base, "public source")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(root, "source.md")
	body := []byte("# Reviewed original\n")
	if err := os.WriteFile(name, body, 0600); err != nil {
		t.Fatal(err)
	}
	sid, err := CurrentUser()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("O:" + sid.String() + "D:P(A;OICI;FA;;;" + sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	if d, err := OpenPinnedTree(root); err == nil {
		d.Close()
		t.Fatal("public source unexpectedly supplied private authority")
	}
	source, err := InspectPinnedTree(root)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	f, err := source.OpenFile("source.md")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(f)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatal("ordinary source bytes were not read", err)
	}
	if _, err := f.Write([]byte("mutated")); err == nil {
		t.Fatal("source handle granted write authority")
	}
	if err := os.WriteFile(name, []byte("mutated"), 0600); err == nil {
		t.Fatal("admitted source allowed a concurrent writer")
	}
	if err := os.Rename(root, root+"-retargeted"); err == nil {
		t.Fatal("source root was not pinned")
	}
	if err := os.Rename(base, base+"-retargeted"); err == nil {
		t.Fatal("ordinary source ancestor was not pinned")
	}
	f.Close()
	if err := os.Link(name, filepath.Join(root, "hardlink.md")); err != nil {
		t.Fatal(err)
	}
	if f, err := source.OpenFile("source.md"); err == nil {
		f.Close()
		t.Fatal("multiply linked source was accepted")
	}
	if err := os.Remove(filepath.Join(root, "hardlink.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(name, filepath.Join(root, "symlink.md")); err != nil {
		t.Fatal("required real file symlink unavailable", err)
	}
	if f, err := source.OpenFile("symlink.md"); err == nil {
		f.Close()
		t.Fatal("source reparse point was followed")
	}
	if _, err := source.ReadDir(1); err == nil {
		t.Fatal("source directory bound was ignored")
	}
	alias := filepath.Join(base, "linked source")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal("required real directory symlink unavailable", err)
	}
	if linked, err := InspectPinnedTree(alias); err == nil {
		linked.Close()
		t.Fatal("source root reparse point was followed")
	}
	if got, err := os.ReadFile(name); err != nil || !bytes.Equal(got, body) {
		t.Fatal("original source bytes changed", err)
	}
}

func TestWindowsMigrationSourceChildRetainsParentCustody(t *testing.T) {
	migrationAccount(t)
	base := t.TempDir()
	root := filepath.Join(base, "source")
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	source, err := InspectPinnedTree(root)
	if err != nil {
		t.Fatal(err)
	}
	child, err := source.Child("nested")
	if err != nil {
		source.Close()
		t.Fatal(err)
	}
	source.Close()
	if err := os.Rename(root, root+"-retargeted"); err == nil {
		t.Fatal("child lost its parent custody when source closed")
	}
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, root+"-released"); err != nil {
		t.Fatal("source custody was not released", err)
	}
}

func TestWindowsMigrationStageResumesAndPublishesTheHeldFile(t *testing.T) {
	parent := migrationPrivateParent(t)
	staging, err := parent.PinnedChild("staging", true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer staging.Close()
	target, err := parent.PinnedChild("target", true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	stage, err := staging.OpenStage("entry.part")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stage.Write([]byte("prefix")); err != nil {
		t.Fatal(err)
	}
	if err := stage.Sync(); err != nil {
		t.Fatal(err)
	}
	stage.Close()
	stage, err = staging.OpenStage("entry.part")
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Close()
	staging.Close()
	if err := os.Rename(staging.File.Name(), staging.File.Name()+"-swapped"); err == nil {
		t.Fatal("stage lost parent custody when its caller closed the parent")
	}
	got, err := io.ReadAll(stage)
	if err != nil || string(got) != "prefix" {
		t.Fatal("deterministic stage did not resume", err)
	}
	if _, err := stage.Write([]byte("-complete")); err != nil {
		t.Fatal(err)
	}
	before, err := stage.Stat()
	if err != nil {
		t.Fatal(err)
	}
	stagePath := filepath.Join(staging.File.Name(), "entry.part")
	if err := os.Rename(stagePath, stagePath+"-swapped"); err == nil {
		t.Fatal("held stage could be replaced")
	}
	if err := os.WriteFile(stagePath, []byte("foreign"), 0600); err == nil {
		t.Fatal("held stage allowed a foreign writer")
	}
	if err := target.Publish("occupied.md", []byte("occupied"), true); err != nil {
		t.Fatal(err)
	}
	if moved, err := stage.Publish(target, "occupied.md"); moved || !errors.Is(err, os.ErrExist) {
		t.Fatal("occupied target did not preserve stage", moved, err)
	}
	if moved, err := stage.Publish(target, "complete.md"); !moved || err != nil {
		t.Fatal("held stage was not published", moved, err)
	}
	after, err := stage.Stat()
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("publication adopted a different file", err)
	}
	if _, err := os.Stat(stagePath); !os.IsNotExist(err) {
		t.Fatal("old stage name remains", err)
	}
	if got, err := os.ReadFile(filepath.Join(target.File.Name(), "complete.md")); err != nil || string(got) != "prefix-complete" {
		t.Fatal("published bytes differ", err)
	}
	if got, err := os.ReadFile(filepath.Join(target.File.Name(), "occupied.md")); err != nil || string(got) != "occupied" {
		t.Fatal("occupied target changed", err)
	}
}

func TestWindowsMigrationStageRejectsUnpinnedAndLinkedInputs(t *testing.T) {
	parent := migrationPrivateParent(t)
	plain, err := Open(parent.File.Name(), false)
	if err != nil {
		t.Fatal(err)
	}
	if stage, err := plain.OpenStage("unadmitted.part"); err == nil {
		stage.Close()
		t.Fatal("stage adopted an unpinned parent")
	}
	plain.Close()
	if _, err := os.Stat(filepath.Join(parent.File.Name(), "unadmitted.part")); !os.IsNotExist(err) {
		t.Fatal("unpinned stage refusal created a file", err)
	}
	if err := parent.Publish("outside.md", []byte("outside original"), true); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent.File.Name(), "outside.md")
	valid, err := parent.OpenStage("outside.md")
	if err != nil {
		t.Fatal("ordinary stage control is not admitted", err)
	}
	valid.Close()
	for _, kind := range []string{"hardlink", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			name := kind + ".part"
			path := filepath.Join(parent.File.Name(), name)
			var err error
			if kind == "hardlink" {
				err = os.Link(outside, path)
			} else {
				err = os.Symlink(outside, path)
			}
			if err != nil {
				t.Fatal("required alias fixture unavailable", err)
			}
			defer os.Remove(path)
			if stage, err := parent.OpenStage(name); err == nil {
				stage.Close()
				t.Fatal("linked stage was accepted")
			}
			if got, err := os.ReadFile(outside); err != nil || string(got) != "outside original" {
				t.Fatal("stage refusal changed the outside source", err)
			}
		})
	}
}

func TestWindowsMigrationMovableRootKeepsArchivedAuthority(t *testing.T) {
	parent := migrationPrivateParent(t)
	home, err := parent.PinnedChild("home", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := home.Publish("new-work.md", []byte("post-migration work"), true); err != nil {
		t.Fatal(err)
	}
	homePath := home.File.Name()
	home.Close()
	movable, err := OpenMovableDirectory(homePath)
	if err != nil {
		t.Fatal(err)
	}
	defer movable.Close()
	before, err := movable.Directory().File.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(homePath, homePath+"-swapped"); err == nil {
		t.Fatal("movable root lost namespace custody")
	}
	occupied, err := parent.PinnedChild("occupied", true, true)
	if err != nil {
		t.Fatal(err)
	}
	occupied.Close()
	if moved, err := movable.MoveTo("occupied"); moved || !errors.Is(err, os.ErrExist) {
		t.Fatal("occupied archive was replaced", moved, err)
	}
	archive := ".maestro-rollback-attempt"
	if moved, err := movable.MoveTo(archive); !moved || err != nil {
		t.Fatal("root custody was not transferred", moved, err)
	}
	archivePath := filepath.Join(parent.File.Name(), archive)
	if movable.Path() != archivePath || movable.Directory().File.Name() != archivePath {
		t.Fatal("archived root labels were not retargeted")
	}
	after, err := movable.Directory().File.Stat()
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("rollback adopted another root", err)
	}
	if err := movable.Directory().Publish("archive-receipt.json", []byte("{\"revoked\":true}\n"), true); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(archivePath, "new-work.md")); err != nil || string(got) != "post-migration work" {
		t.Fatal("rollback lost new work", err)
	}
	if _, err := os.Stat(filepath.Join(archivePath, "archive-receipt.json")); err != nil {
		t.Fatal("receipt was not written through archived root", err)
	}
	if _, err := os.Stat(homePath); !os.IsNotExist(err) {
		t.Fatal("old HOME path was recreated", err)
	}
}

type removalState struct {
	body     []byte
	info     os.FileInfo
	security string
}

func removalSnapshot(t *testing.T, path string) removalState {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return removalState{body, info, sd.String()}
}

func assertRemovalPreserved(t *testing.T, before removalState, path string) {
	t.Helper()
	after := removalSnapshot(t, path)
	if !bytes.Equal(before.body, after.body) || before.info.Size() != after.info.Size() ||
		before.info.Mode() != after.info.Mode() || !before.info.ModTime().Equal(after.info.ModTime()) ||
		!os.SameFile(before.info, after.info) || before.security != after.security {
		t.Fatal("exact removal refusal changed bytes, identity, metadata or security")
	}
}

func setRemovalFixtureAccess(t *testing.T, path string, access uint32) {
	t.Helper()
	original, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	oldACL, _, err := original.DACL()
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := original.Control()
	if err != nil {
		t.Fatal(err)
	}
	restore := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION)
	if control&windows.SE_DACL_PROTECTED != 0 {
		restore = windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION
	}
	t.Cleanup(func() {
		if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, restore, nil, nil, oldACL, nil); err != nil {
			t.Error(err)
		}
	})
	sid, err := CurrentUser()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString(fmt.Sprintf("D:P(A;;0x%x;;;%s)(A;;FA;;;SY)(A;;FA;;;BA)", access, sid.String()))
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsMigrationRemoveExactRequiresCompleteExpectedBytes(t *testing.T) {
	parent := migrationPrivateParent(t)
	body := []byte("planned locator\n")
	name := ".maestro-home.json"
	if err := parent.Publish(name, body, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent.File.Name(), name)
	before := removalSnapshot(t, path)
	for _, expected := range [][]byte{[]byte("foreign locator\n"), []byte("short")} {
		if marked, err := parent.RemoveExact(name, expected); marked || err == nil {
			t.Fatal("different content acquired deletion authority", marked, err)
		}
		assertRemovalPreserved(t, before, path)
	}
	if marked, err := parent.RemoveExact(name, make([]byte, (16<<20)+1)); marked || err == nil || !strings.Contains(err.Error(), "bound") {
		t.Fatal("unbounded expected metadata was admitted", marked, err)
	}
	assertRemovalPreserved(t, before, path)
	if marked, err := parent.RemoveExact(name, body); !marked || err != nil {
		t.Fatal("matching held locator was not retired", marked, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("unshared locator remains after close", err)
	}
	if marked, err := parent.RemoveExact(name, body); marked || !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing locator was created or reported marked", marked, err)
	}
	if err := parent.Publish("empty", nil, true); err != nil {
		t.Fatal(err)
	}
	if marked, err := parent.RemoveExact("empty", nil); !marked || err != nil {
		t.Fatal(marked, err)
	}
}

func TestWindowsMigrationRemoveExactRetainsMarkedObjectAndReader(t *testing.T) {
	parent := migrationPrivateParent(t)
	name, body := "locator.json", []byte("sealed locator")
	if err := parent.Publish(name, body, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent.File.Name(), name)
	native, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(native, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal(err)
	}
	reader := os.NewFile(uintptr(h), path)
	defer reader.Close()
	before, err := reader.Stat()
	if err != nil {
		t.Fatal(err)
	}
	observed := false
	marked, err := parent.removeExact(name, body, func(held *os.File) error {
		observed = true
		current, err := held.Stat()
		if err != nil || !os.SameFile(before, current) {
			t.Fatal("delete mark adopted a replacement", err)
		}
		if err := os.Rename(path, path+"-replaced"); err == nil {
			t.Fatal("marked held name could be replaced")
		}
		if err := os.Remove(path); err == nil {
			t.Fatal("another deleter acquired the marked name")
		}
		if stage, err := parent.OpenStage(name); err == nil {
			stage.Close()
			t.Fatal("marked name acquired a second writer")
		}
		return nil
	})
	if !observed || !marked || err != nil {
		t.Fatal("held deletion was not observed", observed, marked, err)
	}
	got := make([]byte, len(body))
	if _, err := reader.ReadAt(got, 0); err != nil || !bytes.Equal(got, body) {
		t.Fatal("retained reader lost original bytes", err)
	}
	after, err := reader.Stat()
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("retained reader identity changed", err)
	}
	// Legacy delete disposition may retain a pending name until this reader closes.
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("locator remains after all controlled readers close", err)
	}
}

func TestWindowsMigrationRemoveExactExposesPostMarkCloseFailure(t *testing.T) {
	parent := migrationPrivateParent(t)
	body := []byte("sealed locator")
	if err := parent.Publish("locator.json", body, true); err != nil {
		t.Fatal(err)
	}
	marked, err := parent.removeExact("locator.json", body, func(held *os.File) error { return held.Close() })
	if !marked || !errors.Is(err, os.ErrClosed) {
		t.Fatal("late close failure lost the successful delete mark", marked, err)
	}
}

func TestWindowsMigrationRemoveExactRejectsUnsafeOrBusyLeaves(t *testing.T) {
	for _, kind := range []string{"hardlink", "symlink", "writer", "delete-denied", "read-only", "unpinned"} {
		t.Run(kind, func(t *testing.T) {
			parent := migrationPrivateParent(t)
			body := []byte("original locator")
			name, outsideName := "locator.json", "original.json"
			if err := parent.Publish(outsideName, body, true); err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(parent.File.Name(), outsideName)
			path := filepath.Join(parent.File.Name(), name)
			if kind == "hardlink" {
				if err := os.Link(outside, path); err != nil {
					t.Fatal(err)
				}
			} else if kind == "symlink" {
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal("required alias fixture unavailable", err)
				}
			} else if err := parent.Publish(name, body, true); err != nil {
				t.Fatal(err)
			}
			var writer *os.File
			if kind == "writer" {
				var err error
				writer, err = parent.OpenFile(name, os.O_RDWR)
				if err != nil {
					t.Fatal(err)
				}
				defer writer.Close()
			}
			if kind == "read-only" {
				native, _ := windows.UTF16PtrFromString(path)
				attributes, err := windows.GetFileAttributes(native)
				if err != nil {
					t.Fatal(err)
				}
				if err := windows.SetFileAttributes(native, attributes&^windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_ATTRIBUTE_READONLY); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := windows.SetFileAttributes(native, attributes); err != nil {
						t.Error(err)
					}
				})
			}
			if kind == "delete-denied" {
				// Configure the fixture before admission; old handles must not retain deletion rights.
				parentPath := parent.File.Name()
				if err := parent.Close(); err != nil {
					t.Fatal(err)
				}
				parentAccess := uint32(privateDirAccess | windows.WRITE_DAC | windows.WRITE_OWNER)
				leafAccess := uint32(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.WRITE_DAC | windows.WRITE_OWNER)
				const fileDeleteChild = 0x40
				if parentAccess&(windows.DELETE|fileDeleteChild) != 0 || leafAccess&windows.DELETE != 0 {
					t.Fatal("fixture grants an alternate deletion path")
				}
				setRemovalFixtureAccess(t, parentPath, parentAccess)
				setRemovalFixtureAccess(t, path, leafAccess)
				var err error
				parent, err = OpenPinnedTree(parentPath)
				if err != nil {
					t.Fatal("restricted parent is not privately admitted", err)
				}
				defer parent.Close()
				readable, err := parent.OpenFile(name, os.O_RDONLY)
				if err != nil {
					t.Fatal("restricted leaf is not privately readable", err)
				}
				if err := readable.Close(); err != nil {
					t.Fatal(err)
				}
			}
			parentSecurity, err := windows.GetNamedSecurityInfo(parent.File.Name(), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
			if err != nil {
				t.Fatal(err)
			}
			before, external := removalSnapshot(t, path), removalSnapshot(t, outside)
			selected := parent
			if kind == "unpinned" {
				var err error
				selected, err = Open(parent.File.Name(), false)
				if err != nil {
					t.Fatal(err)
				}
				defer selected.Close()
			}
			marked, err := selected.RemoveExact(name, body)
			if marked || err == nil {
				t.Fatal("unsafe or busy leaf was retired", marked, err)
			}
			if kind == "writer" && !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
				t.Fatal("writer refusal was not native sharing contention", err)
			}
			if kind == "delete-denied" && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
				t.Fatal("delete denial was not preserved", err)
			}
			assertRemovalPreserved(t, before, path)
			assertRemovalPreserved(t, external, outside)
			afterParentSecurity, err := windows.GetNamedSecurityInfo(parent.File.Name(), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
			if err != nil || parentSecurity.String() != afterParentSecurity.String() {
				t.Fatal("refusal changed the parent's private security", err)
			}
		})
	}
}
