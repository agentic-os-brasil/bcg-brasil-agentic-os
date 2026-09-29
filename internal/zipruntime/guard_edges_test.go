package zipruntime

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func guardWrite(t *testing.T, root, path string) bool {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"tool_name": "Write", "tool_input": map[string]any{"file_path": path}})
	var out bytes.Buffer
	if err := Run([]string{"codex-hook", "PreToolUse", "--root", root}, bytes.NewReader(b), &out); err != nil {
		t.Fatal(err)
	}
	return strings.Contains(out.String(), "deny")
}
func TestGuardAliasAndMarkerEdges(t *testing.T) {
	for _, name := range []string{"escape", "marker_alias", "accounts_alias", "bom", "empty_segment", "same_case_other_account", "pending_alias"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			accounts := filepath.Join(root, "brain/accounts")
			os.MkdirAll(filepath.Join(accounts, "a/cases/one"), 0700)
			active := filepath.Join(accounts, ".active")
			os.WriteFile(active, []byte("a/one"), 0600)
			target := "brain/accounts/a/cases/one/file.md"
			wantDeny := true
			switch name {
			case "escape":
				if err := os.Symlink(outside, filepath.Join(accounts, "a/cases/one/escape")); err != nil {
					t.Skip(err)
				}
				target = "brain/accounts/a/cases/one/escape/file.md"
			case "marker_alias":
				os.WriteFile(filepath.Join(outside, "marker"), []byte("a/one"), 0600)
				os.Remove(active)
				if err := os.Symlink(filepath.Join(outside, "marker"), active); err != nil {
					t.Skip(err)
				}
			case "accounts_alias":
				os.Rename(accounts, filepath.Join(outside, "accounts"))
				if err := os.Symlink(filepath.Join(outside, "accounts"), accounts); err != nil {
					t.Skip(err)
				}
			case "bom":
				os.WriteFile(active, []byte("\xef\xbb\xbf a/one\r\n"), 0600)
				wantDeny = false
			case "empty_segment":
				os.WriteFile(active, []byte("a/.."), 0600)
			case "same_case_other_account":
				target = "brain/accounts/b/cases/one/file.md"
			case "pending_alias":
				target = "brain/accounts/b/cases/two/file.md"
				os.WriteFile(filepath.Join(outside, "marker"), []byte("b/two"), 0600)
				if err := os.Symlink(filepath.Join(outside, "marker"), filepath.Join(accounts, ".pending")); err != nil {
					t.Skip(err)
				}
			}
			if got := guardWrite(t, root, target); got != wantDeny {
				t.Fatalf("deny=%v want=%v", got, wantDeny)
			}
		})
	}
}
