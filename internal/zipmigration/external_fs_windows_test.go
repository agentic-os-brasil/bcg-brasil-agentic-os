//go:build windows

package zipmigration

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"strings"
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

// These tests exercise admitted internal transactions; public mutation admission
// remains unavailable and its existing refusal test remains authoritative.
func externalNativeTransactionFixture(t *testing.T) ExternalOptions {
	t.Helper()
	externalNativeAccount(t)
	base := t.TempDir()
	source := filepath.Join(base, "source")
	profile := filepath.Join(base, "profile")
	if err := os.MkdirAll(filepath.Join(source, "data/profile/empty"), 0700); err != nil {
		t.Fatal(err)
	}
	externalNativePut(t, source, "VERSION", "0.2.0\n")
	externalNativePut(t, source, "data/profile/note.md", "original v1 source\n")
	if result := Run(source, Options{}); result.State != "committed" {
		t.Fatal("genuine v1 producer failed", result.State, result.Errors)
	}
	externalNativePut(t, source, "brain/owner/note.md", "authored before external migration\n")
	externalNativePut(t, source, "brain/owner/.hidden.md", "hidden authored work\n")
	if err := externalCreateHome(profile); err != nil {
		t.Fatal(err)
	}
	return ExternalOptions{SourceRoot: source, ProductRoot: source, UserHome: profile, Home: filepath.Join(base, "home")}
}
func externalNativePut(t *testing.T, root, name, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func externalNativePlan(t *testing.T, o ExternalOptions) (ExternalPlan, string) {
	t.Helper()
	plan, err := PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	body, err := plan.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return plan, hash(body)
}

type externalNativeEntrySnapshot struct {
	Mode       os.FileMode
	Modified   int64
	Body       string
	Security   string
	Attributes uint32
}

func externalNativeSnapshot(t *testing.T, root string) map[string]externalNativeEntrySnapshot {
	t.Helper()
	snapshot := map[string]externalNativeEntrySnapshot{}
	err := filepath.WalkDir(root, func(full string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		security, err := windows.GetNamedSecurityInfo(full, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.GROUP_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			return err
		}
		nativeName, err := windows.UTF16PtrFromString(full)
		if err != nil {
			return err
		}
		attributes, err := windows.GetFileAttributes(nativeName)
		if err != nil {
			return err
		}
		record := externalNativeEntrySnapshot{Mode: info.Mode(), Modified: info.ModTime().UnixNano(), Security: security.String(), Attributes: attributes}
		if !info.IsDir() {
			body, err := os.ReadFile(full)
			if err != nil {
				return err
			}
			record.Body = string(body)
		}
		relative, err := filepath.Rel(root, full)
		if err != nil {
			return err
		}
		snapshot[relative] = record
		return nil
	})
	if err != nil {
		t.Fatal("native snapshot failed", err)
	}
	return snapshot
}
func externalNativeUnchanged(t *testing.T, root string, before map[string]externalNativeEntrySnapshot) {
	t.Helper()
	after := externalNativeSnapshot(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("native bytes, entry inventory, modes, modification times or owner/group/DACL/attributes changed", root)
	}
}
func externalNativeRead(t *testing.T, root, name, want string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil || string(body) != want {
		t.Fatal("unexpected retained bytes", name, err)
	}
}
func TestWindowsExternalAdapterAdmittedTransactionPreservesV1SourceAndProof(t *testing.T) {
	o := externalNativeTransactionFixture(t)
	source := externalNativeSnapshot(t, o.SourceRoot)
	plan, digest := externalNativePlan(t, o)
	if plan.SourceLayout != "brain-v1" {
		t.Fatal("genuine v1 source not recognized", plan.SourceLayout)
	}
	externalNativeUnchanged(t, o.SourceRoot, source)
	status, err := applyExternal(plan, digest, nil)
	if err != nil || !status.Current || status.State != "committed" {
		t.Fatal("admitted apply failed", status.State, err)
	}
	inspected, err := InspectExternal(o.Home, plan.AttemptID, digest)
	if err != nil || !inspected.Current || inspected.PlanSHA256 != digest {
		t.Fatal("public proof inspection failed", inspected.State, err)
	}
	externalNativeRead(t, o.Home, "brain/owner/note.md", "authored before external migration\n")
	externalNativeRead(t, o.Home, "brain/owner/.hidden.md", "hidden authored work\n")
	if info, err := os.Stat(filepath.Join(o.Home, "brain/owner/empty")); err != nil || !info.IsDir() {
		t.Fatal("authored empty directory lost", err)
	}
	externalNativeRead(t, o.Home, externalName(plan.AttemptID, "commit.json"), string(externalCanonical(externalRecord{1, plan.AttemptID, digest, "committed"})))
	externalNativeRead(t, o.UserHome, ".maestro-home.json", string(externalCanonical(externalLocator{1, o.Home, plan.AttemptID})))
	externalNativeUnchanged(t, o.SourceRoot, source)
	offline := o.SourceRoot + "-offline"
	if err = os.Rename(o.SourceRoot, offline); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Rename(offline, o.SourceRoot); err != nil {
			t.Error(err)
		}
	}()
	if inspected, err = InspectExternal(o.Home, plan.AttemptID, digest); err != nil || !inspected.Current {
		t.Fatal("committed inspection required offline original", inspected.State, err)
	}
	if status, err = applyExternal(plan, digest, nil); err != nil || status.State != "already_migrated" {
		t.Fatal("repeat recopied or required offline original", status.State, err)
	}
	externalNativeUnchanged(t, offline, source)
}
func TestWindowsExternalAdapterAdmittedTransactionResumesAndRollsBackEvolvedWork(t *testing.T) {
	o := externalNativeTransactionFixture(t)
	source := externalNativeSnapshot(t, o.SourceRoot)
	plan, digest := externalNativePlan(t, o)
	stopped := errors.New("synthetic admitted transaction interruption")
	_, err := applyExternal(plan, digest, func(event ExternalProgress) error {
		if event.Phase == "copying" {
			return stopped
		}
		return nil
	})
	if !errors.Is(err, stopped) {
		t.Fatal("copy interruption not observed", err)
	}
	if inspected, err := InspectExternal(o.Home, plan.AttemptID, digest); err == nil && inspected.Current {
		t.Fatal("partial transaction became current")
	}
	if status, err := applyExternal(plan, digest, nil); err != nil || !status.Current {
		t.Fatal("same-attempt resume failed", status.State, err)
	}
	externalNativePut(t, o.Home, "brain/owner/note.md", "new work after external commit\n")
	externalNativePut(t, o.Home, "brain/owner/new.md", "new authored page\n")
	if err = os.Remove(filepath.Join(o.Home, "brain/owner/.hidden.md")); err != nil {
		t.Fatal(err)
	}
	if status, err := applyExternal(plan, digest, nil); err != nil || status.State != "already_migrated" {
		t.Fatal("repeat recopy failed", status.State, err)
	}
	externalNativeRead(t, o.Home, "brain/owner/note.md", "new work after external commit\n")
	if _, err = os.Stat(filepath.Join(o.Home, "brain/owner/.hidden.md")); !os.IsNotExist(err) {
		t.Fatal("deleted work resurrected", err)
	}
	_, err = rollbackExternal(o.Home, plan.AttemptID, digest, func(event ExternalProgress) error {
		if event.Phase == "archived" {
			return stopped
		}
		return nil
	})
	if !errors.Is(err, stopped) {
		t.Fatal("archive interruption not observed", err)
	}
	if inspected, err := InspectExternal(o.Home, plan.AttemptID, digest); err != nil || inspected.Current || inspected.State != "revoked" {
		t.Fatal("interrupted archive proof invalid", inspected.State, err)
	}
	status, err := rollbackExternal(o.Home, plan.AttemptID, digest, nil)
	if err != nil || status.State != "rolled_back" || status.Current || status.RestoredRuntime {
		t.Fatal("archived rollback resume failed", status.State, err)
	}
	externalNativeRead(t, plan.ArchiveRoot, "brain/owner/note.md", "new work after external commit\n")
	externalNativeRead(t, plan.ArchiveRoot, "brain/owner/new.md", "new authored page\n")
	if _, err = os.Stat(filepath.Join(plan.ArchiveRoot, "brain/owner/.hidden.md")); !os.IsNotExist(err) {
		t.Fatal("archived deletion lost", err)
	}
	externalNativeRead(t, plan.ArchiveRoot, externalName(plan.AttemptID, "rollback.json"), string(externalCanonical(externalRecord{1, plan.AttemptID, digest, "rolled_back"})))
	archived := externalNativeSnapshot(t, plan.ArchiveRoot)
	if status, err = rollbackExternal(o.Home, plan.AttemptID, digest, nil); err != nil || status.State != "rolled_back" {
		t.Fatal("rollback replay failed", status.State, err)
	}
	externalNativeUnchanged(t, plan.ArchiveRoot, archived)
	externalNativeUnchanged(t, o.SourceRoot, source)
	if inspected, err := InspectExternal(o.Home, plan.AttemptID, digest); err != nil || inspected.Current || inspected.State != "rolled_back" {
		t.Fatal("public archived proof failed", inspected.State, err)
	}
	if retired, err := RetiredExternalHome(o.UserHome, o.Home); err != nil || !retired {
		t.Fatal("public retirement proof failed", retired, err)
	}
}
func TestWindowsExternalAdapterAdmittedTransactionReusesRetirementAndRefusesForeignProof(t *testing.T) {
	o := externalNativeTransactionFixture(t)
	source := externalNativeSnapshot(t, o.SourceRoot)
	stopped := errors.New("synthetic retirement interruption")
	for attempt := 0; attempt < 2; attempt++ {
		plan, digest := externalNativePlan(t, o)
		if status, err := applyExternal(plan, digest, nil); err != nil || !status.Current {
			t.Fatal("repeated apply failed", attempt, status.State, err)
		}
		_, err := rollbackExternal(o.Home, plan.AttemptID, digest, func(event ExternalProgress) error {
			if event.Phase == "retired" {
				return stopped
			}
			return nil
		})
		if !errors.Is(err, stopped) {
			t.Fatal("retirement interruption not observed", attempt, err)
		}
		if retired, err := RetiredExternalHome(o.UserHome, o.Home); err == nil || retired {
			t.Fatal("incomplete retirement admitted", retired, err)
		}
		if status, err := rollbackExternal(o.Home, plan.AttemptID, digest, nil); err != nil || status.State != "rolled_back" {
			t.Fatal("retirement resume failed", attempt, status.State, err)
		}
		if retired, err := RetiredExternalHome(o.UserHome, o.Home); err != nil || !retired {
			t.Fatal("reused retirement authority invalid", attempt, retired, err)
		}
	}
	plan, digest := externalNativePlan(t, o)
	if status, err := applyExternal(plan, digest, nil); err != nil || !status.Current {
		t.Fatal("third apply failed", status.State, err)
	}
	for _, body := range []string{`{"schema_version":1}`, string(externalCanonical(externalRetirement{1, o.Home + "-foreign", o.UserHome, plan.AttemptID, digest, plan.ArchiveRoot}))} {
		externalNativePut(t, o.UserHome, externalRetirementName(o.Home), body)
		active := externalNativeSnapshot(t, o.Home)
		profile := externalNativeSnapshot(t, o.UserHome)
		if _, err := rollbackExternal(o.Home, plan.AttemptID, digest, nil); err == nil {
			t.Fatal("invalid prior retirement proof accepted")
		}
		externalNativeUnchanged(t, o.Home, active)
		externalNativeUnchanged(t, o.UserHome, profile)
		if _, err := os.Stat(plan.ArchiveRoot); !os.IsNotExist(err) {
			t.Fatal("invalid retirement proof moved home", err)
		}
	}
	externalNativeUnchanged(t, o.SourceRoot, source)
}
func TestWindowsExternalAdapterAdmittedTransactionHonorsProfileLeaseAndForeignLocator(t *testing.T) {
	externalNativeAccount(t)
	if profile := os.Getenv("MAESTRO_EXTERNAL_LEASE_CHILD"); profile != "" {
		guard, err := externalOpen(profile)
		if err != nil {
			t.Fatal(err)
		}
		defer guard.close()
		release, err := externalAcquire(guard, os.Getenv("MAESTRO_EXTERNAL_LEASE_ATTEMPT"))
		if err == nil {
			release()
			t.Fatal("second process acquired held migration lease")
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			t.Fatal("second process failed for a reason other than lock contention", err)
		}
		return
	}
	o := externalNativeTransactionFixture(t)
	source := externalNativeSnapshot(t, o.SourceRoot)
	plan, digest := externalNativePlan(t, o)
	guard, err := externalOpen(o.UserHome)
	if err != nil {
		t.Fatal(err)
	}
	release, err := externalAcquire(guard, plan.AttemptID)
	if err != nil {
		guard.close()
		t.Fatal(err)
	}
	func() {
		defer guard.close()
		defer release()
		profile := externalNativeSnapshot(t, o.UserHome)
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		child := exec.Command(executable, "-test.run=^TestWindowsExternalAdapterAdmittedTransactionHonorsProfileLeaseAndForeignLocator$", "-test.count=1", "-test.timeout=30s")
		child.Env = append(os.Environ(), "MAESTRO_EXTERNAL_LEASE_CHILD="+o.UserHome, "MAESTRO_EXTERNAL_LEASE_ATTEMPT="+plan.AttemptID)
		output, err := child.CombinedOutput()
		if err != nil || !strings.Contains(string(output), "PASS") {
			t.Fatal("real child lease contention proof failed", err, string(output))
		}
		if _, err = applyExternal(plan, digest, nil); !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			t.Fatal("admitted transaction ignored held profile lease", err)
		}
		if _, err = os.Stat(o.Home); !os.IsNotExist(err) {
			t.Fatal("held lease created home", err)
		}
		externalNativeUnchanged(t, o.UserHome, profile)
	}()
	if status, err := applyExternal(plan, digest, nil); err != nil || !status.Current {
		t.Fatal("apply after lease release failed", status.State, err)
	}
	externalNativePut(t, o.UserHome, ".maestro-home.json", string(externalCanonical(externalLocator{1, o.Home, "00000000000000000000000000000000"})))
	active := externalNativeSnapshot(t, o.Home)
	profile := externalNativeSnapshot(t, o.UserHome)
	if _, err = rollbackExternal(o.Home, plan.AttemptID, digest, nil); err == nil {
		t.Fatal("foreign locator revoked home")
	}
	if _, err = applyExternal(plan, digest, nil); err == nil {
		t.Fatal("foreign locator overwritten by repeated apply")
	}
	externalNativeUnchanged(t, o.Home, active)
	externalNativeUnchanged(t, o.UserHome, profile)
	externalNativeUnchanged(t, o.SourceRoot, source)
}
