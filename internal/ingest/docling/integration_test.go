package docling_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func sum(body []byte) string { s := sha256.Sum256(body); return hex.EncodeToString(s[:]) }
func put(t *testing.T, name string, body []byte) {
	t.Helper()
	if err := os.WriteFile(name, body, 0600); err != nil {
		t.Fatal(err)
	}
}
func read(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestAuthenticatedRealDocling(t *testing.T) {
	inputs := os.Getenv("MAESTRO_DOCLING_TEST_INPUTS")
	if inputs == "" {
		t.Skip("explicit isolated official input directory required")
	}
	root := t.TempDir()
	if retained := os.Getenv("MAESTRO_DOCLING_TEST_ROOT"); retained != "" {
		if err := os.Mkdir(retained, 0700); err != nil {
			t.Fatal(err)
		}
		root = retained
	}
	defer func() {
		if os.Getenv("MAESTRO_DOCLING_TEST_ROOT") == "" {
			_ = filepath.WalkDir(root, func(name string, item os.DirEntry, err error) error {
				if err == nil && item.IsDir() {
					return os.Chmod(name, 0700)
				}
				return err
			})
		}
	}()
	data := filepath.Join(root, "state")
	space := filepath.Join(root, "workspace")
	if err := os.Mkdir(space, 0700); err != nil {
		t.Fatal(err)
	}
	w, err := workspace.Initialize(workspace.Options{WorkspacePath: space, DataRoot: data})
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(data, "ingestion", "docling")
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
	manifest := releasecontract.Manifest{SchemaVersion: 1, Product: "maestro", Release: "0.1.0", Channel: "canary", Issuer: releasecontract.Issuer{ID: "engineering-fixture", KeyID: "ephemeral"}, CLI: releasecontract.CLIComponent{Version: "0.1.0", CompatibleBundle: ">=0.1.0 <0.2.0"}, Bundle: releasecontract.BundleComponent{Version: "0.1.0", CompatibleCLI: ">=0.1.0 <0.2.0"}, Migrations: []releasecontract.Migration{}, ReleaseNotes: releasecontract.ReleaseNotes{Name: "notes-0.1.0.md", SHA256: sum([]byte("engineering only"))}, Artifacts: []releasecontract.Artifact{{Kind: "bundle", OS: "any", Arch: "any", Name: "bundle-0.1.0.tar.gz", Size: 1, SHA256: sum([]byte("b")), SignatureRef: "bundle-0.1.0.tar.gz.sig"}, {Kind: "runtime_pack", OS: "darwin", Arch: "arm64", Name: filepath.Base(assembled.ArchivePath), Size: int64(len(archive)), SHA256: sum(archive), SignatureRef: filepath.Base(assembled.ArchivePath) + ".sig"}}}
	body, _ := json.Marshal(manifest)
	put(t, filepath.Join(base, "release-manifest.json"), body)
	put(t, filepath.Join(base, "release-manifest.json.sig"), ed25519.Sign(private, body))
	put(t, assembled.ArchivePath+".sig", ed25519.Sign(private, archive))
	fixture := read(t, "testdata/sample.docx")
	put(t, filepath.Join(space, "sample.docx"), fixture)
	runtime := docling.New(docling.Options{DataRoot: data, AuthorityRegistryPath: registryPath, AuthorityRegistrySHA256: sum(registry), CLIVersion: "0.1.0"})
	request := ingest.Request{SourcePath: "sample.docx", WorkspacePath: space, Policy: ingest.DefaultPolicy()}
	converted, err := runtime.Convert(context.Background(), request, w.WorkspaceID)
	if err != nil || converted.Result.Status != ingest.StatusUsable {
		t.Fatalf("real conversion = %+v err=%v", converted.Result, err)
	}
	if !strings.Contains(string(converted.Markdown), `MAESTRO\_DOCLING\_OFFLINE\_20261101`) || !bytes.Contains(converted.Document, []byte("MAESTRO_DOCLING_OFFLINE_20261101")) {
		t.Fatalf("literal synthetic oracle missing: %s", converted.Markdown)
	}
	var document struct {
		Tables []struct {
			Data struct {
				Rows  int `json:"num_rows"`
				Cols  int `json:"num_cols"`
				Cells []struct {
					Text string `json:"text"`
				} `json:"table_cells"`
			} `json:"data"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(converted.Document, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Tables) != 1 || document.Tables[0].Data.Rows != 2 || document.Tables[0].Data.Cols != 2 {
		t.Fatalf("table oracle = %+v", document)
	}
	cells := map[string]bool{}
	for _, cell := range document.Tables[0].Data.Cells {
		cells[cell.Text] = true
	}
	if len(cells) != 4 || !cells["Item"] || !cells["Count"] || !cells["alpha"] || !cells["7"] {
		t.Fatalf("cell oracle=%v", cells)
	}
	if !converted.Provenance.NetworkDenied || converted.Provenance.PackSHA256 != assembled.ArchiveSHA256 || converted.Result.SourceSHA256 != "6bd282a12170035e8af5352f2de5a21f4fe4bc5c7919283482a26703c206bf81" || !bytes.Equal(fixture, read(t, filepath.Join(space, "sample.docx"))) {
		t.Fatal("provenance or source preservation failed")
	}
	t.Logf("real authenticated engineering pack: files=%d expanded=%d archive_sha=%s markdown=%d document=%d network_denied=%v", assembled.Files, assembled.ExpandedBytes, assembled.ArchiveSHA256, len(converted.Markdown), len(converted.Document), converted.Provenance.NetworkDenied)
	installed := filepath.Join(base, "installed", assembled.ArchiveSHA256)
	for name, file := range map[string]string{"registry": registryPath, "manifest": filepath.Join(base, "release-manifest.json"), "artifact signature": assembled.ArchivePath + ".sig",
		"archive": assembled.ArchivePath, "adapter": filepath.Join(installed, "bridge.py"), "interpreter": filepath.Join(installed, "python/bin/python3.12"), "dependency": filepath.Join(installed, "site-packages/docling/document_converter.py"), "installed inventory": filepath.Join(installed, "pack.json")} {
		t.Run("post-success tamper "+name, func(t *testing.T) {
			old := read(t, file)
			info, err := os.Stat(file)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(file, 0600); err != nil {
				t.Fatal(err)
			}
			put(t, file, append(append([]byte(nil), old...), byte('x')))
			defer func() { put(t, file, old); _ = os.Chmod(file, info.Mode().Perm()) }()
			got, err := runtime.Convert(context.Background(), request, w.WorkspaceID)
			if err != nil || got.Result.Status != ingest.StatusUnavailable || len(got.Document) != 0 || len(got.Markdown) != 0 {
				t.Fatalf("tamper executed: %+v %v", got.Result, err)
			}
		})
	}
	t.Run("unlisted executable input", func(t *testing.T) {
		name := filepath.Join(installed, "site-packages", "surprise.pth")
		_ = os.Chmod(filepath.Dir(name), 0700)
		put(t, name, []byte("import os"))
		_ = os.Chmod(filepath.Dir(name), 0555)
		defer func() {
			_ = os.Chmod(filepath.Dir(name), 0700)
			_ = os.Remove(name)
			_ = os.Chmod(filepath.Dir(name), 0555)
		}()
		got, _ := runtime.Convert(context.Background(), request, w.WorkspaceID)
		if got.Result.Status != ingest.StatusUnavailable {
			t.Fatal(got.Result)
		}
	})
	for name, source := range map[string]string{"absolute": filepath.Join(space, "sample.docx"), "dot": "./sample.docx", "parent": "child/../sample.docx", "traversal": "../sample.docx"} {
		t.Run(name, func(t *testing.T) {
			r := request
			r.SourcePath = source
			got, _ := runtime.Convert(context.Background(), r, w.WorkspaceID)
			if got.Result.Status != ingest.StatusBlocked {
				t.Fatal(got.Result)
			}
		})
	}
	t.Run("wrong registered identity", func(t *testing.T) {
		got, _ := runtime.Convert(context.Background(), request, "other-workspace")
		if got.Result.Status != ingest.StatusBlocked {
			t.Fatal(got.Result)
		}
	})
	t.Run("source symlink", func(t *testing.T) {
		if err := os.Symlink("sample.docx", filepath.Join(space, "alias.docx")); err != nil {
			t.Fatal(err)
		}
		r := request
		r.SourcePath = "alias.docx"
		got, _ := runtime.Convert(context.Background(), r, w.WorkspaceID)
		if got.Result.Status != ingest.StatusBlocked {
			t.Fatal(got.Result)
		}
	})
	t.Run("source hardlink", func(t *testing.T) {
		name := filepath.Join(space, "hard.docx")
		if err := os.Link(filepath.Join(space, "sample.docx"), name); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(name)
		r := request
		r.SourcePath = "hard.docx"
		got, _ := runtime.Convert(context.Background(), r, w.WorkspaceID)
		if got.Result.Status != ingest.StatusBlocked {
			t.Fatal(got.Result)
		}
	})
	t.Run("source size", func(t *testing.T) {
		r := request
		r.Policy.MaxInputBytes = 10
		got, _ := runtime.Convert(context.Background(), r, w.WorkspaceID)
		if got.Result.Status != ingest.StatusBlocked {
			t.Fatal(got.Result)
		}
	})
	t.Run("expansion size", func(t *testing.T) {
		r := request
		r.Policy.MaxOutputBytes = 100
		got, _ := runtime.Convert(context.Background(), r, w.WorkspaceID)
		if got.Result.Status != ingest.StatusBlocked {
			t.Fatal(got.Result)
		}
	})
	t.Run("explicit fallback only", func(t *testing.T) {
		put(t, filepath.Join(space, "note.txt"), []byte("synthetic"))
		r := request
		r.SourcePath = "note.txt"
		got, _ := runtime.Convert(context.Background(), r, w.WorkspaceID)
		if got.Result.Status != ingest.StatusUnavailable || got.Result.Adapter != "docling" {
			t.Fatal(got.Result)
		}
		if _, err := ingest.SelectFallback(ingest.PrimaryUnsupported, ""); err == nil {
			t.Fatal("implicit fallback")
		}
		route, err := ingest.SelectFallback(ingest.PrimaryUnsupported, "markitdown")
		if err != nil || route.Route != "markitdown_fallback" {
			t.Fatal(route, err)
		}
	})
	t.Run("output budget", func(t *testing.T) {
		r := request
		r.Policy.MaxOutputBytes = 2500
		got, _ := runtime.Convert(context.Background(), r, w.WorkspaceID)
		if got.Result.Status != ingest.StatusDegraded || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "output_limit" || len(got.Document) != 0 {
			t.Fatal(got.Result)
		}
	})
	t.Run("installed hardlink", func(t *testing.T) {
		name := filepath.Join(installed, "bridge.py")
		alias := filepath.Join(root, "bridge-alias")
		if err := os.Link(name, alias); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(alias)
		got, _ := runtime.Convert(context.Background(), request, w.WorkspaceID)
		if got.Result.Status != ingest.StatusUnavailable {
			t.Fatal(got.Result)
		}
	})
	t.Run("missing dependency", func(t *testing.T) {
		name := filepath.Join(installed, "site-packages/docling/document_converter.py")
		saved := read(t, name)
		_ = os.Chmod(filepath.Dir(name), 0700)
		if err := os.Remove(name); err != nil {
			t.Fatal(err)
		}
		_ = os.Chmod(filepath.Dir(name), 0555)
		defer func() {
			_ = os.Chmod(filepath.Dir(name), 0700)
			put(t, name, saved)
			_ = os.Chmod(name, 0444)
			_ = os.Chmod(filepath.Dir(name), 0555)
		}()
		got, _ := runtime.Convert(context.Background(), request, w.WorkspaceID)
		if got.Result.Status != ingest.StatusUnavailable {
			t.Fatal(got.Result)
		}
	})
	t.Run("real offline conversion failure", func(t *testing.T) {
		var b bytes.Buffer
		z := zip.NewWriter(&b)
		for _, name := range []string{"[Content_Types].xml", "word/document.xml"} {
			f, _ := z.Create(name)
			_, _ = f.Write([]byte("invalid XML"))
		}
		_ = z.Close()
		put(t, filepath.Join(space, "invalid.docx"), b.Bytes())
		r := request
		r.SourcePath = "invalid.docx"
		got, _ := runtime.Convert(context.Background(), r, w.WorkspaceID)
		if got.Result.Status != ingest.StatusUnavailable || len(got.Document) != 0 {
			t.Fatal(got.Result)
		}
	})
	if destination := os.Getenv("MAESTRO_DOCLING_TEST_EVIDENCE"); destination != "" {
		evidence, _ := json.MarshalIndent(map[string]any{"assembly": assembled, "result": converted.Result, "provenance": converted.Provenance, "authority": "ephemeral engineering fixture; not production", "oracle": "exact marker and 2x2 Item/Count/alpha/7 table"}, "", "  ")
		put(t, destination, evidence)
	}
}
