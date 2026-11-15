package workspaceagent

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentscaffold"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspacepolicy"
)

func assertCaseSetupRecovery(t *testing.T) {
	assertClientStatusCannotRelabelSignedAccount(t)
	assertSetupAncestorReplacementIsConfined(t)
	assertSetupCleanupKeepsChangedPending(t)
	assertInternalCaseCannotAdoptForeignRegistration(t)
	assertExistingCaseSetupBinding(t)
	assertCaseSetupLateEdit(t)
	t.Helper()
	for _, boundary := range []string{"registration", "ledger_create_before_link", "active_publication", "pending_cleanup"} {
		base := t.TempDir()
		root := filepath.Join(base, "state")
		path := filepath.Join(base, "workspace")
		initialized, err := workspace.Initialize(workspace.Options{WorkspacePath: path, DataRoot: root})
		if err != nil {
			t.Fatal(err)
		}
		owner, err := OpenSetup(root, path, initialized.WorkspaceID)
		if err != nil {
			t.Fatal(err)
		}
		input := SetupInput{Mode: "client", ProjectName: "Synthetic resumable first case", AccountID: "synthetic", AccountOwner: "owner", AccountMandate: "Bounded synthetic metadata only."}
		first, err := owner.ReviewPrepare(input, "absent")
		if err != nil {
			t.Fatal(err)
		}
		if boundary == "registration" {
			owner.FaultPoint = func(p string) error {
				if p == boundary {
					return errors.New("synthetic interruption")
				}
				return nil
			}
		}
		status, err := owner.ApplyPrepare(first)
		if boundary == "registration" {
			if err == nil || status.State != "preparing" {
				t.Fatal("registration interruption lost partial progress", status, err)
			}
			owner.FaultPoint = nil
			again, e := owner.ReviewPrepare(input, status.SetupRevision)
			if e != nil {
				t.Fatal(e)
			}
			status, err = owner.ApplyPrepare(again)
		}
		if err != nil || status.State != "pending" {
			t.Fatal("prepare failed", status, err)
		}
		page := SetupPage{Page: status.Setup.BriefPage, Revision: strings.Repeat("a", 64), Text: fmt.Sprintf("---\nid: %s\ntitle: Synthetic first brief\nsummary: Reviewed synthetic first result.\ntype: project-brief\nscope: account/synthetic/case/%s\nstatus: draft\nsensitivity: client-confidential\nupdated: 2026-11-15\n---\n# Synthetic first brief\n", strings.TrimSuffix(status.Setup.BriefPage, ".md"), initialized.WorkspaceID)}
		work := &SetupWork{Objective: "Continue synthetic result", NextStep: "Read same case after restart", Summary: "Reviewed first result"}
		review, err := owner.ReviewFinish(status.SetupRevision, page, work)
		if err != nil {
			t.Fatal(err)
		}
		if boundary != "registration" {
			owner.FaultPoint = func(p string) error {
				if p == boundary {
					return errors.New("synthetic interruption")
				}
				return nil
			}
		}
		status, err = owner.ApplyFinish(review, func() (SetupPage, error) { return page, nil })
		if boundary == "ledger_create_before_link" {
			partial := owner.PartialStatus()
			if partial["workspace_id"] != initialized.WorkspaceID || partial["ledger"] == nil {
				t.Fatal("partial error omitted actual stable ledger progress", partial)
			}
		}
		if boundary != "registration" {
			if err == nil {
				t.Fatal("durable boundary did not interrupt", boundary)
			}
			owner.Close()
			owner, err = OpenSetup(root, path, initialized.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			status, err = owner.Status()
			if err != nil {
				t.Fatal(err)
			}
			review, err = owner.ReviewFinish(status.SetupRevision, page, work)
			if err != nil {
				t.Fatal(err)
			}
			status, err = owner.ApplyFinish(review, func() (SetupPage, error) { return page, nil })
		}
		if err != nil || status.State != "active" {
			t.Fatal("cold retry did not finish", boundary, status, err)
		}
		store, e := execution.OpenConfinedStore(root)
		if e != nil {
			t.Fatal(e)
		}
		items, e := store.List(initialized.WorkspaceID)
		if e != nil || len(items) != 1 {
			t.Fatal("crash duplicated or lost setup work", boundary, items, e)
		}
		item, e := store.Inspect(initialized.WorkspaceID, status.Setup.ItemID)
		if e != nil || item.State.StateRevision != 4 || item.State.State != execution.StatePaused {
			t.Fatal("retry lost paused checkpoint", item, e)
		}
		markers, e := workspacepolicy.SnapshotCaseMarkers(path)
		if e != nil || markers.Pending != "" || strings.TrimSpace(markers.Active) != "synthetic/"+initialized.WorkspaceID {
			t.Fatal("commit or cleanup incomplete", markers, e)
		}
		owner.Close()
	}
	// A changed selector after review cannot create an unauthorized setup record.
	base := t.TempDir()
	root := filepath.Join(base, "state")
	path := filepath.Join(base, "workspace")
	init, err := workspace.Initialize(workspace.Options{WorkspacePath: path, DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := OpenSetup(root, path, init.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	review, err := owner.ReviewPrepare(SetupInput{Mode: "internal", ProjectName: "Synthetic guarded binding"}, "absent")
	if err != nil {
		t.Fatal(err)
	}
	markers := filepath.Join(path, "brain", "accounts")
	if err = os.MkdirAll(markers, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(markers, ".pending"), []byte("foreign/case\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.ApplyPrepare(review); err == nil {
		t.Fatal("changed marker bypassed review")
	}
	if _, err = os.Lstat(owner.path()); !os.IsNotExist(err) {
		t.Fatal("rejected review created setup metadata", err)
	}
}

func assertExistingCaseSetupBinding(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "state")
	path := filepath.Join(base, "workspace")
	current, err := workspace.Initialize(workspace.Options{WorkspacePath: path, DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Initialize(root, current.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	account, err := agentscaffold.Scaffold(root, agentscaffold.Request{AgentID: "account-agent-legacy", Role: "client_account_agent", ScopeKind: "account", ScopeID: "legacy", ParentAgent: "maestro", ParentRole: "hub", Owner: "owner", Mandate: "Preserve the legacy account.", DisplayName: "Curated account alias", Emoji: "🧪", OwnerID: "owner", OwnershipScope: "account"})
	if err != nil {
		t.Fatal(err)
	}
	prior, err := agentscaffold.Scaffold(root, agentscaffold.Request{AgentID: "case-agent-existing-case", Role: "case_agent", ScopeKind: "case", ScopeID: "existing-case", ParentAgent: "maestro", ParentRole: "hub", AccountAgentID: account.Instance.AgentID, DisplayName: "Curated case alias", Emoji: "🧭", OwnerID: "owner", OwnershipScope: "case"})
	if err != nil {
		t.Fatal(err)
	}
	markers := filepath.Join(path, "brain", "accounts")
	if err = os.MkdirAll(markers, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(markers, ".active"), []byte("\uFEFFLEGACY/EXISTING-CASE\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	owner, err := OpenSetup(root, path, current.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	status, err := owner.Status()
	if err != nil || status.State != "reconciliation_required" {
		t.Fatal("legacy selection was reinterpreted as fresh", status, err)
	}
	review, err := owner.ReviewPrepare(SetupInput{Mode: "client", ProjectName: "Existing reviewed case", AccountID: "legacy", AccountOwner: "owner", AccountMandate: "Preserve the legacy account."}, status.SetupRevision)
	if err != nil {
		t.Fatal(err)
	}
	status, err = owner.ApplyPrepare(review)
	if err != nil {
		t.Fatal(err)
	}
	if status.Setup.CaseID != "existing-case" || status.Setup.CaseAgentID != prior.Instance.AgentID || status.Setup.AccountAgentID != account.Instance.AgentID || status.Setup.BriefPage != "accounts/legacy/cases/existing-case/existing-case.md" {
		t.Fatal("legacy IDs or paths changed", status)
	}
	after, err := agentscaffold.Inspect(root, prior.Instance.AgentID)
	if err != nil || after.Instance.HMACSHA256 != prior.Instance.HMACSHA256 || after.Instance.CreatedAt != prior.Instance.CreatedAt || after.Instance.DisplayName != "Curated case alias" {
		t.Fatal("existing signed case was renamed or re-signed", after, err)
	}
	if _, err = agentscaffold.Inspect(root, "case-agent-"+current.WorkspaceID); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("legacy setup created a parallel case", err)
	}
}
func assertCaseSetupLateEdit(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "state")
	path := filepath.Join(base, "workspace")
	current, err := workspace.Initialize(workspace.Options{WorkspacePath: path, DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := OpenSetup(root, path, current.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	review, err := owner.ReviewPrepare(SetupInput{Mode: "client", ProjectName: "Synthetic late edit", AccountID: "synthetic", AccountOwner: "owner", AccountMandate: "Bounded synthetic account."}, "absent")
	if err != nil {
		t.Fatal(err)
	}
	status, err := owner.ApplyPrepare(review)
	if err != nil {
		t.Fatal(err)
	}
	page := SetupPage{Page: status.Setup.BriefPage, Revision: strings.Repeat("a", 64), Text: fmt.Sprintf("---\nid: %s\ntitle: Synthetic brief\nsummary: Synthetic scope.\ntype: project-brief\nscope: account/synthetic/case/%s\nstatus: draft\nsensitivity: client-confidential\nupdated: 2026-11-15\n---\n# Synthetic brief\n", strings.TrimSuffix(status.Setup.BriefPage, ".md"), current.WorkspaceID)}
	work := &SetupWork{Objective: "Continue the accepted case", NextStep: "Read the latest brief", Summary: "Accepted initial handoff"}
	review, err = owner.ReviewFinish(status.SetupRevision, page, work)
	if err != nil {
		t.Fatal(err)
	}
	owner.FaultPoint = func(point string) error {
		if point == "ledger_create_before_link" {
			page.Text += "\nConcurrent author correction.\n"
			page.Revision = strings.Repeat("b", 64)
		}
		return nil
	}
	if _, err = owner.ApplyFinish(review, func() (SetupPage, error) { return page, nil }); !errors.Is(err, ErrSetupChanged) {
		t.Fatal("page changed during ledger publication was not fenced", err)
	}
	markers, err := workspacepolicy.SnapshotCaseMarkers(path)
	if err != nil || markers.Active != "" || markers.Pending == "" {
		t.Fatal("stale brief committed a selection", markers, err)
	}
	status, err = owner.Status()
	if err != nil || status.Ledger == nil || status.Ledger["state_revision"] != 4 {
		t.Fatal("authorized partial ledger work was not reported", status, err)
	}
	owner.FaultPoint = nil
	review, err = owner.ReviewFinish(status.SetupRevision, page, work)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.ApplyFinish(review, func() (SetupPage, error) { return page, nil }); err != nil {
		t.Fatal("reviewed latest page did not recover same work", err)
	}
}

func assertInternalCaseCannotAdoptForeignRegistration(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "state")
	path := filepath.Join(base, "workspace")
	current, err := workspace.Initialize(workspace.Options{WorkspacePath: path, DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := OpenSetup(root, path, current.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	input := SetupInput{Mode: "internal", ProjectName: "Synthetic internal scope"}
	review, err := owner.ReviewPrepare(input, "absent")
	if err != nil {
		t.Fatal(err)
	}
	status, err := owner.ApplyPrepare(review)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(base, "foreign-workspace"), DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Initialize(root, foreign.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	registration, err := agentscaffold.Scaffold(root, agentscaffold.WorkspaceRequest(foreign.WorkspaceID))
	if err != nil {
		t.Fatal(err)
	}
	modified := *status.Setup
	modified.CaseID = foreign.WorkspaceID
	modified.CaseAgentID = registration.Instance.AgentID
	body, _ := json.Marshal(modified)
	if err = os.WriteFile(owner.path(), body, 0600); err != nil {
		t.Fatal(err)
	}
	adopted, e := owner.Status()
	if e == nil {
		t.Errorf("internal status adopted a valid foreign signed workspace: %+v", adopted)
	}
	if e == nil {
		if _, err = owner.ReviewPrepare(input, adopted.SetupRevision); err == nil {
			t.Error("internal prepare adopted substituted foreign registration")
		}
		page := SetupPage{Page: modified.BriefPage, Revision: strings.Repeat("a", 64), Text: fmt.Sprintf("---\nid: %s\ntitle: Synthetic local brief\nsummary: Local result.\ntype: project-brief\nscope: workspace/%s\nstatus: draft\nsensitivity: internal\nupdated: 2026-11-15\n---\n# Synthetic local brief\n", strings.TrimSuffix(modified.BriefPage, ".md"), current.WorkspaceID)}
		if _, err = owner.ReviewFinish(adopted.SetupRevision, page, nil); err == nil {
			t.Error("internal finish adopted substituted foreign registration")
		}
	}
}

func setupReviewedPage(status SetupStatus) SetupPage {
	scope := "workspace/" + status.WorkspaceID
	if status.Setup.Input.Mode == "client" {
		scope = "account/" + status.Setup.Input.AccountID + "/case/" + status.Setup.CaseID
	}
	return SetupPage{Page: status.Setup.BriefPage, Revision: strings.Repeat("a", 64), Text: fmt.Sprintf("---\nid: %s\ntitle: Synthetic reviewed case\nsummary: Scoped first result.\ntype: project-brief\nscope: %s\nstatus: draft\nsensitivity: %s\nupdated: 2026-11-15\n---\n# Synthetic reviewed case\n", strings.TrimSuffix(status.Setup.BriefPage, ".md"), scope, map[bool]string{true: "client-confidential", false: "internal"}[status.Setup.Input.Mode == "client"])}
}
func copySetupFixtureTree(t *testing.T, source, target string) {
	t.Helper()
	if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(source, path)
		if e != nil {
			return e
		}
		dest := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(dest, 0700)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("synthetic copy refuses aliases")
		}
		body, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		return os.WriteFile(dest, body, 0600)
	}); err != nil {
		t.Fatal(err)
	}
}
func setupFixtureTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		if entry.IsDir() {
			out[rel] = fmt.Sprintf("dir:%o", info.Mode().Perm())
			return nil
		}
		body, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		out[rel] = fmt.Sprintf("%o:%x", info.Mode().Perm(), sha256.Sum256(body))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}
func assertSetupAncestorReplacementIsConfined(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "state")
	path := filepath.Join(base, "workspace")
	current, e := workspace.Initialize(workspace.Options{WorkspacePath: path, DataRoot: root})
	if e != nil {
		t.Fatal(e)
	}
	owner, e := OpenSetup(root, path, current.WorkspaceID)
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Close()
	review, e := owner.ReviewPrepare(SetupInput{Mode: "internal", ProjectName: "Synthetic physical binding"}, "absent")
	if e != nil {
		t.Fatal(e)
	}
	status, e := owner.ApplyPrepare(review)
	if e != nil {
		t.Fatal(e)
	}
	page := setupReviewedPage(status)
	work := &SetupWork{Objective: "Continue reviewed result", NextStep: "Read same case", Summary: "Synthetic retained-root handoff"}
	review, e = owner.ReviewFinish(status.SetupRevision, page, work)
	if e != nil {
		t.Fatal(e)
	}
	moved := filepath.Join(base, "retained-state")
	var replacement map[string]string
	attempted := false
	owner.FaultPoint = func(point string) error {
		if point != "ledger_create_before_link" {
			return nil
		}
		attempted = true
		e := os.Rename(root, moved)
		if runtime.GOOS == "windows" {
			if e == nil {
				t.Fatal("native retained root allowed namespace replacement")
			}
			return nil
		}
		if e != nil {
			t.Fatal(e)
		}
		copySetupFixtureTree(t, moved, root)
		replacement = setupFixtureTree(t, root)
		return nil
	}
	_, e = owner.ApplyFinish(review, func() (SetupPage, error) { return page, nil })
	if !attempted {
		t.Fatal("actual ledger boundary was not exercised")
	}
	if runtime.GOOS == "windows" {
		if e != nil {
			t.Fatal("pinned native root did not retain normal workflow", e)
		}
		return
	}
	if e == nil {
		t.Fatal("changed binding was reported completed")
	}
	if !reflect.DeepEqual(replacement, setupFixtureTree(t, root)) {
		t.Fatal("replaced absolute root received setup/ledger effects")
	}
	body, e := os.ReadFile(filepath.Join(moved, "workspaces", current.WorkspaceID, "agent", "setup.json"))
	if e != nil {
		t.Fatal(e)
	}
	var committed SetupState
	if e = json.Unmarshal(body, &committed); e != nil {
		t.Fatal(e)
	}
	store, e := execution.OpenConfinedStore(moved)
	if e != nil {
		t.Fatal(e)
	}
	item, e := store.InspectForSetup(committed.SetupID, owner.workInput(committed, *work))
	if e != nil || item.State.StateRevision != 4 {
		t.Fatal("authorized retained root did not own its effects", item, e)
	}
}
func assertSetupCleanupKeepsChangedPending(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "state")
	path := filepath.Join(base, "workspace")
	current, e := workspace.Initialize(workspace.Options{WorkspacePath: path, DataRoot: root})
	if e != nil {
		t.Fatal(e)
	}
	owner, e := OpenSetup(root, path, current.WorkspaceID)
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Close()
	review, e := owner.ReviewPrepare(SetupInput{Mode: "client", ProjectName: "Synthetic exact cleanup", AccountID: "synthetic", AccountOwner: "owner", AccountMandate: "Bounded client metadata."}, "absent")
	if e != nil {
		t.Fatal(e)
	}
	status, e := owner.ApplyPrepare(review)
	if e != nil {
		t.Fatal(e)
	}
	page := setupReviewedPage(status)
	review, e = owner.ReviewFinish(status.SetupRevision, page, nil)
	if e != nil {
		t.Fatal(e)
	}
	pair := "synthetic/" + current.WorkspaceID
	changed := []byte(pair + "\r\n")
	owner.FaultPoint = func(point string) error {
		if point == "active_publication" {
			return os.WriteFile(filepath.Join(path, "brain", "accounts", ".pending"), changed, 0600)
		}
		return nil
	}
	status, e = owner.ApplyFinish(review, func() (SetupPage, error) { return page, nil })
	if e == nil || status.State != "active_cleanup_pending" {
		t.Fatal("changed post-commit pending was silently adopted", status, e)
	}
	body, e := os.ReadFile(filepath.Join(path, "brain", "accounts", ".pending"))
	if e != nil || string(body) != string(changed) {
		t.Fatal("cleanup deleted or rewrote newer same-pair selector", e)
	}
	owner.FaultPoint = nil
	if _, e = owner.ReviewFinish(status.SetupRevision, page, nil); e == nil {
		t.Fatal("cleanup retry adopted an unreviewed pending preimage")
	}
}

func assertClientStatusCannotRelabelSignedAccount(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "state")
	path := filepath.Join(base, "workspace")
	current, e := workspace.Initialize(workspace.Options{WorkspacePath: path, DataRoot: root})
	if e != nil {
		t.Fatal(e)
	}
	owner, e := OpenSetup(root, path, current.WorkspaceID)
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Close()
	review, e := owner.ReviewPrepare(SetupInput{Mode: "client", ProjectName: "Synthetic account A", AccountID: "account-a", AccountOwner: "owner", AccountMandate: "Bounded account A."}, "absent")
	if e != nil {
		t.Fatal(e)
	}
	status, e := owner.ApplyPrepare(review)
	if e != nil {
		t.Fatal(e)
	}
	page := setupReviewedPage(status)
	review, e = owner.ReviewFinish(status.SetupRevision, page, nil)
	if e != nil {
		t.Fatal(e)
	}
	status, e = owner.ApplyFinish(review, func() (SetupPage, error) { return page, nil })
	if e != nil {
		t.Fatal(e)
	}
	altered := *status.Setup
	altered.Input.AccountID = "account-b"
	altered.BriefPage = "accounts/account-b/cases/" + current.WorkspaceID + "/" + current.WorkspaceID + ".md"
	body, _ := json.Marshal(altered)
	if e = os.WriteFile(owner.path(), body, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(path, "brain", "accounts", ".active"), []byte("account-b/"+current.WorkspaceID+"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if adopted, e := owner.Status(); e == nil {
		t.Errorf("client status accepted signed account A under declared B: %+v", adopted)
	}
}
