package product_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// These tests exercise the public product journey, not the owning converter's
// fixture/assembly matrix. Retained signed inputs are copied, never provisioned.
func policyOwner(t *testing.T, base string) ownerFixture {
	t.Helper()
	f := ownerFixture{options: product.Options{ProductRoot: filepath.Join(base, "product"), UserHome: filepath.Join(base, "owner"), WorkspacePath: filepath.Join(base, "workspace"), LookupEnv: func(string) (string, bool) { return "", false }}}
	for _, name := range []string{f.options.ProductRoot, f.options.UserHome} {
		if err := os.MkdirAll(name, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(f.options.ProductRoot, 0500); err != nil {
		t.Fatal(err)
	}
	h, err := datahome.InitializeFresh(datahome.Options{ProductRoot: f.options.ProductRoot, UserHome: f.options.UserHome, LookupEnv: f.options.LookupEnv})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = workspace.Initialize(workspace.Options{WorkspacePath: f.options.WorkspacePath, DataRoot: filepath.Join(h.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	f.root = filepath.Join(h.Home, "brain")
	return f
}

func policyCopyPack(t *testing.T, retained string, f *ownerFixture, installed bool) string {
	t.Helper()
	source := filepath.Join(retained, "state/ingestion/docling")
	target := filepath.Join(filepath.Dir(f.root), "state/ingestion/docling")
	var manifest releasecontract.Manifest
	if err := json.Unmarshal(readContentFile(t, filepath.Join(source, "release-manifest.json")), &manifest); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"release-manifest.json", "release-manifest.json.sig"} {
		putContentFile(t, filepath.Join(target, name), readContentFile(t, filepath.Join(source, name)))
	}
	for _, artifact := range manifest.Artifacts {
		if artifact.Kind != "runtime_pack" || artifact.OS != runtime.GOOS || artifact.Arch != runtime.GOARCH {
			continue
		}
		for _, name := range []string{artifact.Name, artifact.SignatureRef} {
			putContentFile(t, filepath.Join(target, name), readContentFile(t, filepath.Join(source, name)))
		}
		if installed {
			copyContentFixture(t, filepath.Join(source, "installed", artifact.SHA256), filepath.Join(target, "installed", artifact.SHA256))
		}
		return artifact.SHA256
	}
	t.Fatal("no retained platform runtime")
	return ""
}

func policySeed(t *testing.T, root, seed string) []byte {
	t.Helper()
	authority := readContentFile(t, filepath.Join(root, "authority.json"))
	if seed == "" || fmt.Sprintf("%x", sha256.Sum256(authority)) != seed {
		t.Fatal("independent retained engineering authority seed mismatch")
	}
	return authority
}

func policySave(t *testing.T, directory, name string, value any) {
	t.Helper()
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	putContentFile(t, filepath.Join(directory, name+".json"), body)
}

func policySyntheticSources(t *testing.T, directory string) map[string][]byte {
	t.Helper()
	sources := map[string][]byte{}
	fixtures := "../../internal/ingest/docling/testdata"
	sources["sample.docx"] = readContentFile(t, filepath.Join(fixtures, "sample.docx"))
	for _, name := range []string{"quoted.csv", "spine.epub", "page.htm", "page.html", "generic.json", "structured.md", "literal.txt", "legacy.xls", "sheets.xlsx", "generic.xml", "empty.xlsx"} {
		sources[name] = readContentFile(t, filepath.Join(fixtures, "policy", name))
	}
	// Reserve this source; only this transport's freshly converted DOCX document
	// may populate it. The retained pack's incidental workspace is never read.
	sources["export.json"] = nil
	// A valid local image outside the selected workspace preserves the no-fetch
	// oracle; a missing placeholder file would not prove that bytes were ignored.
	bait := image.NewRGBA(image.Rect(0, 0, 1, 1))
	bait.Set(0, 0, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, bait); err != nil {
		t.Fatal(err)
	}
	baitPath := filepath.Join(directory, "external-bait.png")
	putContentFile(t, baitPath, encoded.Bytes())
	for _, name := range []string{"page.htm", "page.html", "structured.md"} {
		sources[name] = bytes.ReplaceAll(sources[name], []byte("file:///MAESTRO_POLICY_BAIT.svg"), []byte("file://"+baitPath))
	}
	return sources
}

func TestContentPolicyFixtureHandoff(t *testing.T) {
	// TestAuthenticatedRealDocling leaves only this source in its workspace.
	clean := physicalTempDir(t)
	putContentFile(t, filepath.Join(clean, "workspace/sample.docx"), readContentFile(t, "../../internal/ingest/docling/testdata/sample.docx"))
	directory := filepath.Join(clean, "public-sources")
	sources := policySyntheticSources(t, directory)
	want := []string{"quoted.csv", "sample.docx", "spine.epub", "page.htm", "page.html", "generic.json", "structured.md", "literal.txt", "legacy.xls", "sheets.xlsx", "generic.xml", "export.json", "empty.xlsx"}
	if len(sources) != len(want) {
		t.Fatalf("finite public source set: got %d want %d", len(sources), len(want))
	}
	for _, name := range want {
		body, exists := sources[name]
		if !exists || name != "export.json" && len(body) == 0 {
			t.Fatalf("clean producer cannot supply public source %s", name)
		}
	}
	if sources["export.json"] != nil {
		t.Fatal("export must come from this transport's actual public DOCX conversion")
	}
	entries, err := os.ReadDir(filepath.Join(clean, "workspace"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "sample.docx" {
		t.Fatal("fixture preparation changed the clean producer workspace", err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(readContentFile(t, filepath.Join(directory, "external-bait.png"))))
	if err != nil || config.Width != 1 || config.Height != 1 {
		t.Fatal("local no-fetch bait is not a valid synthetic image", err)
	}
	for _, name := range []string{"page.htm", "page.html", "structured.md"} {
		if !bytes.Contains(sources[name], []byte("file://"+filepath.Join(directory, "external-bait.png"))) || bytes.Contains(sources[name], []byte("MAESTRO_POLICY_BAIT.svg")) {
			t.Fatal("valid local no-fetch bait missing", name)
		}
	}
	if !bytes.Equal(sources["literal.txt"], readContentFile(t, "../../internal/ingest/docling/testdata/policy/literal.txt")) {
		t.Fatal("literal whitespace fixture changed")
	}
	if evidence := os.Getenv("MAESTRO_POLICY_C1_EVIDENCE"); evidence != "" {
		manifest := map[string]any{}
		for name, body := range sources {
			manifest[name] = map[string]any{"bytes": len(body), "sha256": fmt.Sprintf("%x", sha256.Sum256(body))}
		}
		manifest["export.json"] = map[string]any{"state": "pending_actual_public_docx_conversion"}
		policySave(t, evidence, "e6-policy-task2-c1-fixture-handoff", map[string]any{"producer_workspace_sources": []string{"sample.docx"}, "sources": manifest, "valid_local_png": true})
	}
}

func policyRead(t *testing.T, cs *mcp.ClientSession, id, format string, limit int) ([]byte, map[string]any) {
	t.Helper()
	var body []byte
	var result map[string]any
	for offset := 0; ; {
		result = contentCall(t, cs, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": id, "format": format, "offset": offset, "max_bytes": limit}, false)
		part := []byte(result["text"].(string))
		next := int(result["next_offset"].(float64))
		if len(part) > limit || !utf8.Valid(part) || next != offset+len(part) || result["has_more"] == true && next <= offset {
			t.Fatal("artifact pagination is not bounded UTF-8 progress", result)
		}
		body = append(body, part...)
		if len(body) > 100<<20 {
			t.Fatal("artifact exceeded canonical output budget")
		}
		if result["has_more"] == false {
			return body, result
		}
		offset = next
	}
}

type policyPublicDocument struct {
	Schema   string `json:"schema_name"`
	Texts    []struct{ Text, Orig, Label string }
	Groups   []struct{ Name, Label string }
	Pictures []struct{ Image json.RawMessage }
	Tables   []struct {
		Data struct {
			Rows  int `json:"num_rows"`
			Cols  int `json:"num_cols"`
			Cells []struct {
				Text   string
				Row    int `json:"start_row_offset_idx"`
				Col    int `json:"start_col_offset_idx"`
				EndCol int `json:"end_col_offset_idx"`
			} `json:"table_cells"`
		}
	}
}

func policyContentOracle(t *testing.T, extension string, source, markdown, document []byte, result map[string]any) {
	t.Helper()
	var d policyPublicDocument
	if err := json.Unmarshal(document, &d); err != nil || d.Schema != "DoclingDocument" {
		t.Fatal("public document is not a DoclingDocument", err)
	}
	for _, picture := range d.Pictures {
		if len(picture.Image) > 0 && string(picture.Image) != "null" {
			t.Fatal("local/remote image bait was fetched")
		}
	}
	cell := func(table, row, col int, want string) {
		t.Helper()
		if table < len(d.Tables) {
			for _, c := range d.Tables[table].Data.Cells {
				if c.Row == row && c.Col == col && c.Text == want {
					return
				}
			}
		}
		t.Fatalf("independent cell oracle: table=%d row=%d col=%d want=%q", table, row, col, want)
	}
	markers := func(want ...string) {
		t.Helper()
		for _, marker := range want {
			if !bytes.Contains(markdown, []byte(marker)) {
				t.Fatalf("public Markdown lacks %q: %s", marker, markdown)
			}
		}
	}
	if extension == ".txt" || extension == ".json" || extension == ".xml" {
		if len(d.Texts) != 1 || d.Texts[0].Text != string(source) || d.Texts[0].Orig != string(source) || d.Texts[0].Label != "code" || result["route"] != "maestro_literal_v1" || result["parser"] != "maestro.literal_utf8.v1" || result["fidelity"] != "textual" || result["fallback_reason"] != "native_generic_literal_unsupported" {
			t.Fatal("literal source or fidelity changed", result)
		}
		markers(strings.TrimSpace(string(source)))
		return
	}
	wantParser := map[string]string{".docx": "docling.docx", ".csv": "docling.csv", ".html": "docling.html", ".htm": "docling.html", ".md": "docling.markdown", ".epub": "docling.epub", ".xlsx": "docling.xlsx", "export": "docling.json", ".xls": "xlrd.biff"}[extension]
	if result["parser"] != wantParser {
		t.Fatal("actual parser missing", result)
	}
	if extension == ".xls" {
		if result["route"] != "maestro_biff_v1" || result["fidelity"] != "textual" || result["fallback_reason"] != "native_xls_requires_unmanaged_office" {
			t.Fatal("BIFF fidelity misrepresented", result)
		}
	} else if result["route"] != "docling_local" || result["fidelity"] != "structured" || result["fallback_reason"] != nil {
		t.Fatal("native route misrepresented", result)
	}
	switch extension {
	case ".docx", "export":
		if len(d.Tables) != 1 || d.Tables[0].Data.Rows != 2 || d.Tables[0].Data.Cols != 2 {
			t.Fatal("DOCX table shape")
		}
		cell(0, 0, 0, "Item")
		cell(0, 0, 1, "Count")
		cell(0, 1, 0, "alpha")
		cell(0, 1, 1, "7")
		markers("alpha", "7")
	case ".csv":
		cell(0, 1, 0, "001")
		cell(0, 1, 1, "comma, value")
		cell(0, 1, 2, "line one\nline two")
		cell(0, 2, 1, `say "hello"`)
		markers("001", "comma, value", "line one", "line two")
	case ".html", ".htm":
		cell(0, 0, 0, "Item")
		cell(0, 1, 0, "beta")
		cell(0, 1, 1, "13")
		markers("Policy HTML", "Ação local", "beta", "13")
	case ".md":
		cell(0, 1, 0, "gamma")
		cell(0, 1, 1, "17")
		found := false
		for _, text := range d.Texts {
			if text.Label == "code" && strings.Contains(text.Text, "<tag>literal_code</tag>") {
				found = true
			}
		}
		if !found {
			t.Fatal("literal fenced code lost")
		}
		markers("Policy Markdown", "first", "second", "gamma", "17", "<tag>literal_code</tag>")
	case ".epub":
		var text string
		for _, v := range d.Texts {
			text += v.Text + "\n"
		}
		if strings.Index(text, "BEFORE_SPINE") < 0 || strings.Index(text, "AFTER_SPINE") < strings.Index(text, "BEFORE_SPINE") || bytes.Index(markdown, []byte("BEFORE")) > bytes.Index(markdown, []byte("AFTER")) {
			t.Fatal("EPUB spine order lost")
		}
		markers("First chapter", "Second chapter", "BEFORE", "AFTER")
	case ".xlsx":
		if len(d.Groups) != 2 || d.Groups[0].Name != "Metrics" || d.Groups[1].Name != "Details" || d.Groups[0].Label != "sheet" {
			t.Fatal("XLSX sheet identity/order")
		}
		cell(0, 1, 0, "delta")
		cell(0, 1, 1, "23")
		cell(0, 1, 2, "29")
		cell(0, 2, 0, "True")
		cell(1, 1, 0, "0007")
		cell(1, 1, 1, "3.5")
		merged := false
		for _, c := range d.Tables[0].Data.Cells {
			if c.Text == "Merged heading" && c.Col == 0 && c.EndCol == 2 {
				merged = true
			}
		}
		if !merged || bytes.Contains(markdown, []byte("100")) {
			t.Fatal("XLSX merged span or cached formula semantics")
		}
		markers("Metrics", "Details", "delta", "29", "0007")
	case ".xls":
		cell(0, 1, 0, "0009")
		cell(0, 1, 1, "31")
		cell(0, 1, 2, "2024-02-29T00:00:00")
		cell(0, 1, 3, "37")
		cell(0, 1, 4, "")
		cell(1, 1, 1, "4.25")
		markers("Legacy", "Notes", "0009", "31", "2024-02-29", "37", "legacy-note")
		if bytes.Index(markdown, []byte("Legacy")) > bytes.Index(markdown, []byte("Notes")) {
			t.Fatal("BIFF sheet order")
		}
	}
}

func policyResources(t *testing.T, cs *mcp.ClientSession) {
	t.Helper()
	for _, uri := range []string{"maestro://methods/catalog.json", "maestro://methods/LICENSE.md", "maestro://methods/agent-skill-policy.json", "maestro://methods/pocock-upstream.json", "maestro://methods/ingest-content/SKILL.md"} {
		response, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
		if err != nil || len(response.Contents) != 1 || response.Contents[0].Text == "" {
			t.Fatal("packaged resource unavailable", uri, err)
		}
		if strings.HasSuffix(uri, "ingest-content/SKILL.md") && (!strings.Contains(response.Contents[0].Text, "formats") || !strings.Contains(response.Contents[0].Text, "BIFF")) {
			t.Fatal("installed PFMT recipe absent")
		}
	}
}

func TestContentPolicyRealJourneys(t *testing.T) {
	retained, seed := os.Getenv("MAESTRO_DOCLING_RETAINED_ROOT"), os.Getenv("MAESTRO_DOCLING_RETAINED_SEED")
	if retained == "" || seed == "" {
		t.Skip("explicit retained current engineering runtime and independent authority seed required")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("current retained profile requires darwin/arm64")
	}
	authority := policySeed(t, retained, seed)
	base := os.Getenv("MAESTRO_POLICY_PUBLIC_ROOT")
	if base == "" {
		base = physicalTempDir(t)
	} else if err := os.Mkdir(base, 0700); err != nil {
		t.Fatal("public proof destination must be new", err)
	}
	binary := filepath.Join(base, "server/maestro-mcp")
	if err := os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-mod=readonly", "-ldflags", "-X main.AuthorityRegistrySHA256="+seed, "-o", binary, "./cmd/maestro-mcp")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("current production server build: %s %v", out, err)
	}
	putContentFile(t, filepath.Join(base, "managed/trust/release-authority-registry.json"), authority)
	policySave(t, base, "server-identity", map[string]any{"sha256": fmt.Sprintf("%x", sha256.Sum256(readContentFile(t, binary))), "registry_sha256": seed, "engineering_only": true})
	for _, mode := range []string{"sdk", "compiled_stdio"} {
		t.Run(mode, func(t *testing.T) {
			modeRoot := filepath.Join(base, mode)
			f := policyOwner(t, filepath.Join(modeRoot, "binding"))
			packSHA := policyCopyPack(t, retained, &f, true)
			f.options.AuthorityRegistryPath = filepath.Join(base, "managed/trust/release-authority-registry.json")
			f.options.AuthorityRegistrySHA256, f.options.CLIVersion = seed, "0.1.0"
			packetRoot := filepath.Join(modeRoot, "code-plugin")
			if mode == "compiled_stdio" {
				if err := packaging.BuildPlugin(packaging.Options{Destination: packetRoot, Server: binary, Version: "0.1.0", Binding: f.options}); err != nil {
					t.Fatal(err)
				}
				f.binary = filepath.Join(packetRoot, "scripts/maestro-mcp")
			}
			var packetBefore map[string]string
			if f.binary != "" {
				packetBefore = contentTree(t, packetRoot, true)
			}
			cs := connectContent(t, f, nil, "")
			status := contentCall(t, cs, "maestro_query", map[string]any{"operation": "content.status"}, false)
			if status["state"] != "ready" || status["pack_sha256"] != packSHA {
				t.Fatal("current verified profile unavailable", status)
			}
			assertPolicyCoverage(t, status, "ready", "")
			policySave(t, modeRoot, "status", status)
			policyResources(t, cs)
			cs.Close()
			sources := policySyntheticSources(t, modeRoot)
			fixtures := []struct{ extension, name string }{{".csv", "quoted.csv"}, {".docx", "sample.docx"}, {".epub", "spine.epub"}, {".htm", "page.htm"}, {".html", "page.html"}, {".json", "generic.json"}, {".md", "structured.md"}, {".txt", "literal.txt"}, {".xls", "legacy.xls"}, {".xlsx", "sheets.xlsx"}, {".xml", "generic.xml"}, {"export", "export.json"}}
			var firstID string
			var firstSource []byte
			var exportOrigin map[string]any
			for _, fixture := range fixtures {
				name := strings.TrimPrefix(fixture.extension, ".")
				if name == "export" {
					name = "validated_export"
				}
				t.Run(name, func(t *testing.T) {
					source := sources[fixture.name]
					if len(source) == 0 {
						t.Fatal("source requires this transport's successful public DOCX conversion", fixture.name)
					}
					if fixture.extension == "export" && (exportOrigin == nil || exportOrigin["document_sha256"] != fmt.Sprintf("%x", sha256.Sum256(source)) || exportOrigin["pack_sha256"] != packSHA) {
						t.Fatal("export source lost its actual public DOCX provenance")
					}
					putContentFile(t, filepath.Join(f.options.WorkspacePath, fixture.name), source)
					evidence := filepath.Join(modeRoot, "evidence", name)
					var expected map[string]any
					reviews := 0
					host := func(_ context.Context, request *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
						start := strings.Index(request.Params.Message, "\n{")
						var shown map[string]any
						if start < 0 || json.Unmarshal([]byte(request.Params.Message[start+1:]), &shown) != nil || !reflect.DeepEqual(shown, expected) {
							t.Errorf("trusted review differs from exact intended domain content: %v", shown)
						}
						if strings.Contains(request.Params.Message, f.options.UserHome) || strings.Contains(request.Params.Message, "session_id") {
							t.Error("private review proof/root leaked")
						}
						policySave(t, evidence, fmt.Sprintf("review-%d", reviews), shown)
						reviews++
						return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
					}
					options := &mcp.ClientOptions{ElicitationHandler: host}
					client := connectContent(t, f, options, "")
					started := time.Now()
					converted := contentCall(t, client, "maestro_apply", map[string]any{"operation": "content.convert", "source_path": fixture.name}, false)
					policySave(t, evidence, "conversion", converted)
					receipt := converted["result"].(map[string]any)
					provenance := converted["provenance"].(map[string]any)
					sourceSHA := fmt.Sprintf("%x", sha256.Sum256(source))
					if receipt["status"] != "usable" || receipt["source_sha256"] != sourceSHA || provenance["pack_sha256"] != packSHA || provenance["network_denied"] != true {
						t.Fatal("actual public conversion failed", converted)
					}
					id := receipt["artifact_ref"].(string)
					markdown, last := policyRead(t, client, id, "markdown", 4096)
					document, structured := policyRead(t, client, id, "document", 4096)
					policySave(t, evidence, "markdown-read", last)
					policySave(t, evidence, "document-read", structured)
					// Artifact IDs address the published envelope, so its stored result
					// cannot contain its own content-derived ID. Readback names it separately.
					storedReceipt := map[string]any{}
					for key, value := range receipt {
						if key != "artifact_ref" {
							storedReceipt[key] = value
						}
					}
					if last["artifact_id"] != id || structured["artifact_id"] != id || !reflect.DeepEqual(storedReceipt, last["result"]) || !reflect.DeepEqual(storedReceipt, structured["result"]) || !reflect.DeepEqual(provenance, last["provenance"]) || !reflect.DeepEqual(provenance, structured["provenance"]) {
						t.Fatal("immutable artifact lost actual route/provenance")
					}
					if fmt.Sprintf("%x", sha256.Sum256(markdown)) != provenance["markdown_sha256"] || fmt.Sprintf("%x", sha256.Sum256(document)) != provenance["document_sha256"] {
						t.Fatal("public output hash mismatch")
					}
					policyContentOracle(t, fixture.extension, source, markdown, document, receipt)
					putContentFile(t, filepath.Join(evidence, "document.json"), document)
					putContentFile(t, filepath.Join(evidence, "markdown.md"), markdown)
					page := "knowledge/policy/" + name + ".md"
					draft := fmt.Sprintf("# Reviewed policy source\n\nSource: %s\nSource SHA256: %s\nArtifact: %s\nPack SHA256: %s\nRoute: %s\nParser: %s\nFidelity: %s\n\n%s\n\n## Review\n", fixture.name, sourceSHA, id, packSHA, receipt["route"], receipt["parser"], receipt["fidelity"], strings.TrimRight(string(markdown), "\r\n"))
					if fixture.extension == ".txt" {
						// Exercise a real multibyte split without paging every format one rune at a time.
						offset := bytes.Index(markdown, []byte("ç"))
						if offset < 0 {
							t.Fatal("UTF-8 fixture marker absent")
						}
						part := contentCall(t, client, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": id, "format": "markdown", "offset": offset, "max_bytes": 4}, false)
						text := part["text"].(string)
						if len(text) > 4 || !utf8.ValidString(text) || !strings.HasPrefix(text, "ç") {
							t.Fatal("multibyte slice invalid", part)
						}
						contentCall(t, client, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": id, "format": "markdown", "offset": offset + 1, "max_bytes": 4}, true)
						policySave(t, evidence, "utf8-slice", part)
					}
					metadata := contentCall(t, client, "maestro_context", map[string]any{}, false)
					expected = map[string]any{"Operation": "knowledge.create", "Workspace": metadata["workspace_id"], "Source document": fixture.name, "Source SHA-256": sourceSHA, "Converted artifact": id, "Runtime pack SHA-256": packSHA, "Page": page, "Section": "", "Current page": "Page does not exist", "Exact proposed text": draft, "Expected page revision": ""}
					written := contentCall(t, client, "maestro_apply", map[string]any{"operation": "knowledge.create", "artifact_id": id, "page": page, "text": draft}, false)
					if written["state"] != "written" {
						t.Fatal(written)
					}
					policySave(t, evidence, "create", written)
					marker := "- Revisão exata UTF-8: ação e evidência " + name
					appendArgs := map[string]any{"operation": "knowledge.append", "artifact_id": id, "page": page, "section": "## Review", "text": marker, "expected_page_revision": written["revision"]}
					expected["Operation"], expected["Section"], expected["Current page"], expected["Exact proposed text"], expected["Expected page revision"] = "knowledge.append", "## Review", draft, marker, written["revision"]
					appended := contentCall(t, client, "maestro_apply", appendArgs, false)
					if appended["state"] != "written" || reviews != 2 {
						t.Fatal("exact review/effect missing", appended, reviews)
					}
					policySave(t, evidence, "append", appended)
					client.Close()
					finalText := draft + marker + "\n"
					expected["Current page"] = finalText
					restarted := connectContent(t, f, options, "")
					replay := contentCall(t, restarted, "maestro_apply", appendArgs, false)
					if replay["state"] != "unchanged" || replay["operation_id"] != appended["operation_id"] || !reflect.DeepEqual(replay["provenance"], appended["provenance"]) {
						t.Fatal("restart changed canonical intent", replay)
					}
					collected := contentCall(t, restarted, "maestro_query", map[string]any{"operation": "knowledge.collect", "purpose": "Verify exact reviewed source after restart", "pages": []string{page}}, false)
					policySave(t, evidence, "restart-collect", collected)
					policySave(t, evidence, "restart-replay", replay)
					pages := collected["pages"].([]any)
					if len(pages) != 1 {
						t.Fatal("named page missing", collected)
					}
					knowledge := pages[0].(map[string]any)
					if knowledge["revision"] != appended["revision"] || knowledge["content"] != finalText || strings.Count(knowledge["content"].(string), marker) != 1 || knowledge["pointer"] == "" {
						t.Fatal("exact restarted knowledge readback mismatch", knowledge)
					}
					after := contentCall(t, restarted, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": id, "format": "markdown"}, false)
					if after["artifact_id"] != id || !reflect.DeepEqual(after["provenance"], provenance) || !reflect.DeepEqual(after["result"], storedReceipt) {
						t.Fatal("restart lost artifact provenance")
					}
					if !bytes.Equal(source, readContentFile(t, filepath.Join(f.options.WorkspacePath, fixture.name))) {
						t.Fatal("source mutated")
					}
					if firstID == "" {
						firstID, firstSource = id, source
					}
					if fixture.extension == ".docx" {
						sources["export.json"] = append([]byte(nil), document...)
						exportOrigin = map[string]any{"origin": "actual_public_docx_artifact", "artifact_id": id, "source_sha256": sourceSHA, "document_sha256": provenance["document_sha256"], "pack_sha256": provenance["pack_sha256"]}
						policySave(t, modeRoot, "export-source", exportOrigin)
					}
					t.Logf("actual public conversion/read/exact create+append/restart+retry passed source=%s elapsed=%s route=%s parser=%s fidelity=%s artifact=%s revision=%s pack=%s", fixture.name, time.Since(started), receipt["route"], receipt["parser"], receipt["fidelity"], id, knowledge["revision"], packSHA)
				})
			}
			if firstID == "" {
				t.Fatal("no actual converted artifact for controls")
			}
			t.Run("controls", func(t *testing.T) { policyControls(t, f, firstID, firstSource, modeRoot) })
			t.Run("partial", func(t *testing.T) {
				client := connectContent(t, f, nil, "")
				putContentFile(t, filepath.Join(f.options.WorkspacePath, "empty.xlsx"), sources["empty.xlsx"])
				got := contentCall(t, client, "maestro_apply", map[string]any{"operation": "content.convert", "source_path": "empty.xlsx"}, false)
				policySave(t, modeRoot, "partial", got)
				result := got["result"].(map[string]any)
				if result["status"] != "partial" || result["artifact_ref"] != nil || !reflect.DeepEqual(result["warnings"], []any{"converter_content_empty"}) {
					t.Fatal("empty workbook acquired usable artifact", got)
				}
				putContentFile(t, filepath.Join(f.options.WorkspacePath, "invalid.json"), []byte("{bad json"))
				bad := contentCall(t, client, "maestro_apply", map[string]any{"operation": "content.convert", "source_path": "invalid.json"}, false)["result"].(map[string]any)
				if bad["status"] != "blocked" || bad["artifact_ref"] != nil {
					t.Fatal("malformed source became usable", bad)
				}
				contentCall(t, client, "maestro_apply", map[string]any{"operation": "knowledge.create", "artifact_id": strings.Repeat("0", 64), "page": "knowledge/partial.md", "text": "must not exist"}, true)
			})
			t.Run("parser_tamper", func(t *testing.T) {
				client := connectContent(t, f, nil, "")
				installed := filepath.Join(filepath.Dir(f.root), "state/ingestion/docling/installed", packSHA, "site-packages/marko/parser.py")
				original := readContentFile(t, installed)
				if err := os.Chmod(installed, 0600); err != nil {
					t.Fatal(err)
				}
				putContentFile(t, installed, append([]byte("raise RuntimeError('TAMPER_EXECUTED')\n"), original...))
				if err := os.Chmod(installed, 0444); err != nil {
					t.Fatal(err)
				}
				defer func() {
					_ = os.Chmod(installed, 0600)
					putContentFile(t, installed, original)
					_ = os.Chmod(installed, 0444)
				}()
				status := contentCall(t, client, "maestro_query", map[string]any{"operation": "content.status"}, false)
				assertPolicyCoverage(t, status, "unavailable", "installed_pack_changed")
				got := contentCall(t, client, "maestro_apply", map[string]any{"operation": "content.convert", "source_path": "structured.md"}, false)
				policySave(t, modeRoot, "parser-tamper", got)
				result := got["result"].(map[string]any)
				if result["status"] != "unavailable" || result["artifact_ref"] != nil {
					t.Fatal("changed parser acquired artifact", got)
				}
				entries, err := os.ReadDir(filepath.Join(filepath.Dir(f.root), "state/ingestion/docling/executions"))
				if err != nil || len(entries) != 0 {
					t.Fatal("tamper executed or retained snapshot", err)
				}
			})
			if mode == "compiled_stdio" {
				t.Run("mcpb_readback", func(t *testing.T) { policyMCPB(t, f, binary, modeRoot, firstID, packSHA) })
				if !reflect.DeepEqual(packetBefore, contentTree(t, packetRoot, false)) {
					t.Fatal("read-only managed Code packet changed")
				}
			}
			if entries, err := os.ReadDir(f.options.ProductRoot); err != nil || len(entries) != 0 {
				t.Fatal("managed product written", err)
			}
		})
	}
}

func policyControls(t *testing.T, f ownerFixture, id string, source []byte, evidence string) {
	t.Helper()
	client := connectContent(t, f, nil, "")
	refusals := map[string]any{}
	for _, selection := range []string{"../quoted.csv", filepath.Join(f.options.WorkspacePath, "quoted.csv")} {
		got := contentCall(t, client, "maestro_apply", map[string]any{"operation": "content.convert", "source_path": selection}, false)["result"].(map[string]any)
		if got["status"] != "blocked" || got["artifact_ref"] != nil {
			t.Fatal("unsafe source admitted", got)
		}
		refusals[selection] = got
	}
	for _, link := range []string{"symlink", "hardlink"} {
		alias := filepath.Join(f.options.WorkspacePath, link+".csv")
		var err error
		if link == "symlink" {
			err = os.Symlink("quoted.csv", alias)
		} else {
			err = os.Link(filepath.Join(f.options.WorkspacePath, "quoted.csv"), alias)
		}
		if err != nil {
			t.Fatal(err)
		}
		got := contentCall(t, client, "maestro_apply", map[string]any{"operation": "content.convert", "source_path": link + ".csv"}, false)["result"].(map[string]any)
		if err = os.Remove(alias); err != nil {
			t.Fatal(err)
		}
		if got["status"] != "blocked" {
			t.Fatal("aliased source admitted", got)
		}
		refusals[link] = got
	}
	reject := map[string]any{"operation": "knowledge.create", "artifact_id": id, "page": "knowledge/refused.md", "text": "must not appear"}
	refusals["no_form"] = contentCall(t, client, "maestro_apply", reject, true)
	declined := connectContent(t, f, &mcp.ClientOptions{ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "decline"}, nil
	}}, "")
	refusals["declined"] = contentCall(t, declined, "maestro_apply", reject, true)
	for _, field := range []string{"approved", "authority", "workspace_id", "reader", "session_id"} {
		args := map[string]any{}
		for k, v := range reject {
			args[k] = v
		}
		args[field] = true
		refusals["model_"+field] = contentCall(t, client, "maestro_apply", args, true)
	}
	bound, err := product.Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	mutation := product.KnowledgeMutation{Operation: "knowledge.create", ArtifactID: id, Page: "knowledge/refused.md", Text: "must not appear"}
	review, err := bound.KnowledgeReview(mutation)
	if err != nil {
		t.Fatal(err)
	}
	changed := mutation
	changed.Text = "retargeted text"
	if _, err = bound.ApplyKnowledgeReview(changed, review.ReviewDigest, true); err == nil {
		t.Fatal("changed draft accepted stale review")
	}
	refusals["changed_draft"] = err.Error()
	changed = mutation
	changed.Page = "knowledge/retargeted.md"
	if _, err = bound.ApplyKnowledgeReview(changed, review.ReviewDigest, true); err == nil {
		t.Fatal("retargeted page accepted stale review")
	}
	refusals["changed_page"] = err.Error()
	putContentFile(t, filepath.Join(f.options.WorkspacePath, "quoted.csv"), []byte("changed source"))
	if _, err = bound.ApplyKnowledgeReview(mutation, review.ReviewDigest, true); err == nil {
		t.Fatal("changed source accepted old review")
	}
	refusals["changed_source"] = err.Error()
	putContentFile(t, filepath.Join(f.options.WorkspacePath, "quoted.csv"), source)
	metadata := contentCall(t, client, "maestro_context", map[string]any{}, false)
	artifact := filepath.Join(filepath.Dir(f.root), "state/content", metadata["workspace_id"].(string), "artifacts", id+".json")
	original := readContentFile(t, artifact)
	putContentFile(t, artifact, append(append([]byte(nil), original...), ' '))
	refusals["changed_artifact_read"] = contentCall(t, client, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": id, "format": "markdown"}, true)
	if _, err = bound.ApplyKnowledgeReview(mutation, review.ReviewDigest, true); err == nil {
		t.Fatal("changed artifact accepted review")
	}
	refusals["changed_artifact_review"] = err.Error()
	putContentFile(t, artifact, original)
	foreign := f
	foreign.binary = ""
	foreign.options.WorkspacePath = filepath.Join(filepath.Dir(f.options.WorkspacePath), "foreign-workspace")
	if _, err = workspace.Initialize(workspace.Options{WorkspacePath: foreign.options.WorkspacePath, DataRoot: filepath.Join(filepath.Dir(f.root), "state")}); err != nil {
		t.Fatal(err)
	}
	other := connectContent(t, foreign, nil, "")
	otherMeta := contentCall(t, other, "maestro_context", map[string]any{}, false)
	putContentFile(t, filepath.Join(filepath.Dir(f.root), "state/content", otherMeta["workspace_id"].(string), "artifacts", id+".json"), original)
	refusals["foreign_artifact"] = contentCall(t, other, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": id, "format": "markdown"}, true)
	foreignBound, err := product.Open(foreign.options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = foreignBound.ApplyKnowledgeReview(mutation, review.ReviewDigest, true); err == nil {
		t.Fatal("foreign scope accepted review")
	}
	refusals["foreign_review"] = err.Error()
	page := "knowledge/policy/csv.md"
	before := contentCall(t, client, "maestro_query", map[string]any{"operation": "knowledge.collect", "purpose": "Check stale review", "pages": []string{page}}, false)["pages"].([]any)[0].(map[string]any)
	accept := &mcp.ClientOptions{ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	}}
	staleClient := connectContent(t, f, accept, "")
	stale := map[string]any{"operation": "knowledge.append", "artifact_id": id, "page": page, "section": "## Review", "text": "must stay proposed", "expected_page_revision": strings.Repeat("0", 64)}
	conflict := contentCall(t, staleClient, "maestro_apply", stale, false)
	if conflict["state"] != "proposed" || conflict["proposal"] != nil {
		t.Fatal("stale revision leaked/applied", conflict)
	}
	newer := before["content"].(string) + "\nOWNER NEWER CONTENT\n"
	race := connectContent(t, f, &mcp.ClientOptions{ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		putContentFile(t, filepath.Join(f.options.WorkspacePath, "brain", page), []byte(newer))
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	}}, "")
	stale["expected_page_revision"], stale["text"] = before["revision"], "must lose concurrent race"
	refusals["concurrent_page"] = contentCall(t, race, "maestro_apply", stale, true)
	after := contentCall(t, client, "maestro_query", map[string]any{"operation": "knowledge.collect", "purpose": "Verify newer page preserved", "pages": []string{page}}, false)
	if after["pages"].([]any)[0].(map[string]any)["content"] != newer {
		t.Fatal("concurrent page lost")
	}
	if _, err = os.Stat(filepath.Join(f.options.WorkspacePath, "brain/knowledge/refused.md")); !os.IsNotExist(err) {
		t.Fatal("declined/no-form/stale-review page written")
	}
	policySave(t, evidence, "controls", map[string]any{"stale_conflict": conflict, "newer_page": after, "refusals": refusals})
}

func policyMCPB(t *testing.T, f ownerFixture, binary, root, id, packSHA string) {
	t.Helper()
	packet := packaging.Options{Destination: filepath.Join(root, "content.mcpb"), Server: binary, Version: "0.1.0", Binding: f.options}
	if err := packaging.BuildExtension(packet); err != nil {
		t.Fatal(err)
	}
	desktop := filepath.Join(root, "mcpb-packet")
	extractProductPacketFixture(t, packet.Destination, desktop)
	var err error
	before := contentTree(t, desktop, true)
	f.binary = filepath.Join(desktop, "server/maestro-mcp")
	client := connectContentMCPB(t, f, desktop)
	status := contentCall(t, client, "maestro_query", map[string]any{"operation": "content.status"}, false)
	assertPolicyCoverage(t, status, "ready", "")
	if status["pack_sha256"] != packSHA {
		t.Fatal(status)
	}
	policyResources(t, client)
	contentCall(t, client, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": id, "format": "document", "max_bytes": 97}, false)
	client.Close()
	restarted := connectContentMCPB(t, f, desktop)
	collected := contentCall(t, restarted, "maestro_query", map[string]any{"operation": "knowledge.collect", "purpose": "MCPB restarted readback", "pages": []string{"knowledge/policy/txt.md"}}, false)
	pages := collected["pages"].([]any)
	var expected map[string]any
	if err = json.Unmarshal(readContentFile(t, filepath.Join(root, "evidence/txt/restart-collect.json")), &expected); err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || !reflect.DeepEqual(pages, expected["pages"]) {
		t.Fatal("MCPB restarted page absent", collected)
	}
	var converted map[string]any
	if err = json.Unmarshal(readContentFile(t, filepath.Join(root, "evidence/txt/conversion.json")), &converted); err != nil {
		t.Fatal(err)
	}
	txtID := converted["result"].(map[string]any)["artifact_ref"].(string)
	document, readback := policyRead(t, restarted, txtID, "document", 4096)
	if !bytes.Equal(document, readContentFile(t, filepath.Join(root, "evidence/txt/document.json"))) || !reflect.DeepEqual(readback["provenance"], converted["provenance"]) {
		t.Fatal("MCPB restarted artifact/provenance changed")
	}
	policySave(t, root, "mcpb-readback", collected)
	policySave(t, root, "packet-identity", map[string]any{"mcpb_sha256": fmt.Sprintf("%x", sha256.Sum256(readContentFile(t, packet.Destination))), "server_sha256": fmt.Sprintf("%x", sha256.Sum256(readContentFile(t, f.binary))), "pack_sha256": packSHA})
	if !reflect.DeepEqual(before, contentTree(t, desktop, false)) {
		t.Fatal("managed MCPB packet changed")
	}
}

func TestContentOldProfileUnavailable(t *testing.T) {
	root, seed := os.Getenv("MAESTRO_DOCLING_E4_ROOT"), os.Getenv("MAESTRO_DOCLING_E4_SEED")
	if root == "" || seed == "" {
		t.Skip("explicit original E4 engineering fixture and independent seed required")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("old retained profile is darwin/arm64")
	}
	authority := policySeed(t, root, seed)
	f := newOwnerFixture(t, false)
	registry := filepath.Join(filepath.Dir(f.root), "old-authority.json")
	putContentFile(t, registry, authority)
	f.options.AuthorityRegistryPath, f.options.AuthorityRegistrySHA256, f.options.CLIVersion = registry, seed, "0.1.0"
	client := connectContent(t, f, nil, "")
	missing := contentCall(t, client, "maestro_query", map[string]any{"operation": "content.status"}, false)
	assertPolicyCoverage(t, missing, "unavailable", "pack_unavailable")
	policyCopyPack(t, root, &f, false)
	status := contentCall(t, connectContent(t, f, nil, ""), "maestro_query", map[string]any{"operation": "content.status"}, false)
	assertPolicyCoverage(t, status, "unavailable", "pack_untrusted")
	t.Logf("properly signed original E4 profile refused before installed-tree/execution: %v", status)
}
