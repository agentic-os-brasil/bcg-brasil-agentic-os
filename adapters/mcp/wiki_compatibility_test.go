//go:build !windows

package product_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPrivateWikiEventControlRefusesActualOldService(t *testing.T) {
	helper := os.Getenv("MAESTRO_WIKI_OLD_SERVICE_HELPER")
	if helper == "" {
		t.Skip("actual old-service witness not selected")
	}
	if !filepath.IsAbs(helper) {
		t.Fatal("old-service witness must be absolute")
	}
	f := newOwnerFixture(t, false)
	prepareMemoryFixture(t, f)
	cs := f.connect(t, cadenceApprove)
	memoryCall(t, cs, "memory.capture", map[string]any{"text": "private-wiki-body: ACTUAL_OLD_READER_REFUSAL"}, false)
	memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": time.Now().UTC().Format("2006-01-02")}, false)
	q := cadenceCall(t, cs, "qualify", map[string]any{"job_id": "wiki-incremental-sync", "expected_revision": 0}, false)
	cadenceCall(t, cs, "enroll", map[string]any{"job_id": "wiki-incremental-sync", "expected_revision": q["revision"], "timezone": "UTC"}, false)
	before := cadenceTree(t, f)
	body, err := json.Marshal(map[string]string{"product_root": f.options.ProductRoot, "user_home": f.options.UserHome, "workspace": f.options.WorkspacePath})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, helper)
	cmd.Stdin = bytes.NewReader(body)
	output, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "refused:event_only" {
		t.Fatalf("actual old decoder did not refuse the protected schedule: %s %v", output, err)
	}
	if cadenceTree(t, f) != before {
		t.Fatal("old service changed protected owner state")
	}
}

func TestPrivateWikiChangedExecutableReadsHistoricalButNeedsCurrentCompilation(t *testing.T) {
	f := newOwnerFixture(t, true)
	prepareMemoryFixture(t, f)
	cs := f.connect(t, cadenceApprove)
	memoryCall(t, cs, "memory.capture", map[string]any{"text": "private-wiki-body: STABLE_SOURCE_ACROSS_ACTUAL_BUILDS"}, false)
	memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": time.Now().UTC().Format("2006-01-02")}, false)
	q := cadenceCall(t, cs, "qualify", map[string]any{"job_id": "wiki-incremental-sync", "expected_revision": 0}, false)
	old := wikiCall(t, cs, "status", nil, false)
	generation := old["generation"].(string)
	oldPage := wikiCall(t, cs, "read", map[string]any{"generation": generation, "reference": "index.md"}, false)["content"].(string)
	workspace := cadenceCall(t, cs, "status", nil, false)["workspace_id"].(string)
	manifest := filepath.Join(f.root, "memory", "workspaces", workspace, ".private-wiki", "versions", generation, "manifest.json")
	oldManifest, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.Close(); err != nil {
		t.Fatal(err)
	}
	oldBinary, err := os.ReadFile(f.binary)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(filepath.Dir(f.binary), "maestro-mcp-current-wiki-build")
	build := exec.Command("go", "build", "-ldflags", "-X main.AuthorityRegistrySHA256=actual-wiki-current-build", "-o", replacement, "./cmd/maestro-mcp")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("actual rebuild: %s %v", output, err)
	}
	currentBinary, err := os.ReadFile(replacement)
	if err != nil || sha256.Sum256(oldBinary) == sha256.Sum256(currentBinary) {
		t.Fatal("the rebuilt executable did not change actual bytes")
	}
	f.binary = replacement
	cs = f.connect(t, cadenceApprove)
	status := wikiCall(t, cs, "status", nil, false)
	if status["state"] != "current" || status["implementation_current"] != false || status["configuration_current"] != true || status["generation"] != generation {
		t.Fatal("historical producer evidence was replaced or unreadable")
	}
	if page := wikiCall(t, cs, "read", map[string]any{"generation": generation, "reference": "index.md"}, false)["content"].(string); page != oldPage {
		t.Fatal("current reader rewrote compatible historical content")
	}
	prior := cadenceJob(t, cadenceCall(t, cs, "status", nil, false), "wiki-incremental-sync")
	if prior["qualification_state"] != "stale" || prior["qualification"].(map[string]any)["id"] != q["qualification"].(map[string]any)["id"] {
		t.Fatal("changed executable retained or relabelled old qualification")
	}
	before := cadenceTree(t, f)
	cadenceCall(t, cs, "qualify", map[string]any{"job_id": "wiki-integrity-check", "expected_revision": 0}, true)
	if cadenceTree(t, f) != before {
		t.Fatal("historical read manufactured current qualification state")
	}
	qualified := cadenceCall(t, cs, "qualify", map[string]any{"job_id": "wiki-reconcile", "expected_revision": 0}, false)
	if qualified["qualification_state"] != "qualified" {
		t.Fatal(qualified)
	}
	status = wikiCall(t, cs, "status", nil, false)
	if status["implementation_current"] != true || status["generation"] == generation {
		t.Fatal("current compilation did not produce genuine new evidence")
	}
	unchanged, err := os.ReadFile(manifest)
	if err != nil || !bytes.Equal(unchanged, oldManifest) {
		t.Fatal("current compilation substituted historical provenance")
	}
	var stored struct {
		Implementation string `json:"implementation"`
	}
	if json.Unmarshal(unchanged, &stored) != nil || stored.Implementation != fmt.Sprintf("%x", sha256.Sum256(oldBinary)) {
		t.Fatal("historical producer does not match actual original executable")
	}
}

func TestPrivateWikiRunningProcessCannotAdoptReplacedExecutable(t *testing.T) {
	for _, mode := range []string{"warm", "before-first-operation"} {
		t.Run(mode, func(t *testing.T) { runningProcessReplacement(t, mode == "warm") })
	}
}

func runningProcessReplacement(t *testing.T, warm bool) {
	f := newOwnerFixture(t, true)
	prepareMemoryFixture(t, f)
	command := exec.Command(f.binary, "serve", "--product-root", f.options.ProductRoot, "--user-home", f.options.UserHome, "--workspace", f.options.WorkspacePath)
	client := mcp.NewClient(&mcp.Implementation{Name: "running-image-witness", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: cadenceApprove})
	cs, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: command, TerminateDuration: time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	pid := command.Process.Pid
	oldBinary, err := os.ReadFile(f.binary)
	if err != nil {
		t.Fatal(err)
	}
	oldGeneration := ""
	if warm {
		memoryCall(t, cs, "memory.capture", map[string]any{"text": "private-wiki-body: ORIGINAL_RUNNING_PROCESS"}, false)
		memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": time.Now().UTC().Format("2006-01-02")}, false)
		cadenceCall(t, cs, "qualify", map[string]any{"job_id": "wiki-incremental-sync", "expected_revision": 0}, false)
		oldGeneration = wikiCall(t, cs, "status", nil, false)["generation"].(string)
	}
	replacement := filepath.Join(filepath.Dir(f.binary), "replacement-running-image")
	build := exec.Command("go", "build", "-ldflags", "-X main.AuthorityRegistrySHA256=atomic-running-image-replacement", "-o", replacement, "./cmd/maestro-mcp")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("replacement build: %s %v", output, err)
	}
	newBinary, err := os.ReadFile(replacement)
	if err != nil || sha256.Sum256(newBinary) == sha256.Sum256(oldBinary) {
		t.Fatal("actual replacement bytes did not differ")
	}
	if err := os.Rename(replacement, f.binary); err != nil {
		t.Fatal(err)
	}
	// A separate current process creates new source while the original MCP
	// process and its established transport remain alive.
	current := f.connect(t, cadenceApprove)
	memoryCall(t, current, "memory.capture", map[string]any{"text": "private-wiki-body: SOURCE_FROM_REPLACEMENT_PROCESS"}, false)
	memoryCall(t, current, "memory.consolidate", map[string]any{"cycle": "daily", "date": time.Now().UTC().Format("2006-01-02")}, false)
	workspace := cadenceCall(t, current, "status", nil, false)["workspace_id"].(string)
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: map[string]any{"operation": "maintenance.qualify", "job_id": "wiki-reconcile", "expected_revision": 0}})
	if err != nil {
		t.Fatal(err)
	}
	if command.Process.Pid != pid || command.ProcessState != nil {
		t.Fatal("the original process was replaced or restarted")
	}
	root := filepath.Join(f.root, "memory", "workspaces", workspace, ".private-wiki")
	body, err := os.ReadFile(filepath.Join(root, "current.json"))
	var pointer struct {
		Generation string `json:"generation"`
	}
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err == nil && json.Unmarshal(body, &pointer) != nil {
		t.Fatal("current pointer unavailable")
	}
	if out.IsError && pointer.Generation == oldGeneration {
		return
	}
	body, err = os.ReadFile(filepath.Join(root, "versions", pointer.Generation, "manifest.json"))
	if err != nil {
		t.Fatal("replaced process neither refused nor preserved its current pointer")
	}
	var stored struct {
		Implementation string `json:"implementation"`
	}
	if json.Unmarshal(body, &stored) != nil {
		t.Fatal("actual produced manifest unavailable")
	}
	t.Fatalf("replaced live process published/qualified instead of requiring restart: stored=%s old=%x replacement=%x", stored.Implementation, sha256.Sum256(oldBinary), sha256.Sum256(newBinary))
}
