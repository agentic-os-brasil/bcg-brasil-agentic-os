package adaptercfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallStatusAndUninstallPreserveOtherHooks(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"other"}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	installed, err := Install("codex", workspace)
	if err != nil || installed.State != "installed" {
		t.Fatalf("install = %#v, %v", installed, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "other") || !strings.Contains(string(data), "bcgos hook session-start --runtime codex") {
		t.Fatalf("config = %s, %v", data, err)
	}
	status, err := Inspect("codex", workspace)
	if err != nil || status.State != "installed" {
		t.Fatalf("status = %#v, %v", status, err)
	}
	removed, err := Uninstall("codex", workspace)
	if err != nil || removed.State != "removed" {
		t.Fatalf("remove = %#v, %v", removed, err)
	}
	data, err = os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "other") || strings.Contains(string(data), "bcgos hook session-start --runtime codex") {
		t.Fatalf("config after remove = %s, %v", data, err)
	}
}

func TestUninstallRemovesOnlyOwnedHookInsideSharedGroup(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := `{"hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"other"},{"type":"command","command":"bcgos hook session-start --runtime claude"}]}]}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall("claude", workspace); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "other") || strings.Contains(string(data), "bcgos hook session-start --runtime claude") || !strings.Contains(string(data), "startup") {
		t.Fatalf("config = %s, %v", data, err)
	}
}

func TestInstallRejectsMalformedSessionStartInsteadOfOverwriting(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"SessionStart":"invalid"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Install("codex", workspace); err == nil {
		t.Fatal("Install accepted malformed SessionStart")
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "invalid") {
		t.Fatalf("config changed: %s, %v", data, err)
	}
}

func TestInstallRejectsMalformedSessionStartGroupInsteadOfOverwriting(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"SessionStart":["invalid"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Install("codex", workspace); err == nil {
		t.Fatal("Install accepted malformed group")
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "invalid") {
		t.Fatalf("config changed: %s, %v", data, err)
	}
}
