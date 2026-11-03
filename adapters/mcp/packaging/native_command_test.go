package packaging

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func engineeringHost(t *testing.T, o Options) *NativeHost {
	t.Helper()
	prepared, _, err := prepare(o)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := bindingFingerprint(o.Binding)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := profileFingerprint(prepared)
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	body, _, err := readNativeBinary(host)
	if err != nil {
		t.Fatal(err)
	}
	return &NativeHost{binding: binding, serverSHA: digest(prepared.serverBody), hostPath: host, hostSHA: digest(body), profileSHA: profile, version: EnforcementMinimumVersion, evidenceClass: "engineering_fixture", ready: true}
}
func TestNativeAdmissionCannotBeImportedOrTransferToAnotherTarget(t *testing.T) {
	in := nativeFixture(t)
	var imported NativeHost
	if json.Unmarshal([]byte(`{"ready":true,"approved":true,"evidenceClass":"native_observed","version":"9.0.0"}`), &imported) != nil {
		t.Fatal("fixture decode")
	}
	o := in.Packet
	o.NativeHost = &imported
	if err := BuildPlugin(o); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(o.Destination, "agents")); !os.IsNotExist(err) {
		t.Fatal("serialized flags minted native files")
	}
	o.Destination = filepath.Join(filepath.Dir(o.Destination), "other-target-plugin")
	o.NativeHost = engineeringHost(t, o)
	h, err := datahome.Resolve(datahome.Options{ProductRoot: o.Binding.ProductRoot, UserHome: o.Binding.UserHome, LookupEnv: o.Binding.LookupEnv})
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(o.Binding.WorkspacePath), "other-workspace")
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: other, DataRoot: filepath.Join(h.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	o.Binding.WorkspacePath = other
	if err := BuildPlugin(o); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(o.Destination, "maestro-packet.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p packetInventory
	if json.Unmarshal(body, &p) != nil || p.Mode != "mcp_only" || p.Native.Reason != "native_admission_stale_or_invalid" {
		t.Fatal("admission transferred between two valid target workspaces")
	}
	for _, path := range []string{"agents", "hooks"} {
		if _, err := os.Stat(filepath.Join(o.Destination, path)); !os.IsNotExist(err) {
			t.Fatal("stale target activated native surface")
		}
	}
}

func TestNativePacketExecutesSelectedHookArgumentVectors(t *testing.T) {
	in := nativeFixture(t)
	binary := filepath.Join(filepath.Dir(in.Packet.Destination), "maestro-mcp-built")
	build := exec.Command("go", "build", "-o", binary, "../cmd/maestro-mcp")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real shared server: %s: %v", output, err)
	}
	in.Packet.Server = binary
	in.Packet.NativeHost = engineeringHost(t, in.Packet)
	if err := BuildPlugin(in.Packet); err != nil {
		t.Fatal(err)
	}
	var config struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command   string   `json:"command"`
				Args      []string `json:"args"`
				OnFailure string   `json:"onFailure"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	body, err := os.ReadFile(filepath.Join(in.Packet.Destination, "hooks/hooks.json"))
	if err != nil || json.Unmarshal(body, &config) != nil {
		t.Fatal("packet hook config")
	}
	if len(config.Hooks) != 7 {
		t.Fatal("seven event oracle differs")
	}
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "SubagentStart", "SubagentStop", "Stop"} {
		rows := config.Hooks[event]
		if len(rows) != 1 || len(rows[0].Hooks) != 1 {
			t.Fatal("unexpected hook multiplicity")
		}
		hook := rows[0].Hooks[0]
		want := []string{"hook", "--event", event, "--product-root", in.Packet.Binding.ProductRoot, "--user-home", in.Packet.Binding.UserHome, "--workspace", in.Packet.Binding.WorkspacePath}
		if hook.Command != "${CLAUDE_PLUGIN_ROOT}/scripts/maestro-mcp" || !reflect.DeepEqual(hook.Args, want) {
			t.Fatalf("real exec vector differs for %s", event)
		}
		if event == "PreToolUse" || event == "UserPromptSubmit" {
			if hook.OnFailure != "block" {
				t.Fatal("enforcement failure is not blocking")
			}
		} else if hook.OnFailure != "" {
			t.Fatal("nonblocking event advertised failure prevention")
		}
	}
	files, err := claudeagents.Project(claudeagents.PluginNamespace)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		body, err := os.ReadFile(filepath.Join(in.Packet.Destination, filepath.FromSlash(file.Path)))
		if err != nil || !bytes.Equal(body, file.Body) {
			t.Fatal("packet agent body differs from canonical projection")
		}
	}
	run := func(event string, extra map[string]any) ([]byte, []byte, error) {
		hook := config.Hooks[event][0].Hooks[0]
		command := filepath.Join(in.Packet.Destination, "scripts", "maestro-mcp")
		input := map[string]any{"hook_event_name": event, "session_id": "compiled-packet-session", "cwd": in.Packet.Binding.WorkspacePath, "stop_hook_active": false}
		for k, v := range extra {
			input[k] = v
		}
		body, _ := json.Marshal(input)
		cmd := exec.Command(command, hook.Args...)
		cmd.Stdin = bytes.NewReader(body)
		var out, stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		err := cmd.Run()
		return out.Bytes(), stderr.Bytes(), err
	}
	before, err := startupSnapshot(in.Packet.Binding)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := run("SessionStart", nil); err != nil {
		t.Fatal(err)
	}
	after, err := startupSnapshot(in.Packet.Binding)
	if err != nil || before != after {
		t.Fatal("packaged SessionStart wrote selected state")
	}
	if _, _, err := run("UserPromptSubmit", map[string]any{"prompt": "PRIVATE_SYNTHETIC_PACKET_SENTINEL"}); err != nil {
		t.Fatal(err)
	}
	read := map[string]any{"tool_name": "Read", "tool_use_id": "read", "tool_input": map[string]any{"file_path": filepath.Join(in.Packet.Binding.WorkspacePath, "README.md")}}
	out, _, err := run("PreToolUse", read)
	if err != nil || string(bytes.TrimSpace(out)) != "{}" {
		t.Fatalf("allowed packet tool changed host permissions: %s %v", out, err)
	}
	for _, tc := range []struct {
		name   string
		fields map[string]any
	}{{"outside", map[string]any{"tool_name": "Read", "tool_use_id": "outside", "tool_input": map[string]any{"file_path": filepath.Join(filepath.Dir(in.Packet.Binding.WorkspacePath), "outside.md")}}}, {"root removal", map[string]any{"tool_name": "Bash", "tool_use_id": "remove", "tool_input": map[string]any{"command": "rm -rf /"}}}, {"spoofed agent", map[string]any{"tool_name": "Agent", "tool_use_id": "spoof", "tool_input": map[string]any{"subagent_type": "prefix:maestro-product:case-agent"}}}} {
		t.Run(tc.name, func(t *testing.T) {
			out, stderr, err := run("PreToolUse", tc.fields)
			if err != nil || !bytes.Contains(out, []byte(`"permissionDecision":"deny"`)) || bytes.Contains(stderr, []byte("PRIVATE_")) {
				t.Fatalf("packaged guard did not deny: %s %s %v", out, stderr, err)
			}
		})
	}
	command := filepath.Join(in.Packet.Destination, "scripts", "maestro-mcp")
	cmd := exec.Command(command, config.Hooks["PreToolUse"][0].Hooks[0].Args...)
	cmd.Stdin = bytes.NewBufferString(`{}`)
	if err := cmd.Run(); err == nil {
		t.Fatal("packaged malformed enforcement input succeeded")
	} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 2 {
		t.Fatalf("wrong malformed enforcement exit: %v", err)
	}
	metadata := filepath.Join(in.Packet.Binding.WorkspacePath, ".bcgos", "workspace.json")
	original, err := os.ReadFile(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadata, []byte(`{"schema_version":1,"workspace_id":"invalid"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run("PreToolUse", read); err == nil {
		t.Fatal("changed target binding executed packet hook")
	}
	if err := os.WriteFile(metadata, original, 0600); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(in.ScratchRoot)
	if err != nil || len(entries) != 0 {
		t.Fatal("ordinary packet/hook launched qualification")
	}
	// Public operator dry-run is exercised with this newly built controlled
	// binary as the selected executable; it cannot impersonate Code's version.
	args := []string{"native-review", "--product-root", in.Packet.Binding.ProductRoot, "--user-home", in.Packet.Binding.UserHome, "--workspace", in.Packet.Binding.WorkspacePath, "--server", binary, "--destination", filepath.Join(filepath.Dir(in.Packet.Destination), "review-only-plugin"), "--version", "0.2.0", "--native-executable", binary, "--native-scratch-root", in.ScratchRoot, "--native-challenge-directory", in.ChallengeDirectory, "--max-budget-usd", "0.2"}
	cmd = exec.Command(command, args...)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("real native-review entrypoint: %s %v", output, err)
	}
	var review NativeReview
	if json.Unmarshal(output, &review) != nil || review.Plan.State != "unavailable" || review.Plan.Reason != "native_enforcement_version_unsupported" {
		t.Fatalf("controlled binary was promoted to supported Code: %s", output)
	}
	prepared, _, err := prepare(in.Packet)
	if err != nil {
		t.Fatal(err)
	}
	candidateRoot := filepath.Join(in.ScratchRoot, "compiled-candidate")
	if err := os.Mkdir(candidateRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(in.ChallengeDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	probe, err := prepareNativeProbe(prepared, candidateRoot, in.ChallengeDirectory, 0, "missing")
	if err != nil {
		t.Fatal(err)
	}
	var candidate struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Args []string `json:"args"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	body, err = os.ReadFile(filepath.Join(probe.plugin, "hooks/hooks.json"))
	if err != nil || json.Unmarshal(body, &candidate) != nil {
		t.Fatal("compiled candidate hooks missing")
	}
	callCandidate := func(event string, row int, extra map[string]any) []byte {
		t.Helper()
		fields := map[string]any{"hook_event_name": event, "session_id": probe.session, "cwd": probe.binding.WorkspacePath, "stop_hook_active": false}
		for k, v := range extra {
			fields[k] = v
		}
		body, _ := json.Marshal(fields)
		cmd := exec.Command(filepath.Join(probe.plugin, "scripts", "maestro-mcp"), candidate.Hooks[event][row].Hooks[0].Args...)
		cmd.Stdin = bytes.NewReader(body)
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("compiled candidate %s row%d: %v", event, row, err)
		}
		return output
	}
	out = callCandidate("SessionStart", 0, nil)
	if !bytes.Contains(out, []byte("maestro_native_startup_unchanged:"+probe.nonce)) {
		t.Fatal("compiled startup instrumentation missing")
	}
	callCandidate("UserPromptSubmit", 0, nil)
	publicRead := map[string]any{"tool_name": "Read", "tool_use_id": "public-negative", "tool_input": map[string]any{"file_path": probe.outside}}
	out = callCandidate("PreToolUse", 0, publicRead)
	if !bytes.Contains(out, []byte("maestro_native_canonical_hook:"+probe.nonce)) || !bytes.Contains(out, []byte(`"permissionDecision":"deny"`)) {
		t.Fatal("compiled canonical wrapper lost its own denial")
	}
	out = callCandidate("PreToolUse", 1, publicRead)
	if string(bytes.TrimSpace(out)) != "{}" {
		t.Fatal("compiled instrumentation supplied the negative control verdict")
	}
	out = callCandidate("PreToolUse", 1, map[string]any{"tool_name": "Write", "tool_use_id": "unreviewed", "tool_input": map[string]any{"file_path": filepath.Join(probe.binding.WorkspacePath, "client.txt"), "content": "unreviewed"}})
	if !bytes.Contains(out, []byte(`"permissionDecision":"deny"`)) {
		t.Fatal("compiled qualification scope allowed unreviewed effects")
	}
}
