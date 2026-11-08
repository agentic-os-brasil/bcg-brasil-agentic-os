//go:build windows

package zipmigration

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func externalNativeAccount(t *testing.T) {
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
func externalNativeTree(t *testing.T) *externalTree {
	t.Helper()
	externalNativeAccount(t)
	tree, err := externalOpen(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tree.close() })
	return tree
}
func TestWindowsExternalAdapterResumesTheSameDeterministicStage(t *testing.T) {
	tree := externalNativeTree(t)
	name := "brain/owner/note.md"
	body := []byte("synthetic authored work\n")
	temporary := externalTemporary(name)
	directory, closeDirectory, err := tree.privateDirectory(path.Dir(temporary), true)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := directory.OpenStage(path.Base(temporary))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stage.Write(body[:8]); err != nil {
		t.Fatal(err)
	}
	before, err := stage.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err = stage.Close(); err != nil {
		t.Fatal(err)
	}
	closeDirectory()
	if err = externalEnsure(tree, name, body); err != nil {
		t.Fatal(err)
	}
	after, err := tree.stat(name)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("stage identity was replaced", err)
	}
	got, err := externalRead(tree, name, true)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatal("resumed bytes differ", err)
	}
	if err = externalEnsure(tree, name, []byte("foreign replacement")); err == nil {
		t.Fatal("existing author content replaced")
	}
	got, err = externalRead(tree, name, true)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatal("foreign publication changed target", err)
	}
}
func TestWindowsExternalAdapterRefusesForeignStagePrefixAndHardlink(t *testing.T) {
	tree := externalNativeTree(t)
	name := "brain/owner/note.md"
	temporary := externalTemporary(name)
	directory, closeDirectory, err := tree.privateDirectory(path.Dir(temporary), true)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := directory.OpenStage(path.Base(temporary))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stage.Write([]byte("foreign prefix")); err != nil {
		t.Fatal(err)
	}
	if err = stage.Close(); err != nil {
		t.Fatal(err)
	}
	closeDirectory()
	if err = externalEnsure(tree, name, []byte("reviewed bytes")); err == nil {
		t.Fatal("foreign prefix accepted")
	}
	if _, err = tree.stat(name); !os.IsNotExist(err) {
		t.Fatal("foreign prefix published", err)
	}
	linked := filepath.Join(tree.project, filepath.FromSlash(temporary))
	if err = os.Link(linked, filepath.Join(tree.project, "linked-stage")); err != nil {
		t.Fatal(err)
	}
	if err = externalEnsure(tree, name, []byte("foreign prefix")); err == nil {
		t.Fatal("linked stage accepted")
	}
	got, err := os.ReadFile(linked)
	if err != nil || string(got) != "foreign prefix" {
		t.Fatal("stage changed", err)
	}
}
func TestWindowsExternalAdapterSourceInventoryRetainsRootAndOriginalBytes(t *testing.T) {
	externalNativeAccount(t)
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "data/profile"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "VERSION"), []byte("0.1.12\n"), 0600); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(source, "data/profile/note.md")
	if err := os.WriteFile(original, []byte("original\n"), 0400); err != nil {
		t.Fatal(err)
	}
	originalUTF16, err := windows.UTF16PtrFromString(original)
	if err != nil {
		t.Fatal(err)
	}
	if err = windows.SetFileAttributes(originalUTF16, windows.FILE_ATTRIBUTE_READONLY); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { windows.SetFileAttributes(originalUTF16, windows.FILE_ATTRIBUTE_NORMAL) })
	tree, err := externalOpenSource(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.close()
	version, layout, records, err := externalSourceTree(tree)
	if err != nil || version != "0.1.12" || layout != "legacy-data" || len(records) != 2 {
		t.Fatal("source inventory failed", version, layout, len(records), err)
	}
	if err = os.Rename(source, source+"-replaced"); err == nil {
		t.Fatal("admitted source root was movable")
	}
	if err = tree.mkdir("unauthorized"); err == nil {
		t.Fatal("read-only source admitted mutation")
	}
	_, _, again, err := externalSourceTree(tree)
	if err != nil || fingerprint(again) != fingerprint(records) {
		t.Fatal("source inventory changed", err)
	}
	got, err := os.ReadFile(original)
	if err != nil || string(got) != "original\n" {
		t.Fatal("original changed", err)
	}
}
func TestWindowsExternalAdapterMovableRootWritesReceiptsInArchive(t *testing.T) {
	externalNativeAccount(t)
	parent := t.TempDir()
	home := filepath.Join(parent, "home")
	if err := externalCreateHome(home); err != nil {
		t.Fatal(err)
	}
	tree, err := externalOpenMovable(home)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.close()
	before, err := tree.stat(".")
	if err != nil {
		t.Fatal(err)
	}
	if err = externalEnsure(tree, "brain/.maestro/revoked.json", []byte("revoked\n")); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(parent, "archive")
	moved, err := tree.move(archive)
	if err != nil || !moved {
		t.Fatal("held move failed", moved, err)
	}
	after, err := tree.stat(".")
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("root identity changed", err)
	}
	if err = externalEnsure(tree, "brain/.maestro/rollback.json", []byte("rolled_back\n")); err != nil {
		t.Fatal("archived receipt failed", err)
	}
	if _, err = os.Stat(home); !os.IsNotExist(err) {
		t.Fatal("old home remained", err)
	}
	got, err := os.ReadFile(filepath.Join(archive, "brain/.maestro/rollback.json"))
	if err != nil || string(got) != "rolled_back\n" {
		t.Fatal("receipt escaped archive", err)
	}
}
func TestWindowsExternalAdapterLocatorRetirementRequiresExactBytes(t *testing.T) {
	tree := externalNativeTree(t)
	name := ".maestro-home.json"
	body := []byte("owned locator\n")
	if err := externalEnsure(tree, name, body); err != nil {
		t.Fatal(err)
	}
	if marked, err := tree.removeExact(name, []byte("another attempt\n")); err == nil || marked {
		t.Fatal("foreign locator retired", marked, err)
	}
	got, err := externalRead(tree, name, true)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatal("foreign comparison changed locator", err)
	}
	if marked, err := tree.removeExact(name, body); err != nil || !marked {
		t.Fatal("owned locator not retired", marked, err)
	}
	if _, err = tree.stat(name); !os.IsNotExist(err) {
		t.Fatal("retired locator remained", err)
	}
}

func TestWindowsExternalAdapterV1ReceiptAcceptsEvolvedAuthoredBrain(t *testing.T) {
	externalNativeAccount(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "data/profile"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "VERSION"), []byte("0.2.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data/profile/note.md"), []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if result := Run(root, Options{}); result.State != "committed" {
		t.Fatal("actual v1 fixture did not commit", result.State, result.Errors)
	}
	if err := os.WriteFile(filepath.Join(root, "brain/owner/note.md"), []byte("authored after v1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tree, err := externalOpenSource(root)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.close()
	_, layout, _, err := externalSourceTree(tree)
	if err != nil || layout != "brain-v1" {
		t.Fatal("evolved committed brain refused", layout, err)
	}
	if err = os.WriteFile(filepath.Join(root, "data/profile/note.md"), []byte("source drift\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = externalSourceTree(tree); err == nil {
		t.Fatal("v1 source drift accepted")
	}
}

func TestWindowsExternalAdapterPlanValidationKeepsInitialMovableCustody(t *testing.T) {
	externalNativeAccount(t)
	base := t.TempDir()
	source := filepath.Join(base, "source")
	profile := filepath.Join(base, "profile")
	home := filepath.Join(base, "home")
	if err := os.MkdirAll(filepath.Join(source, "data/profile"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "VERSION"), []byte("0.1.12\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "data/profile/note.md"), []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := externalCreateHome(profile); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanExternal(ExternalOptions{SourceRoot: source, ProductRoot: source, UserHome: profile, Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if err = externalCreateHome(home); err != nil {
		t.Fatal(err)
	}
	tree, err := externalOpenMovable(home)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.close()
	if err = externalValidatePlan(plan, tree); err != nil {
		t.Fatal("validation reopened its movable root", err)
	}
	if err = externalRoots(plan.ExternalOptions, true, tree); err != nil {
		t.Fatal("preflight reopened its movable root", err)
	}
	if err = externalArchiveRoots(plan, true, tree); err != nil {
		t.Fatal("archive preflight reopened its movable root", err)
	}
}
