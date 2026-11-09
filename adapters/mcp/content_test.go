package product_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func contentCall(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any, fail bool) map[string]any {
	t.Helper()
	out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError != fail {
		t.Fatalf("%s: %+v content=%v", args["operation"], out, out.Content)
	}
	var value map[string]any
	if len(out.Content) != 1 {
		t.Fatalf("unexpected content: %+v", out)
	}
	if err := json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func TestContentUnseededStatus(t *testing.T) {
	f := newOwnerFixture(t, false)
	cs := f.connect(t, nil)
	got := contentCall(t, cs, "maestro_query", map[string]any{"operation": "content.status"}, false)
	if got["state"] != "unavailable" {
		t.Fatalf("unseeded runtime: %v", got)
	}
	assertPolicyCoverage(t, got, "unavailable", "authority_unavailable")
	contentCall(t, cs, "maestro_apply", map[string]any{"operation": "content.convert", "source_path": "../outside.docx"}, false)
	contentCall(t, cs, "maestro_query", map[string]any{"operation": "content.status", "authority": "model"}, true)
	if runtime.GOOS == "darwin" {
		t.Run("portable_mcpb_readonly", func(t *testing.T) {
			base := physicalTempDir(t)
			binary := filepath.Join(base, "maestro-mcp")
			if out, err := exec.Command("go", "build", "-o", binary, "./cmd/maestro-mcp").CombinedOutput(); err != nil {
				t.Fatalf("actual unseeded build: %s %v", out, err)
			}
			packet := packaging.Options{Destination: filepath.Join(base, "readback.mcpb"), Server: binary, Version: "0.1.0", Binding: f.options}
			if err := packaging.BuildExtension(packet); err != nil {
				t.Fatal(err)
			}
			desktop := filepath.Join(base, "desktop [space] $() &")
			extractProductPacketFixture(t, packet.Destination, desktop)
			before := contentTree(t, desktop, true)
			fixture := f
			fixture.binary = filepath.Join(desktop, "server/maestro-mcp")
			client := connectContentMCPB(t, fixture, desktop)
			status := contentCall(t, client, "maestro_query", map[string]any{"operation": "content.status"}, false)
			assertPolicyCoverage(t, status, "unavailable", "authority_unavailable")
			context := contentCall(t, client, "maestro_context", map[string]any{}, false)
			if context["workspace_id"] == nil || context["workspace_id"] == "" {
				t.Fatal("emitted readback has no admitted workspace")
			}
			if err := client.Close(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, contentTree(t, desktop, false)) {
				t.Fatal("readback changed managed file bytes or modes")
			}
		})
	}
}

func assertPolicyCoverage(t *testing.T, got map[string]any, state, reason string) {
	t.Helper()
	want := map[string][]string{
		".csv":  {"docling_local", "docling.csv", "structured", ""},
		".docx": {"docling_local", "docling.docx", "structured", ""},
		".epub": {"docling_local", "docling.epub", "structured", ""},
		".htm":  {"docling_local", "docling.html", "structured", ""},
		".html": {"docling_local", "docling.html", "structured", ""},
		".json": {"maestro_literal_v1", "maestro.literal_utf8.v1", "textual", "native_generic_literal_unsupported"},
		".md":   {"docling_local", "docling.markdown", "structured", ""},
		".txt":  {"maestro_literal_v1", "maestro.literal_utf8.v1", "textual", "native_generic_literal_unsupported"},
		".xls":  {"maestro_biff_v1", "xlrd.biff", "textual", "native_xls_requires_unmanaged_office"},
		".xlsx": {"docling_local", "docling.xlsx", "structured", ""},
		".xml":  {"maestro_literal_v1", "maestro.literal_utf8.v1", "textual", "native_generic_literal_unsupported"},
	}
	formats, ok := got["formats"].([]any)
	if !ok || len(formats) != len(want) {
		t.Fatalf("public installed format coverage missing: %v", got)
	}
	previous := ""
	for _, entry := range formats {
		format := entry.(map[string]any)
		ext := format["extension"].(string)
		expected, exists := want[ext]
		if !exists || ext <= previous || format["state"] != state || reason != "" && format["reason"] != reason {
			t.Fatalf("format availability: %v", format)
		}
		previous = ext
		for i, field := range []string{"route", "parser", "fidelity", "fallback_reason"} {
			if expected[i] != "" && format[field] != expected[i] || expected[i] == "" && format[field] != nil {
				t.Fatalf("%s %s: %v", ext, field, format)
			}
		}
		if ext == ".json" {
			alternatives, ok := format["alternatives"].([]any)
			if !ok || len(alternatives) != 1 {
				t.Fatal("validated Docling JSON alternative missing", format)
			}
			alternative := alternatives[0].(map[string]any)
			if alternative["condition"] != "validated_docling_document" || alternative["route"] != "docling_local" || alternative["parser"] != "docling.json" || alternative["fidelity"] != "structured" {
				t.Fatal("JSON export misrepresented", alternative)
			}
		}
	}
}

func copyContentFixture(t *testing.T, source, target string) {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			dirs = append(dirs, path)
			return os.MkdirAll(dest, 0700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("fixture must contain only regular inputs: %s", path)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err = os.WriteFile(dest, body, 0600); err != nil {
			return err
		}
		return os.Chmod(dest, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		rel, _ := filepath.Rel(source, dirs[i])
		info, err := os.Stat(dirs[i])
		if err != nil {
			t.Fatal(err)
		}
		if err = os.Chmod(filepath.Join(target, rel), info.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
	}
}
func putContentFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
}
func readContentFile(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
func contentTree(t *testing.T, root string, readonly bool) map[string]string {
	t.Helper()
	out := map[string]string{}
	// Managed file modes are part of the packet inventory. Protect the namespace
	// with read-only directories and detect file writes/mode changes by snapshot;
	// do not manufacture a different distribution mode just for this fixture.
	if readonly {
		if _, err := packaging.ValidatePacket(root); err != nil {
			t.Fatal(err)
		}
	}
	if err := filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if readonly {
				return os.Chmod(name, 0500)
			}
			return nil
		}
		body, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, name)
		out[rel] = fmt.Sprintf("%v:%x", info.Mode(), sha256.Sum256(body))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				_ = os.Chmod(name, 0700)
			}
			return err
		})
	})
	return out
}
func connectContent(t *testing.T, f ownerFixture, options *mcp.ClientOptions, protocol string) *mcp.ClientSession {
	t.Helper()
	var transport mcp.Transport
	if f.binary != "" {
		transport = &mcp.CommandTransport{Command: exec.Command(f.binary, "serve", "--product-root", f.options.ProductRoot, "--user-home", f.options.UserHome, "--workspace", f.options.WorkspacePath), TerminateDuration: time.Second}
	} else {
		bound, err := product.Open(f.options)
		if err != nil {
			t.Fatal(err)
		}
		a, z := mcp.NewInMemoryTransports()
		transport = z
		server, err := bound.Server().Connect(context.Background(), a, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { server.Close() })
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "content-engineering-test", Version: "1"}, options).Connect(context.Background(), transport, &mcp.ClientSessionOptions{ProtocolVersion: protocol})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}
func TestContentRealJourneys(t *testing.T) {
	retained, seed := os.Getenv("MAESTRO_DOCLING_RETAINED_ROOT"), os.Getenv("MAESTRO_DOCLING_RETAINED_SEED")
	if retained == "" || seed == "" {
		t.Skip("explicit final retained authenticated engineering fixture and independent registry seed required")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("retained real Docling fixture requires darwin/arm64")
	}
	authority := readContentFile(t, filepath.Join(retained, "authority.json"))
	if fmt.Sprintf("%x", sha256.Sum256(authority)) != seed {
		t.Fatal("independent authority seed mismatch")
	}
	base := physicalTempDir(t)
	binary := filepath.Join(base, "server", "maestro-mcp")
	if err := os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-mod=readonly", "-ldflags", "-X main.AuthorityRegistrySHA256="+seed+" -X main.Version=0.1.0 -X 'main.CompatibleBundle=>=0.1.0 <0.2.0'", "-o", binary, "./cmd/maestro-mcp")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("current seeded build: %s %v", out, err)
	}
	putContentFile(t, filepath.Join(base, "managed/trust/release-authority-registry.json"), authority)
	t.Logf("current seeded binary sha256=%x", sha256.Sum256(readContentFile(t, binary)))
	for _, mode := range []string{"sdk", "compiled_stdio"} {
		t.Run(mode, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			data := filepath.Join(filepath.Dir(f.root), "state")
			sourceBase := filepath.Join(retained, "state/ingestion/docling")
			targetBase := filepath.Join(data, "ingestion/docling")
			manifestBody := readContentFile(t, filepath.Join(sourceBase, "release-manifest.json"))
			var manifest releasecontract.Manifest
			if err := json.Unmarshal(manifestBody, &manifest); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"release-manifest.json", "release-manifest.json.sig"} {
				putContentFile(t, filepath.Join(targetBase, name), readContentFile(t, filepath.Join(sourceBase, name)))
			}
			var packSHA string
			for _, artifact := range manifest.Artifacts {
				if artifact.Kind == "runtime_pack" && artifact.OS == runtime.GOOS && artifact.Arch == runtime.GOARCH {
					packSHA = artifact.SHA256
					for _, name := range []string{artifact.Name, artifact.SignatureRef} {
						putContentFile(t, filepath.Join(targetBase, name), readContentFile(t, filepath.Join(sourceBase, name)))
					}
					copyContentFixture(t, filepath.Join(sourceBase, "installed", packSHA), filepath.Join(targetBase, "installed", packSHA))
				}
			}
			if packSHA == "" {
				t.Fatal("fixture has no concrete runtime")
			}
			t.Cleanup(func() {
				_ = filepath.WalkDir(targetBase, func(name string, item os.DirEntry, err error) error {
					if err == nil && item.IsDir() {
						_ = os.Chmod(name, 0700)
					}
					return err
				})
			})
			source := readContentFile(t, filepath.Join(retained, "workspace/sample.docx"))
			putContentFile(t, filepath.Join(f.options.WorkspacePath, "sample.docx"), source)
			f.options.AuthorityRegistryPath = filepath.Join(base, "managed/trust/release-authority-registry.json")
			f.options.AuthorityRegistrySHA256 = seed
			f.options.CLIVersion = "0.1.0"
			var packetRoot string
			if mode == "compiled_stdio" {
				packetRoot = filepath.Join(filepath.Dir(f.root), "..", "..", "content-plugin")
				packetRoot = filepath.Clean(packetRoot)
				packet := packaging.Options{Destination: packetRoot, Server: binary, Version: "0.1.0", Binding: f.options}
				if err := packaging.BuildPlugin(packet); err != nil {
					t.Fatal(err)
				}
				f.binary = filepath.Join(packetRoot, "scripts/maestro-mcp")
			}
			var before map[string]string
			if packetRoot != "" {
				before = contentTree(t, packetRoot, true)
			}
			progress := contentConversionProgress{terminal: make(chan struct{}, 1)}
			reviewed := 0
			expectedText := ""
			host := func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				if !strings.Contains(req.Params.Message, "sample.docx") || !strings.Contains(req.Params.Message, "knowledge/") || expectedText != "" && !strings.Contains(req.Params.Message, expectedText) {
					t.Errorf("incomplete domain review: %s", req.Params.Message)
				}
				if strings.Contains(req.Params.Message, "session_id") || strings.Contains(req.Params.Message, f.options.UserHome) {
					t.Error("domain form leaked internal proof/root")
				}
				reviewed++
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}
			options := &mcp.ClientOptions{ElicitationHandler: host, ProgressNotificationHandler: progress.record}
			cs := connectContent(t, f, options, "")
			params := &mcp.CallToolParams{Name: "maestro_apply", Arguments: map[string]any{"operation": "content.convert", "source_path": "sample.docx"}}
			params.SetProgressToken("engineering-conversion")
			started := time.Now()
			converted, err := cs.CallTool(context.Background(), params)
			if err != nil || converted.IsError {
				t.Fatalf("actual converter: %+v %v", converted, err)
			}
			t.Logf("actual authenticated conversion elapsed=%s", time.Since(started))
			// Wire names are deliberately decoded independently from the production envelope.
			var wire map[string]any
			if err := json.Unmarshal([]byte(converted.Content[0].(*mcp.TextContent).Text), &wire); err != nil {
				t.Fatal(err)
			}
			receipt := wire["result"].(map[string]any)
			if receipt["status"] != "usable" || receipt["source_sha256"] != "6bd282a12170035e8af5352f2de5a21f4fe4bc5c7919283482a26703c206bf81" {
				t.Fatalf("real converter receipt: %v", wire)
			}
			id := receipt["artifact_ref"].(string)
			provenance := wire["provenance"].(map[string]any)
			if provenance["pack_sha256"] != packSHA || provenance["network_denied"] != true {
				t.Fatal(provenance)
			}
			// The SDK may complete CallTool before its queued progress handler.
			progressCtx, cancelProgress := context.WithTimeout(t.Context(), 30*time.Second)
			gotProgress, progressErr := progress.wait(progressCtx)
			cancelProgress()
			if progressErr != nil {
				t.Fatalf("pinned SDK terminal progress missing: %v (%v)", gotProgress, progressErr)
			}
			if len(gotProgress) < 2 || gotProgress[0] != 0 || gotProgress[len(gotProgress)-1] != 2 {
				t.Fatalf("pinned SDK progress missing: %v", gotProgress)
			}
			var markdown string
			offset := 0
			for {
				page := contentCall(t, cs, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": id, "format": "markdown", "offset": offset, "max_bytes": 4}, false)
				text := page["text"].(string)
				if len(text) > 4 {
					t.Fatal("unbounded artifact slice")
				}
				markdown += text
				offset = int(page["next_offset"].(float64))
				if page["has_more"] == false {
					break
				}
			}
			if !strings.Contains(markdown, `MAESTRO\_DOCLING\_OFFLINE\_20261101`) || !strings.Contains(markdown, "alpha") || !strings.Contains(markdown, "7") {
				t.Fatalf("independent document oracle: %s", markdown)
			}
			document := contentCall(t, cs, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": id, "format": "document"}, false)
			if !strings.Contains(document["text"].(string), "MAESTRO_DOCLING_OFFLINE_20261101") {
				t.Fatal("structured document lost")
			}
			draft := "# Reviewed local evidence\n\n## Evidence\n" + markdown
			create := map[string]any{"operation": "knowledge.create", "artifact_id": id, "page": "knowledge/docling.md", "text": draft}
			expectedText = "Reviewed local evidence"
			written := contentCall(t, cs, "maestro_apply", create, false)
			if written["state"] != "written" {
				t.Fatal(written)
			}
			expectedText = "Revisão UTF-8: ação e evidência"
			appendArgs := map[string]any{"operation": "knowledge.append", "artifact_id": id, "page": "knowledge/docling.md", "section": "## Evidence", "text": expectedText, "expected_page_revision": written["revision"]}
			appended := contentCall(t, cs, "maestro_apply", appendArgs, false)
			if appended["state"] != "written" || reviewed != 2 {
				t.Fatalf("review/effect: %v reviews=%d", appended, reviewed)
			}
			// Close the transport after the effect, then retry the identical intent under a new server.
			cs.Close()
			restarted := connectContent(t, f, options, "")
			replay := contentCall(t, restarted, "maestro_apply", appendArgs, false)
			if replay["state"] != "unchanged" || replay["operation_id"] != appended["operation_id"] || fmt.Sprint(replay["provenance"]) != fmt.Sprint(appended["provenance"]) {
				t.Fatalf("reconnect changed canonical intent: %v -> %v", appended, replay)
			}
			collected := contentCall(t, restarted, "maestro_query", map[string]any{"operation": "knowledge.collect", "purpose": "Read reviewed source evidence after restart", "pages": []string{"knowledge/docling.md"}}, false)
			knowledge := collected["pages"].([]any)[0].(map[string]any)
			if knowledge["revision"] != appended["revision"] || strings.Count(knowledge["content"].(string), expectedText) != 1 || !strings.Contains(knowledge["content"].(string), markdown) {
				t.Fatalf("restart lost or duplicated knowledge: %v", knowledge)
			}
			afterArtifact := contentCall(t, restarted, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": id, "format": "markdown"}, false)
			if fmt.Sprint(afterArtifact["provenance"]) != fmt.Sprint(provenance) {
				t.Fatal("artifact provenance changed after restart")
			}

			// No host form and explicit decline cannot write a new page.
			declined := connectContent(t, f, &mcp.ClientOptions{ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "decline"}, nil
			}}, "")
			rejectArgs := map[string]any{"operation": "knowledge.create", "artifact_id": id, "page": "knowledge/refused.md", "text": "must not appear"}
			for _, session := range []*mcp.ClientSession{declined, connectContent(t, f, nil, "")} {
				contentCall(t, session, "maestro_apply", rejectArgs, true)
			}
			if _, err := os.Stat(filepath.Join(f.options.WorkspacePath, "brain/knowledge/refused.md")); !os.IsNotExist(err) {
				t.Fatal("declined/no-form write occurred")
			}
			for _, field := range []string{"approved", "session_id", "workspace_id", "reader", "authority", "origin"} {
				args := map[string]any{}
				for k, v := range rejectArgs {
					args[k] = v
				}
				args[field] = "model"
				contentCall(t, restarted, "maestro_apply", args, true)
			}

			// Review changes preserve the page, and canonical stale revisions remain proposals.
			expectedText = "stale draft"
			stale := map[string]any{"operation": "knowledge.append", "artifact_id": id, "page": "knowledge/docling.md", "section": "## Evidence", "text": expectedText, "expected_page_revision": written["revision"]}
			conflict := contentCall(t, restarted, "maestro_apply", stale, false)
			if conflict["state"] != "proposed" {
				t.Fatal("stale revision was applied", conflict)
			}
			concurrentText := knowledge["content"].(string) + "\nOWNER NEWER CONTENT\n"
			racing := connectContent(t, f, &mcp.ClientOptions{ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				putContentFile(t, filepath.Join(f.options.WorkspacePath, "brain/knowledge/docling.md"), []byte(concurrentText))
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}}, "")
			raceArgs := map[string]any{"operation": "knowledge.append", "artifact_id": id, "page": "knowledge/docling.md", "section": "## Evidence", "text": "must lose concurrent race", "expected_page_revision": appended["revision"]}
			contentCall(t, racing, "maestro_apply", raceArgs, true)
			if string(readContentFile(t, filepath.Join(f.options.WorkspacePath, "brain/knowledge/docling.md"))) != concurrentText {
				t.Fatal("concurrent edit lost")
			}

			bound, err := product.Open(f.options)
			if err != nil {
				t.Fatal(err)
			}
			local := product.KnowledgeMutation{Operation: "knowledge.create", ArtifactID: id, Page: "knowledge/local.md", Text: "Local operator reviewed evidence"}
			review, err := bound.KnowledgeReview(local)
			if err != nil {
				t.Fatal(err)
			}
			changed := local
			changed.Page = "knowledge/retarget.md"
			if _, err := bound.ApplyKnowledgeReview(changed, review.ReviewDigest, true); err == nil {
				t.Fatal("retargeted local approval accepted")
			}
			changed = local
			changed.Text = "modified draft"
			if _, err := bound.ApplyKnowledgeReview(changed, review.ReviewDigest, true); err == nil {
				t.Fatal("changed draft approval accepted")
			}
			if _, err := bound.ApplyKnowledgeReview(local, review.ReviewDigest, false); err == nil {
				t.Fatal("local confirmation omitted")
			}

			foreign := f
			foreign.binary = ""
			foreign.options.WorkspacePath = filepath.Join(filepath.Dir(f.options.WorkspacePath), "second-workspace")
			if _, err := workspace.Initialize(workspace.Options{WorkspacePath: foreign.options.WorkspacePath, DataRoot: data}); err != nil {
				t.Fatal(err)
			}
			foreignClient := connectContent(t, foreign, nil, "")
			contentCall(t, foreignClient, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": id, "format": "markdown"}, true)
			foreignBound, err := product.Open(foreign.options)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := foreignBound.ApplyKnowledgeReview(local, review.ReviewDigest, true); err == nil {
				t.Fatal("foreign workspace accepted approval")
			}
			ownMeta := contentCall(t, restarted, "maestro_context", map[string]any{}, false)
			otherMeta := contentCall(t, foreignClient, "maestro_context", map[string]any{}, false)
			artifactPath := filepath.Join(data, "content", ownMeta["workspace_id"].(string), "artifacts", id+".json")
			originalArtifact := readContentFile(t, artifactPath)
			putContentFile(t, filepath.Join(data, "content", otherMeta["workspace_id"].(string), "artifacts", id+".json"), originalArtifact)
			contentCall(t, foreignClient, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": id, "format": "markdown"}, true)
			putContentFile(t, artifactPath, append(append([]byte(nil), originalArtifact...), ' '))
			contentCall(t, restarted, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": id, "format": "markdown"}, true)
			if _, err := bound.ApplyKnowledgeReview(local, review.ReviewDigest, true); err == nil {
				t.Fatal("tampered artifact accepted")
			}
			putContentFile(t, artifactPath, originalArtifact)
			putContentFile(t, filepath.Join(f.options.WorkspacePath, "sample.docx"), []byte("changed source"))
			if _, err := bound.ApplyKnowledgeReview(local, review.ReviewDigest, true); err == nil {
				t.Fatal("changed source accepted")
			}
			putContentFile(t, filepath.Join(f.options.WorkspacePath, "sample.docx"), source)

			// A same-bytes alias created after exact review must revoke source eligibility.
			sourcePath := filepath.Join(f.options.WorkspacePath, "sample.docx")
			if err := os.Chmod(sourcePath, 0644); err != nil {
				t.Fatal(err)
			}
			aliasMutation := local
			aliasMutation.Page = "knowledge/source-alias.md"
			aliasReview, err := bound.KnowledgeReview(aliasMutation)
			if err != nil {
				t.Fatal(err)
			}
			beforeKnowledge := contentTree(t, filepath.Join(f.options.WorkspacePath, "brain"), false)
			beforeArtifacts := contentTree(t, filepath.Dir(artifactPath), false)
			alias := filepath.Join(filepath.Dir(f.options.WorkspacePath), "same-bytes-source-link.docx")
			if err := os.Link(sourcePath, alias); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Remove(alias) })
			_, aliasErr := bound.ApplyKnowledgeReview(aliasMutation, aliasReview.ReviewDigest, true)
			if err := os.Remove(alias); err != nil {
				t.Fatal(err)
			}
			if aliasErr == nil {
				t.Fatal("hardlinked source retained prior review authority")
			}
			if fmt.Sprint(beforeKnowledge) != fmt.Sprint(contentTree(t, filepath.Join(f.options.WorkspacePath, "brain"), false)) || fmt.Sprint(beforeArtifacts) != fmt.Sprint(contentTree(t, filepath.Dir(artifactPath), false)) {
				t.Fatal("alias refusal changed knowledge or artifacts")
			}
			freshReview, err := bound.KnowledgeReview(aliasMutation)
			if err != nil {
				t.Fatal("restored ordinary readable source cannot be reviewed", err)
			}
			if restored, err := bound.ApplyKnowledgeReview(aliasMutation, freshReview.ReviewDigest, true); err != nil || restored.State != "written" {
				t.Fatal("fresh legitimate source review failed", restored, err)
			}
			if err := os.Chmod(sourcePath, 0600); err != nil {
				t.Fatal(err)
			}
			t.Log("post-review same-bytes source hardlink refused without knowledge/artifact changes; restored readable source accepted fresh review")

			// The operator path uses actual stdin and exact digest flags in the current binary.
			if mode == "compiled_stdio" {
				payload, _ := json.Marshal(local)
				cli := func(command string, extra []string, body []byte, fail bool) []byte {
					args := append([]string{command, "--product-root", f.options.ProductRoot, "--user-home", f.options.UserHome, "--workspace", f.options.WorkspacePath}, extra...)
					cmd := exec.Command(f.binary, args...)
					cmd.Stdin = bytes.NewReader(body)
					out, err := cmd.CombinedOutput()
					if (err != nil) != fail {
						t.Fatalf("%s: %s %v", command, out, err)
					}
					return out
				}
				var operatorReview struct {
					ReviewDigest string `json:"review_digest"`
				}
				if err := json.Unmarshal(cli("knowledge-review", nil, payload, false), &operatorReview); err != nil {
					t.Fatal(err)
				}
				cli("knowledge-apply", []string{"--review-digest", operatorReview.ReviewDigest}, payload, true)
				cli("knowledge-apply", []string{"--review-digest", strings.Repeat("0", 64), "--confirm"}, payload, true)
				cli("knowledge-apply", []string{"--review-digest", operatorReview.ReviewDigest, "--confirm"}, payload, false)
				packet := packaging.Options{Destination: filepath.Join(filepath.Dir(packetRoot), "content.mcpb"), Server: binary, Version: "0.1.0", Binding: f.options}
				if err := packaging.BuildExtension(packet); err != nil {
					t.Fatal(err)
				}
				desktop := filepath.Join(filepath.Dir(packetRoot), "desktop")
				extractProductPacketFixture(t, packet.Destination, desktop)
				contentTree(t, desktop, true)
				desktopFixture := f
				desktopFixture.binary = filepath.Join(desktop, "server/maestro-mcp")
				desktopClient := connectContentMCPB(t, desktopFixture, desktop)
				status := contentCall(t, desktopClient, "maestro_query", map[string]any{"operation": "content.status"}, false)
				if status["state"] != "ready" || status["pack_sha256"] != packSHA {
					t.Fatalf("Desktop packet fixed trust seed: %v", status)
				}
				contentCall(t, desktopClient, "maestro_query", map[string]any{"operation": "knowledge.collect", "purpose": "Desktop readback", "pages": []string{"knowledge/local.md"}}, false)
				after := contentTree(t, packetRoot, false)
				if fmt.Sprint(before) != fmt.Sprint(after) {
					t.Fatal("managed packet changed")
				}
			}
			expectedText = "Legacy reviewed evidence"
			legacy := connectContent(t, f, &mcp.ClientOptions{ElicitationHandler: host}, "2025-11-25")
			legacyArgs := map[string]any{"operation": "knowledge.create", "artifact_id": id, "page": "knowledge/legacy.md", "text": expectedText}
			if got := contentCall(t, legacy, "maestro_apply", legacyArgs, false); got["state"] != "written" {
				t.Fatal(got)
			}
			expectedText = "Large reviewed page"
			largeText := "# Large reviewed page\n\n## Evidence\n" + strings.Repeat("a\t", 90<<10)
			largeArgs := map[string]any{"operation": "knowledge.create", "artifact_id": id, "page": "knowledge/large.md", "text": largeText}
			large := contentCall(t, restarted, "maestro_apply", largeArgs, false)
			largeMutation := product.KnowledgeMutation{Operation: "knowledge.append", ArtifactID: id, Page: "knowledge/large.md", Section: "## Evidence", Text: "Complete large page reviewed", ExpectedPageRevision: large["revision"].(string)}
			largeReview, err := bound.KnowledgeReview(largeMutation)
			if err != nil || largeReview.Current == nil || largeReview.Current.Content != largeText {
				t.Fatal("large page full review unavailable", err)
			}
			expectedText = largeMutation.Text
			contentCall(t, restarted, "maestro_apply", map[string]any{"operation": largeMutation.Operation, "artifact_id": id, "page": largeMutation.Page, "section": largeMutation.Section, "text": largeMutation.Text, "expected_page_revision": largeMutation.ExpectedPageRevision}, false)
			// A stale append may expose a full proposal only to the private operator.
			largePath := filepath.Join(f.options.WorkspacePath, "brain", "knowledge/large.md")
			largeBefore := readContentFile(t, largePath)
			largeMutation.Text = "Stale large-page draft"
			expectedText = largeMutation.Text
			staleLargeReview, err := bound.KnowledgeReview(largeMutation)
			if err != nil || staleLargeReview.Current == nil || staleLargeReview.Current.Content != string(largeBefore) {
				t.Fatal("stale large-page private review unavailable", err)
			}
			publicConflict := contentCall(t, restarted, "maestro_apply", map[string]any{"operation": largeMutation.Operation, "artifact_id": id, "page": largeMutation.Page, "section": largeMutation.Section, "text": largeMutation.Text, "expected_page_revision": largeMutation.ExpectedPageRevision}, false)
			if publicConflict["state"] != "proposed" || publicConflict["revision"] != staleLargeReview.Current.Revision || publicConflict["reason"] == "" || publicConflict["operation_id"] == "" || publicConflict["operation"] != "append-entry" || publicConflict["page"] != largeMutation.Page || publicConflict["provenance"] == nil {
				t.Fatal("stale large-page conflict metadata missing")
			}
			publicBody, err := json.Marshal(publicConflict)
			if err != nil {
				t.Fatal(err)
			}
			if _, exposed := publicConflict["proposal"]; exposed || bytes.Contains(publicBody, []byte("Large reviewed page")) || bytes.Contains(publicBody, []byte(largeMutation.Text)) {
				t.Fatal("MCP conflict exposed private large-page proposal")
			}
			// Use a distinct intent so the canonical receipt replay cannot hide a proposal.
			largeMutation.Text = "Private operator stale draft"
			privateReview, err := bound.KnowledgeReview(largeMutation)
			if err != nil {
				t.Fatal(err)
			}
			privateConflict, err := bound.ApplyKnowledgeReview(largeMutation, privateReview.ReviewDigest, true)
			if err != nil || privateConflict.State != "proposed" || len(privateConflict.Proposal) <= 128<<10 || !strings.Contains(privateConflict.Proposal, string(largeBefore)) || !strings.Contains(privateConflict.Proposal, largeMutation.Text) {
				t.Fatal("private operator canonical proposal unavailable", err)
			}
			if !bytes.Equal(largeBefore, readContentFile(t, largePath)) {
				t.Fatal("stale large-page append changed canonical page")
			}
			t.Log("MCP stale large-page conflict preserves metadata without proposal; private operator retains full canonical proposal; page unchanged")
			collectedLarge := contentCall(t, restarted, "maestro_query", map[string]any{"operation": "knowledge.collect", "purpose": "Verify ordinary budget remains bounded", "pages": []string{"knowledge/large.md"}}, false)
			if len(collectedLarge["omissions"].([]any)) != 1 {
				t.Fatal("ordinary collection budget widened")
			}

			// Cancel an actual admitted conversion at the supported progress notification.
			cancelCtx, cancel := context.WithCancel(context.Background())
			cancelClient := connectContent(t, f, &mcp.ClientOptions{ProgressNotificationHandler: func(context.Context, *mcp.ProgressNotificationClientRequest) { cancel() }}, "")
			cancellation := &mcp.CallToolParams{Name: "maestro_apply", Arguments: map[string]any{"operation": "content.convert", "source_path": "sample.docx"}}
			cancellation.SetProgressToken("cancel-real-conversion")
			cancelStarted := time.Now()
			cancelResult, cancelErr := cancelClient.CallTool(cancelCtx, cancellation)
			cancel()
			cancelClient.Close()
			if cancelErr == nil && (cancelResult == nil || !cancelResult.IsError) {
				t.Fatal("caller cancellation did not refuse conversion")
			}
			entries, err := os.ReadDir(filepath.Dir(artifactPath))
			if err != nil || len(entries) != 1 {
				t.Fatal("cancelled conversion published an artifact", err)
			}
			t.Logf("actual protocol cancellation returned in %s; immutable artifact count unchanged", time.Since(cancelStarted))
			if !bytes.Equal(source, readContentFile(t, filepath.Join(f.options.WorkspacePath, "sample.docx"))) {
				t.Fatal("source changed")
			}
			if entries, err := os.ReadDir(f.options.ProductRoot); err != nil || len(entries) != 0 {
				t.Fatal("managed product was written")
			}
			t.Logf("reviewed knowledge and immutable artifact readback passed; pack=%s; protocol=%s", packSHA, restarted.InitializeResult().ProtocolVersion)
		})
	}
}

func TestKnowledgeMutationCanonicalPageBudget(t *testing.T) {
	request := map[string]any{"operation": "knowledge.create", "artifact_id": strings.Repeat("a", 64), "page": "knowledge/large.md", "text": strings.Repeat("\t", 256<<10)}
	body, _ := json.Marshal(request)
	if _, err := product.DecodeKnowledgeMutation(body); err != nil {
		t.Fatalf("canonical 256KiB page with JSON escaping refused: %v", err)
	}
	request["text"] = strings.Repeat("a", (256<<10)+1)
	body, _ = json.Marshal(request)
	if _, err := product.DecodeKnowledgeMutation(body); err == nil {
		t.Fatal("oversized page admitted")
	}
}

func TestKnowledgeMutationRequiresClosedUTF8Request(t *testing.T) {
	request := map[string]any{"operation": "knowledge.append", "artifact_id": strings.Repeat("a", 64), "page": "knowledge/review.md", "section": "## Evidence", "text": "Revisão: ação e evidência", "expected_page_revision": strings.Repeat("b", 64)}
	body, _ := json.Marshal(request)
	decoded, err := product.DecodeKnowledgeMutation(body)
	if err != nil || decoded.Text != request["text"] {
		t.Fatal("valid UTF-8 request changed or refused", err)
	}
	for _, field := range []string{"approved", "workspace_id", "source_path", "origin"} {
		request[field] = "model-supplied"
		body, _ := json.Marshal(request)
		if _, err := product.DecodeKnowledgeMutation(body); err == nil {
			t.Errorf("unowned %s admitted", field)
		}
		delete(request, field)
	}
	request["text"] = strings.Repeat("a", 8<<10)
	body, _ = json.Marshal(request)
	if _, err := product.DecodeKnowledgeMutation(body); err != nil {
		t.Fatal("bounded append refused", err)
	}
	request["text"] = strings.Repeat("a", (8<<10)+1)
	body, _ = json.Marshal(request)
	if _, err := product.DecodeKnowledgeMutation(body); err == nil {
		t.Fatal("oversized append admitted")
	}
	request["text"] = "selected-UTF8-marker"
	body, _ = json.Marshal(request)
	for _, malformed := range [][]byte{bytes.Replace(body, []byte("selected-UTF8-marker"), []byte{0xff}, 1), append(body, []byte("{}")...)} {
		if _, err := product.DecodeKnowledgeMutation(malformed); err == nil {
			t.Fatal("malformed UTF-8 or trailing JSON admitted")
		}
	}
	delete(request, "expected_page_revision")
	body, _ = json.Marshal(request)
	if _, err := product.DecodeKnowledgeMutation(body); err == nil {
		t.Fatal("unfenced append admitted")
	}
}

// Readback uses the actual emitted MCPB configuration and selected external
// binding. This is a real packet executable test, not Desktop loader acceptance.
func connectContentMCPB(t *testing.T, f ownerFixture, root string) *mcp.ClientSession {
	t.Helper()
	if status, err := packaging.ValidatePacket(root); err != nil || status.Format != "mcpb" {
		t.Fatalf("complete MCPB unavailable: %+v %v", status, err)
	}
	var manifest struct {
		Server struct {
			Type   string `json:"type"`
			Entry  string `json:"entry_point"`
			Config struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
			} `json:"mcp_config"`
		} `json:"server"`
	}
	if err := json.Unmarshal(readContentFile(t, filepath.Join(root, "manifest.json")), &manifest); err != nil {
		t.Fatal(err)
	}
	server := manifest.Server
	wantArgs := []string{"serve", "--packet", "--product-root", "${__dirname}", "--user-home", "${user_config.user_home}", "--workspace", "${user_config.workspace}"}
	if server.Type != "binary" || server.Entry != "server/"+productTestBinaryName() || server.Config.Command != "${__dirname}/"+server.Entry || !reflect.DeepEqual(server.Config.Args, wantArgs) {
		t.Fatal("MCPB readback changed the exact emitted executable/argv contract")
	}
	replace := strings.NewReplacer("${__dirname}", root, "${user_config.user_home}", f.options.UserHome, "${user_config.workspace}", f.options.WorkspacePath)
	command := replace.Replace(server.Config.Command)
	if command != f.binary {
		t.Fatal("MCPB readback selected another executable")
	}
	for i := range server.Config.Args {
		server.Config.Args[i] = replace.Replace(server.Config.Args[i])
	}
	client, err := mcp.NewClient(&mcp.Implementation{Name: "content-mcpb-readback", Version: "1"}, nil).Connect(context.Background(), &mcp.CommandTransport{Command: exec.Command(command, server.Config.Args...), TerminateDuration: time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}
