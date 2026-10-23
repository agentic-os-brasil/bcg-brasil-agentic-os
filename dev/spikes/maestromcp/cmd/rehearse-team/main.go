// rehearse-team proves publication, local materialization and execution-state
// continuity with real MCP subprocesses. It does not activate a Claude host.
package main

import (
	"archive/zip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
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
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/distribution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/spike"
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
	Target        string        `json:"target"`
	SDK           string        `json:"sdk"`
	Observations  []observation `json:"observations"`
	Limits        []string      `json:"limits"`
	FailedStep    string        `json:"failed_step,omitempty"`
}
type runner struct {
	ctx                            context.Context
	server, installer, destination string
	created                        bool
	step                           string
	receipt                        receipt
}

func main() {
	r := &runner{}
	flag.StringVar(&r.server, "server", "", "absolute native Mach-O or PE server")
	flag.StringVar(&r.installer, "installer", "", "absolute native installer artifact; never executed by this rehearsal")
	flag.StringVar(&r.destination, "destination", "", "new physical directory below the current user's Developer")
	flag.Parse()
	if flag.NArg() != 0 || r.server == "" || r.installer == "" || r.destination == "" {
		fmt.Fprintln(os.Stderr, "required: --server ABS --installer ABS --destination NEWDIR")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	r.ctx = ctx
	r.receipt = receipt{SchemaVersion: 1, Status: "FAIL", Target: runtime.GOOS + "-" + runtime.GOARCH, SDK: "github.com/modelcontextprotocol/go-sdk@v1.8.0 CommandTransport", Limits: []string{
		"Actual Claude Code and Desktop host installation, registration and activation: NOT_RUN",
		"Actual SharePoint upload, OneDrive synchronization and team onboarding: NOT_RUN",
		"Installer subprocess and embedded installer key binding: NOT_RUN; Apply library uses an ephemeral synthetic key",
		"Ephemeral Ed25519 key exists only in this process; no production or team release authority granted",
		"Model calls, authentication, hooks, permissions and native Windows qualification: NOT_RUN",
	}}
	err := r.run()
	if err == nil {
		r.receipt.Status = "PASS"
	} else {
		r.receipt.FailedStep = r.step
		fmt.Fprintln(os.Stderr, err)
	}
	body, encodeErr := json.MarshalIndent(r.receipt, "", "  ")
	if encodeErr != nil {
		fmt.Fprintln(os.Stderr, encodeErr)
		os.Exit(1)
	}
	if r.created {
		if writeErr := os.WriteFile(filepath.Join(r.destination, "rehearsal-team-receipt.json"), append(body, '\n'), 0600); writeErr != nil {
			fmt.Fprintln(os.Stderr, writeErr)
			os.Exit(1)
		}
	}
	fmt.Println(string(body))
	if err != nil {
		os.Exit(1)
	}
}
func (r *runner) pass(name string, evidence any) {
	r.receipt.Observations = append(r.receipt.Observations, observation{name, "PASS", evidence})
}
func (r *runner) run() error {
	r.step = "prepare_isolated_destination"
	if !filepath.IsAbs(r.server) || !filepath.IsAbs(r.installer) {
		return errors.New("executable inputs must be absolute")
	}
	if err := createDestination(r.destination); err != nil {
		return err
	}
	r.created = true
	target := r.receipt.Target
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	feed, home := filepath.Join(r.destination, "Synthetic Synced Feed"), filepath.Join(r.destination, "Recipient Local")
	publish := func(version string, sequence uint64) (distribution.Manifest, error) {
		return distribution.Publish(distribution.ReleaseInput{FeedRoot: feed, Version: version, Target: target, Server: r.server, Installer: r.installer, Sequence: sequence, PrivateKey: key})
	}
	input := distribution.InstallInput{FeedRoot: feed, Home: home, Target: target, PublicKey: pub}
	r.step = "publish_and_apply_0_0_3"
	manifest1, err := publish("0.0.3", 1)
	if err != nil {
		return err
	}
	oldEnvelope, err := os.ReadFile(filepath.Join(feed, "feed.json"))
	if err != nil {
		return err
	}
	first, err := distribution.Apply(input)
	if err != nil {
		return err
	}
	if !first.Changed || first.Version != "0.0.3" || first.Sequence != 1 {
		return errors.New("initial local generation was not activated")
	}
	active, err := distribution.Inspect(home)
	if err != nil {
		return err
	}
	if active.Version != first.Version || active.Sequence != 1 {
		return errors.New("initial catalog points to wrong generation")
	}
	oldProduct := filepath.Dir(first.Extension)
	originalProducts, err := snapshot(oldProduct)
	if err != nil {
		return err
	}
	if err := checkPackageBinaries(first, manifest1.Artifacts[0].SHA256); err != nil {
		return err
	}
	r.pass("signed_publication_materializes_local_plugin_and_mcpb", map[string]any{"version": first.Version, "sequence": first.Sequence, "server_sha256": manifest1.Artifacts[0].SHA256, "installer_sha256": manifest1.Artifacts[1].SHA256, "installer_executed": false})

	r.step = "real_v003_mcp_checkpoint"
	old, err := connectPlugin(r.ctx, first)
	if err != nil {
		return err
	}
	defer old.Close()
	initial, err := readContext(r.ctx, old)
	if err != nil {
		return err
	}
	if initial.Workspace != "case-a" || initial.State != "running" || initial.StateRevision != 2 || initial.AttemptID == "" {
		return errors.New("initial context differs from seeded case")
	}
	args := checkpoint(initial, "TEAM_SHAREPOINT_CONTINUITY", "Resume the same synthetic case after distribution update", true)
	result, err := call(r.ctx, old, "maestro_checkpoint", args)
	if err != nil {
		return err
	}
	var saved spike.CheckpointOutput
	if err := decode(result, &saved); err != nil {
		return err
	}
	paused, err := readContext(r.ctx, old)
	if err != nil {
		return err
	}
	if saved.State != "paused" || saved.StateRevision != 4 || paused.Workspace != "case-a" || paused.ItemID != initial.ItemID || paused.State != "paused" || paused.StateRevision != 4 || paused.AttemptID != "" || paused.Summary != "TEAM_SHAREPOINT_CONTINUITY" || paused.NextStep != args["next_step"] {
		return errors.New("exact checkpoint and paused revision were not persisted")
	}
	stateBefore, err := snapshot(first.StateRoot)
	if err != nil {
		return err
	}
	r.pass("real_v003_mcp_saves_exact_checkpoint_and_pauses", map[string]any{"initial_revision": 2, "paused_revision": 4, "summary": paused.Summary, "all_state_files_snapshotted": len(stateBefore)})

	r.step = "publish_and_apply_0_0_4"
	manifest2, err := publish("0.0.4", 2)
	if err != nil {
		return err
	}
	second, err := distribution.Apply(input)
	if err != nil {
		return err
	}
	if !second.Changed || second.Version != "0.0.4" || second.Sequence != 2 || second.StateRoot != first.StateRoot {
		return errors.New("update did not preserve fixed local state binding")
	}
	if err := unchanged(first.StateRoot, stateBefore); err != nil {
		return err
	}
	if err := unchanged(oldProduct, originalProducts); err != nil {
		return err
	}
	if err := checkPackageBinaries(second, manifest2.Artifacts[0].SHA256); err != nil {
		return err
	}
	active, err = distribution.Inspect(home)
	if err != nil {
		return err
	}
	if active.Version != "0.0.4" || active.Sequence != 2 {
		return errors.New("new local catalog was not promoted")
	}
	r.pass("v004_update_preserves_all_state_and_previous_generation", map[string]any{"version": second.Version, "sequence": second.Sequence, "state_unchanged": true, "previous_generation_unchanged": true})
	r.step = "same_feed_is_noop"
	homeBefore, err := snapshot(home)
	if err != nil {
		return err
	}
	repeated, err := distribution.Apply(input)
	if err != nil {
		return err
	}
	if repeated.Changed || repeated.Version != "0.0.4" || repeated.Sequence != 2 {
		return errors.New("same feed was not a no-op")
	}
	if err := unchanged(home, homeBefore); err != nil {
		return err
	}
	r.pass("same_signed_feed_is_noop", map[string]any{"changed": false, "all_local_files_unchanged": true})

	r.step = "real_v004_mcp_continuity_and_resume"
	next, err := connectPlugin(r.ctx, second)
	if err != nil {
		return err
	}
	defer next.Close()
	recovered, err := readContext(r.ctx, next)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(recovered, paused) {
		return errors.New("new plugin subprocess lost exact checkpoint")
	}
	if err := unchanged(first.StateRoot, stateBefore); err != nil {
		return err
	}
	result, err = call(r.ctx, next, "maestro_resume", map[string]any{"expected_revision": 4})
	if err != nil {
		return err
	}
	var resumed spike.ContextOutput
	if err := decode(result, &resumed); err != nil {
		return err
	}
	if resumed.Workspace != "case-a" || resumed.ItemID != initial.ItemID || resumed.State != "running" || resumed.StateRevision != 5 || resumed.AttemptID == "" || resumed.AttemptID == initial.AttemptID || resumed.Summary != paused.Summary || resumed.NextStep != paused.NextStep {
		return errors.New("resume did not preserve handoff and create a new attempt")
	}
	verified, err := readContext(r.ctx, next)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(resumed, verified) {
		return errors.New("resume receipt differs from persisted context")
	}
	r.pass("new_generation_mcp_reads_exact_checkpoint_and_resumes", map[string]any{"read_revision": 4, "resumed_revision": 5, "new_attempt": true, "summary_preserved": true, "next_step_preserved": true})

	r.step = "old_live_mcp_writer_is_rejected"
	stateRunning, err := snapshot(first.StateRoot)
	if err != nil {
		return err
	}
	rejected, transportErr := old.CallTool(r.ctx, &mcp.CallToolParams{Name: "maestro_checkpoint", Arguments: checkpoint(initial, "STALE_MUST_NOT_COMMIT", "reject", false)})
	if transportErr != nil {
		return fmt.Errorf("stale proof encountered transport failure: %w", transportErr)
	}
	if rejectionCode(rejected) != "revision_conflict" {
		return errors.New("old process did not return exact revision_conflict")
	}
	current, err := readContext(r.ctx, old)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, resumed) {
		return errors.New("old live process failed to read current state")
	}
	if err := unchanged(first.StateRoot, stateRunning); err != nil {
		return err
	}
	r.pass("old_live_writer_rejected_without_any_state_change", map[string]any{"code": "revision_conflict", "attempted_revision": 2, "current_revision": 5, "all_state_files_unchanged": true})

	r.step = "corrupt_future_payload_rejected"
	activeBefore, err := distribution.Inspect(home)
	if err != nil {
		return err
	}
	homeBefore, err = snapshot(home)
	if err != nil {
		return err
	}
	future, err := publish("0.0.5", 3)
	if err != nil {
		return err
	}
	corrupt := filepath.Join(feed, filepath.FromSlash(future.Artifacts[0].Path))
	f, err := os.OpenFile(corrupt, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	_, writeErr := f.Write([]byte("CORRUPTED_SYNTHETIC_FUTURE_PAYLOAD"))
	if closeErr := f.Close(); writeErr != nil || closeErr != nil {
		return errors.Join(writeErr, closeErr)
	}
	if _, err = distribution.Apply(input); err == nil || err.Error() != "artifact incomplete or changed; active installation preserved" {
		return errors.New("corrupt payload did not produce the expected integrity rejection")
	}
	if err := unchanged(home, homeBefore); err != nil {
		return err
	}
	activeAfter, err := distribution.Inspect(home)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(activeBefore, activeAfter) {
		return errors.New("corrupt update changed active generation")
	}
	r.pass("corrupt_future_payload_preserves_active_v004_and_state", map[string]any{"rejected_sequence": 3, "active_version": "0.0.4", "all_local_files_unchanged": true})

	r.step = "older_signed_publication_replay_rejected"
	if err := os.WriteFile(filepath.Join(feed, "feed.json"), oldEnvelope, 0600); err != nil {
		return err
	}
	if _, err = distribution.Apply(input); err == nil || err.Error() != "older publication replay rejected" {
		return errors.New("old envelope did not produce exact replay rejection")
	}
	if err := unchanged(home, homeBefore); err != nil {
		return err
	}
	activeAfter, err = distribution.Inspect(home)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(activeBefore, activeAfter) {
		return errors.New("replay changed active generation")
	}
	if err := unchanged(first.StateRoot, stateRunning); err != nil {
		return err
	}
	r.pass("older_valid_signed_publication_rejected_without_change", map[string]any{"replayed_sequence": 1, "active_sequence": 2, "active_version": "0.0.4", "state_revision": 5, "all_local_files_unchanged": true})
	return nil
}

func createDestination(root string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	developer := filepath.Join(home, "Developer")
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || !strings.HasPrefix(root, developer+string(filepath.Separator)) {
		return errors.New("destination must be a clean absolute path below Developer")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(root))
	if err != nil {
		return err
	}
	if parent != filepath.Dir(root) {
		return errors.New("destination parent contains a symlink")
	}
	return os.Mkdir(root, 0700)
}
func checkpoint(c spike.ContextOutput, summary, next string, pause bool) map[string]any {
	return map[string]any{"expected_revision": c.StateRevision, "attempt_id": c.AttemptID, "summary": summary, "next_step": next, "pause": pause}
}
func rejectionCode(result *mcp.CallToolResult) string {
	if result == nil || !result.IsError || len(result.Content) != 1 {
		return ""
	}
	t, ok := result.Content[0].(*mcp.TextContent)
	if !ok || t.Text != "revision_conflict" {
		return ""
	}
	return t.Text
}
func connectPlugin(ctx context.Context, receipt distribution.Receipt) (*mcp.ClientSession, error) {
	root := filepath.Join(filepath.Dir(receipt.Extension), "plugin")
	body, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	if err != nil {
		return nil, err
	}
	type launch struct {
		Type    string            `json:"type"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
	}
	var config struct {
		Servers map[string]launch `json:"mcpServers"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, err
	}
	binary := "scripts/maestro-mcp-spike"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	declared, ok := config.Servers["maestro"]
	if !ok || len(config.Servers) != 1 || declared.Type != "stdio" || declared.Command != "${CLAUDE_PLUGIN_ROOT}/"+binary || len(declared.Env) != 0 || !reflect.DeepEqual(declared.Args, []string{"serve", "--root", receipt.StateRoot, "--workspace", "case-a"}) {
		return nil, errors.New("materialized MCP configuration differs from fixed-root adapter")
	}
	cmd := exec.CommandContext(ctx, filepath.Join(root, filepath.FromSlash(binary)), declared.Args...)
	cmd.Dir = root
	cmd.Stderr = os.Stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "maestro-team-distribution-rehearsal", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil)
	if err != nil {
		return nil, err
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		session.Close()
		return nil, err
	}
	names := []string{}
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if tools.NextCursor != "" || !reflect.DeepEqual(names, []string{"maestro_checkpoint", "maestro_context", "maestro_resume"}) {
		session.Close()
		return nil, errors.New("unexpected MCP tools")
	}
	return session, nil
}
func call(ctx context.Context, s *mcp.ClientSession, name string, args any) (*mcp.CallToolResult, error) {
	r, err := s.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, err
	}
	if r == nil || r.IsError {
		return nil, errors.New("expected MCP operation rejected")
	}
	return r, nil
}
func decode(r *mcp.CallToolResult, out any) error {
	if r.StructuredContent == nil {
		return errors.New("missing structured tool result")
	}
	b, err := json.Marshal(r.StructuredContent)
	if err != nil {
		return err
	}
	if len(b) > 4096 {
		return errors.New("tool result exceeds bound")
	}
	return json.Unmarshal(b, out)
}
func readContext(ctx context.Context, s *mcp.ClientSession) (spike.ContextOutput, error) {
	var c spike.ContextOutput
	r, err := call(ctx, s, "maestro_context", map[string]any{})
	if err != nil {
		return c, err
	}
	err = decode(r, &c)
	return c, err
}

type fingerprint struct {
	SHA256 string
	Mode   fs.FileMode
}

func snapshot(root string) (map[string]fingerprint, error) {
	result := map[string]fingerprint{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("snapshot requires regular files")
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(rel)] = fingerprint{hex.EncodeToString(sum[:]), info.Mode()}
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
func checkPackageBinaries(receipt distribution.Receipt, want string) error {
	binary := "maestro-mcp-spike"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	body, err := os.ReadFile(filepath.Join(filepath.Dir(receipt.Extension), "plugin", "scripts", binary))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != want {
		return errors.New("plugin server bytes differ from signed release")
	}
	z, err := zip.OpenReader(receipt.Extension)
	if err != nil {
		return err
	}
	defer z.Close()
	if len(z.File) != 2 {
		return errors.New("extension contains unexpected files")
	}
	found := false
	for _, entry := range z.File {
		if entry.Name == "manifest.json" {
			continue
		}
		if entry.Name != "server/"+binary || found {
			return errors.New("unexpected extension executable path")
		}
		found = true
		in, err := entry.Open()
		if err != nil {
			return err
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, in)
		closeErr := in.Close()
		if copyErr != nil || closeErr != nil {
			return errors.Join(copyErr, closeErr)
		}
		if hex.EncodeToString(h.Sum(nil)) != want {
			return errors.New("extension server bytes differ from signed release")
		}
	}
	if !found {
		return errors.New("extension executable missing")
	}
	return nil
}
