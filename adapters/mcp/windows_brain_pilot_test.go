//go:build windows

package product_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The existing credentialed driver supplies the actual shipping server and
// current-user private temp. This family never substitutes an elevated token.
func TestWindowsAuthoredBrainPilot(t *testing.T) {
	windowsPrivateOrdinaryUser(t)
	root := windowsPrivateRoot(t)
	product, home, workspacePath := filepath.Join(root, "product"), filepath.Join(root, "owner"), filepath.Join(root, "workspace")
	for _, p := range []string{product, home} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	server := os.Getenv("MAESTRO_WINDOWS_CORE_SERVER")
	args := []string{"--product-root", product, "--user-home", home, "--workspace", workspacePath}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, server, append([]string{"init"}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("public authored native setup: %s %v", out, err)
	}
	managed := windowsCoreTree(t, product)
	approved := true
	var intervene func() error
	connect := func() *mcp.ClientSession {
		client := mcp.NewClient(&mcp.Implementation{Name: "windows-authored-brain-proof", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			if intervene != nil {
				if err := intervene(); err != nil {
					return nil, err
				}
				intervene = nil
			}
			if approved {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}
			return &mcp.ElicitResult{Action: "decline"}, nil
		}})
		cs, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.CommandContext(ctx, server, append([]string{"serve"}, args...)...), TerminateDuration: time.Second}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cs.Close() })
		return cs
	}
	cs := connect()
	id := contentCall(t, cs, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
	before := windowsCoreTree(t, workspacePath)
	brainCall(t, cs, "status", nil, false)
	brainCall(t, cs, "templates", nil, false)
	approved = false
	brainCall(t, cs, "prepare", nil, true)
	if !reflect.DeepEqual(before, windowsCoreTree(t, workspacePath)) {
		t.Fatal("query or declined preparation changed workspace")
	}
	approved = true
	brainCall(t, cs, "prepare", nil, false)
	const page = "projects/native-brief.md"
	body := authoredBrief(id, page, "Native reviewed brief")
	created := brainCall(t, cs, "create", map[string]any{"page": page, "text": body}, false)
	if created["state"] != "written" {
		t.Fatal("ordinary attended native page was not written", created)
	}
	changed := brainCall(t, cs, "append", map[string]any{"page": page, "section": "## Decisions", "text": "- Native reviewed decision.", "expected_page_revision": created["revision"]}, false)
	if err := cs.Close(); err != nil {
		t.Fatal(err)
	}
	cs = connect()
	read := brainCall(t, cs, "read", map[string]any{"page": page, "purpose": "Continue actual native authored page after restart"}, false)
	if read["revision"] != changed["revision"] || strings.Count(read["text"].(string), "Native reviewed decision.") != 1 {
		t.Fatal("native restart lost authored decision", read)
	}
	later := strings.Replace(read["text"].(string), "## Related", "- Native newer work.\n\n## Related", 1)
	intervene = func() error {
		return scheduler.WritePrivateFileAtomic(filepath.Join(workspacePath, "brain", filepath.FromSlash(page)), []byte(later))
	}
	brainCall(t, cs, "update", map[string]any{"page": page, "text": body, "expected_page_revision": read["revision"]}, true)
	after := brainCall(t, cs, "read", map[string]any{"page": page, "purpose": "Preserve intervening native work after stale review"}, false)
	if after["text"] != later {
		t.Fatal("native stale review overwrote newer prose", after)
	}
	// A hardlink leaf is not an authored page, even when its bytes are valid.
	outside := filepath.Join(root, "outside.md")
	if err := scheduler.WritePrivateFileAtomic(outside, []byte(body)); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(workspacePath, "brain", "alias.md")
	if err := os.Link(outside, alias); err != nil {
		t.Fatal("native hardlink fixture unavailable", err)
	}
	brainCall(t, cs, "read", map[string]any{"page": "alias.md", "purpose": "Refuse a hardlink alias"}, true)
	brainCall(t, cs, "create", map[string]any{"page": "alias.md", "text": authoredBrief(id, "alias.md", "Alias")}, true)
	actual, err := os.ReadFile(outside)
	if err != nil || string(actual) != body {
		t.Fatal("native alias touched outside bytes", err)
	}
	if !reflect.DeepEqual(managed, windowsCoreTree(t, product)) {
		t.Fatal("authored native journey changed managed product")
	}
	t.Log("native_authored_brain shipping_server=true attended_create=true restarted_read=true stale_preserves_newer=true declined_prepare_readonly=true hardlink_refused=true")

	t.Run("eager_registered_brain_identity", func(t *testing.T) {
		local := windowsPrivateRoot(t)
		registered, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(local, "workspace"), DataRoot: filepath.Join(local, "state")})
		if err != nil {
			t.Fatal(err)
		}
		engine, err := atlasops.OpenWorkspace(atlas.Options{DataRoot: registered.DataRoot, WorkspacePath: registered.WorkspacePath, WorkspaceID: registered.WorkspaceID})
		if err != nil {
			t.Fatal(err)
		}
		old := engine.Root() + "-old"
		if err = os.Rename(engine.Root(), old); err != nil {
			t.Fatal("native replace-directory fixture unavailable", err)
		}
		if err = scheduler.EnsurePrivateDirectory(engine.Root()); err != nil {
			t.Fatal(err)
		}
		original, err := scheduler.ReadPrivateFile(filepath.Join(old, "README.md"), 256<<10)
		if err != nil {
			t.Fatal(err)
		}
		if err = scheduler.WritePrivateFileAtomic(filepath.Join(engine.Root(), "README.md"), original); err != nil {
			t.Fatal(err)
		}
		if _, err = engine.ReviewPage(atlasops.CollectRequest{Purpose: "Refuse a replaced admitted brain", Reader: atlasops.ReaderOwnerSession, Pages: []string{"README.md"}}); err == nil {
			t.Fatal("lazy path identity accepted a replacement brain")
		}
		if _, err = engine.CreatePage(atlasops.CreatePageRequest{Page: "must-not-land.md", Body: "# Refused\n", Provenance: atlasops.Provenance{Origin: atlasops.OriginAttended, SessionID: "native-adversarial", IdempotencyKey: "refuse-replaced-brain"}}); err == nil {
			t.Fatal("replaced binding acquired new write authority")
		}
	})
}
