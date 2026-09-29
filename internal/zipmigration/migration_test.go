package zipmigration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// This fixture models an interrupted process after its first durable copy.
// It uses the real planner and persistence path, never an alternate copier.
func interrupted(t *testing.T) (string, Result, []byte) {
	t.Helper()
	root := t.TempDir()
	put(t, root, "data/owner/a.md", "a")
	put(t, root, "data/owner/b.md", "b")
	handle, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	e := engine{root: handle, project: root}
	records, err := e.inventory("data")
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.plan(records)
	if err != nil {
		t.Fatal(err)
	}
	planPath := control + "/attempts/" + p.AttemptID + "/plan.json"
	sha, err := e.write(planPath, p, true)
	if err != nil {
		t.Fatal(err)
	}
	state := fromPlan("partial", p, sha)
	state.Resumable = true
	if _, err = e.write(control+"/state.json", state, false); err != nil {
		t.Fatal(err)
	}
	for _, entry := range p.Entries {
		if entry.Source == "owner" || entry.Source == "owner/a.md" {
			if err = e.copyEntry(entry); err != nil {
				t.Fatal(err)
			}
		}
	}
	content, err := handle.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	return root, state, content
}
func TestInterruptedCopyResumesSameImmutablePlan(t *testing.T) {
	root, first, before := interrupted(t)
	if got := Run(root, Options{Status: true}); got.State != "partial" || !got.Resumable {
		t.Fatal(got)
	}
	got := Run(root, Options{})
	if got.State != "committed" || got.AttemptID != first.AttemptID {
		t.Fatal(got)
	}
	after, _ := os.ReadFile(filepath.Join(root, control, "attempts", got.AttemptID, "plan.json"))
	if string(before) != string(after) {
		t.Fatal("plan was rewritten")
	}
	b, err := os.ReadFile(filepath.Join(root, "brain/owner/b.md"))
	if err != nil || string(b) != "b" {
		t.Fatal("resume omitted file")
	}
}
func TestInterruptedCopyRejectsUnownedTargetContent(t *testing.T) {
	root, _, _ := interrupted(t)
	put(t, root, "brain/another-owner.md", "authored after interruption")
	got := Run(root, Options{})
	if got.State != "blocked" {
		t.Fatalf("ambiguous new target content accepted: %+v", got)
	}
}

func TestPartialSourceDriftIsBlockedNotResumable(t *testing.T) {
	root, _, _ := interrupted(t)
	put(t, root, "data/owner/b.md", "changed after interruption")
	got := Run(root, Options{Status: true})
	if got.State != "blocked" || got.Resumable {
		t.Fatalf("unsafe resume advertised: %+v", got)
	}
}

func TestCopyFailureRetriesAfterPermissionRecovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACL failure is a native-device acceptance case; chmod does not remove write authority")
	}
	root, first, _ := interrupted(t)
	staging := filepath.Join(root, control, "staging")
	if err := os.Chmod(staging, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(staging, 0700)
	failed := Run(root, Options{})
	if failed.State != "partial" || len(failed.Errors) == 0 {
		t.Fatalf("copy failure was hidden: %+v", failed)
	}
	if _, err := os.Stat(filepath.Join(root, "brain/owner/b.md")); !os.IsNotExist(err) {
		t.Fatal("failed copy created target")
	}
	if err := os.Chmod(staging, 0700); err != nil {
		t.Fatal(err)
	}
	recovered := Run(root, Options{})
	if recovered.State != "committed" || recovered.AttemptID != first.AttemptID {
		t.Fatal(recovered)
	}
}
func TestLegacyResolverValidatesBeforeReturningActualContentPath(t *testing.T) {
	root := t.TempDir()
	put(t, root, "data/workspaces/project/context.md", "real legacy context")
	if got := Run(root, Options{}); got.State != "committed" {
		t.Fatal(got)
	}
	resolved := Run(root, Options{ResolveLegacy: "workspaces"})
	if resolved.State != "committed" {
		t.Fatal(resolved)
	}
	b, err := os.ReadFile(filepath.Join(resolved.Path, "project/context.md"))
	if err != nil || string(b) != "real legacy context" {
		t.Fatal("legacy consumer cannot read original content")
	}
	put(t, root, "data/workspaces/project/context.md", "changed")
	if got := Run(root, Options{ResolveLegacy: "workspaces"}); got.State != "blocked" || got.Path != "" {
		t.Fatal("stale source was exposed")
	}
}
func TestUnicodeAndCaseMappedCollisionsArePortable(t *testing.T) {
	for _, pair := range [][2]string{{"craft/index.md", "owner/atlas/craft/index.md"}, {"learnings/index.md", "owner/atlas/learnings/index.md"}, {"profile/Name.json", "owner/name.json"}, {"profile/São.json", "owner/são.json"}, {"profile/straße.json", "owner/STRASSE.json"}} {
		root := t.TempDir()
		put(t, root, "data/"+pair[0], "one")
		put(t, root, "data/"+pair[1], "two")
		got := Run(root, Options{})
		if got.State != "blocked" || !strings.Contains(strings.Join(got.Errors, " "), "collision") {
			t.Fatal(got)
		}
	}
}
func TestPlanOrReceiptTamperingInvalidatesCommittedState(t *testing.T) {
	for _, file := range []string{"plan.json", "receipt.json"} {
		t.Run(file, func(t *testing.T) {
			root := t.TempDir()
			put(t, root, "data/owner/a.md", "a")
			first := Run(root, Options{})
			if first.State != "committed" {
				t.Fatal(first)
			}
			put(t, root, control+"/attempts/"+first.AttemptID+"/"+file, "{}")
			if got := Run(root, Options{Status: true}); got.State != "blocked" {
				t.Fatal(got)
			}
		})
	}
}
func TestCommittedEditsKeepRuntimeAndLegacyReadable(t *testing.T) {
	root := t.TempDir()
	put(t, root, "data/owner/a.md", "old")
	put(t, root, "data/workspaces/p/context.md", "retained")
	first := Run(root, Options{})
	if first.State != "committed" {
		t.Fatal(first)
	}
	receiptPath := filepath.Join(root, control, "attempts", first.AttemptID, "receipt.json")
	before, _ := os.ReadFile(receiptPath)
	put(t, root, "brain/owner/a.md", "new authored work")
	next := Run(root, Options{})
	if next.State != "committed" || next.Verification != "target_evolved" || next.Current {
		t.Fatal(next)
	}
	after, _ := os.ReadFile(receiptPath)
	if string(before) != string(after) {
		t.Fatal("historical receipt rewritten")
	}
	resolved := Run(root, Options{ResolveLegacy: "workspaces"})
	if resolved.State != "committed" || resolved.Path == "" {
		t.Fatal(resolved)
	}
	put(t, root, "data/owner/a.md", "unrelated legacy edit")
	resolved = Run(root, Options{ResolveLegacy: "workspaces"})
	if resolved.State != "committed" || resolved.Path == "" {
		t.Fatal("unrelated namespace blocked valid legacy adapter", resolved)
	}
}
func TestReadOnlyDestinationReportsFailureWithoutSuccessMarker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACL failure is a native-device acceptance case; chmod does not remove write authority")
	}
	root := t.TempDir()
	put(t, root, "data/owner/a.md", "a")
	if err := os.Chmod(root, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(root, 0700)
	got := Run(root, Options{})
	if got.State == "committed" {
		t.Fatal("read-only destination reported success")
	}
	if _, err := os.Stat(filepath.Join(root, "data/.migrated-to-brain")); !os.IsNotExist(err) {
		t.Fatal("original marker was written")
	}
}
func TestOriginalInventoryIncludesEmptyAndHiddenEntries(t *testing.T) {
	root := t.TempDir()
	put(t, root, "data/.private", "secret")
	put(t, root, "data/README.md", "local notes")
	os.MkdirAll(filepath.Join(root, "data/agents/empty"), 0700)
	got := Run(root, Options{})
	if got.State != "committed" {
		t.Fatal(got)
	}
	b, _ := os.ReadFile(filepath.Join(root, control, "attempts", got.AttemptID, "plan.json"))
	var p Plan
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Entries) != 4 {
		t.Fatalf("omitted inventory entries: %+v", p.Entries)
	}
}

func put(t *testing.T, root, path, content string) {
	t.Helper()
	name := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestPreservesEveryNamespaceAndOriginal(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"owner/identity.json", "agents/custom/agent.md", "workspaces/project-a/context.md", "canary/state.json", "custom/ação 1.md", ".private"} {
		put(t, root, "data/"+p, "original")
	}
	got := Run(root, Options{})
	if got.State != "committed" {
		t.Fatalf("migration omitted or blocked valid workspace: %+v", got)
	}
	for _, p := range []string{"owner/identity.json", "custom/ação 1.md", ".private"} {
		b, err := os.ReadFile(filepath.Join(root, "brain", p))
		if err != nil || string(b) != "original" {
			t.Fatalf("missing copy %s: %s %v", p, b, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "data/.migrated-to-brain")); !os.IsNotExist(err) {
		t.Fatal("original data was changed")
	}
}
func TestDualTreeNeverGreenOrOverwrite(t *testing.T) {
	root := t.TempDir()
	put(t, root, "data/owner/a.md", "old")
	put(t, root, "data/.migrated-to-brain", "stale")
	put(t, root, "brain/owner/a.md", "new")
	put(t, root, "brain/.initialized", "stale")
	got := Run(root, Options{})
	if got.State != "blocked" {
		t.Fatalf("dual tree green: %+v", got)
	}
	b, _ := os.ReadFile(filepath.Join(root, "brain/owner/a.md"))
	if string(b) != "new" {
		t.Fatal("overwritten")
	}
}
func TestDriftIsNeverGreen(t *testing.T) {
	for _, path := range []string{"data/owner/a.md", "brain/owner/a.md"} {
		t.Run(path, func(t *testing.T) {
			root := t.TempDir()
			put(t, root, "data/owner/a.md", "original")
			if got := Run(root, Options{}); got.State != "committed" {
				t.Fatal(got)
			}
			put(t, root, path, "changed")
			got := Run(root, Options{Status: true})
			if path == "data/owner/a.md" {
				if got.State != "blocked" {
					t.Fatal(got)
				}
			} else {
				if got.State != "committed" || got.Current || got.Verification != "target_evolved" {
					t.Fatalf("authored edit blocked runtime or reused stale PASS: %+v", got)
				}
			}
		})
	}
}
func TestDryRunDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	put(t, root, "data/owner/a.md", "x")
	if got := Run(root, Options{DryRun: true}); got.State != "planned" {
		t.Fatal(got)
	}
	if _, err := os.Stat(filepath.Join(root, "brain")); !os.IsNotExist(err) {
		t.Fatal("dry run wrote brain")
	}
}
func TestCollisionAndSymlinkBlockBeforeCopy(t *testing.T) {
	root := t.TempDir()
	put(t, root, "data/profile/identity.json", "profile")
	put(t, root, "data/owner/identity.json", "owner")
	if got := Run(root, Options{}); got.State != "blocked" {
		t.Fatal(got)
	}
	if _, err := os.Stat(filepath.Join(root, "brain/owner/identity.json")); !os.IsNotExist(err) {
		t.Fatal("collision copied")
	}
	root = t.TempDir()
	put(t, root, "data/owner/a.md", "x")
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "data/escape")); err != nil {
		t.Skip(err)
	}
	if got := Run(root, Options{}); got.State != "blocked" {
		t.Fatal(got)
	}
}
func TestCaseMarkersAndEmptyDirectories(t *testing.T) {
	root := t.TempDir()
	put(t, root, "data/cases/example/brain/canon/a.md", "a")
	put(t, root, "data/cases/.active", "example\n")
	put(t, root, "data/cases/.pending", "example\n")
	os.MkdirAll(filepath.Join(root, "data/custom/empty"), 0700)
	if got := Run(root, Options{}); got.State != "committed" {
		t.Fatal(got)
	}
	for _, m := range []string{".active", ".pending"} {
		b, err := os.ReadFile(filepath.Join(root, "brain/accounts", m))
		if err != nil || string(b) != "_sem-conta/example\n" {
			t.Fatalf("marker %s %q %v", m, b, err)
		}
	}
	if info, err := os.Stat(filepath.Join(root, "brain/custom/empty")); err != nil || !info.IsDir() {
		t.Fatal("empty directory omitted")
	}
}
func TestRollbackPreservesNewWork(t *testing.T) {
	root := t.TempDir()
	put(t, root, "data/owner/a.md", "original")
	first := Run(root, Options{})
	if first.State != "committed" {
		t.Fatal(first)
	}
	put(t, root, "brain/owner/a.md", "newer")
	got := Run(root, Options{Rollback: first.AttemptID})
	if got.State != "rolled_back" || got.RestoredRuntime {
		t.Fatal(got)
	}
	for path, want := range map[string]string{"data/owner/a.md": "original", "brain/owner/a.md": "newer"} {
		b, _ := os.ReadFile(filepath.Join(root, path))
		if string(b) != want {
			t.Fatal("rollback changed data")
		}
	}
}
