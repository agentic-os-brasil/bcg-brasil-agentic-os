//go:build maestro_experimental_installed_seed

package packaging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

// This fixture is renderer metadata only. It grants no installed binding,
// NativeHost, current record or actual native-client qualification.
func experimentalRecipeOptions(t *testing.T, platform string) Options {
	t.Helper()
	root := t.TempDir()
	return Options{Carrier: &StableCarrier{Profile: installer.StableClientProfile, Protocol: installer.StableClientProtocol, ID: strings.Repeat("1", 32), Version: installer.StableCarrierVersion}, Binding: product.Options{ProductRoot: filepath.Join(root, "managed"), UserHome: filepath.Join(root, "owner"), WorkspacePath: filepath.Join(root, "workspace")}, serverBody: []byte("synthetic renderer image"), target: Target{OS: platform, Arch: "arm64"}, stableIdentity: releasecontract.ServerIdentity{EmbeddedInputsSHA256: strings.Repeat("2", 64)}}
}

func TestEISANativeRecipeRoundTripAndTamper(t *testing.T) {
	o := experimentalRecipeOptions(t, "darwin")
	stable := stableBinding(o)
	selection := strings.Repeat("a", 64)
	files, err := canonicalNativeProfile("darwin", selection, stable)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := canonicalNativeProfile("darwin", selection, nil); err == nil {
		t.Fatal("unbound portable EISA native profile admitted")
	}
	p, err := nativeManifest(o.target, NativeCapability{State: "qualified", Profile: nativeProfile, EvidenceClass: "engineering_fixture", VendorProvenance: "renderer_only_not_NativeHost_or_current_record"})
	if err != nil {
		t.Fatal(err)
	}
	p.BindingSHA256 = selection
	p.HooksSHA256 = digest(files["hooks/hooks.json"])
	render := func(p nativePacketManifest, hooks []byte) (map[string][]byte, error) {
		body, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		inventory := map[string][]byte{nativeManifestName: append(body, '\n'), "hooks/hooks.json": hooks}
		return nativeRequiredPayload(func(name string, _ int64) ([]byte, error) {
			b, ok := inventory[name]
			if !ok {
				return nil, fmt.Errorf("absent fixture %s", name)
			}
			return b, nil
		}, "plugin", o.target, nil, stable)
	}
	inspected, err := render(p, files["hooks/hooks.json"])
	if err != nil || !bytes.Equal(inspected["hooks/hooks.json"], files["hooks/hooks.json"]) {
		t.Fatalf("canonical renderer/inspector differ: %v", err)
	}
	for _, change := range []string{"command", "managed_root", "carrier", "event", "selection", "failure_policy", "timeout"} {
		t.Run(change, func(t *testing.T) {
			var config map[string]any
			if err := json.Unmarshal(files["hooks/hooks.json"], &config); err != nil {
				t.Fatal(err)
			}
			hook := config["hooks"].(map[string]any)["PreToolUse"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
			args := hook["args"].([]any)
			switch change {
			case "command":
				hook["command"] = "foreign-installer"
			case "event":
				args[2] = "Stop"
			case "selection":
				args[4] = strings.Repeat("b", 64)
			case "managed_root":
				args[6] = filepath.Join(o.Binding.ProductRoot, "foreign")
			case "carrier":
				args[12] = strings.Repeat("4", 32)
			case "failure_policy":
				hook["onFailure"] = "allow"
			case "timeout":
				hook["timeout"] = 90
			}
			bad, err := json.MarshalIndent(config, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			bad = append(bad, '\n')
			altered := p
			altered.HooksSHA256 = digest(bad)
			if _, err := render(altered, bad); err == nil {
				t.Fatal("self-consistent tampered recipe passed canonical inspection")
			}
		})
	}
}

func TestEISAEmittedPrivateProbeJSONRestoresAllSevenMCPCommands(t *testing.T) {
	for _, platform := range []string{"darwin", "win32"} {
		t.Run(platform, func(t *testing.T) {
			o := experimentalRecipeOptions(t, "darwin")
			files, err := canonicalNativeProfile(platform, "", stableBinding(o))
			if err != nil {
				t.Fatal(err)
			}
			var config map[string]any
			if err := json.Unmarshal(files["hooks/hooks.json"], &config); err != nil {
				t.Fatal(err)
			}
			hooks := config["hooks"].(map[string]any)
			admitted := []string{"serve", "--packet", "--experimental-seed-sha256", strings.Repeat("a", 64), "--experimental-installed-root", o.Binding.ProductRoot}
			// Reproduce the first-decode startup object being restored after all
			// rows in the second-decode object have already been instrumented.
			startup := map[string]any{"type": "command", "command": filepath.Join(o.Binding.ProductRoot, "maestro-install"), "args": append([]string{"native-startup-check", "--nonce", "unit"}, admitted[1:]...), "timeout": nativeStartupCheckSeconds}
			instrumentNativeProbeRows(hooks, platform, admitted)
			hooks["SessionStart"] = []any{map[string]any{"hooks": []any{startup}}}
			fault := map[string]any{"matcher": "Edit", "hooks": []any{map[string]any{"command": "absent-native-guard", "args": []any{}, "onFailure": "block", "timeout": float64(1)}}}
			hooks["PreToolUse"] = append(hooks["PreToolUse"].([]any), fault)
			body, err := marshalNativeProbeHooks(config, platform)
			if err != nil {
				t.Fatal(err)
			}
			var emitted map[string]any
			if err := json.Unmarshal(body, &emitted); err != nil {
				t.Fatal(err)
			}
			actual := emitted["hooks"].(map[string]any)
			if len(actual) != 7 {
				t.Fatal("event count changed")
			}
			for event, raw := range actual {
				hook := raw.([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
				if hook["command"] != "${CLAUDE_PLUGIN_ROOT}/scripts/"+binaryName(platform) {
					t.Fatalf("emitted %s retained installer command", event)
				}
				args := hook["args"].([]any)
				tail := make([]string, len(admitted)-1)
				for i, v := range args[len(args)-len(tail):] {
					tail[i] = v.(string)
				}
				if !reflect.DeepEqual(tail, admitted[1:]) {
					t.Fatalf("emitted %s lost EISA argv", event)
				}
			}
			if !reflect.DeepEqual(actual["PreToolUse"].([]any)[1], fault) {
				t.Fatal("separate missing/fault control was rewritten")
			}
		})
	}
}

func TestEISAFinalNativeHooksUseAdmittedStableLauncher(t *testing.T) {
	for _, platform := range []string{"darwin", "windows"} {
		t.Run(platform, func(t *testing.T) {
			o := experimentalRecipeOptions(t, platform)
			files, err := nativeProfileFiles(o)
			if err != nil {
				t.Fatal(err)
			}
			var config struct {
				Hooks map[string][]struct {
					Hooks []struct {
						Command   string
						Args      []string
						Timeout   int
						OnFailure string
					}
				}
			}
			if err := json.Unmarshal(files["hooks/hooks.json"], &config); err != nil {
				t.Fatal(err)
			}
			name := "maestro-install"
			if platform == "windows" {
				name += ".exe"
			}
			if len(config.Hooks) != 7 {
				t.Fatal("seven native events required")
			}
			for event, rows := range config.Hooks {
				if len(rows) != 1 || len(rows[0].Hooks) != 1 {
					t.Fatal("closed hook row required")
				}
				hook := rows[0].Hooks[0]
				expectedArgs := []string{"stable-native-hook", "--event", event, "--binding-sha256", "", "--product-root", o.Binding.ProductRoot, "--user-home", o.Binding.UserHome, "--workspace", o.Binding.WorkspacePath, "--carrier-id", o.Carrier.ID}
				expectedFailure := ""
				if event == "PreToolUse" || event == "UserPromptSubmit" {
					expectedFailure = "block"
				}
				if hook.Command != filepath.Join(o.Binding.ProductRoot, name) || !reflect.DeepEqual(hook.Args, expectedArgs) || hook.Timeout != 5 || hook.OnFailure != expectedFailure {
					t.Fatalf("final %s still bypasses independently admitted stable seed: %+v", event, hook)
				}
			}
		})
	}
}
