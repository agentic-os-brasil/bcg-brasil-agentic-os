//go:build windows

package winprivate

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
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
