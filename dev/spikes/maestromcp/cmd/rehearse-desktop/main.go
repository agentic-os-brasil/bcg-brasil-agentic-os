// rehearse-desktop proves two packaging adapters share one execution ledger.
// SDK clients launch the declared binaries; neither actual Claude host nor a
// model is involved, and the receipt keeps that boundary explicit.
package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/spike"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type observation struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Evidence any    `json:"evidence,omitempty"`
}
type receipt struct {
	SchemaVersion int           `json:"schema_version"`
	Status        string        `json:"status"`
	SDK           string        `json:"sdk"`
	Observations  []observation `json:"observations"`
	Limits        []string      `json:"limits"`
	Error         string        `json:"error,omitempty"`
}
type runner struct {
	ctx                                                         context.Context
	destination, server, pluginBuilder, extensionBuilder, state string
	fixture                                                     spike.Fixture
	receipt                                                     receipt
	created                                                     bool
}

func main() {
	r := &runner{}
	flag.StringVar(&r.server, "server", "", "native server binary")
	flag.StringVar(&r.pluginBuilder, "plugin-builder", "", "native plugin builder binary")
	flag.StringVar(&r.extensionBuilder, "extension-builder", "", "native Desktop extension builder binary")
	flag.StringVar(&r.destination, "destination", "", "new physical artifact directory below current user's Developer")
	flag.Parse()
	if flag.NArg() != 0 || r.server == "" || r.pluginBuilder == "" || r.extensionBuilder == "" || r.destination == "" {
		fmt.Fprintln(os.Stderr, "required: --server ABS --plugin-builder ABS --extension-builder ABS --destination NEWDIR")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	r.ctx = ctx
	r.receipt = receipt{SchemaVersion: 1, Status: "FAIL", SDK: "github.com/modelcontextprotocol/go-sdk@v1.8.0 CommandTransport", Limits: []string{"Actual Claude Code host: NOT_RUN", "Actual Claude Desktop host and extension installation: NOT_RUN", "Model calls, skills, hooks, host permissions and authentication: NOT_RUN", "Synthetic ledger only; this MCP does not sandbox arbitrary host filesystem tools"}}
	err := r.run()
	if err == nil {
		r.receipt.Status = "PASS"
	} else {
		r.receipt.Error = err.Error()
	}
	data, errJSON := json.MarshalIndent(r.receipt, "", "  ")
	if errJSON != nil {
		fmt.Fprintln(os.Stderr, errJSON)
		os.Exit(1)
	}
	if r.created {
		if err := os.WriteFile(filepath.Join(r.destination, "rehearsal-desktop-receipt.json"), append(data, '\n'), 0600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	fmt.Println(string(data))
	if err != nil {
		os.Exit(1)
	}
}
func (r *runner) pass(name string, evidence any) {
	r.receipt.Observations = append(r.receipt.Observations, observation{name, "PASS", evidence})
}
func (r *runner) command(binary string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(r.ctx, binary, args...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

func (r *runner) run() error {
	for _, binary := range []string{r.server, r.pluginBuilder, r.extensionBuilder} {
		if !filepath.IsAbs(binary) {
			return errors.New("all executable paths must be absolute")
		}
	}
	if err := createDestination(r.destination); err != nil {
		return err
	}
	r.created = true
	r.state = filepath.Join(r.destination, "State")
	seed, err := r.command(r.server, "seed", "--root", r.state)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	if err := json.Unmarshal(seed, &r.fixture); err != nil {
		return err
	}
	seededLedger, err := snapshot(filepath.Join(r.state, "state"))
	if err != nil {
		return err
	}
	plugin := filepath.Join(r.destination, "PluginV1")
	if _, err := r.command(r.pluginBuilder, "--destination", plugin, "--server", r.server, "--state-root", r.state, "--workspace", "case-a", "--version", "0.0.1"); err != nil {
		return fmt.Errorf("plugin build: %w", err)
	}
	archive1, expanded1 := filepath.Join(r.destination, "ExtensionV1.mcpb"), filepath.Join(r.destination, "ExtensionV1")
	if err := r.extension(archive1, expanded1, "0.0.1"); err != nil {
		return err
	}
	if err := unchanged(filepath.Join(r.state, "state"), seededLedger); err != nil {
		return fmt.Errorf("initial package build changed state: %w", err)
	}
	productBefore, err := snapshot(plugin)
	if err != nil {
		return err
	}
	archiveBefore, err := snapshotFile(archive1)
	if err != nil {
		return err
	}
	expandedBefore, err := snapshot(expanded1)
	if err != nil {
		return err
	}
	serverBefore, err := snapshotFile(r.server)
	if err != nil {
		return err
	}
	pluginBinary, err := snapshotFile(filepath.Join(plugin, "scripts", "maestro-mcp-spike"))
	if err != nil {
		return err
	}
	extensionBinary, err := snapshotFile(filepath.Join(expanded1, "server", "maestro-mcp-spike"))
	if err != nil {
		return err
	}
	if pluginBinary.SHA256 != serverBefore.SHA256 || extensionBinary.SHA256 != serverBefore.SHA256 {
		return errors.New("packaged executable bytes differ from source server")
	}
	r.pass("both_adapters_bundle_identical_server_without_mutating_state", map[string]any{"server_sha256": serverBefore.SHA256, "initial_ledger_files": len(seededLedger)})
	codeCmd, err := adapterCommand(r.ctx, plugin, r.state, false)
	if err != nil {
		return err
	}
	code, err := connect(r.ctx, codeCmd)
	if err != nil {
		return err
	}
	defer code.Close()
	initial, err := readContext(r.ctx, code)
	if err != nil {
		return err
	}
	if initial.State != "running" || initial.StateRevision != 2 || initial.AttemptID == "" {
		return errors.New("seed did not expose running context")
	}
	if err := discover(r.ctx, code); err != nil {
		return err
	}
	r.pass("code_packaging_adapter_connects_via_official_sdk", map[string]any{"protocol_version": code.InitializeResult().ProtocolVersion, "command_source": "PluginV1/.mcp.json", "actual_code_host": "NOT_RUN"})
	_, err = call(r.ctx, code, "maestro_checkpoint", checkpoint(initial, "CODE_TO_DESKTOP_CHECKPOINT", "Continue synthetic handoff from Desktop", true))
	if err != nil {
		return err
	}
	paused, err := readContext(r.ctx, code)
	if err != nil {
		return err
	}
	if paused.Workspace != "case-a" || paused.ItemID != initial.ItemID || paused.State != "paused" || paused.StateRevision != initial.StateRevision+2 || paused.AttemptID != "" || paused.Summary != "CODE_TO_DESKTOP_CHECKPOINT" || paused.NextStep != "Continue synthetic handoff from Desktop" {
		return errors.New("Code adapter did not persist the exact submitted checkpoint and paused revision")
	}
	desktopCmd, err := adapterCommand(r.ctx, expanded1, r.state, true)
	if err != nil {
		return err
	}
	desktop, err := connect(r.ctx, desktopCmd)
	if err != nil {
		return err
	}
	defer desktop.Close()
	if err := discover(r.ctx, desktop); err != nil {
		return err
	}
	fromDesktop, err := readContext(r.ctx, desktop)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(fromDesktop, paused) {
		return errors.New("Desktop adapter did not recover exact Code checkpoint")
	}
	r.pass("independent_desktop_adapter_reads_exact_paused_checkpoint", map[string]any{"protocol_version": desktop.InitializeResult().ProtocolVersion, "state_revision": paused.StateRevision, "summary": paused.Summary, "actual_desktop_host": "NOT_RUN"})
	resumeResult, err := call(r.ctx, desktop, "maestro_resume", map[string]any{"expected_revision": paused.StateRevision})
	if err != nil {
		return err
	}
	var resumed spike.ContextOutput
	if err := decode(resumeResult, &resumed); err != nil {
		return err
	}
	if resumed.State != "running" || resumed.AttemptID == "" || resumed.AttemptID == initial.AttemptID || resumed.Summary != paused.Summary || resumed.NextStep != paused.NextStep {
		return errors.New("Desktop resume failed continuity/new-attempt contract")
	}
	r.pass("desktop_resume_creates_new_attempt_and_preserves_handoff", map[string]any{"state_revision": resumed.StateRevision, "new_attempt": true})
	staleRevision := checkpoint(initial, "stale Code revision", "reject", false)
	if err := r.rejectUnchanged(code, staleRevision); err != nil {
		return fmt.Errorf("stale Code revision: %w", err)
	}
	staleAttempt := checkpoint(resumed, "stale Code attempt", "reject", false)
	staleAttempt["attempt_id"] = initial.AttemptID
	if err := r.rejectUnchanged(code, staleAttempt); err != nil {
		return fmt.Errorf("stale Code attempt: %w", err)
	}
	r.pass("old_code_revision_and_attempt_rejected_without_mutation", map[string]int{"rejected_calls": 2})
	codeCurrent, err := readContext(r.ctx, code)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(codeCurrent, resumed) {
		return errors.New("already-running Code process retained stale context")
	}
	beforeRace, err := r.revisions()
	if err != nil {
		return err
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]*mcp.CallToolResult, 2)
	failures := make([]error, 2)
	sessions := []*mcp.ClientSession{code, desktop}
	summaries := []string{"RACE_CODE_WINNER", "RACE_DESKTOP_WINNER"}
	for i := range sessions {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], failures[i] = sessions[i].CallTool(r.ctx, &mcp.CallToolParams{Name: "maestro_checkpoint", Arguments: checkpoint(resumed, summaries[i], "Inspect the one committed winner", false)})
		}(i)
	}
	close(start)
	wg.Wait()
	winner := -1
	successes := 0
	for i, result := range results {
		if failures[i] == nil && result != nil && !result.IsError {
			winner = i
			successes++
		}
	}
	if r.ctx.Err() != nil {
		return r.ctx.Err()
	}
	if successes != 1 {
		return fmt.Errorf("concurrent writers produced %d successes, expected exactly one", successes)
	}
	loser := 1 - winner
	if failures[loser] != nil || results[loser] == nil || !results[loser].IsError {
		return errors.New("race loser did not return an MCP tool rejection")
	}
	loserCode := conflictCode(results[loser])
	if loserCode == "" {
		return errors.New("race loser did not return revision_conflict or item_busy")
	}
	final, err := readContext(r.ctx, code)
	if err != nil {
		return err
	}
	finalDesktop, err := readContext(r.ctx, desktop)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(final, finalDesktop) || final.StateRevision != resumed.StateRevision+1 || final.Summary != summaries[winner] || final.AttemptID != resumed.AttemptID {
		return errors.New("race final state lost or duplicated an update")
	}
	afterRace, err := r.revisions()
	if err != nil {
		return err
	}
	if len(afterRace) != len(beforeRace)+1 {
		return errors.New("race did not create exactly one immutable revision")
	}
	for i, old := range beforeRace {
		if !reflect.DeepEqual(old, afterRace[i]) {
			return errors.New("race altered an earlier immutable revision")
		}
	}
	latest := afterRace[len(afterRace)-1]
	if latest.State.StateRevision != final.StateRevision || latest.Checkpoint == nil || latest.Checkpoint.Summary != summaries[winner] {
		return errors.New("immutable winning commit does not match projected context")
	}
	r.pass("two_live_processes_contend_one_revision_without_lost_update", map[string]any{"attempted_writers": 2, "successful_writers": 1, "rejected_writers": 1, "loser_mcp_is_error": true, "loser_code": loserCode, "code_process_id": codeCmd.Process.Pid, "desktop_process_id": desktopCmd.Process.Pid, "winner": []string{"code_adapter", "desktop_adapter"}[winner], "before_revision": resumed.StateRevision, "after_revision": final.StateRevision, "immutable_revisions": len(afterRace), "earlier_commits_unchanged": true})
	ledgerBefore, err := snapshot(filepath.Join(r.state, "state"))
	if err != nil {
		return err
	}
	archive2, expanded2 := filepath.Join(r.destination, "ExtensionV2.mcpb"), filepath.Join(r.destination, "ExtensionV2")
	if err := r.extension(archive2, expanded2, "0.0.2"); err != nil {
		return err
	}
	v2FilesBefore, err := snapshot(expanded2)
	if err != nil {
		return err
	}
	archive2Before, err := snapshotFile(archive2)
	if err != nil {
		return err
	}
	v2Binary, err := snapshotFile(filepath.Join(expanded2, "server", "maestro-mcp-spike"))
	if err != nil {
		return err
	}
	if v2Binary.SHA256 != serverBefore.SHA256 {
		return errors.New("v2 package changed server bytes")
	}
	if err := unchanged(filepath.Join(r.state, "state"), ledgerBefore); err != nil {
		return fmt.Errorf("extension rebuild changed owner state: %w", err)
	}
	v2Cmd, err := adapterCommand(r.ctx, expanded2, r.state, true)
	if err != nil {
		return err
	}
	v2, err := connect(r.ctx, v2Cmd)
	if err != nil {
		return err
	}
	defer v2.Close()
	v2Context, err := readContext(r.ctx, v2)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(v2Context, final) {
		return errors.New("v2 extension lost shared state")
	}
	if err := unchanged(filepath.Join(r.state, "state"), ledgerBefore); err != nil {
		return fmt.Errorf("v2 read mutated ledger: %w", err)
	}
	if err := unchanged(plugin, productBefore); err != nil {
		return err
	}
	if err := unchanged(expanded1, expandedBefore); err != nil {
		return err
	}
	if err := unchanged(expanded2, v2FilesBefore); err != nil {
		return err
	}
	archive2After, err := snapshotFile(archive2)
	if err != nil || archive2After != archive2Before {
		return errors.New("v2 package changed")
	}
	archiveAfter, err := snapshotFile(archive1)
	if err != nil || archiveAfter != archiveBefore {
		return errors.New("v1 package changed")
	}
	serverAfter, err := snapshotFile(r.server)
	if err != nil || serverAfter != serverBefore {
		return errors.New("source server binary changed")
	}
	r.pass("v2_package_reconnect_preserves_ledger_and_v1_products", map[string]any{"versions": []string{"0.0.1", "0.0.2"}, "state_revision": final.StateRevision, "state_files_unchanged": true, "source_and_v1_files_unchanged": true, "actual_host_update": "NOT_RUN"})
	r.pass("artifacts_ready_for_host_tests", map[string]string{"state_root": r.state, "plugin": plugin, "extension_v1": archive1, "extension_v2": archive2})
	return nil
}

func (r *runner) extension(archive, expanded, version string) error {
	if _, err := r.command(r.extensionBuilder, "--destination", archive, "--server", r.server, "--state-root", r.state, "--workspace", "case-a", "--version", version); err != nil {
		return fmt.Errorf("extension %s build: %w", version, err)
	}
	return extractBundle(archive, expanded)
}
func checkpoint(h spike.ContextOutput, summary, next string, pause bool) map[string]any {
	return map[string]any{"expected_revision": h.StateRevision, "attempt_id": h.AttemptID, "summary": summary, "next_step": next, "pause": pause}
}

// Only these bounded store conflicts qualify as the expected race rejection.
// A generic tool error, transport failure or extended error body is not proof.
func conflictCode(result *mcp.CallToolResult) string {
	if result == nil || !result.IsError || len(result.Content) != 1 {
		return ""
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || (text.Text != "revision_conflict" && text.Text != "item_busy") {
		return ""
	}
	return text.Text
}
func connect(ctx context.Context, cmd *exec.Cmd) (*mcp.ClientSession, error) {
	cmd.Stderr = os.Stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "maestro-desktop-adapter-rehearsal", Version: "0.0.1"}, nil)
	return client.Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil)
}
func discover(ctx context.Context, session *mcp.ClientSession) error {
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return err
	}
	var names []string
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if !reflect.DeepEqual(names, []string{"maestro_checkpoint", "maestro_context", "maestro_resume"}) || result.NextCursor != "" {
		return errors.New("unexpected adapter tool discovery")
	}
	return nil
}
func call(ctx context.Context, session *mcp.ClientSession, name string, args any) (*mcp.CallToolResult, error) {
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, err
	}
	if result == nil || result.IsError {
		return nil, fmt.Errorf("tool %s rejected", name)
	}
	return result, nil
}
func decode(result *mcp.CallToolResult, target any) error {
	if result.StructuredContent == nil {
		return errors.New("missing structured content")
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return err
	}
	if len(data) > 4096 {
		return errors.New("context response exceeds bound")
	}
	return json.Unmarshal(data, target)
}
func readContext(ctx context.Context, session *mcp.ClientSession) (spike.ContextOutput, error) {
	var h spike.ContextOutput
	result, err := call(ctx, session, "maestro_context", map[string]any{})
	if err != nil {
		return h, err
	}
	err = decode(result, &h)
	return h, err
}
func (r *runner) rejectUnchanged(session *mcp.ClientSession, args map[string]any) error {
	before, err := snapshot(filepath.Join(r.state, "state"))
	if err != nil {
		return err
	}
	result, callErr := session.CallTool(r.ctx, &mcp.CallToolParams{Name: "maestro_checkpoint", Arguments: args})
	if callErr != nil || result == nil || !result.IsError {
		return errors.New("stale writer did not return an MCP tool rejection")
	}
	if _, err := readContext(r.ctx, session); err != nil {
		return fmt.Errorf("server unavailable after denial: %w", err)
	}
	return unchanged(filepath.Join(r.state, "state"), before)
}

func (r *runner) revisions() ([]execution.Revision, error) {
	c := r.fixture.Cases["case-a"]
	root := filepath.Join(r.state, "state", "workspaces", c.WorkspaceID, "execution", "items", c.ItemID, "revisions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var result []execution.Revision
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil, errors.New("unexpected immutable revision entry")
		}
		data, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		var rev execution.Revision
		if err := json.Unmarshal(data, &rev); err != nil {
			return nil, err
		}
		result = append(result, rev)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].State.StateRevision < result[j].State.StateRevision })
	for i, rev := range result {
		if rev.State.StateRevision != i+1 || rev.State.ItemID != c.ItemID || rev.State.WorkspaceID != c.WorkspaceID || rev.Transition.StateRevision != rev.State.StateRevision {
			return nil, errors.New("immutable revision history is not a complete matching sequence")
		}
	}
	return result, nil
}

func createDestination(root string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	developer := filepath.Join(home, "Developer")
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || !strings.HasPrefix(root, developer+string(filepath.Separator)) {
		return errors.New("destination must be a clean absolute path below current user's Developer")
	}
	resolved, err := filepath.EvalSymlinks(filepath.Dir(root))
	if err != nil {
		return err
	}
	if resolved != filepath.Dir(root) {
		return errors.New("destination parent contains a symlink")
	}
	return os.Mkdir(root, 0700)
}

type fingerprint struct {
	SHA256 string
	Mode   fs.FileMode
}

func snapshotFile(path string) (fingerprint, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return fingerprint{}, err
	}
	if !info.Mode().IsRegular() {
		return fingerprint{}, errors.New("snapshot requires regular files")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fingerprint{}, err
	}
	sum := sha256.Sum256(data)
	return fingerprint{hex.EncodeToString(sum[:]), info.Mode()}, nil
}
func snapshot(root string) (map[string]fingerprint, error) {
	result := map[string]fingerprint{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		fp, err := snapshotFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(rel)] = fp
		return nil
	})
	return result, err
}
func unchanged(root string, before map[string]fingerprint) error {
	after, err := snapshot(root)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, after) {
		return errors.New("file set, bytes or permissions changed")
	}
	return nil
}

func extractBundle(archive, destination string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	if len(z.File) != 2 {
		return errors.New("generated bundle must contain exactly two files")
	}
	seen := map[string]bool{}
	var total uint64
	for _, entry := range z.File {
		if (entry.Name != "manifest.json" && entry.Name != "server/maestro-mcp-spike") || seen[entry.Name] || !entry.Mode().IsRegular() {
			return errors.New("unexpected or unsafe bundle entry")
		}
		seen[entry.Name] = true
		if entry.UncompressedSize64 > 64<<20 {
			return errors.New("bundle entry exceeds size limit")
		}
		total += entry.UncompressedSize64
	}
	if !seen["manifest.json"] || !seen["server/maestro-mcp-spike"] || total > 64<<20 {
		return errors.New("invalid generated bundle files or size")
	}
	if err := createDestination(destination); err != nil {
		return err
	}
	for _, entry := range z.File {
		path := filepath.Join(destination, filepath.FromSlash(entry.Name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		mode := fs.FileMode(0600)
		if entry.Name == "server/maestro-mcp-spike" {
			if entry.Mode().Perm()&0100 == 0 {
				return errors.New("bundle binary lacks owner executable bit")
			}
			mode = 0700
		}
		in, err := entry.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			in.Close()
			return err
		}
		n, copyErr := io.Copy(out, io.LimitReader(in, 64<<20+1))
		closeErr := out.Close()
		inputErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if inputErr != nil {
			return inputErr
		}
		if n != int64(entry.UncompressedSize64) {
			return errors.New("bundle entry size mismatch")
		}
	}
	return nil
}

type launch struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

func adapterCommand(ctx context.Context, root, state string, desktop bool) (*exec.Cmd, error) {
	var config launch
	template, relative := "${CLAUDE_PLUGIN_ROOT}", "scripts/maestro-mcp-spike"
	if desktop {
		data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
		if err != nil {
			return nil, err
		}
		var manifest struct {
			Version string `json:"version"`
			Server  struct {
				Type       string `json:"type"`
				EntryPoint string `json:"entry_point"`
				Config     launch `json:"mcp_config"`
			} `json:"server"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, err
		}
		if manifest.Server.Type != "binary" || manifest.Server.EntryPoint != "server/maestro-mcp-spike" || (manifest.Version != "0.0.1" && manifest.Version != "0.0.2") {
			return nil, errors.New("unexpected Desktop manifest adapter")
		}
		config = manifest.Server.Config
		template, relative = "${__dirname}", "server/maestro-mcp-spike"
	} else {
		data, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
		if err != nil {
			return nil, err
		}
		var manifest struct {
			Servers map[string]launch `json:"mcpServers"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, err
		}
		if len(manifest.Servers) != 1 {
			return nil, errors.New("unexpected plugin server count")
		}
		var ok bool
		config, ok = manifest.Servers["maestro"]
		if !ok {
			return nil, errors.New("missing plugin maestro server")
		}
	}
	if config.Command != template+"/"+relative || len(config.Env) != 0 || !reflect.DeepEqual(config.Args, []string{"serve", "--root", state, "--workspace", "case-a"}) {
		return nil, errors.New("adapter config is not the expected fixed-root subprocess")
	}
	command := filepath.Join(root, filepath.FromSlash(relative))
	cmd := exec.CommandContext(ctx, command, config.Args...)
	cmd.Dir = root
	return cmd, nil
}
