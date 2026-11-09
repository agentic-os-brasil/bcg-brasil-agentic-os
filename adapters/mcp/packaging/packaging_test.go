package packaging_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	baseskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/skills"
	techcoreskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/tech-core/skills"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"testing"
)

func TestPackageRequiresAdmittedBinding(t *testing.T) {
	if packaging.BuildPlugin(packaging.Options{}) == nil {
		t.Fatal("unbound package admitted")
	}
}

func TestCompleteMethodPackets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("owner admission unavailable")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prod, home, ws := filepath.Join(base, "product"), filepath.Join(base, "owner"), filepath.Join(base, "workspace")
	for _, p := range []string{prod, home} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	lookup := func(string) (string, bool) { return "", false }
	h, err := datahome.InitializeFresh(datahome.Options{ProductRoot: prod, UserHome: home, LookupEnv: lookup})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: ws, DataRoot: filepath.Join(h.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	server := filepath.Join(base, "maestro-mcp")
	if output, err := exec.Command("go", "build", "-o", server, "../cmd/maestro-mcp").CombinedOutput(); err != nil {
		t.Fatalf("rebuild product server: %s: %v", output, err)
	}
	opts := packaging.Options{Destination: filepath.Join(base, "plugin"), Server: server, Version: "0.2.0", Binding: product.Options{ProductRoot: prod, UserHome: home, WorkspacePath: ws, LookupEnv: lookup}}
	if err := packaging.BuildPlugin(opts); err != nil {
		t.Fatal(err)
	}
	oracle, err := os.ReadFile("testdata/method-ids.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Fields(string(oracle))
	if len(want) != 92 {
		t.Fatal("Spec056 method oracle must contain 92 IDs")
	}
	entries, err := os.ReadDir(filepath.Join(opts.Destination, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		if e.Name() != "operator" && e.Name() != "resume" {
			got = append(got, e.Name())
		}
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("complete canonical methods missing: got %d IDs: %v", len(got), got)
	}
	indexBody, err := os.ReadFile(filepath.Join(opts.Destination, "methods-index.json"))
	var index struct {
		Methods []struct {
			ID string `json:"id"`
		} `json:"methods"`
	}
	if err != nil || json.Unmarshal(indexBody, &index) != nil {
		t.Fatal("canonical method index missing")
	}
	var indexIDs []string
	for _, m := range index.Methods {
		indexIDs = append(indexIDs, m.ID)
	}
	if !reflect.DeepEqual(indexIDs, want) {
		t.Fatal("canonical packet index differs from explicit method oracle")
	}
	var projection struct {
		HashScope string            `json:"hash_scope"`
		Files     map[string]string `json:"files"`
	}
	body, err := os.ReadFile(filepath.Join(opts.Destination, "methods-projection.json"))
	if err != nil || json.Unmarshal(body, &projection) != nil || projection.HashScope != "distributed_plugin_bytes" {
		t.Fatal("missing distributed integrity manifest")
	}
	for _, id := range want {
		files, err := baseskills.Files(id)
		if err != nil {
			files, err = techcoreskills.Files(id)
		}
		if err != nil {
			t.Fatal(err)
		}
		for path, canonical := range files {
			relative := "skills/" + id + "/" + path
			installed, err := os.ReadFile(filepath.Join(opts.Destination, filepath.FromSlash(relative)))
			if err != nil {
				t.Fatal(err)
			}
			if projection.Files[relative] != fmt.Sprintf("%x", sha256.Sum256(installed)) {
				t.Fatalf("distributed hash mismatch: %s", relative)
			}
			if !strings.HasSuffix(path, ".md") && !bytes.Equal(canonical, installed) {
				t.Fatalf("non-Markdown bytes changed: %s", relative)
			}
			if path == "SKILL.md" {
				end := bytes.Index(canonical[4:], []byte("\n---\n"))
				if end < 0 {
					t.Fatalf("missing frontmatter: %s", id)
				}
				end += 9
				if !bytes.Equal(canonical[:end], installed[:end]) {
					t.Fatalf("canonical metadata changed: %s", id)
				}
				if !bytes.Contains(installed, []byte("../../METHODS.md")) {
					t.Fatalf("method authority pointer missing: %s", id)
				}
				switch id {
				case "maestro-onboarding", "maestro-operator", "dream-memory", "maestro-runtime-checkup", "maestro-environment-setup", "maestro-doctor":
					// Preserve complete owner and maintenance recipes, removing only
					// the checked projection authority paragraph and invocation sigil.
					projected := installed[end:]
					authorityEnd := bytes.Index(projected, []byte("\n\n"))
					if authorityEnd < 0 {
						t.Fatalf("method authority paragraph incomplete: %s", id)
					}
					projected = projected[authorityEnd+1:]
					projected = bytes.ReplaceAll(projected, []byte("maestro-product:"), nil)
					expected := append([]byte(nil), canonical[end:]...)
					for _, methodID := range want {
						expected = bytes.ReplaceAll(expected, []byte("$"+methodID), []byte("/"+methodID))
					}
					if !bytes.Equal(projected, expected) {
						t.Fatalf("complete owner/maintenance recipe differs from canonical bytes: %s", id)
					}
				}
			}
		}
	}
	operator, err := os.ReadFile(filepath.Join(opts.Destination, "skills/operator/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"maestro://methods/maestro-onboarding/SKILL.md", "owner.status", "selection_required", "next_question", "review_required", "owner.onboarding_review", "owner.confirm_onboarding", "owner-onboarding-review/owner-confirm-onboarding"} {
		if !bytes.Contains(operator, []byte(required)) {
			t.Fatalf("operator alias lost resumable owner guidance: %s", required)
		}
	}
	provenanceBody, err := os.ReadFile(filepath.Join(opts.Destination, "pocock-upstream.json"))
	var provenance struct {
		Canonical map[string]any `json:"canonical"`
		Overlay   string         `json:"authority_overlay"`
	}
	var upstream map[string]any
	if err != nil || json.Unmarshal(provenanceBody, &provenance) != nil || json.Unmarshal(techcoreskills.Provenance(), &upstream) != nil || !reflect.DeepEqual(provenance.Canonical, upstream) || provenance.Overlay != "SKILL.md_pointer_to_packaged_METHODS.md" {
		t.Fatal("source provenance/projection authority changed")
	}
	tdd, err := os.ReadFile(filepath.Join(opts.Destination, "skills/pocock-tdd/SKILL.md"))
	if err != nil || !bytes.Contains(tdd, []byte("call the Skill tool with \"maestro-product:pocock-codebase-design\"")) {
		t.Fatal("plugin method invocation lost namespace")
	}
	caseSetup, err := os.ReadFile(filepath.Join(opts.Destination, "skills/case-agent-setup/SKILL.md"))
	if err != nil || !bytes.Contains(caseSetup, []byte("(`brain-index.py`)")) || bytes.Contains(caseSetup, []byte("maestro-product:brain-index.py")) {
		t.Error("index compiler filename changed during invocation projection")
	}
	for _, id := range []string{"yoda", "client-delivery-gate"} {
		method, err := os.ReadFile(filepath.Join(opts.Destination, "skills", id, "SKILL.md"))
		if err != nil || !bytes.Contains(method, []byte("subagent_type: \"yoda\"")) || bytes.Contains(method, []byte("subagent_type: \"maestro-product:yoda\"")) {
			t.Errorf("native Agent identity changed during Skill projection: %s", id)
		}
	}
	grill, err := os.ReadFile(filepath.Join(opts.Destination, "skills/pocock-grill-with-docs/SKILL.md"))
	if err != nil || !bytes.Contains(grill, []byte("Call the Skill tool twice, for \"maestro-product:pocock-grilling\" and \"maestro-product:pocock-domain-modeling\"")) {
		t.Error("multiple quoted Skill invocations lost namespace")
	}
	toTickets, err := os.ReadFile(filepath.Join(opts.Destination, "skills/pocock-to-tickets/SKILL.md"))
	if err != nil || !bytes.Contains(toTickets, []byte("`/maestro-product:pocock-setup-matt-pocock-skills`")) {
		t.Error("slash-command invocation lost namespace")
	}
	onboarding, err := os.ReadFile(filepath.Join(opts.Destination, "skills/maestro-onboarding/SKILL.md"))
	if err != nil || !bytes.Contains(onboarding, []byte("../interaction-profile/SKILL.md")) {
		t.Fatal("relative packaged reference damaged by namespace rewrite")
	}
	license, _ := os.ReadFile("../../../LICENSE.md")
	shipped, err := os.ReadFile(filepath.Join(opts.Destination, "LICENSE.md"))
	if err != nil || !bytes.Equal(license, shipped) {
		t.Fatal("canonical license missing")
	}
	var pluginConfig struct {
		Servers map[string]struct {
			Args []string `json:"args"`
		} `json:"mcpServers"`
	}
	body, err = os.ReadFile(filepath.Join(opts.Destination, ".mcp.json"))
	if err != nil || json.Unmarshal(body, &pluginConfig) != nil {
		t.Fatal("plugin server config missing")
	}
	wantArgs := []string{"serve", "--packet", "--product-root", "${CLAUDE_PLUGIN_ROOT}", "--user-home", "${user_config.user_home}", "--workspace", "${user_config.workspace}"}
	if !reflect.DeepEqual(pluginConfig.Servers["maestro"].Args, wantArgs) {
		t.Fatal("plugin runtime selection contract changed")
	}
	serverBytes, err := os.ReadFile(server)
	if err != nil {
		t.Fatal(err)
	}
	serverName := "maestro-mcp"
	if runtime.GOOS == "windows" {
		serverName += ".exe"
	}
	pluginServer, err := os.ReadFile(filepath.Join(opts.Destination, "scripts", serverName))
	if err != nil || !bytes.Equal(serverBytes, pluginServer) {
		t.Fatal("Code plugin server differs from selected shared executable")
	}
	if runtime.GOOS != "darwin" {
		return
	}
	opts.Destination = filepath.Join(base, "maestro.mcpb")
	if err := packaging.BuildExtension(opts); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(opts.Destination)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if len(archive.File) < 250 {
		t.Fatalf("MCPB managed payload is incomplete: %d", len(archive.File))
	}
	wantArgs[3] = "${__dirname}"
	for _, f := range archive.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		if f.Name == "manifest.json" {
			var manifest struct {
				Server struct {
					Config struct {
						Args []string `json:"args"`
					} `json:"mcp_config"`
				} `json:"server"`
			}
			if json.Unmarshal(content, &manifest) != nil || !reflect.DeepEqual(manifest.Server.Config.Args, wantArgs) {
				t.Fatal("MCPB binding changed")
			}
		} else if f.Name == "server/maestro-mcp" {
			binary, err := os.ReadFile(server)
			if err != nil || !bytes.Equal(binary, content) {
				t.Fatal("MCPB server differs from selected shared executable")
			}
		}
	}
}
