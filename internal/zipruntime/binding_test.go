package zipruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func bindingFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "Install space ' quote $ literal")
	os.MkdirAll(filepath.Join(root, ".codex/hooks"), 0700)
	os.WriteFile(filepath.Join(root, "VERSION"), []byte("0.2.0\n"), 0600)
	rel := "runtime/" + runtime.GOOS + "-" + runtime.GOARCH + "/maestro-runtime"
	if runtime.GOOS == "windows" {
		rel += ".exe"
	}
	os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0700)
	body := []byte("fixture binary")
	os.WriteFile(filepath.Join(root, rel), body, 0700)
	sha := sha256.Sum256(body)
	b, _ := json.Marshal(map[string]any{"schema_version": 1, "version": "0.2.0", "artifacts": []any{map[string]string{"os": runtime.GOOS, "arch": runtime.GOARCH, "path": rel, "sha256": hex.EncodeToString(sha[:])}}})
	os.WriteFile(filepath.Join(root, "runtime/manifest.json"), b, 0600)
	os.WriteFile(filepath.Join(root, ".codex/hooks/run.sh"), []byte("#!/bin/sh\nprintf trusted-wrapper"), 0700)
	os.WriteFile(filepath.Join(root, ".codex/hooks/run.ps1"), []byte("Write-Output trusted-wrapper"), 0600)
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}
func TestBoundCodexIgnoresNestedHijackAndQuotesRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX execution fixture")
	}
	root := bindingFixture(t)
	if err := BindCodex(root); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "client/.codex/hooks")
	os.MkdirAll(nested, 0700)
	os.WriteFile(filepath.Join(nested, "run.sh"), []byte("#!/bin/sh\nprintf HIJACK"), 0700)
	var doc map[string]any
	b, readErr := os.ReadFile(filepath.Join(root, ".codex/hooks.json"))
	if readErr != nil {
		t.Fatal("binding did not create config", readErr)
	}
	json.Unmarshal(b, &doc)
	hooks := doc["hooks"].(map[string]any)
	record := hooks["PreToolUse"].([]any)[0].(map[string]any)
	cmd := record["hooks"].([]any)[0].(map[string]any)["command"].(string)
	c := exec.Command("/bin/sh", "-c", cmd)
	c.Dir = filepath.Join(root, "client")
	out, err := c.CombinedOutput()
	if err != nil || string(out) != "trusted-wrapper" {
		t.Fatalf("%s %v", out, err)
	}
}
func TestBindingPreservesUserHooksAndRejectsTamper(t *testing.T) {
	root := bindingFixture(t)
	path := filepath.Join(root, ".codex/hooks.json")
	os.WriteFile(path, []byte("{\"custom\":true,\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Read\",\"hooks\":[{\"type\":\"command\",\"command\":\"echo user-owned\"}]}]}}"), 0600)
	if err := BindCodex(root); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)
	if !strings.Contains(string(first), "user-owned") || !strings.Contains(string(first), "custom") {
		t.Fatal("user config lost")
	}
	if !strings.Contains(string(first), "Maestro managed") {
		t.Fatal("native hooks not bound")
	}
	if err := BindCodex(root); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Fatal("not idempotent")
	}
	modified := strings.Replace(string(first), "trusted", "changed", 1)
	_ = modified
	var doc map[string]any
	json.Unmarshal(first, &doc)
	hooks := doc["hooks"].(map[string]any)
	r := hooks["Stop"].([]any)[0].(map[string]any)
	r["hooks"].([]any)[0].(map[string]any)["command"] = "echo changed"
	b, _ := json.Marshal(doc)
	os.WriteFile(path, b, 0600)
	if BindCodex(root) == nil {
		t.Fatal("modified managed hook overwritten")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(b) {
		t.Fatal("failed bind changed config")
	}
}
func TestBindingRejectsAliasedConfigAndInvalidManifest(t *testing.T) {
	root := bindingFixture(t)
	os.WriteFile(filepath.Join(root, "runtime/manifest.json"), []byte("{}"), 0600)
	if BindCodex(root) == nil {
		t.Fatal("invalid manifest accepted")
	}
	root = bindingFixture(t)
	outside := t.TempDir()
	os.Rename(filepath.Join(root, ".codex"), filepath.Join(outside, "saved"))
	if os.Symlink(filepath.Join(outside, "saved"), filepath.Join(root, ".codex")) == nil {
		if BindCodex(root) == nil {
			t.Fatal("symlink config accepted")
		}
	}
}

func TestBindingTemplateAndPowerShellQuotes(t *testing.T) {
	root := bindingFixture(t)
	template, err := os.ReadFile("../../installers/zip/user-template/.codex/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, ".codex/hooks.json"), template, 0600)
	if err = BindCodex(root); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	b, _ := os.ReadFile(filepath.Join(root, ".codex/hooks.json"))
	json.Unmarshal(b, &doc)
	records := doc["hooks"].(map[string]any)["PreToolUse"].([]any)
	if len(records) != 1 {
		t.Fatal("placeholder not replaced")
	}
	handler := records[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	native, err := exec.LookPath("pwsh")
	if err != nil {
		native = "/opt/homebrew/bin/pwsh"
		if _, err = os.Stat(native); err != nil {
			t.Skip("PowerShell unavailable")
		}
	}
	cmd := handler["commandWindows"].(string)
	parts := strings.Fields(cmd)
	c := exec.Command(native, parts[1:]...)
	c.Dir = t.TempDir()
	out, err := c.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "trusted-wrapper" {
		t.Fatalf("PowerShell binding: %s %v", out, err)
	}
}

func TestBoundPackagedWrapperFromNestedCWD(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("native macOS packaged wrapper")
	}
	root := bindingFixture(t)
	for _, p := range []string{".codex/hooks/run.sh", ".claude/hooks/lib/maestro-runtime.sh"} {
		b, e := os.ReadFile(filepath.Join("../../installers/zip/user-template", p))
		if e != nil {
			t.Fatal(e)
		}
		os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0700)
		os.WriteFile(filepath.Join(root, p), b, 0700)
	}
	rel := "runtime/darwin-" + runtime.GOARCH + "/maestro-runtime"
	if err := os.Remove(filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin/go"), "build", "-o", filepath.Join(root, rel), "../../cmd/maestro-runtime")
	if out, e := build.CombinedOutput(); e != nil {
		t.Fatalf("build fixture: %s %v", out, e)
	}
	binary, e := os.ReadFile(filepath.Join(root, rel))
	if e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256(binary)
	manifest := map[string]any{"schema_version": 1, "version": "0.2.0", "artifacts": []any{map[string]string{"os": "darwin", "arch": runtime.GOARCH, "path": rel, "sha256": hex.EncodeToString(hash[:])}}}
	b, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(root, "runtime/manifest.json"), b, 0600)
	if e = BindCodex(root); e != nil {
		t.Fatal(e)
	}
	nested := filepath.Join(root, "attacker/.codex/hooks")
	os.MkdirAll(nested, 0700)
	os.WriteFile(filepath.Join(nested, "run.sh"), []byte("echo HIJACK"), 0700)
	b, _ = os.ReadFile(filepath.Join(root, ".codex/hooks.json"))
	var doc map[string]any
	json.Unmarshal(b, &doc)
	record := doc["hooks"].(map[string]any)["PreToolUse"].([]any)[0].(map[string]any)
	command := record["hooks"].([]any)[0].(map[string]any)["command"].(string)
	call := exec.Command("/bin/sh", "-c", command)
	call.Dir = filepath.Join(root, "attacker")
	call.Stdin = strings.NewReader("{\"tool_name\":\"Write\",\"tool_input\":{\"file_path\":\"brain/accounts/foreign/cases/other/file.md\"}}")
	out, e := call.CombinedOutput()
	if e != nil || !strings.Contains(string(out), "permissionDecision") || strings.Contains(string(out), "HIJACK") {
		t.Fatalf("packaged wrapper escaped binding: %s %v", out, e)
	}
}
