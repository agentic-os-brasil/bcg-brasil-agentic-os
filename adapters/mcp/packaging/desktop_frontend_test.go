package packaging

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"
)

func TestDesktopMethodProjectionUsesSeparateNamespace(t *testing.T) {
	files, err := methodPayload(claudeagents.DesktopMethodNamespace)
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string][]string{
		"pocock-tdd":             {"Skill tool with \"maestro-desktop-methods:pocock-codebase-design\""},
		"pocock-grill-with-docs": {"\"maestro-desktop-methods:pocock-grilling\" and \"maestro-desktop-methods:pocock-domain-modeling\""},
		"pocock-to-tickets":      {"/maestro-desktop-methods:pocock-setup-matt-pocock-skills"},
		"yoda":                   {"subagent_type: \"maestro-desktop-methods:yoda\""},
		"pocock-code-review":     {"Agent(subagent_type=\"maestro-desktop-methods:repository-spec-reviewer\"", "Agent(subagent_type=\"maestro-desktop-methods:repository-standards-reviewer\""},
		"case-agent-setup":       {"`brain-index.py`"},
		"maestro-onboarding":     {"../interaction-profile/SKILL.md"},
	}
	for id, required := range checks {
		for _, want := range required {
			if !bytes.Contains(files["skills/"+id+"/SKILL.md"], []byte(want)) {
				t.Errorf("%s missing %s", id, want)
			}
		}
	}
	canonical, err := product.PackagedMethods()
	if err != nil {
		t.Fatal(err)
	}
	var projection struct {
		Namespace string            `json:"namespace"`
		HashScope string            `json:"hash_scope"`
		Files     map[string]string `json:"files"`
	}
	if err := json.Unmarshal(files["methods-projection.json"], &projection); err != nil {
		t.Fatal(err)
	}
	if projection.Namespace != claudeagents.DesktopMethodNamespace || projection.HashScope != "distributed_plugin_bytes" {
		t.Fatal("wrong projection identity/hash scope")
	}
	for _, method := range canonical.Methods {
		for name, body := range method.Files {
			path := "skills/" + method.ID + "/" + name
			rendered, ok := files[path]
			if !ok {
				t.Fatal("missing", path)
			}
			if projection.Files[path] != fmt.Sprintf("%x", sha256.Sum256(rendered)) {
				t.Fatal("wrong projection hash", path)
			}
			if !strings.HasSuffix(name, ".md") && !bytes.Equal(body, rendered) {
				t.Fatal("non-Markdown source changed", path)
			}
			if strings.HasSuffix(name, ".md") && bytes.Contains(rendered, []byte("maestro-product:")) {
				t.Fatal("cross-plugin invocation", path)
			}
			if name == "SKILL.md" {
				end := bytes.Index(body[4:], []byte("\n---\n")) + 9
				if end < 9 || !bytes.Equal(body[:end], rendered[:end]) {
					t.Fatal("frontmatter changed", path)
				}
				for _, uri := range []string{"maestro://methods/METHODS.md", "maestro://methods/agent-skill-policy.json", "next_offset", "has_more"} {
					if !bytes.Contains(rendered, []byte(uri)) {
						t.Fatal("authority preflight lost", path, uri)
					}
				}
			}
		}
	}
	for _, name := range []string{"METHODS.md", "agent-skill-policy.json", "method-contracts.json", "methods-index.json", "LICENSE.md"} {
		if !bytes.Equal(files[name], canonical.Assets[name]) {
			t.Fatal("canonical metadata changed", name)
		}
	}
	var provenance struct {
		Canonical json.RawMessage `json:"canonical"`
		Namespace string          `json:"namespace"`
	}
	if err := json.Unmarshal(files["pocock-upstream.json"], &provenance); err != nil {
		t.Fatal(err)
	}
	var got, want any
	if json.Unmarshal(provenance.Canonical, &got) != nil || json.Unmarshal(canonical.Assets["pocock-upstream.json"], &want) != nil || !reflect.DeepEqual(got, want) || provenance.Namespace != claudeagents.DesktopMethodNamespace {
		t.Fatal("upstream provenance lost")
	}
	if _, err := methodPayload("unadmitted-frontend"); err == nil {
		t.Fatal("arbitrary namespace accepted")
	}
}

func TestCodeMethodPayloadBaseline(t *testing.T) {
	files, err := methodPayload("maestro-product")
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(files)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(body)); got != "f7c513677b0d8023eebc8ab4356aa306db47e3ec03759cdaf0579fa11839aab0" {
		t.Fatal("Code method bytes changed", got)
	}
	agents, err := claudeagents.Project("maestro-product")
	if err != nil {
		t.Fatal(err)
	}
	body, err = json.Marshal(agents)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(body)); got != "2480196c75ac276f3118fd7ad3b8418d495cdfb6ba1903d7c2aa1f52ac4279c2" {
		t.Fatal("Code native bytes changed", got)
	}
}

func TestBuildDesktopMethodFrontendClosedDeterministicOutput(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var priorArchive, priorInventory []byte
	for _, dest := range []string{"first", "second"} {
		root := filepath.Join(parent, dest)
		report, err := BuildDesktopMethodFrontend(DesktopFrontendOptions{Destination: root, Version: "0.1.48"})
		if err != nil {
			t.Fatal(err)
		}
		if report.Name != "maestro-desktop-methods" || report.Name == claudeagents.PluginNamespace || report.Mode != "instructions_only" || report.NativeQualification != "unavailable" || report.FileCount != 273 {
			t.Fatalf("wrong identity/scope: %+v", report)
		}
		payload := filepath.Join(root, report.Name)
		for _, name := range []string{".mcp.json", "hooks", "scripts", "agents", "bin", "managed/runtime", "managed/memory", "maestro-packet.json", "packet-inventory.json"} {
			if _, err := os.Lstat(filepath.Join(payload, name)); !os.IsNotExist(err) {
				t.Fatal("active/private carrier included", name, err)
			}
		}
		if managedNativePluginID(report.Name + "@synced") {
			t.Fatal("instruction frontend admitted as native plugin")
		}
		raw, err := os.ReadFile(filepath.Join(payload, ".claude-plugin/plugin.json"))
		if err != nil {
			t.Fatal(err)
		}
		var manifest map[string]any
		if json.Unmarshal(raw, &manifest) != nil || len(manifest) != 4 || manifest["name"] != report.Name || manifest["version"] != report.Version {
			t.Fatalf("wrong public manifest: %s", raw)
		}
		invBody, err := os.ReadFile(filepath.Join(root, "inventory.json"))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(invBody)) != report.InventorySHA256 {
			t.Fatal("wrong inventory hash")
		}
		var inv []DesktopFrontendFile
		if json.Unmarshal(invBody, &inv) != nil || len(inv) != report.FileCount {
			t.Fatal("wrong inventory")
		}
		zipBody, err := os.ReadFile(filepath.Join(root, report.Archive))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(zipBody)) != report.ArchiveSHA256 {
			t.Fatal("wrong archive hash")
		}
		z, err := zip.NewReader(bytes.NewReader(zipBody), int64(len(zipBody)))
		if err != nil {
			t.Fatal(err)
		}
		if len(z.File) != len(inv) {
			t.Fatal("archive not closed against inventory")
		}
		total := 0
		skillDescriptions := 0
		publicFiles := map[string][]byte{}
		for i, row := range inv {
			if i > 0 && inv[i-1].Path >= row.Path {
				t.Fatal("unsorted/duplicate inventory")
			}
			body, err := os.ReadFile(filepath.Join(payload, filepath.FromSlash(row.Path)))
			if err != nil {
				t.Fatal(err)
			}
			if len(body) != row.Bytes || fmt.Sprintf("%x", sha256.Sum256(body)) != row.SHA256 {
				t.Fatal("wrong payload hash", row.Path)
			}
			file := z.File[i]
			if file.Name != report.Name+"/"+row.Path || file.Mode().Perm() != 0644 {
				t.Fatal("wrong archive entry", row.Path)
			}
			in, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			packed, readErr := io.ReadAll(in)
			closeErr := in.Close()
			if readErr != nil || closeErr != nil || !bytes.Equal(packed, body) {
				t.Fatal("archive bytes differ", row.Path)
			}
			if strings.HasPrefix(row.Path, "skills/") && strings.HasSuffix(row.Path, "/SKILL.md") {
				skillDescriptions++
				if !bytes.HasPrefix(packed, []byte("---\n")) {
					t.Fatal("skill frontmatter missing", row.Path)
				}
				end := bytes.Index(packed[4:], []byte("\n---\n"))
				if end < 0 {
					t.Fatal("skill frontmatter incomplete", row.Path)
				}
				var metadata struct {
					Description string `yaml:"description"`
				}
				if err := yaml.Unmarshal(packed[4:4+end], &metadata); err != nil || metadata.Description == "" {
					t.Fatal("skill description unavailable", row.Path, err)
				}
				if strings.ContainsAny(metadata.Description, "<>") {
					t.Errorf("Desktop skill description contains XML markup: %s: %s", row.Path, metadata.Description)
				}
			}
			publicFiles[row.Path] = body
			total += len(body)
		}
		t.Logf("checked %d emitted skill descriptions in %s archive", skillDescriptions, dest)
		if skillDescriptions != 94 {
			t.Fatalf("wrong emitted skill description count: %d", skillDescriptions)
		}
		assertPublicMethodPrivacy(t, publicFiles)
		if total != report.PayloadBytes {
			t.Fatal("wrong payload size")
		}
		readme, err := os.ReadFile(filepath.Join(payload, "README.md"))
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{"protected changes require successful review", "Backend command-line conversion or tests do not", "no duplicate MCP connector", "different capabilities"} {
			if !bytes.Contains(readme, []byte(text)) {
				t.Fatal("README lost qualification limit", text)
			}
		}
		if priorArchive != nil && (!bytes.Equal(priorArchive, zipBody) || !bytes.Equal(priorInventory, invBody)) {
			t.Fatal("nondeterministic frontend")
		}
		priorArchive, priorInventory = zipBody, invBody
		if _, err := BuildDesktopMethodFrontend(DesktopFrontendOptions{Destination: root, Version: "0.1.48"}); err == nil {
			t.Fatal("existing output overwritten")
		}
		after, _ := os.ReadFile(filepath.Join(root, report.Archive))
		if !bytes.Equal(after, zipBody) {
			t.Fatal("existing output changed")
		}
	}
}

func TestDesktopFrontendRejectsInvalidSelectionWithoutOutput(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"", "01.2.3", "1.2.3-extra", "../1.2.3"} {
		dest := filepath.Join(root, "output")
		if _, err := BuildDesktopMethodFrontend(DesktopFrontendOptions{Destination: dest, Version: version}); err == nil {
			t.Fatal("invalid version accepted", version)
		}
		if _, err := os.Lstat(dest); !os.IsNotExist(err) {
			t.Fatal("invalid request wrote output")
		}
	}
	if _, err := BuildDesktopMethodFrontend(DesktopFrontendOptions{Destination: "relative", Version: "1.2.3"}); err == nil {
		t.Fatal("relative output accepted")
	}
}

func TestDesktopFrontendLeavesMCPBMethodsCanonical(t *testing.T) {
	canonical, err := product.PackagedMethods()
	if err != nil {
		t.Fatal(err)
	}
	files, err := methodPayload("")
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string][]byte{}
	for name, body := range canonical.Assets {
		expected[name] = body
	}
	for _, method := range canonical.Methods {
		for name, body := range method.Files {
			expected["skills/"+method.ID+"/"+name] = body
		}
	}
	if !reflect.DeepEqual(files, expected) {
		t.Fatal("default/MCPB method payload differs from canonical bytes")
	}
}
