//go:build windows

package atlasops

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatelock"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"golang.org/x/sys/windows"
)

func nativeAtlasAccount(t *testing.T) {
	t.Helper()
	if os.Getenv("MAESTRO_WINDOWS_CORE_PROOF") != "1" {
		t.Skip("dedicated actual ordinary-account driver required")
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
		t.Fatal("actual ordinary nonadmin Windows token required", err, elevated, member)
	}
}
func nativeAtlasEngine(t *testing.T) (*Engine, atlas.Options) {
	t.Helper()
	nativeAtlasAccount(t)
	base := t.TempDir()
	registered, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(base, "workspace"), DataRoot: filepath.Join(base, "state")})
	if err != nil {
		t.Fatal(err)
	}
	options := atlas.Options{DataRoot: registered.DataRoot, WorkspacePath: registered.WorkspacePath, WorkspaceID: registered.WorkspaceID}
	engine, err := OpenWorkspace(options)
	if err != nil {
		t.Fatal(err)
	}
	return engine, options
}

// Stage1 qualifies real pin/read/publication/lock behavior while the public
// workspace mutation availability gate remains closed. It creates no runtime
// bypass and never treats source or a cross-build as native filesystem proof.
func TestWindowsAtlasPublicationKernel(t *testing.T) {
	nativeAtlasAccount(t)
	t.Run("held_binding_journal_and_lock_custody", func(t *testing.T) {
		engine, options := nativeAtlasEngine(t)
		if err := scheduler.EnsurePrivateDirectory(engine.journalDirPath("")); err != nil {
			t.Fatal(err)
		}
		release, err := engine.acquireCorpusLock()
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		for _, path := range []string{filepath.Dir(options.WorkspacePath), options.DataRoot, options.WorkspacePath, engine.root, engine.journalDirPath(""), engine.journalDirPath("grants.lock")} {
			renamed := path + "-retargeted"
			if err := os.Rename(path, renamed); err == nil {
				_ = os.Rename(renamed, path)
				t.Fatalf("held atlas transition allowed rename of %s", filepath.Base(path))
			}
		}
		journal, err := winprivate.OpenPinned(engine.journalDirPath(""))
		if err != nil {
			t.Fatal(err)
		}
		defer journal.Close()
		contender, err := journal.OpenLock("grants.lock")
		if err != nil {
			t.Fatal(err)
		}
		defer contender.Close()
		var overlapped windows.Overlapped
		err = windows.LockFileEx(windows.Handle(contender.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			if err == nil {
				_ = windows.UnlockFileEx(windows.Handle(contender.Fd()), 0, 1, 0, &overlapped)
			}
			t.Fatal("persistent corpus lock did not order another native handle", err)
		}
		page := filepath.Join(engine.root, "native.md")
		first, second := []byte("# First native page\n"), []byte("# Complete replacement\n")
		if err = publishAtlasFile(page, first, true, true); err != nil {
			t.Fatal(err)
		}
		if err = publishAtlasFile(page, second, true, true); !errors.Is(err, os.ErrExist) {
			t.Fatal("exclusive native publication did not preserve occupied target", err)
		}
		if got, err := readAtlasFile(page, 256<<10, true); err != nil || !bytes.Equal(got, first) {
			t.Fatal("occupied publication changed first page", err)
		}
		if err = publishAtlasFile(page, second, false, true); err != nil {
			t.Fatal(err)
		}
		if got, err := readAtlasFile(page, 256<<10, true); err != nil || !bytes.Equal(got, second) {
			t.Fatal("held native replacement lost complete bytes", err)
		}
		t.Log("native_atlas_custody bound_directories_pinned=true journal_pinned=true lock_leaf_pinned=true native_lock_exclusive=true occupied_target_preserved=true replacement_readback=true")
	})
	t.Run("legacy_lock_bytes_and_custody_survive_refusal", func(t *testing.T) {
		engine, options := nativeAtlasEngine(t)
		if _, err := engine.CreatePage(CreatePageRequest{Page: "preserved.md", Body: "# Preserve existing native page\n", Provenance: attended("legacy-lock-seed")}); err != nil {
			t.Fatal(err)
		}
		if _, err := engine.CreateGrant(GrantRequest{Ritual: "legacy-lock-proof", RitualVersion: "1", Segment: "projects", Operations: []string{"create-page"}, Cadence: "manual", Reader: ReaderOwnerSession}); err != nil {
			t.Fatal(err)
		}
		path := engine.journalDirPath("grants.lock")
		const token = "0123456789abcdef0123456789abcdef"
		journal, err := winprivate.OpenPinned(engine.journalDirPath(""))
		if err != nil {
			t.Fatal(err)
		}
		file, err := journal.OpenLock("grants.lock")
		if err != nil {
			_ = journal.Close()
			t.Fatal(err)
		}
		_, writeErr := file.WriteString(token)
		if err = errors.Join(writeErr, file.Close(), journal.Close()); err != nil {
			t.Fatal(err)
		}
		security := func() string {
			descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.GROUP_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
			if err != nil {
				t.Fatal(err)
			}
			return descriptor.String()
		}
		beforeSecurity := security()
		beforeWorkspace, beforeData := snapshotAtlasText(t, options.WorkspacePath), snapshotAtlasText(t, options.DataRoot)
		release, err := engine.acquireCorpusLock()
		if release != nil {
			if closeErr := release(); closeErr != nil {
				t.Fatal(closeErr)
			}
		}
		if err == nil || errors.Is(err, privatelock.ErrBusy) || !strings.Contains(err.Error(), "legacy or unexpected data") {
			t.Error("legacy lock was not refused distinctly", err)
		}
		if _, err = engine.CreatePage(CreatePageRequest{Page: "must-not-land.md", Body: "# Must not land\n", Provenance: attended("refuse-legacy-lock")}); err == nil || errors.Is(err, privatelock.ErrBusy) || !strings.Contains(err.Error(), "legacy or unexpected data") {
			t.Error("public mutation did not preserve legacy lock refusal", err)
		}
		if !reflect.DeepEqual(beforeWorkspace, snapshotAtlasText(t, options.WorkspacePath)) || !reflect.DeepEqual(beforeData, snapshotAtlasText(t, options.DataRoot)) || security() != beforeSecurity {
			t.Fatal("legacy refusal changed page, journal, grant, token, mode or security descriptor")
		}
		// Only the legacy owner verifies and removes its exact token; native
		// refusal must not retain a handle that blocks this owner release.
		if got, err := os.ReadFile(path); err != nil || string(got) != token {
			t.Fatal("legacy owner lost its token", err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal("legacy owner could not release its own guard", err)
		}
		release, err = engine.acquireCorpusLock()
		if err != nil {
			t.Fatal("native acquire after legacy owner's release failed", err)
		}
		first := nativeAtlasIdentitySnapshot(t, path)
		if err = release(); err != nil {
			t.Fatal(err)
		}
		release, err = engine.acquireCorpusLock()
		if err != nil {
			t.Fatal("native guard could not be reacquired", err)
		}
		second := nativeAtlasIdentitySnapshot(t, path)
		if err = release(); err != nil {
			t.Fatal(err)
		}
		if first.Size() != 0 || second.Size() != 0 || !os.SameFile(first, second) {
			t.Fatal("native release replaced or filled its persistent empty guard")
		}
		// Preallocate a distinct same-byte object. The closed snapshot must
		// neither block replacement nor lazily adopt that replacement's ID.
		control, replacement := filepath.Join(engine.root, "identity-control"), filepath.Join(engine.root, "identity-replacement")
		for _, target := range []string{control, replacement} {
			if err := scheduler.WriteNewPrivateFile(target, []byte("same-native-identity-bytes")); err != nil {
				t.Fatal(err)
			}
		}
		beforeReplacement := nativeAtlasIdentitySnapshot(t, control)
		if err := os.Rename(replacement, control); err != nil {
			t.Fatal("closed identity snapshot blocked same-byte replacement", err)
		}
		if os.SameFile(beforeReplacement, nativeAtlasIdentitySnapshot(t, control)) {
			t.Fatal("native identity oracle accepted a distinct same-byte replacement")
		}
		if got, err := scheduler.ReadPrivateFile(control, 64); err != nil || string(got) != "same-native-identity-bytes" {
			t.Fatal("identity replacement control changed bytes", err)
		}
		t.Log("native_atlas_legacy_lock distinct_refusal=true token_preserved=true page_journal_grant_unchanged=true security_unchanged=true legacy_owner_released=true native_reacquired_same_empty_guard=true")
	})
	t.Run("aliases_and_unsafe_acl_preserve_outside_bytes", func(t *testing.T) {
		engine, _ := nativeAtlasEngine(t)
		outside := filepath.Join(filepath.Dir(engine.root), "outside.md")
		body := []byte("# Outside synthetic bytes\n")
		if err := scheduler.WritePrivateFileAtomic(outside, body); err != nil {
			t.Fatal(err)
		}
		alias := filepath.Join(engine.root, "alias.md")
		if err := os.Link(outside, alias); err != nil {
			t.Fatal("required native hardlink fixture unavailable", err)
		}
		if _, err := readAtlasFile(alias, 256<<10, true); err == nil {
			t.Fatal("native atlas read followed hardlink")
		}
		if err := publishAtlasFile(alias, []byte("must not replace"), false, true); err == nil {
			t.Fatal("native replacement accepted hardlink")
		}
		if got, err := os.ReadFile(outside); err != nil || !bytes.Equal(got, body) {
			t.Fatal("outside hardlink bytes changed", err)
		}
		junction := filepath.Join(engine.root, "junction")
		// A junction does not require developer mode or symbolic-link privileges.
		target := filepath.Join(filepath.Dir(engine.root), "junction-target")
		if err := scheduler.EnsurePrivateDirectory(target); err != nil {
			t.Fatal(err)
		}
		if err := scheduler.WritePrivateFileAtomic(filepath.Join(target, "outside.md"), body); err != nil {
			t.Fatal(err)
		}
		if err := nativeAtlasJunction(junction, target); err != nil {
			t.Fatal("required ordinary-user junction fixture unavailable", err)
		}
		if _, err := readAtlasFile(filepath.Join(junction, "outside.md"), 256<<10, true); err == nil {
			t.Fatal("native atlas read traversed junction")
		}
		if err := publishAtlasFile(filepath.Join(junction, "outside.md"), []byte("must not land"), false, true); err == nil {
			t.Fatal("native atlas publication traversed junction")
		}
		if got, err := os.ReadFile(filepath.Join(target, "outside.md")); err != nil || !bytes.Equal(got, body) {
			t.Fatal("junction outside bytes changed", err)
		}
		unsafe := filepath.Join(engine.root, "unsafe.md")
		if err := publishAtlasFile(unsafe, body, true, true); err != nil {
			t.Fatal(err)
		}
		descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
		if err != nil {
			t.Fatal(err)
		}
		acl, _, err := descriptor.DACL()
		if err != nil {
			t.Fatal(err)
		}
		if err = windows.SetNamedSecurityInfo(unsafe, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
			t.Fatal(err)
		}
		if _, err = readAtlasFile(unsafe, 256<<10, true); err == nil {
			t.Fatal("native atlas read admitted unsafe DACL")
		}
		if err = publishAtlasFile(unsafe, []byte("must not repair"), false, true); err == nil {
			t.Fatal("native atlas publication repaired unsafe DACL")
		}
		if got, err := os.ReadFile(unsafe); err != nil || !bytes.Equal(got, body) {
			t.Fatal("unsafe-DACL bytes changed", err)
		}
		t.Log("native_atlas_confinement hardlink_refused=true junction_refused=true unsafe_acl_refused=true outside_bytes_preserved=true")
	})
	t.Run("eager_binding_and_retargeted_journal", func(t *testing.T) {
		engine, _ := nativeAtlasEngine(t)
		old := engine.root + "-old"
		if err := os.Rename(engine.root, old); err != nil {
			t.Fatal(err)
		}
		if err := scheduler.EnsurePrivateDirectory(engine.root); err != nil {
			t.Fatal(err)
		}
		body, err := scheduler.ReadPrivateFile(filepath.Join(old, "README.md"), 256<<10)
		if err != nil {
			t.Fatal(err)
		}
		if err = scheduler.WritePrivateFileAtomic(filepath.Join(engine.root, "README.md"), body); err != nil {
			t.Fatal(err)
		}
		if _, err = engine.ReviewPage(CollectRequest{Purpose: "Refuse replacement brain identity", Reader: ReaderOwnerSession, Pages: []string{"README.md"}}); err == nil {
			t.Fatal("replaced brain satisfied lazy binding identity")
		}
		if err = scheduler.EnsurePrivateDirectory(engine.journalDirPath("")); err != nil {
			t.Fatal(err)
		}
		if release, err := engine.acquireCorpusLock(); err == nil {
			_ = release()
			t.Fatal("pin acquisition admitted a replacement brain")
		}
		t.Log("native_atlas_binding eager_identity=true changed_root_refused=true guard_rechecks_pinned_binding=true")
	})
}

// Stage2 runs this separately, only after actual kernel confinement evidence
// permits enabling the public backend. No conditional success or skipped
// recovery branch can stand in for an interrupted real page effect.
func TestWindowsAtlasJournalKernel(t *testing.T) {
	nativeAtlasAccount(t)
	for _, operation := range []string{"append", "update"} {
		t.Run(operation, func(t *testing.T) {
			engine, options := nativeAtlasEngine(t)
			const page = "journal.md"
			initial, err := engine.CreatePage(CreatePageRequest{Page: page, Body: "# Journal\n\n## Decisions\n", Provenance: attended("native-initial")})
			if err != nil {
				t.Fatal(err)
			}
			apply := func(e *Engine) (Result, error) {
				if operation == "append" {
					return e.AppendEntry(AppendEntryRequest{Page: page, Section: "## Decisions", Entry: "- Exactly once after interruption.", ExpectedRevision: initial.Revision, Provenance: attended("native-effect")})
				}
				return e.UpdatePage(UpdatePageRequest{Page: page, Body: "# Exactly once after interruption.\n", ExpectedRevision: initial.Revision, Provenance: attended("native-effect")})
			}
			interrupted := errors.New("synthetic interruption after actual native page publication")
			engine.afterPageEffect = func() error { return interrupted }
			if _, err = apply(engine); !errors.Is(err, interrupted) {
				t.Fatal("native page-effect interruption not reached", err)
			}
			restarted, err := OpenWorkspace(options)
			if err != nil {
				t.Fatal(err)
			}
			result, err := apply(restarted)
			if err != nil || result.State != StateUnchanged {
				t.Fatal("native explicit retry did not finalize actual prepared effect", result, err)
			}
			read, err := restarted.ReviewPage(CollectRequest{Purpose: "Read actual recovered native journal effect", Reader: ReaderOwnerSession, Pages: []string{page}})
			if err != nil || strings.Count(read.Content, "Exactly once after interruption.") != 1 {
				t.Fatal("native journal recovery lost or duplicated effect", err)
			}
			t.Logf("native_atlas_journal operation=%s actual_page_effect=true interrupted_receipt=true restarted_explicit_retry=true exactly_once=true", operation)
		})
	}
}

func nativeAtlasJunction(alias, target string) error {
	command := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"), "/c", "mklink", "/J", alias, target)
	return command.Run()
}

// File.Stat eagerly captures the opened Windows object ID. Close before the
// next transition so this oracle adds no custody or delayed pathname lookup.
func nativeAtlasIdentitySnapshot(t *testing.T, path string) os.FileInfo {
	t.Helper()
	file, err := scheduler.OpenRegularFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, statErr := file.Stat()
	if err := errors.Join(statErr, file.Close()); err != nil {
		t.Fatal(err)
	}
	return info
}
