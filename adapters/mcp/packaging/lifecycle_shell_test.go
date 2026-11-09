package packaging

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

func TestLifecyclePowerShellCommandPreservesLiteralArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this parser oracle uses a POSIX recorder; native Windows command execution remains separate acceptance")
	}
	powershell, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("PowerShell parser oracle unavailable on this host")
	}
	root := t.TempDir()
	plugin := filepath.Join(root, "plugin ' $HOME $(touch injected)")
	if err := os.MkdirAll(filepath.Join(plugin, "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugin, "scripts", "maestro-mcp.exe"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ARGV_FILE\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	o := Options{target: Target{OS: "windows", Arch: "amd64"}, Binding: product.Options{
		ProductRoot:   filepath.Join(root, `C:\Developer\product ' $HOME $(touch injected)`),
		UserHome:      filepath.Join(root, `C:\Developer\owner with spaces`),
		WorkspacePath: filepath.Join(root, `C:\Developer\workspace ' $env:HOME`),
	}}
	o.LifecycleContext = true
	files, err := lifecycleProfileFiles(o.target.Platform(), lifecycleBinding(o), nil)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Hooks map[string][]struct {
			Hooks []struct{ Command, Shell string } `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(files["hooks/hooks.json"], &config); err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "Stop"} {
		hook := config.Hooks[event][0].Hooks[0]
		if hook.Shell != "powershell" {
			t.Fatal("Windows hook must select the native PowerShell shell")
		}
		output := filepath.Join(root, event+".argv")
		command := exec.Command(powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", hook.Command)
		command.Dir = root
		command.Env = append(os.Environ(), "CLAUDE_PLUGIN_ROOT="+plugin, "ARGV_FILE="+output)
		if body, err := command.CombinedOutput(); err != nil {
			t.Fatalf("execute generated PowerShell %s: %s: %v", event, body, err)
		}
		body, err := os.ReadFile(output)
		want := []string{"lifecycle-hook", "--event", event, "--packet", "--product-root", plugin, "--user-home", o.Binding.UserHome, "--workspace", o.Binding.WorkspacePath}
		if err != nil || !reflect.DeepEqual(strings.Split(strings.TrimSuffix(string(body), "\n"), "\n"), want) {
			t.Fatalf("PowerShell changed literal %s arguments: %q: %v", event, body, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "injected")); !os.IsNotExist(err) {
		t.Fatal("a literal path was interpreted as PowerShell code")
	}
}
