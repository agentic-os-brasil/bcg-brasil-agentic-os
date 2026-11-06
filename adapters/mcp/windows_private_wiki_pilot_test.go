//go:build windows

package product_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
)

// The ordinary-user driver supplies the same shipping executable used by Core.
// This journey starts with reviewed memory, never a fabricated wiki manifest.
func TestWindowsPrivateWikiPilot(t *testing.T) {
	windowsPrivateOrdinaryUser(t)
	for _, transport := range []string{"sdk", "compiled_stdio"} {
		t.Run(transport, func(t *testing.T) {
			base := windowsPrivateRoot(t)
			f := ownerFixture{options: product.Options{ProductRoot: filepath.Join(base, "product"), UserHome: filepath.Join(base, "owner"), WorkspacePath: filepath.Join(base, "workspace"), LookupEnv: func(string) (string, bool) { return "", false }}}
			for _, path := range []string{f.options.ProductRoot, f.options.UserHome} {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			shipping := os.Getenv("MAESTRO_WINDOWS_CORE_SERVER")
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
			defer cancel()
			args := []string{"init", "--product-root", f.options.ProductRoot, "--user-home", f.options.UserHome, "--workspace", f.options.WorkspacePath}
			if out, err := exec.CommandContext(ctx, shipping, args...).CombinedOutput(); err != nil {
				t.Fatalf("private wiki native setup: %s %v", out, err)
			}
			home, err := datahome.Resolve(datahome.Options{ProductRoot: f.options.ProductRoot, UserHome: f.options.UserHome, LookupEnv: f.options.LookupEnv})
			if err != nil {
				t.Fatal(err)
			}
			f.root = filepath.Join(home.Home, "brain")
			if transport == "compiled_stdio" {
				f.binary = shipping
			}
			managed := windowsCoreTree(t, f.options.ProductRoot)
			cs := f.connect(t, cadenceApprove)
			const marker = "WINDOWS_PRIVATE_WIKI reviewed synthetic navigation and source"
			capture := memoryCall(t, cs, "memory.capture", map[string]any{"text": marker}, false)
			day := time.Now().UTC().Format("2006-01-02")
			memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": day}, false)
			current := memoryCall(t, cs, "memory.context", nil, false)
			if !strings.Contains(current["text"].(string), marker) {
				t.Fatal("actual native committed source is absent")
			}
			qualified := cadenceCall(t, cs, "qualify", map[string]any{"job_id": "wiki-incremental-sync", "expected_revision": 0}, false)
			if qualified["qualification_state"] != "qualified" || qualified["state"] != "inactive" {
				t.Fatal("native private wiki did not compile through owning maintenance", qualified)
			}
			status := wikiCall(t, cs, "status", nil, false)
			generation, ok := status["generation"].(string)
			if !ok || generation == "" || status["state"] != "current" {
				t.Fatal("native graph is not current", status)
			}
			if err = cs.Close(); err != nil {
				t.Fatal(err)
			}
			cs = f.connect(t, cadenceApprove)
			restarted := wikiCall(t, cs, "status", nil, false)
			if restarted["generation"] != generation || restarted["state"] != "current" {
				t.Fatal("restarted native client lost the admitted graph", restarted)
			}
			index := wikiCall(t, cs, "read", map[string]any{"generation": generation, "reference": "index.md", "max_bytes": 4096}, false)["content"].(string)
			link := regexp.MustCompile(`\((concepts/[a-f0-9]{64}\.md)\)`).FindStringSubmatch(index)
			if len(link) != 2 {
				t.Fatal("native concept route absent")
			}
			concept := wikiCall(t, cs, "read", map[string]any{"generation": generation, "reference": link[1], "max_bytes": 4096}, false)["content"].(string)
			reference := regexp.MustCompile(`maestro://memory/[a-f0-9]{64}`).FindString(concept)
			if !strings.Contains(concept, marker) || reference == "" {
				t.Fatal("native concept lacks committed source navigation")
			}
			source := wikiCall(t, cs, "source", map[string]any{"generation": generation, "reference": reference, "max_bytes": 4096}, false)["content"].(string)
			backlinks := wikiCall(t, cs, "read", map[string]any{"generation": generation, "reference": "backlinks.json", "max_bytes": 4096}, false)["content"].(string)
			if !strings.Contains(source, marker) || !strings.Contains(backlinks, link[1]) {
				t.Fatal("native source/backlink readback absent")
			}
			if !reflect.DeepEqual(managed, windowsCoreTree(t, f.options.ProductRoot)) {
				t.Fatal("private graph modified managed product")
			}
			t.Log("native_private_wiki reviewed_capture=true committed_rollup=true owning_compilation=true same_graph_after_restart=true concept=true backlinks=true source_drilldown=true managed_preserved=true transport=" + transport)
			workspaceID := contentCall(t, cs, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
			if err = cs.Close(); err != nil {
				t.Fatal(err)
			}
			windowsPrivateWikiCustody(t, f, workspaceID, generation, link[1], marker, capture)
		})
	}
}
