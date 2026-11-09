package packaging_test

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
)

func TestLifecycleContextPacketIsExplicitAndNonEnforcing(t *testing.T) {
	o := nativePacketOptions(t)
	o.LifecycleContext = true
	if err := packaging.BuildPlugin(o); err != nil {
		t.Fatal(err)
	}
	var packet struct {
		Mode      string                 `json:"mode"`
		Native    struct{ State string } `json:"native"`
		Lifecycle struct {
			Profile         string `json:"profile"`
			State           string `json:"state"`
			NativeExecution string `json:"native_execution"`
			Enforcement     string `json:"enforcement"`
		} `json:"lifecycle"`
		Files []struct{ Path, SHA256 string } `json:"files"`
	}
	body, err := os.ReadFile(filepath.Join(o.Destination, "maestro-packet.json"))
	if err != nil || json.Unmarshal(body, &packet) != nil {
		t.Fatal("lifecycle packet metadata missing")
	}
	if packet.Mode != "mcp_lifecycle" || packet.Native.State != "unavailable" || packet.Lifecycle.Profile != "lifecycle_context" || packet.Lifecycle.State != "configured" || packet.Lifecycle.NativeExecution != "not_observed" || packet.Lifecycle.Enforcement != "unavailable" {
		t.Fatalf("passive profile mismatch: mode=%q native=%q lifecycle=%+v", packet.Mode, packet.Native.State, packet.Lifecycle)
	}
	if _, err := os.Lstat(filepath.Join(o.Destination, "agents")); !os.IsNotExist(err) {
		t.Fatal("passive lifecycle packet projected native agents")
	}
	hooksPath := filepath.Join(o.Destination, "hooks", "hooks.json")
	hooksBody, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Hooks map[string][]struct {
			Hooks []map[string]any `json:"hooks"`
		} `json:"hooks"`
	}
	if json.Unmarshal(hooksBody, &config) != nil || len(config.Hooks) != 3 {
		t.Fatalf("exactly three passive events required: %s", hooksBody)
	}
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "Stop"} {
		matchers := config.Hooks[event]
		if len(matchers) != 1 || len(matchers[0].Hooks) != 1 {
			t.Fatalf("missing single %s handler", event)
		}
		hook := matchers[0].Hooks[0]
		if hook["type"] != "command" || hook["timeout"] != float64(5) || hook["onFailure"] != nil || hook["args"] != nil {
			t.Fatalf("passive command claims enforcement or unsupported exec form: %#v", hook)
		}
		if event == "Stop" && hook["async"] != true {
			t.Fatal("Stop receipt must not block the client")
		}
		if command, ok := hook["command"].(string); !ok || !strings.Contains(command, "lifecycle-hook") || strings.Contains(command, "native-qualify") {
			t.Fatalf("wrong lifecycle executable route: %#v", hook)
		}
	}
	inventoryBody, err := os.ReadFile(filepath.Join(o.Destination, packaging.InventoryName))
	if err != nil || json.Unmarshal(inventoryBody, &packet) != nil {
		t.Fatal("canonical closed packet inventory missing")
	}
	seenHook := false
	for _, member := range packet.Files {
		if member.Path != "hooks/hooks.json" {
			continue
		}
		seenHook = true
		sum := sha256.Sum256(hooksBody)
		if member.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatal("hook bytes were not inventoried")
		}
	}
	if !seenHook {
		t.Fatal("hook file omitted from inventory")
	}
	plain := nativePacketOptions(t)
	if err := packaging.BuildPlugin(plain); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(plain.Destination, "hooks")); !os.IsNotExist(err) {
		t.Fatal("ordinary MCP-only packaging activated hooks")
	}
}

func TestLifecycleContextCommandPreservesLiteralPluginAndBindingPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the POSIX command oracle uses the platform shell; Windows has a separate native command contract")
	}
	parent := filepath.Join(t.TempDir(), "spaces ' $HOME $(touch never-created)")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", parent)
	o := nativePacketOptions(t)
	o.LifecycleContext = true
	if err := packaging.BuildPlugin(o); err != nil {
		t.Fatal(err)
	}
	var config struct {
		Hooks map[string][]struct {
			Hooks []struct{ Command string } `json:"hooks"`
		} `json:"hooks"`
	}
	body, err := os.ReadFile(filepath.Join(o.Destination, "hooks", "hooks.json"))
	if err != nil || json.Unmarshal(body, &config) != nil {
		t.Fatal("hook command missing")
	}
	// A separate harmless recorder observes the generated shell argument vector.
	recorderRoot := filepath.Join(parent, "plugin ' $HOME $(touch injected)")
	if err := os.MkdirAll(filepath.Join(recorderRoot, "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recorderRoot, "scripts", "maestro-mcp"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ARGV_FILE\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "Stop"} {
		output := filepath.Join(parent, event+".argv")
		command := exec.Command("/bin/sh", "-c", config.Hooks[event][0].Hooks[0].Command)
		command.Dir = parent
		command.Env = append(os.Environ(), "CLAUDE_PLUGIN_ROOT="+recorderRoot, "ARGV_FILE="+output)
		if text, err := command.CombinedOutput(); err != nil {
			t.Fatalf("execute exact generated %s command: %s: %v", event, text, err)
		}
		got, err := os.ReadFile(output)
		want := []string{"lifecycle-hook", "--event", event, "--packet", "--product-root", recorderRoot, "--user-home", o.Binding.UserHome, "--workspace", o.Binding.WorkspacePath}
		if err != nil || !reflect.DeepEqual(strings.Split(strings.TrimSuffix(string(got), "\n"), "\n"), want) {
			t.Fatalf("shell changed literal %s bindings: %q; %v", event, got, err)
		}
	}
	for _, marker := range []string{"never-created", "injected"} {
		if _, err := os.Lstat(filepath.Join(parent, marker)); !os.IsNotExist(err) {
			t.Fatal("a path was interpreted as shell code")
		}
	}
}

func TestLifecycleContextCannotBeProjectedAsDesktopHooks(t *testing.T) {
	o := nativePacketOptions(t)
	o.Destination += ".mcpb"
	o.LifecycleContext = true
	if err := packaging.BuildExtension(o); err == nil {
		t.Fatal("Desktop accepted a Code-only lifecycle request")
	}
	if _, err := os.Lstat(o.Destination); !os.IsNotExist(err) {
		t.Fatal("unsupported Desktop profile created output")
	}
	plain := nativePacketOptions(t)
	plain.Destination += ".mcpb"
	err := packaging.BuildExtension(plain)
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		if err == nil || !strings.Contains(err.Error(), "Desktop extension requires macOS Mach-O or Windows PE executable") {
			t.Fatalf("unsupported Desktop executable was not refused: %v", err)
		}
		if _, err := os.Lstat(plain.Destination); !os.IsNotExist(err) {
			t.Fatal("unsupported Desktop executable created output")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(plain.Destination)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for _, file := range z.File {
		if strings.HasPrefix(file.Name, "hooks/") {
			t.Fatal("ordinary MCPB contains active Code hooks")
		}
	}
}

func TestLifecycleAndNativeProfilesAreMutuallyExclusive(t *testing.T) {
	o := nativePacketOptions(t)
	o.LifecycleContext = true
	o.NativeHost = &packaging.NativeHost{}
	if err := packaging.BuildPlugin(o); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatal("lifecycle and native profile selection was not refused explicitly")
	}
	if _, err := os.Lstat(o.Destination); !os.IsNotExist(err) {
		t.Fatal("conflicting profiles created a packet")
	}
	o.NativeHost = nil
	if _, err := packaging.ReviewNative(context.Background(), packaging.NativeOptions{Packet: o, MaxBudgetUSD: .25}); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatal("native review accepted the passive profile")
	}
}
