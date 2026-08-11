package claudeagents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallProjectsManagedNativeAgentsIdempotently(t *testing.T) {
	workspace := t.TempDir()
	first, err := Install(workspace)
	if err != nil || first.State != "installed" || len(first.Agents) != 5 {
		t.Fatalf("status=%#v err=%v", first, err)
	}
	caseBody, err := os.ReadFile(filepath.Join(workspace, ".claude", "agents", "case-agent.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(caseBody), "name: case-agent") || !strings.Contains(string(caseBody), "bounded_case_packet") {
		t.Fatalf("case agent=%s", caseBody)
	}
	if _, err := Install(workspace); err != nil {
		t.Fatal(err)
	}
	status, err := Inspect(workspace)
	if err != nil || status.State != "installed" {
		t.Fatalf("inspect=%#v err=%v", status, err)
	}
}

func TestGuardToolEnforcesToolFreeAndWorkspaceBoundaries(t *testing.T) {
	workspace := t.TempDir()
	if reason, managed := GuardTool("walter", "Read", json.RawMessage(`{"file_path":"note.md"}`), workspace, workspace); !managed || reason == "" {
		t.Fatalf("Walter tool call was not denied: managed=%v reason=%q", managed, reason)
	}
	if reason, managed := GuardTool("case-agent", "Read", json.RawMessage(`{"file_path":"brain/projects/a.md"}`), workspace, workspace); !managed || reason != "" {
		t.Fatalf("local Case read was denied: managed=%v reason=%q", managed, reason)
	}
	if reason, _ := GuardTool("case-agent", "Read", json.RawMessage(`{"file_path":"brain/projects/a.md"}`), "", workspace); reason == "" {
		t.Fatal("Case read without native CWD was allowed")
	}
	if reason, _ := GuardTool("case-agent", "Read", json.RawMessage(`{"file_path":"brain/projects/a.md"}`), ".", workspace); reason == "" {
		t.Fatal("Case read with relative native CWD was allowed")
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if reason, _ := GuardTool("case-agent", "Write", json.RawMessage(`{"file_path":"`+outside+`"}`), workspace, workspace); reason == "" {
		t.Fatal("cross-workspace Case write was allowed")
	}
	if reason, _ := GuardTool("case-agent", "Bash", json.RawMessage(`{"command":"pwd"}`), workspace, workspace); reason == "" {
		t.Fatal("Case shell call was allowed")
	}
}

func TestGuardToolRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		t.Fatal(err)
	}
	if reason, _ := GuardTool("case-agent", "Read", json.RawMessage(`{"file_path":"escape/client.md"}`), workspace, workspace); reason == "" {
		t.Fatal("symlink escape was allowed")
	}
}

func TestGuardToolCanonicalizesWorkspaceAndCWD(t *testing.T) {
	realWorkspace := t.TempDir()
	workspaceAlias := filepath.Join(t.TempDir(), "workspace")
	if err := os.Symlink(realWorkspace, workspaceAlias); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(realWorkspace, "brain")
	if err := os.Mkdir(inside, 0o700); err != nil {
		t.Fatal(err)
	}
	insideAlias := filepath.Join(realWorkspace, "current")
	if err := os.Symlink(inside, insideAlias); err != nil {
		t.Fatal(err)
	}
	if reason, _ := GuardTool("case-agent", "Glob", json.RawMessage(`{"pattern":"*.md"}`), insideAlias, workspaceAlias); reason != "" {
		t.Fatalf("canonical in-workspace cwd was denied: %q", reason)
	}
	if reason, _ := GuardTool("case-agent", "Read", json.RawMessage(`{"file_path":"note.md"}`), insideAlias, workspaceAlias); reason != "" {
		t.Fatalf("relative target from canonical cwd was denied: %q", reason)
	}
}

func TestGuardToolRejectsSymlinkedCWDOutsideEvenWithoutToolPath(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	cwdAlias := filepath.Join(workspace, "borrowed")
	if err := os.Symlink(outside, cwdAlias); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"Glob", "Grep"} {
		if reason, _ := GuardTool("case-agent", tool, json.RawMessage(`{"pattern":"*.md"}`), cwdAlias, workspace); reason == "" {
			t.Fatalf("%s without path allowed a cwd symlink escape", tool)
		}
	}
}

func TestGuardToolRejectsTraversalAndNonexistentTargetBelowOutsideSymlink(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		t.Fatal(err)
	}
	inputs := []string{
		`{"file_path":"../outside.md"}`,
		`{"file_path":"escape/not-created/yet.md"}`,
	}
	for _, input := range inputs {
		if reason, _ := GuardTool("case-agent", "Write", json.RawMessage(input), workspace, workspace); reason == "" {
			t.Fatalf("unsafe path was allowed: %s", input)
		}
	}
	outsideFile := filepath.Join(outside, "outside.md")
	input := `{"file_path":"safe.md","path":"` + outsideFile + `"}`
	if reason, _ := GuardTool("case-agent", "Write", json.RawMessage(input), workspace, workspace); reason == "" {
		t.Fatal("unsafe secondary path field was allowed")
	}
}

func TestInstallRefusesUserOwnedCollision(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, ".claude", "agents", "walter.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("user owned\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(workspace); err == nil {
		t.Fatal("user-owned agent was replaced")
	}
	body, _ := os.ReadFile(path)
	if string(body) != "user owned\n" {
		t.Fatal("collision changed")
	}
}

func TestProjectionRefusesSymlinkedManagedDirectories(t *testing.T) {
	for _, test := range []struct {
		name string
		wire func(workspace, outside string) error
	}{
		{
			name: "claude directory",
			wire: func(workspace, outside string) error {
				return os.Symlink(outside, filepath.Join(workspace, ".claude"))
			},
		},
		{
			name: "agents directory",
			wire: func(workspace, outside string) error {
				if err := os.Mkdir(filepath.Join(workspace, ".claude"), 0o700); err != nil {
					return err
				}
				return os.Symlink(outside, filepath.Join(workspace, ".claude", "agents"))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			outside := t.TempDir()
			sentinel := filepath.Join(outside, "sentinel")
			if err := os.WriteFile(sentinel, []byte("untouched"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := test.wire(workspace, outside); err != nil {
				t.Fatal(err)
			}
			if _, err := Install(workspace); err == nil {
				t.Fatal("Install followed a managed-directory symlink")
			}
			if _, err := Inspect(workspace); err == nil {
				t.Fatal("Inspect followed a managed-directory symlink")
			}
			if _, err := Uninstall(workspace); err == nil {
				t.Fatal("Uninstall followed a managed-directory symlink")
			}
			body, err := os.ReadFile(sentinel)
			if err != nil || string(body) != "untouched" {
				t.Fatalf("outside sentinel changed: body=%q err=%v", body, err)
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 1 {
				t.Fatalf("outside directory changed: entries=%v err=%v", entries, err)
			}
		})
	}
}

func TestProjectionRefusesManagedFileSymlink(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, ".claude", "agents")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte(managedMarker+"outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "walter.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(workspace); err == nil {
		t.Fatal("Install followed a managed-file symlink")
	}
	if _, err := Inspect(workspace); err == nil {
		t.Fatal("Inspect followed a managed-file symlink")
	}
	if _, err := Uninstall(workspace); err == nil {
		t.Fatal("Uninstall followed a managed-file symlink")
	}
	body, err := os.ReadFile(outside)
	if err != nil || string(body) != managedMarker+"outside\n" {
		t.Fatalf("outside file changed: body=%q err=%v", body, err)
	}
}
