// Command rehearse exercises the local MCP server through real SDK stdio
// sessions. Its only writes are a newly seeded synthetic Developer directory
// and the receipt within it; it never loads a Claude profile or calls a model.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const markerA = "MCP_REHEARSAL_CASE_A_PRIVATE_CHECKPOINT"
const nextA = "Read the synthetic artifact and validate recommendation A"

type handoff struct {
	Workspace     string   `json:"workspace"`
	ItemID        string   `json:"item_id"`
	State         string   `json:"state"`
	StateRevision int      `json:"state_revision"`
	AttemptID     string   `json:"attempt_id,omitempty"`
	Summary       string   `json:"summary,omitempty"`
	NextStep      string   `json:"next_step"`
	Blocker       string   `json:"blocker,omitempty"`
	ArtifactRefs  []string `json:"artifact_refs,omitempty"`
}

type observation struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Evidence any    `json:"evidence,omitempty"`
}

type receipt struct {
	SchemaVersion int           `json:"schema_version"`
	Status        string        `json:"status"`
	SDK           string        `json:"sdk"`
	Transport     string        `json:"transport"`
	Observations  []observation `json:"observations"`
	Limits        []string      `json:"limits"`
	Error         string        `json:"error,omitempty"`
}

type runner struct {
	ctx                              context.Context
	server, root                     string
	receipt                          receipt
	seeded                           bool
	pluginBuilder, pluginDestination string
}

func main() {
	server := flag.String("server", "", "absolute path to the synthetic MCP server")
	destination := flag.String("destination", "", "new physical directory under Developer")
	p1 := flag.String("plugin-v1", "", "optional first built plugin root")
	p2 := flag.String("plugin-v2", "", "optional second built plugin root")
	builder := flag.String("plugin-builder", "", "optional native builder to create both revisions after seeding")
	pluginDestination := flag.String("plugin-destination", "", "new physical directory for generated plugin revisions")
	flag.Parse()
	if flag.NArg() != 0 || *server == "" || *destination == "" || (*p1 == "") != (*p2 == "") || (*builder == "") != (*pluginDestination == "") || (*builder != "" && *p1 != "") {
		fmt.Fprintln(os.Stderr, "required: --server BINARY --destination NEWDIR; plugin-v1 and plugin-v2 must be paired")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	r := &runner{ctx: ctx, server: *server, root: *destination, receipt: receipt{SchemaVersion: 1, Status: "FAIL", SDK: sdkVersion(), Transport: "official Go SDK CommandTransport / stdio", Limits: []string{"No model or Claude host was invoked", "No Code tab, corporate policy, synced-folder or production migration qualification", "Synthetic execution ledger only; filesystem access by an arbitrary model tool is not sandboxed by this MCP"}}}
	r.pluginBuilder, r.pluginDestination = *builder, *pluginDestination
	err := r.run(*p1, *p2)
	if err == nil {
		r.receipt.Status = "PASS"
	} else {
		r.receipt.Error = err.Error()
	}
	data, marshalErr := json.MarshalIndent(r.receipt, "", "  ")
	if marshalErr != nil {
		fmt.Fprintln(os.Stderr, marshalErr)
		os.Exit(1)
	}
	if r.seeded {
		if writeErr := os.WriteFile(filepath.Join(r.root, "rehearsal-receipt.json"), append(data, '\n'), 0600); writeErr != nil {
			fmt.Fprintln(os.Stderr, writeErr)
			os.Exit(1)
		}
	}
	fmt.Println(string(data))
	if err != nil {
		os.Exit(1)
	}
}

func sdkVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, d := range info.Deps {
			if d.Path == "github.com/modelcontextprotocol/go-sdk" {
				return d.Path + "@" + d.Version
			}
		}
	}
	return "github.com/modelcontextprotocol/go-sdk@unknown"
}

func (r *runner) pass(name string, evidence any) {
	r.receipt.Observations = append(r.receipt.Observations, observation{name, "PASS", evidence})
}

func (r *runner) run(p1, p2 string) error {
	if err := newPhysicalDestination(r.root); err != nil {
		return err
	}
	if !filepath.IsAbs(r.server) {
		return errors.New("server must be an absolute path")
	}
	if r.pluginBuilder != "" {
		if !filepath.IsAbs(r.pluginBuilder) {
			return errors.New("plugin-builder must be absolute")
		}
		if err := newPhysicalDestination(r.pluginDestination); err != nil {
			return fmt.Errorf("plugin destination: %w", err)
		}
		if contains(r.root, r.pluginDestination) || contains(r.pluginDestination, r.root) {
			return errors.New("plugin and state directories must be separate")
		}
	}
	seed := exec.CommandContext(r.ctx, r.server, "seed", "--root", r.root)
	seed.Stderr = os.Stderr
	if out, err := seed.Output(); err != nil {
		return fmt.Errorf("seed: %w", err)
	} else if !json.Valid(out) {
		return errors.New("seed returned non-JSON output")
	}
	r.seeded = true
	r.pass("new_synthetic_ledger_seeded", map[string]any{"root": r.root, "existing_state_reused": false})
	if r.pluginBuilder != "" {
		if err := os.Mkdir(r.pluginDestination, 0700); err != nil {
			return err
		}
		p1, p2 = filepath.Join(r.pluginDestination, "v1"), filepath.Join(r.pluginDestination, "v2")
		for _, revision := range []struct{ path, version string }{{p1, "0.0.1"}, {p2, "0.0.2"}} {
			cmd := exec.CommandContext(r.ctx, r.pluginBuilder, "--destination", revision.path, "--server", r.server, "--state-root", r.root, "--workspace", "case-a", "--version", revision.version)
			cmd.Stderr = os.Stderr
			if _, err := cmd.Output(); err != nil {
				return fmt.Errorf("build plugin %s: %w", revision.version, err)
			}
		}
		r.pass("native_plugins_built_outside_owner_state", map[string]string{"plugin_v1": p1, "plugin_v2": p2, "state_root": r.root})
	}
	a, err := r.connect("case-a")
	if err != nil {
		return err
	}
	defer a.Close()
	tools, err := a.ListTools(r.ctx, nil)
	if err != nil {
		return err
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if !reflect.DeepEqual(names, []string{"maestro_checkpoint", "maestro_context", "maestro_resume"}) || tools.NextCursor != "" {
		return fmt.Errorf("unexpected discovery: %v", names)
	}
	r.pass("sdk_initialize_and_tools_discovery", map[string]any{"protocol_version": a.InitializeResult().ProtocolVersion, "server": a.InitializeResult().ServerInfo, "tools": names})
	before, err := snapshot(filepath.Join(r.root, "state"))
	if err != nil {
		return err
	}
	initial, err := r.read(a)
	if err != nil {
		return err
	}
	if initial.Workspace != "case-a" || initial.State != "running" || initial.StateRevision != 2 || initial.AttemptID == "" {
		return fmt.Errorf("unexpected initial context: %+v", initial)
	}
	if _, err := r.read(a); err != nil {
		return err
	}
	if err := unchanged(filepath.Join(r.root, "state"), before); err != nil {
		return fmt.Errorf("read mutated ledger: %w", err)
	}
	r.pass("bounded_context_reads_do_not_mutate_files", map[string]any{"max_response_bytes": 4096, "state_revision": initial.StateRevision, "ledger_files": len(before)})
	checkpoint := map[string]any{"expected_revision": initial.StateRevision, "attempt_id": initial.AttemptID, "summary": markerA, "next_step": nextA, "pause": true}
	if _, err := r.call(a, "maestro_checkpoint", checkpoint); err != nil {
		return err
	}
	if err := a.Close(); err != nil {
		return err
	}
	a2, err := r.connect("case-a")
	if err != nil {
		return err
	}
	defer a2.Close()
	paused, err := r.read(a2)
	if err != nil {
		return err
	}
	if paused.State != "paused" || paused.Summary != markerA || paused.NextStep != nextA || paused.StateRevision <= initial.StateRevision {
		return fmt.Errorf("checkpoint not restored after restart: %+v", paused)
	}
	r.pass("checkpoint_and_pause_survive_fresh_server_process", map[string]any{"state": paused.State, "state_revision": paused.StateRevision, "summary": paused.Summary, "next_step": paused.NextStep})
	if p1 != "" {
		if err := r.plugins(p1, p2, paused); err != nil {
			return err
		}
	} else {
		r.receipt.Observations = append(r.receipt.Observations, observation{"plugin_revision_switch_uses_same_checkpoint", "NOT_RUN", "supply paired --plugin-v1 and --plugin-v2"})
	}
	resumedResult, err := r.call(a2, "maestro_resume", map[string]any{"expected_revision": paused.StateRevision})
	if err != nil {
		return err
	}
	var resumed handoff
	if err := decode(resumedResult, &resumed); err != nil {
		return err
	}
	if resumed.State != "running" || resumed.AttemptID == "" || resumed.AttemptID == initial.AttemptID || resumed.StateRevision <= paused.StateRevision || resumed.Summary != markerA || resumed.NextStep != nextA {
		return fmt.Errorf("resume did not create a new attempt: %+v", resumed)
	}
	r.pass("resume_creates_new_attempt", map[string]any{"state_revision": resumed.StateRevision, "new_attempt": true, "checkpoint_preserved": resumed.Summary == markerA})
	stale := map[string]any{"expected_revision": resumed.StateRevision, "attempt_id": initial.AttemptID, "summary": "stale attempt must not persist", "next_step": "reject", "pause": false}
	if err := r.deniedWithoutMutation(a2, "maestro_checkpoint", stale); err != nil {
		return fmt.Errorf("stale attempt: %w", err)
	}
	r.pass("prior_attempt_cannot_checkpoint_after_resume", nil)
	if err := r.deniedWithoutMutation(a2, "maestro_resume", map[string]any{"expected_revision": paused.StateRevision}); err != nil {
		return fmt.Errorf("stale revision: %w", err)
	}
	r.pass("stale_revision_cannot_resume", nil)
	for _, field := range []string{"root", "workspace", "item_id"} {
		for _, tool := range names {
			args := map[string]any{field: "case-b"}
			if tool == "maestro_checkpoint" {
				for k, v := range checkpoint {
					args[k] = v
				}
				args["expected_revision"] = resumed.StateRevision
				args["attempt_id"] = resumed.AttemptID
			}
			if tool == "maestro_resume" {
				args["expected_revision"] = resumed.StateRevision
			}
			if err := r.deniedWithoutMutation(a2, tool, args); err != nil {
				return fmt.Errorf("%s %s override: %w", tool, field, err)
			}
		}
	}
	r.pass("all_three_tools_reject_root_workspace_and_item_overrides", map[string]int{"rejected_calls": 9})
	oversize := map[string]any{"expected_revision": resumed.StateRevision, "attempt_id": resumed.AttemptID, "summary": strings.Repeat("x", 100*1024), "next_step": "reject oversized checkpoint", "pause": false}
	if err := r.deniedWithoutMutation(a2, "maestro_checkpoint", oversize); err != nil {
		return fmt.Errorf("oversized checkpoint: %w", err)
	}
	r.pass("oversized_checkpoint_rejected_without_ledger_mutation", map[string]int{"summary_bytes": 100 * 1024})
	b, err := r.connect("case-b")
	if err != nil {
		return err
	}
	defer b.Close()
	bctx, err := r.read(b)
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(bctx)
	if bctx.Workspace != "case-b" || bctx.ItemID == initial.ItemID || strings.Contains(string(encoded), markerA) || bctx.StateRevision != 2 {
		return fmt.Errorf("case B contains A context or changed state: %+v", bctx)
	}
	badB := map[string]any{"expected_revision": bctx.StateRevision, "attempt_id": resumed.AttemptID, "summary": markerA, "next_step": nextA, "pause": true}
	if err := r.deniedWithoutMutation(b, "maestro_checkpoint", badB); err != nil {
		return fmt.Errorf("cross-workspace attempt: %w", err)
	}
	endB, err := r.read(b)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(endB, bctx) {
		return errors.New("case B context changed after rejected cross-workspace write")
	}
	r.pass("workspace_b_neither_reads_nor_accepts_a_checkpoint", map[string]any{"workspace": bctx.Workspace, "unchanged_revision": bctx.StateRevision})
	final, err := r.read(a2)
	if err != nil {
		return err
	}
	if final.StateRevision != resumed.StateRevision || final.Summary != markerA || final.NextStep != nextA {
		return errors.New("negative probes changed the authoritative A checkpoint")
	}
	r.pass("authoritative_checkpoint_preserved_after_negative_probes", map[string]any{"state_revision": final.StateRevision, "summary": final.Summary})
	return nil
}

func (r *runner) connect(workspace string) (*mcp.ClientSession, error) {
	return r.connectCommand(exec.CommandContext(r.ctx, r.server, "serve", "--root", r.root, "--workspace", workspace))
}

func (r *runner) connectCommand(cmd *exec.Cmd) (*mcp.ClientSession, error) {
	cmd.Stderr = os.Stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "maestro-architecture-rehearsal", Version: "0.0.1"}, nil)
	return client.Connect(r.ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil)
}

func (r *runner) call(session *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, error) {
	result, err := session.CallTool(r.ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("tool %s rejected input", name)
	}
	return result, nil
}

func decode(result *mcp.CallToolResult, target any) error {
	if result.StructuredContent == nil {
		return errors.New("tool omitted structured content")
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return err
	}
	if len(data) > 4096 {
		return fmt.Errorf("context exceeded 4096-byte rehearsal bound: %d", len(data))
	}
	return json.Unmarshal(data, target)
}

func (r *runner) read(session *mcp.ClientSession) (handoff, error) {
	var h handoff
	result, err := r.call(session, "maestro_context", map[string]any{})
	if err != nil {
		return h, err
	}
	err = decode(result, &h)
	return h, err
}

func (r *runner) deniedWithoutMutation(session *mcp.ClientSession, tool string, args map[string]any) error {
	before, err := snapshot(filepath.Join(r.root, "state"))
	if err != nil {
		return err
	}
	result, callErr := session.CallTool(r.ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if callErr == nil && !result.IsError {
		return errors.New("invalid call was accepted")
	}
	if r.ctx.Err() != nil {
		return r.ctx.Err()
	}
	// A dead process must not count as successful input validation.
	if _, err := r.read(session); err != nil {
		return fmt.Errorf("server unavailable after rejected call: %w", err)
	}
	return unchanged(filepath.Join(r.root, "state"), before)
}

func newPhysicalDestination(root string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve current user home: %w", err)
	}
	developer := filepath.Join(home, "Developer")
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || !strings.HasPrefix(root, developer+string(filepath.Separator)) {
		return errors.New("destination must be an absolute clean path below Developer")
	}
	if _, err := os.Lstat(root); err == nil {
		return errors.New("destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(root)
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return fmt.Errorf("destination parent: %w", err)
	}
	if resolved != parent {
		return errors.New("destination parent contains a symlink")
	}
	return nil
}

func contains(parent, child string) bool {
	return parent == child || strings.HasPrefix(child, parent+string(filepath.Separator))
}

type fileFingerprint struct {
	SHA256 string `json:"sha256"`
	Mode   string `json:"mode"`
}

func snapshot(root string) (map[string]fileFingerprint, error) {
	files := map[string]fileFingerprint{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("snapshot refuses symlink")
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("snapshot refuses non-regular file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = fileFingerprint{hex.EncodeToString(sum[:]), info.Mode().String()}
		return nil
	})
	return files, err
}

func unchanged(root string, before map[string]fileFingerprint) error {
	after, err := snapshot(root)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, after) {
		return errors.New("filesystem content, file set or permissions changed")
	}
	return nil
}

func (r *runner) plugins(p1, p2 string, want handoff) error {
	state, err := snapshot(filepath.Join(r.root, "state"))
	if err != nil {
		return err
	}
	var versions []string
	for _, root := range []string{p1, p2} {
		before, err := snapshot(root)
		if err != nil {
			return err
		}
		manifest, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "plugin.json"))
		if err != nil {
			return err
		}
		var metadata struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(manifest, &metadata); err != nil {
			return err
		}
		versions = append(versions, metadata.Version)
		cmd, err := pluginCommand(r.ctx, root, r.root)
		if err != nil {
			return err
		}
		session, err := r.connectCommand(cmd)
		if err != nil {
			return err
		}
		got, readErr := r.read(session)
		closeErr := session.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("plugin revision %s did not read the same durable handoff", metadata.Version)
		}
		if err := unchanged(root, before); err != nil {
			return fmt.Errorf("plugin was mutated: %w", err)
		}
		if err := unchanged(filepath.Join(r.root, "state"), state); err != nil {
			return fmt.Errorf("plugin read changed owner state: %w", err)
		}
	}
	if versions[0] == "" || versions[0] == versions[1] {
		return errors.New("plugin revisions must be distinct and nonempty")
	}
	r.pass("plugin_revision_switch_uses_same_checkpoint", map[string]any{"plugin_versions": versions, "launch_source": "each plugin .mcp.json bundled executable", "product_files_unchanged": true, "state_files_unchanged": true, "host_update_semantics": "NOT_RUN"})
	return nil
}

func pluginCommand(ctx context.Context, root, stateRoot string) (*exec.Cmd, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("plugin root must be absolute")
	}
	data, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	if err != nil {
		return nil, err
	}
	var config struct {
		Servers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	if len(config.Servers) != 1 {
		return nil, errors.New("plugin must declare exactly one synthetic MCP server")
	}
	for _, server := range config.Servers {
		replace := strings.NewReplacer("${CLAUDE_PLUGIN_ROOT}", root, "${MAESTRO_SPIKE_ROOT}", stateRoot, "${MAESTRO_SPIKE_WORKSPACE}", "case-a")
		command := replace.Replace(server.Command)
		if !filepath.IsAbs(command) || !strings.HasPrefix(command, root+string(filepath.Separator)) {
			return nil, errors.New("plugin command must resolve inside its product directory")
		}
		args := make([]string, len(server.Args))
		for i, a := range server.Args {
			args[i] = replace.Replace(a)
			if strings.Contains(args[i], "${") {
				return nil, errors.New("unresolved plugin argument variable")
			}
		}
		cmd := exec.CommandContext(ctx, command, args...)
		cmd.Env = os.Environ()
		for k, v := range server.Env {
			cmd.Env = append(cmd.Env, k+"="+replace.Replace(v))
		}
		cmd.Dir = root
		return cmd, nil
	}
	return nil, errors.New("missing MCP server")
}
