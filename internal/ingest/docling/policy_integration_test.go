package docling_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

type policyDocument struct {
	Schema   string `json:"schema_name"`
	Pictures []struct {
		Image json.RawMessage `json:"image"`
	} `json:"pictures"`
	Groups []struct {
		Name  string `json:"name"`
		Label string `json:"label"`
	} `json:"groups"`
	Texts []struct {
		Text  string `json:"text"`
		Orig  string `json:"orig"`
		Label string `json:"label"`
	} `json:"texts"`
	Tables []struct {
		Data struct {
			Rows  int `json:"num_rows"`
			Cols  int `json:"num_cols"`
			Cells []struct {
				Text   string `json:"text"`
				Row    int    `json:"start_row_offset_idx"`
				Col    int    `json:"start_col_offset_idx"`
				EndCol int    `json:"end_col_offset_idx"`
			} `json:"table_cells"`
		} `json:"data"`
	} `json:"tables"`
}

func policyFixtureRuntime(t *testing.T) (*docling.Runtime, string, string, string) {
	t.Helper()
	inputs, root := os.Getenv("MAESTRO_POLICY_TEST_INPUTS"), os.Getenv("MAESTRO_POLICY_TEST_ROOT")
	if inputs == "" || root == "" {
		t.Skip("explicit isolated policy inputs and new retained engineering root required")
	}
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	data, space := filepath.Join(root, "state"), filepath.Join(root, "workspace")
	if err := os.Mkdir(space, 0700); err != nil {
		t.Fatal(err)
	}
	w, err := workspace.Initialize(workspace.Options{WorkspacePath: space, DataRoot: data})
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(data, "ingestion/docling")
	if err := os.MkdirAll(filepath.Dir(base), 0700); err != nil {
		t.Fatal(err)
	}
	assembled, err := docling.Assemble(inputs, base)
	if err != nil {
		t.Fatal(err)
	}
	archive := read(t, assembled.ArchivePath)
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, _ := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "engineering-fixture", "key_id": "ephemeral", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": "2020-01-01T00:00:00Z", "valid_until": "2100-01-01T00:00:00Z"}}})
	registryPath := filepath.Join(root, "authority.json")
	put(t, registryPath, registry)
	manifest := releasecontract.Manifest{SchemaVersion: 1, Product: "maestro", Release: "0.1.0", Channel: "canary", Issuer: releasecontract.Issuer{ID: "engineering-fixture", KeyID: "ephemeral"}, CLI: releasecontract.CLIComponent{Version: "0.1.0", CompatibleBundle: ">=0.1.0 <0.2.0"}, Bundle: releasecontract.BundleComponent{Version: "0.1.0", CompatibleCLI: ">=0.1.0 <0.2.0"}, Migrations: []releasecontract.Migration{}, ReleaseNotes: releasecontract.ReleaseNotes{Name: "notes-0.1.0.md", SHA256: sum([]byte("engineering only"))}, Artifacts: []releasecontract.Artifact{{Kind: "bundle", OS: "any", Arch: "any", Name: "bundle-0.1.0.tar.gz", Size: 1, SHA256: sum([]byte("b")), SignatureRef: "bundle-0.1.0.tar.gz.sig"}, {Kind: "runtime_pack", OS: "darwin", Arch: "arm64", Name: filepath.Base(assembled.ArchivePath), Size: int64(len(archive)), SHA256: assembled.ArchiveSHA256, SignatureRef: filepath.Base(assembled.ArchivePath) + ".sig"}}}
	body, _ := json.Marshal(manifest)
	put(t, filepath.Join(base, "release-manifest.json"), body)
	put(t, filepath.Join(base, "release-manifest.json.sig"), ed25519.Sign(private, body))
	put(t, assembled.ArchivePath+".sig", ed25519.Sign(private, archive))
	r := docling.New(docling.Options{DataRoot: data, AuthorityRegistryPath: registryPath, AuthorityRegistrySHA256: sum(registry), CLIVersion: "0.1.0"})
	identity, _ := json.Marshal(map[string]any{"assembly": assembled, "registry_sha256": sum(registry), "manifest_sha256": sum(body), "lock_sha256": docling.LockSHA256, "engineering_only": true})
	put(t, filepath.Join(root, "runtime-identity.json"), identity)
	t.Logf("authenticated policy fixture files=%d expanded=%d archive=%s registry=%s", assembled.Files, assembled.ExpandedBytes, assembled.ArchiveSHA256, sum(registry))
	return r, root, space, w.WorkspaceID
}

func TestAuthenticatedPolicyFormats(t *testing.T) {
	r, root, space, id := policyFixtureRuntime(t)
	evidence := filepath.Join(root, "outputs")
	if err := os.Mkdir(evidence, 0700); err != nil {
		t.Fatal(err)
	}
	convert := func(t *testing.T, name string, body []byte) docling.Conversion {
		t.Helper()
		body = bytes.ReplaceAll(body, []byte("file:///MAESTRO_POLICY_BAIT.svg"), []byte("file://"+filepath.Join(root, "external-bait.png")))
		put(t, filepath.Join(space, name), body)
		started := time.Now()
		got, err := r.Convert(context.Background(), ingest.Request{SourcePath: name, WorkspacePath: space, Policy: ingest.DefaultPolicy()}, id)
		metadata, _ := json.Marshal(got)
		put(t, filepath.Join(evidence, name+".result.json"), metadata)
		if len(got.Document) > 0 {
			put(t, filepath.Join(evidence, name+".document.json"), got.Document)
			put(t, filepath.Join(evidence, name+".md"), got.Markdown)
		}
		if err != nil || got.Result.Status != ingest.StatusUsable {
			t.Fatalf("%s conversion: %+v %v", name, got.Result, err)
		}
		if !bytes.Equal(body, read(t, filepath.Join(space, name))) || got.Result.SourceSHA256 != sum(body) || !got.Provenance.NetworkDenied {
			t.Fatal("source changed or offline provenance absent")
		}
		t.Logf("%s elapsed=%s route=%s parser=%s fidelity=%s markdown=%d structured=%d", name, time.Since(started), got.Result.Route, got.Result.Parser, got.Result.Fidelity, len(got.Markdown), len(got.Document))
		return got
	}
	parse := func(t *testing.T, got docling.Conversion) policyDocument {
		t.Helper()
		var d policyDocument
		if json.Unmarshal(got.Document, &d) != nil || d.Schema != "DoclingDocument" {
			t.Fatal("not a real DoclingDocument")
		}
		for _, picture := range d.Pictures {
			if len(picture.Image) > 0 && string(picture.Image) != "null" {
				t.Fatal("external image bytes were loaded")
			}
		}
		return d
	}
	cell := func(t *testing.T, d policyDocument, table, row, col int, want string) {
		t.Helper()
		if table < len(d.Tables) {
			for _, c := range d.Tables[table].Data.Cells {
				if c.Row == row && c.Col == col && c.Text == want {
					return
				}
			}
		}
		t.Fatalf("missing table %d cell (%d,%d)=%q: %+v", table, row, col, want, d.Tables)
	}
	markdown := func(t *testing.T, got docling.Conversion, markers ...string) {
		t.Helper()
		for _, want := range markers {
			if !bytes.Contains(got.Markdown, []byte(want)) {
				t.Fatalf("Markdown lacks %q: %s", want, got.Markdown)
			}
		}
	}
	// A valid raster bait would become a nonnil ImageRef if local fetching ran.
	// It lives outside the admitted workspace and is never an ingestion input.
	bait := image.NewRGBA(image.Rect(0, 0, 1, 1))
	bait.Set(0, 0, color.RGBA{R: 255, A: 255})
	var pngBody bytes.Buffer
	if err := png.Encode(&pngBody, bait); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(root, "external-bait.png"), pngBody.Bytes())
	var docxExport []byte
	t.Run("docx", func(t *testing.T) {
		got := convert(t, "sample.docx", read(t, "testdata/sample.docx"))
		d := parse(t, got)
		cell(t, d, 0, 0, 0, "Item")
		cell(t, d, 0, 0, 1, "Count")
		cell(t, d, 0, 1, 0, "alpha")
		cell(t, d, 0, 1, 1, "7")
		markdown(t, got, "alpha", "7")
		docxExport = got.Document
	})
	for _, name := range []string{"literal.txt", "generic.json", "generic.xml"} {
		t.Run(name, func(t *testing.T) {
			body := read(t, filepath.Join("testdata/policy", name))
			got := convert(t, name, body)
			d := parse(t, got)
			if got.Result.Route != "maestro_literal_v1" || got.Result.Fidelity != ingest.FidelityTextual || got.Result.FallbackReason != "native_generic_literal_unsupported" || len(d.Texts) != 1 || d.Texts[0].Text != string(body) || d.Texts[0].Orig != string(body) || d.Texts[0].Label != "code" {
				t.Fatalf("literal changed or misclassified: %+v %+v", got.Result, d)
			}
			markdown(t, got, strings.TrimSpace(string(body)))
		})
	}
	t.Run("csv", func(t *testing.T) {
		got := convert(t, "quoted.csv", read(t, "testdata/policy/quoted.csv"))
		d := parse(t, got)
		cell(t, d, 0, 1, 0, "001")
		cell(t, d, 0, 1, 1, "comma, value")
		cell(t, d, 0, 1, 2, "line one\nline two")
		cell(t, d, 0, 2, 1, `say "hello"`)
		markdown(t, got, "001", "comma, value", "line one", "line two")
	})
	for _, name := range []string{"page.html", "page.htm"} {
		t.Run(name, func(t *testing.T) {
			got := convert(t, name, read(t, filepath.Join("testdata/policy", name)))
			d := parse(t, got)
			cell(t, d, 0, 0, 0, "Item")
			cell(t, d, 0, 1, 0, "beta")
			cell(t, d, 0, 1, 1, "13")
			markdown(t, got, "Policy HTML", "Ação local", "beta", "13")
		})
	}
	t.Run("markdown", func(t *testing.T) {
		got := convert(t, "structured.md", read(t, "testdata/policy/structured.md"))
		d := parse(t, got)
		cell(t, d, 0, 1, 0, "gamma")
		cell(t, d, 0, 1, 1, "17")
		found := false
		for _, text := range d.Texts {
			if text.Label == "code" && strings.Contains(text.Text, "<tag>literal_code</tag>") {
				found = true
			}
		}
		if !found {
			t.Fatal("fenced code structure lost")
		}
		markdown(t, got, "Policy Markdown", "first", "second", "gamma", "17", "<tag>literal_code</tag>")
	})
	t.Run("epub", func(t *testing.T) {
		got := convert(t, "spine.epub", read(t, "testdata/policy/spine.epub"))
		d := parse(t, got)
		var text string
		for _, v := range d.Texts {
			text += v.Text + "\n"
		}
		if strings.Index(text, "BEFORE_SPINE") < 0 || strings.Index(text, "AFTER_SPINE") < strings.Index(text, "BEFORE_SPINE") {
			t.Fatalf("spine order lost: %s", text)
		}
		markdown(t, got, "First chapter", "Second chapter")
		if bytes.Index(got.Markdown, []byte("BEFORE")) > bytes.Index(got.Markdown, []byte("AFTER")) {
			t.Fatal("Markdown spine order")
		}
	})
	t.Run("xlsx", func(t *testing.T) {
		got := convert(t, "sheets.xlsx", read(t, "testdata/policy/sheets.xlsx"))
		d := parse(t, got)
		if len(d.Groups) != 2 || d.Groups[0].Name != "Metrics" || d.Groups[0].Label != "sheet" || d.Groups[1].Name != "Details" || len(d.Texts) != 2 || d.Texts[0].Text != "Metrics" || d.Texts[1].Text != "Details" {
			t.Fatalf("native sheet identity/order lost: %+v %+v", d.Groups, d.Texts)
		}
		cell(t, d, 0, 1, 0, "delta")
		cell(t, d, 0, 1, 1, "23")
		cell(t, d, 0, 1, 2, "29")
		cell(t, d, 0, 2, 0, "True")
		cell(t, d, 1, 1, 0, "0007")
		cell(t, d, 1, 1, 1, "3.5")
		merged := false
		for _, c := range d.Tables[0].Data.Cells {
			if c.Text == "Merged heading" && c.Col == 0 && c.EndCol == 2 {
				merged = true
			}
		}
		if !merged {
			t.Fatal("merged span lost")
		}
		markdown(t, got, "Metrics", "Details", "delta", "29", "0007")
		if bytes.Contains(got.Markdown, []byte("100")) {
			t.Fatal("uncached formula was evaluated")
		}
	})
	t.Run("xls", func(t *testing.T) {
		got := convert(t, "legacy.xls", read(t, "testdata/policy/legacy.xls"))
		d := parse(t, got)
		if got.Result.Parser != "xlrd.biff" || got.Result.Fidelity != ingest.FidelityTextual || got.Result.FallbackReason == "" {
			t.Fatalf("BIFF route: %+v", got.Result)
		}
		cell(t, d, 0, 1, 0, "0009")
		cell(t, d, 0, 1, 1, "31")
		cell(t, d, 0, 1, 2, "2024-02-29T00:00:00")
		cell(t, d, 0, 1, 3, "37")
		cell(t, d, 0, 1, 4, "")
		cell(t, d, 1, 1, 1, "4.25")
		markdown(t, got, "Legacy", "Notes", "0009", "31", "2024-02-29", "37", "legacy-note")
	})
	t.Run("genuine Docling JSON", func(t *testing.T) {
		if docxExport == nil {
			t.Fatal("DOCX prerequisite failed")
		}
		got := convert(t, "export.json", docxExport)
		if got.Result.Parser != "docling.json" || got.Result.Fidelity != ingest.FidelityStructured {
			t.Fatalf("export route: %+v", got.Result)
		}
		d := parse(t, got)
		cell(t, d, 0, 1, 0, "alpha")
		markdown(t, got, "alpha", "7")
	})
	for _, test := range []struct{ name, body string }{
		{"root-array.json", "[1, true, null, {\"x\":\"ação\"}]\n"},
		{"root-string.json", "\"scalar\\nvalue\"\n"},
		{"root-number.json", "42\n"},
		{"root-claimed-export.json", "{\"schema_name\":\"DoclingDocument\",\"custom\":true}\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := convert(t, test.name, []byte(test.body))
			d := parse(t, got)
			if len(d.Texts) != 1 || d.Texts[0].Text != test.body || got.Result.Fidelity != ingest.FidelityTextual {
				t.Fatal("generic JSON root lost")
			}
			markdown(t, got, strings.TrimSpace(test.body))
		})
	}
	t.Run("output budget", func(t *testing.T) {
		policy := ingest.DefaultPolicy()
		policy.MaxOutputBytes = 512
		got, err := r.Convert(context.Background(), ingest.Request{SourcePath: "literal.txt", WorkspacePath: space, Policy: policy}, id)
		if err != nil || got.Result.Status != ingest.StatusDegraded || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "output_limit" || len(got.Document) != 0 || len(got.Markdown) != 0 {
			t.Fatalf("output limit: %+v %v", got.Result, err)
		}
	})
	t.Run("parser tamper before execution", func(t *testing.T) {
		status := r.Status(context.Background())
		if status.State != "ready" {
			t.Fatal(status)
		}
		name := filepath.Join(root, "state/ingestion/docling/installed", status.PackSHA256, "site-packages/marko/parser.py")
		old := read(t, name)
		if err := os.Chmod(name, 0600); err != nil {
			t.Fatal(err)
		}
		put(t, name, append([]byte("raise RuntimeError('TAMPER_EXECUTED')\n"), old...))
		_ = os.Chmod(name, 0444)
		defer func() { _ = os.Chmod(name, 0600); put(t, name, old); _ = os.Chmod(name, 0444) }()
		got, err := r.Convert(context.Background(), ingest.Request{SourcePath: "structured.md", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, id)
		if err != nil || got.Result.Status != ingest.StatusUnavailable || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "installed_pack_changed" || len(got.Document) > 0 {
			t.Fatalf("tampered parser executed: %+v %v", got.Result, err)
		}
		entries, err := os.ReadDir(filepath.Join(root, "state/ingestion/docling/executions"))
		if err != nil || len(entries) != 0 {
			t.Fatalf("execution snapshot created before verification: %v %v", entries, err)
		}
	})
}
