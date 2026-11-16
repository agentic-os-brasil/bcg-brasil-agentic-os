package main

import (
	"archive/zip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The explicit fixture is read-only; the fresh key exists only in test memory.
func TestEngineeringRuntimePilotRealJourney(t *testing.T) {
	retained, seed := os.Getenv("MAESTRO_ENGINEERING_PILOT_RETAINED_ROOT"), os.Getenv("MAESTRO_ENGINEERING_PILOT_RETAINED_REGISTRY_SHA256")
	if retained == "" || seed == "" {
		t.Skip("explicit authenticated retained runtime and independent public registry digest required")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("the retained engineering pilot runtime requires Darwin ARM64")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	old := docling.New(docling.Options{DataRoot: filepath.Join(retained, "state"), AuthorityRegistryPath: filepath.Join(retained, "authority.json"), AuthorityRegistrySHA256: seed, CLIVersion: "0.1.0"}).Status(ctx)
	if old.State != "ready" {
		t.Fatalf("retained authenticated runtime unavailable: %+v", old)
	}
	_, source, _, _ := runtime.Caller(0)
	sourceRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "..", ".."))
	base := os.Getenv("MAESTRO_ENGINEERING_PILOT_PROOF")
	if base == "" {
		base = filepath.Join(t.TempDir(), "proof")
	}
	if err := os.Mkdir(base, 0700); err != nil {
		t.Fatal("proof destination must be new", err)
	}
	save := func(name string, value any) {
		t.Helper()
		b, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		writePilotFixture(t, filepath.Join(base, name+".json"), append(b, '\n'))
	}
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	issued := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	registry, _ := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "maestro-engineering-pilot-20261116", "key_id": "pilot-20261116", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": issued.Format(time.RFC3339), "valid_until": issued.Add(14 * 24 * time.Hour).Format(time.RFC3339)}}})
	registryPath := filepath.Join(base, "public-registry.json")
	writePilotFixture(t, registryPath, registry)
	var oldManifest releasecontract.Manifest
	oldBody, err := os.ReadFile(filepath.Join(retained, "state/ingestion/docling/release-manifest.json"))
	if err != nil || json.Unmarshal(oldBody, &oldManifest) != nil {
		t.Fatal("retained public manifest unavailable", err)
	}
	archive := ""
	for _, artifact := range oldManifest.Artifacts {
		if artifact.Kind == "runtime_pack" && artifact.SHA256 == old.PackSHA256 && artifact.OS == runtime.GOOS && artifact.Arch == runtime.GOARCH {
			archive = filepath.Join(retained, "state/ingestion/docling", artifact.Name)
		}
	}
	if archive == "" {
		t.Fatal("retained authenticated archive not selected")
	}
	candidate, err := releasepack.BuildModernCandidate(ctx, releasepack.ModernOptions{Root: sourceRoot, Output: filepath.Join(base, "factory"), Version: "0.1.43", InstallerVersion: "0.1.43", Channel: "canary", AuthorityRegistry: registryPath, Target: releasepack.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}, DoclingPack: archive})
	if err != nil {
		t.Fatal(err)
	}
	release := filepath.Join(base, "signed")
	manifest, err := releasepack.SignCandidate(releasepack.SignCandidateOptions{Candidate: candidate.Directory, Output: release, Registry: registryPath, Issuer: "maestro-engineering-pilot-20261116", KeyID: "pilot-20261116", PrivateKey: private})
	if err != nil {
		t.Fatal(err)
	}
	private = nil
	root := filepath.Join(base, "participant")
	o := product.Options{ProductRoot: filepath.Join(root, "product"), UserHome: filepath.Join(root, "owner"), WorkspacePath: filepath.Join(root, "workspace")}
	server := filepath.Join(o.ProductRoot, "server", "maestro-mcp")
	for _, artifact := range manifest.Artifacts {
		if artifact.Kind == "mcp_server" && artifact.OS == runtime.GOOS && artifact.Arch == runtime.GOARCH {
			b, err := os.ReadFile(filepath.Join(release, artifact.Name))
			if err != nil {
				t.Fatal(err)
			}
			writePilotFixture(t, server, b)
		}
	}
	if err := os.Chmod(server, 0700); err != nil {
		t.Fatal(err)
	}
	writePilotFixture(t, filepath.Join(o.ProductRoot, "managed/trust/release-authority-registry.json"), registry)
	if err := os.MkdirAll(o.UserHome, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "exports"), 0700); err != nil {
		t.Fatal(err)
	}
	common := []string{"--product-root", o.ProductRoot, "--user-home", o.UserHome, "--workspace", o.WorkspacePath}
	run := func(command string, extra ...string) ([]byte, error) {
		args := append(append([]string{command}, common...), extra...)
		cmd := exec.CommandContext(ctx, server, args...)
		if command == "pilot-runtime-apply" {
			// Real participant launchers intentionally retain a private umask.
			vector := append([]string{"-c", `umask 077; exec "$@"`, "private-runtime-placement", server}, args...)
			cmd = exec.CommandContext(ctx, "/bin/sh", vector...)
		}
		return cmd.CombinedOutput()
	}
	if b, err := run("init"); err != nil {
		t.Fatalf("actual seeded init: %v %s", err, b)
	} else {
		writePilotFixture(t, filepath.Join(root, "preparation.json"), b)
	}
	reviewBytes, err := run("pilot-runtime-review", "--release-directory", release)
	if err != nil {
		t.Fatalf("actual complete runtime review: %v %s", err, reviewBytes)
	}
	var review packaging.EngineeringRuntimeReview
	if err := json.Unmarshal(reviewBytes, &review); err != nil || review.WorkspaceID == "" || review.Runtime.PackSHA256 != old.PackSHA256 {
		t.Fatal("actual review is incomplete", err, string(reviewBytes))
	}
	save("complete-review", review)
	state := filepath.Join(o.UserHome, ".maestro", "state", "ingestion", "docling")
	for _, extra := range [][]string{{"--review-digest", review.ReviewSHA256}, {"--review-digest", strings.Repeat("0", 64), "--confirm"}} {
		if b, err := run("pilot-runtime-apply", append([]string{"--release-directory", release}, extra...)...); err == nil {
			t.Fatal("unconfirmed or wrong review accepted", string(b))
		}
		if _, err := os.Lstat(state); !os.IsNotExist(err) {
			t.Fatal("declined/wrong review placed a runtime", err)
		}
	}
	apply := []string{"--release-directory", release, "--review-digest", review.ReviewSHA256, "--confirm"}
	notesPath := filepath.Join(release, manifest.ReleaseNotes.Name)
	notes := mustEngineeringRead(t, notesPath)
	if err := os.WriteFile(notesPath, append(append([]byte{}, notes...), 'x'), 0600); err != nil {
		t.Fatal(err)
	}
	if b, err := run("pilot-runtime-apply", apply...); err == nil {
		t.Fatal("changed signed release accepted", string(b))
	}
	if err := os.WriteFile(notesPath, notes, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(o.UserHome, ".maestro-install"), 0700); err != nil {
		t.Fatal(err)
	}
	if b, err := run("pilot-runtime-apply", apply...); err == nil {
		t.Fatal("enrolled state accepted by the engineering route", string(b))
	}
	if err := os.Remove(filepath.Join(o.UserHome, ".maestro-install")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(state); !os.IsNotExist(err) {
		t.Fatal("stale/enrolled refusal placed a runtime", err)
	}
	placed, err := run("pilot-runtime-apply", apply...)
	if err != nil {
		t.Fatalf("actual explicit runtime placement: %v %s", err, placed)
	}
	var outcome packaging.EngineeringRuntimeResult
	if err := json.Unmarshal(placed, &outcome); err != nil || outcome.State != "ready" || outcome.AlreadyReady {
		t.Fatal("actual placement did not finish", err, string(placed))
	}
	save("placement", outcome)
	repeated, err := run("pilot-runtime-apply", apply...)
	if err != nil || json.Unmarshal(repeated, &outcome) != nil || !outcome.AlreadyReady {
		t.Fatalf("same reviewed placement did not replay: %v %s", err, repeated)
	}
	save("repeated-placement", outcome)
	code := filepath.Join(root, "exports/code")
	if b, err := run("plugin", "--code-pilot", "--owner-review-channel", "local-browser", "--lifecycle-context", "--server", server, "--destination", code, "--version", "0.1.43"); err != nil {
		t.Fatalf("actual seeded Code export: %v %s", err, b)
	}
	mcpb := filepath.Join(root, "exports/maestro-product.mcpb")
	if b, err := run("mcpb", "--desktop-pilot", "--engineering-runtime-pilot", "--owner-review-channel", "local-browser", "--server", server, "--destination", mcpb, "--version", "0.1.43"); err != nil {
		t.Fatalf("actual engineering Desktop export: %v %s", err, b)
	}
	doctor, err := run("pilot-check")
	if err != nil {
		t.Fatalf("actual EPRT CodePilot doctor refused: %v %s", err, doctor)
	}
	var diagnostic pilotReport
	if err := json.Unmarshal(doctor, &diagnostic); err != nil || diagnostic.Status == "blocked" || diagnostic.Runtime == nil || diagnostic.WorkspaceID != review.WorkspaceID {
		t.Fatal("actual EPRT diagnostic incomplete", err, string(doctor))
	}
	converterVerified := false
	for _, check := range diagnostic.Checks {
		if check.Name == "document_runtime" && check.Status == "passed" {
			converterVerified = true
		}
	}
	if !converterVerified {
		t.Fatal("actual CodePilot converter not verified by doctor")
	}
	save("actual-codepilot-doctor", diagnostic)
	desktop := filepath.Join(base, "desktop-installed")
	engineeringExtract(t, mcpb, desktop)
	// Reuse the closed Desktop materialization and wrong-image/private-state refusals.
	assertDesktopPilotNativeMaterialization(t, server, desktop, mcpb, o)
	sourceBytes, err := os.ReadFile(filepath.Join(sourceRoot, "internal/ingest/docling/testdata/sample.docx"))
	if err != nil {
		t.Fatal(err)
	}
	writePilotFixture(t, filepath.Join(o.WorkspacePath, "code.docx"), sourceBytes)
	writePilotFixture(t, filepath.Join(o.WorkspacePath, "mcpb.docx"), sourceBytes)
	var artifact string
	for _, carrier := range []string{"code", "mcpb"} {
		t.Run(carrier, func(t *testing.T) {
			command, args, err := pilotPacketCommand(code, o)
			if err != nil {
				t.Fatal(err)
			}
			if carrier == "mcpb" {
				body, err := os.ReadFile(filepath.Join(desktop, "manifest.json"))
				var m struct {
					Server struct {
						Config struct {
							Command string   `json:"command"`
							Args    []string `json:"args"`
						} `json:"mcp_config"`
					} `json:"server"`
				}
				if err != nil || json.Unmarshal(body, &m) != nil {
					t.Fatal("emitted Desktop launch configuration unavailable", err)
				}
				command = strings.ReplaceAll(m.Server.Config.Command, "${__dirname}", desktop)
				args = m.Server.Config.Args
				for i := range args {
					args[i] = strings.ReplaceAll(args[i], "${__dirname}", desktop)
				}
			}
			cmd := exec.CommandContext(ctx, command, args...)
			cmd.Dir = o.WorkspacePath
			client, err := mcp.NewClient(&mcp.Implementation{Name: "actual-engineering-pilot", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			call := func(tool string, input map[string]any) map[string]any {
				t.Helper()
				result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: input})
				if err != nil || result.IsError || len(result.Content) != 1 {
					t.Fatalf("actual emitted call: %+v %v", result, err)
				}
				text, ok := result.Content[0].(*mcp.TextContent)
				var value map[string]any
				if !ok || json.Unmarshal([]byte(text.Text), &value) != nil {
					t.Fatal("actual response is not JSON")
				}
				return value
			}
			metadata := call("maestro_context", map[string]any{})
			if metadata["workspace_id"] != review.WorkspaceID {
				t.Fatal("carrier changed workspace", metadata)
			}
			availability := call("maestro_query", map[string]any{"operation": "content.status"})
			if availability["state"] != "ready" || availability["pack_sha256"] != old.PackSHA256 {
				t.Fatal("relocated authority did not activate the real converter", availability)
			}
			if carrier == "mcpb" {
				cross := call("maestro_query", map[string]any{"operation": "content.read", "artifact_id": artifact, "format": "markdown", "max_bytes": 4096})
				if cross["artifact_id"] != artifact {
					t.Fatal("Desktop backend lost the Code artifact", cross)
				}
				save("cross-carrier-readback", cross)
			}
			{
				converted := call("maestro_apply", map[string]any{"operation": "content.convert", "source_path": carrier + ".docx"})
				receipt := converted["result"].(map[string]any)
				provenance := converted["provenance"].(map[string]any)
				if receipt["status"] != "usable" || receipt["source_sha256"] != fmt.Sprintf("%x", sha256.Sum256(sourceBytes)) || provenance["pack_sha256"] != old.PackSHA256 || provenance["network_denied"] != true {
					t.Fatal("real converter did not return authenticated usable output", converted)
				}
				artifact = receipt["artifact_ref"].(string)
				save(carrier+"-actual-conversion", converted)
			}
			read := call("maestro_query", map[string]any{"operation": "content.read", "artifact_id": artifact, "format": "markdown", "max_bytes": 4096})
			markdown, _ := read["text"].(string)
			if !strings.Contains(markdown, `MAESTRO\_DOCLING\_OFFLINE\_20261101`) || !strings.Contains(markdown, "alpha") || !strings.Contains(markdown, "7") {
				t.Fatal("actual DOCX/table output oracle failed", markdown)
			}
			save(carrier+"-artifact-readback", read)
			if err := client.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, name := range []string{"code.docx", "mcpb.docx"} {
		if !reflect.DeepEqual(sourceBytes, mustEngineeringRead(t, filepath.Join(o.WorkspacePath, name))) {
			t.Fatal("original synthetic source changed")
		}
	}
	save("result", map[string]any{"state": "passed", "runtime": review.Runtime, "actual_conversion": true, "code_emitted_backend": true, "desktop_emitted_backend": true, "actual_client_ui": false, "native_windows": false, "persistent_signing_key": false})
}

func mustEngineeringRead(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func engineeringExtract(t *testing.T, archive, target string) {
	t.Helper()
	if _, err := packaging.ValidateArchive(archive); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if !filepath.IsLocal(entry.Name) || entry.Mode()&os.ModeSymlink != 0 {
			t.Fatal("unsafe generated archive")
		}
		stream, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(io.LimitReader(stream, (128<<20)+1))
		closeErr := stream.Close()
		if readErr != nil || closeErr != nil || len(body) > 128<<20 {
			t.Fatal("generated entry unavailable", readErr, closeErr)
		}
		writePilotFixture(t, filepath.Join(target, entry.Name), body)
		if err := os.Chmod(filepath.Join(target, entry.Name), entry.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := packaging.ValidatePacket(target); err != nil {
		t.Fatal(err)
	}
}
