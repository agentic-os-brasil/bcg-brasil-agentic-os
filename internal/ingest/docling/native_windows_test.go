//go:build windows

package docling_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"golang.org/x/sys/windows"
)

func TestWindowsRealAuthenticatedDocling(t *testing.T) {
	inputs := os.Getenv("MAESTRO_WINDOWS_DOCLING_TEST_INPUTS")
	if inputs == "" {
		t.Skip("explicit exact official Windows input directory required")
	}
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Fatal("native conversion qualification requires an ordinary non-elevated Windows user")
	}
	assertNativeMetadataFingerprint(t)
	_, source, _, _ := goruntime.Caller(0)
	sourceRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", ".."))
	root := filepath.Join(t.TempDir(), "private")
	directory, err := winprivate.Fresh(root)
	if err != nil {
		t.Fatal(err)
	}
	directory.Close()
	profile := os.Getenv("MAESTRO_WINDOWS_DOCLING_TEST_PROFILE")
	if profile == "" {
		profile = docling.LegacyProfile
	}
	descriptor, err := docling.DescribeProfile("windows", "amd64", profile)
	if err != nil {
		t.Fatal(err)
	}
	assemble, err := docling.AssembleProfile(inputs, filepath.Join(root, "unsigned-pack"), "windows", "amd64", profile)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("maestro_native_runtime_pack_sha256=%s", assemble.ArchiveSHA256)
	t.Logf("maestro_native_runtime_profile=%s lock_sha256=%s", profile, descriptor.LockSHA256)
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, _ := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "engineering-fixture", "key_id": "ephemeral", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": "2020-01-01T00:00:00Z", "valid_until": "2100-01-01T00:00:00Z"}}})
	registryPath := filepath.Join(root, "fixture-authority.json")
	put(t, registryPath, registry)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	candidate, err := releasepack.BuildModernCandidate(ctx, releasepack.ModernOptions{Root: sourceRoot, Output: filepath.Join(root, "factory"), Version: "0.1.42", InstallerVersion: "0.1.42", Channel: "canary", AuthorityRegistry: registryPath, Target: releasepack.Target{OS: "windows", Arch: "amd64"}, DoclingPack: assemble.ArchivePath})
	if err != nil {
		t.Fatal(err)
	}
	signed := filepath.Join(root, "signed")
	if _, err = releasepack.SignCandidate(releasepack.SignCandidateOptions{Candidate: candidate.Directory, Output: signed, Registry: registryPath, Issuer: "engineering-fixture", KeyID: "ephemeral", PrivateKey: private}); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, "data")
	space := filepath.Join(root, "workspace")
	if err = winprivate.MkdirAll(space); err != nil {
		t.Fatal(err)
	}
	w, err := workspace.Initialize(workspace.Options{WorkspacePath: space, DataRoot: data})
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "actual-shared-runtime.exe")
	build := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-o", executable, "./internal/ingest/docling/testdata/native-runtime")
	build.Dir = sourceRoot
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOPROXY=off", "GOSUMDB=off")
	if body, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compile actual shared runtime: %v: %s", err, body)
	}
	options := docling.Options{DataRoot: data, AuthorityRegistryPath: registryPath, AuthorityRegistrySHA256: sum(registry), CLIVersion: "0.1.42"}
	invoke := func(action string, request ingest.Request, out any) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"Action": action, "Release": signed, "WorkspaceID": w.WorkspaceID, "Options": options, "Source": request})
		command := exec.CommandContext(ctx, executable)
		command.Stdin = bytes.NewReader(body)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("actual shared runtime %s failed: %v: %s", action, err, output)
		}
		if action == "convert" || action == "export_fixture" {
			var wire struct {
				Metadata docling.Conversion `json:"metadata"`
				Markdown []byte             `json:"markdown"`
				Document []byte             `json:"document"`
			}
			conversion, ok := out.(*docling.Conversion)
			if !ok || json.Unmarshal(output, &wire) != nil {
				t.Fatal("actual conversion fixture response invalid")
			}
			*conversion = wire.Metadata
			conversion.Markdown, conversion.Document = wire.Markdown, json.RawMessage(wire.Document)
		} else if json.Unmarshal(output, out) != nil {
			t.Fatal("actual runtime metadata response invalid")
		}
	}
	var availability docling.Availability
	invoke("place", ingest.Request{}, &availability)
	if availability.State != "ready" || availability.Profile != profile {
		t.Fatalf("explicit actual converter proof did not qualify: %+v", availability)
	}
	proofPath := filepath.Join(data, "ingestion", "docling", "windows-qualification.dpapi")
	proof := read(t, proofPath)
	before := nativeFixtureMetadata(t, data)
	invoke("status", ingest.Request{}, &availability)
	if availability.State != "ready" || before != nativeFixtureMetadata(t, data) {
		t.Fatalf("status failed proof validation or changed private state: %+v", availability)
	}
	for _, name := range []string{"sample.docx", "literal.txt", "structured.md", "quoted.csv", "generic.json", "generic.xml", "page.html", "page.htm", "spine.epub", "legacy.xls", "sheets.xlsx"} {
		t.Run(name, func(t *testing.T) {
			fixture := filepath.Join(sourceRoot, "internal", "ingest", "docling", "testdata", "policy", name)
			if name == "sample.docx" {
				fixture = filepath.Join(sourceRoot, "internal", "ingest", "docling", "testdata", name)
			}
			original := read(t, fixture)
			put(t, filepath.Join(space, name), original)
			var conversion docling.Conversion
			invoke("convert", ingest.Request{SourcePath: name, WorkspacePath: space, Policy: ingest.DefaultPolicy()}, &conversion)
			if conversion.Result.Status != ingest.StatusUsable || !conversion.Provenance.NetworkDenied || conversion.Provenance.LockSHA256 != descriptor.LockSHA256 || conversion.Provenance.PackSHA256 != assemble.ArchiveSHA256 {
				t.Fatalf("real Windows conversion/provenance: %+v", conversion)
			}
			if !bytes.Equal(original, read(t, filepath.Join(space, name))) {
				t.Fatal("source bytes changed")
			}
		})
	}

	if profile == docling.DocumentsProfile {
		convertDocument := func(t *testing.T, name string, body []byte) docling.Conversion {
			t.Helper()
			put(t, filepath.Join(space, name), body)
			var got docling.Conversion
			invoke("convert", ingest.Request{SourcePath: name, WorkspacePath: space, Policy: ingest.DefaultPolicy()}, &got)
			if !bytes.Equal(body, read(t, filepath.Join(space, name))) || got.Result.Status != ingest.StatusBlocked && got.Result.SourceSHA256 != sum(body) {
				t.Fatal("native document source identity changed")
			}
			if got.Result.Status == ingest.StatusUsable && (got.Provenance.PackSHA256 != assemble.ArchiveSHA256 || !got.Provenance.NetworkDenied || got.Provenance.LockSHA256 != descriptor.LockSHA256) {
				t.Fatalf("native document pack provenance differs: %+v", got)
			}
			return got
		}
		for _, name := range []string{"native.pdf", "native.pptx"} {
			t.Run(name, func(t *testing.T) {
				got := convertDocument(t, name, read(t, filepath.Join(sourceRoot, "internal/ingest/docling/testdata/documents", name)))
				assertNativeDocument(t, got, name, descriptor.LockSHA256)
			})
		}
		for name, want := range nativePDFRefusals() {
			t.Run(name, func(t *testing.T) {
				got := convertDocument(t, name, read(t, filepath.Join(sourceRoot, "internal/ingest/docling/testdata/documents", name)))
				assertNativePDFRefusal(t, got, name, want)
			})
		}
		base := read(t, filepath.Join(sourceRoot, "internal/ingest/docling/testdata/documents/native.pptx"))
		for _, test := range nativePPTXRefusalCases() {
			t.Run("pptx_"+strings.ReplaceAll(test.name, " ", "_"), func(t *testing.T) {
				body := editXLSX(t, base, test.edit)
				got := convertDocument(t, "unsafe.pptx", body)
				if got.Result.Status != ingest.StatusBlocked || len(got.Result.Warnings) != 1 || got.Result.Warnings[0] != "source_format_invalid" || len(got.Document) != 0 || len(got.Markdown) != 0 {
					t.Fatalf("unsafe PPTX reached actual converter: %+v", got)
				}
			})
		}
	}

	t.Run("validated_export", func(t *testing.T) {
		var conversion docling.Conversion
		invoke("export_fixture", ingest.Request{SourcePath: "sample.docx", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, &conversion)
		invoke("convert", ingest.Request{SourcePath: "validated-export.json", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, &conversion)
		if conversion.Result.Status != ingest.StatusUsable || !conversion.Provenance.NetworkDenied {
			t.Fatalf("real export classification+conversion failed: %+v", conversion)
		}
	})
	t.Run("installed_tamper", func(t *testing.T) {
		bridge := filepath.Join(data, "ingestion", "docling", "installed", assemble.ArchiveSHA256, "bridge.py")
		original := read(t, bridge)
		if err := os.Chmod(bridge, 0600); err != nil {
			t.Fatal(err)
		}
		put(t, bridge, []byte("changed authenticated converter"))
		invoke("status", ingest.Request{}, &availability)
		if availability.State == "ready" || availability.Reason != "installed_pack_changed" {
			t.Fatalf("installed tamper became ready: %+v", availability)
		}
		put(t, bridge, original)
		if err := os.Chmod(bridge, 0444); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("missing_native_runtime", func(t *testing.T) {
		name := filepath.Join(data, "ingestion", "docling", "installed", assemble.ArchiveSHA256, "python", "msvcp140.dll")
		original := read(t, name)
		if err := os.Remove(name); err != nil {
			t.Fatal(err)
		}
		var conversion docling.Conversion
		invoke("convert", ingest.Request{SourcePath: "sample.docx", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, &conversion)
		if conversion.Result.Status != ingest.StatusUnavailable || len(conversion.Result.Warnings) != 1 || conversion.Result.Warnings[0] != "installed_pack_changed" {
			t.Fatalf("ambient runtime satisfied missing authenticated DLL: %+v", conversion)
		}
		put(t, name, original)
		if err := os.Chmod(name, 0444); err != nil {
			t.Fatal(err)
		}
	})
	// Missing/forged proof cannot be promoted by a read-only status query.
	if err = os.Remove(proofPath); err != nil {
		t.Fatal(err)
	}
	before = nativeFixtureMetadata(t, data)
	invoke("status", ingest.Request{}, &availability)
	if availability.State == "ready" || availability.Reason != "requalification_required" || before != nativeFixtureMetadata(t, data) {
		t.Fatalf("missing proof self-repaired or became ready: %+v", availability)
	}
	put(t, proofPath, []byte("self-asserted ready"))
	before = nativeFixtureMetadata(t, data)
	invoke("status", ingest.Request{}, &availability)
	if availability.State == "ready" || before != nativeFixtureMetadata(t, data) {
		t.Fatal("forged proof accepted or repaired")
	}
	put(t, proofPath, proof)
	invoke("place", ingest.Request{}, &availability)
	if availability.State != "ready" || availability.Profile != profile {
		t.Fatal("unchanged signed pack cannot requalify after actual repeated smoke")
	}
	t.Logf("engineering only: real native Windows converter + zero-capability kernel controls observed; pack_sha256=%s files=%d expanded=%d", assemble.ArchiveSHA256, assemble.Files, assemble.ExpandedBytes)
}

func nativeFixtureMetadata(t *testing.T, root string) string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(name string, item os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := item.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			info = nativeHeldDirectoryInfo(t, name)
		}
		entries = append(entries, strings.Join([]string{name, info.ModTime().UTC().Format(time.RFC3339Nano), info.Mode().String()}, "|"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return nativeMetadataFingerprint(entries)
}

// Directory listings can contain stale NTFS metadata. Query the held object
// without omitting any directory, name, modification time or portable mode.
func nativeHeldDirectoryInfo(t *testing.T, name string) os.FileInfo {
	t.Helper()
	directory, err := winprivate.Inspect(name)
	if err != nil {
		t.Fatal("held directory metadata open failed")
	}
	info, statErr := directory.File.Stat()
	closeErr := directory.Close()
	if statErr != nil || closeErr != nil {
		t.Fatal("held directory metadata query or close failed")
	}
	if !info.IsDir() || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		t.Fatal("held directory metadata type invalid")
	}
	return info
}

// Preserve every tracked name, modification time and portable mode.
func nativeMetadataFingerprint(entries []string) string {
	sort.Strings(entries)
	return strings.Join(entries, "\n")
}

// Pure in-memory checks execute inside the existing original native parent.
// Cross-compilation does not establish that these Windows-only checks ran.
func assertNativeMetadataFingerprint(t *testing.T) {
	t.Helper()
	// Literal rows exercise the exact original full-name/time/mode sort/join.
	rows := []string{"b|2026-11-20T00:00:00Z|-rw-------", "a|2026-11-20T00:00:00Z|drwx------"}
	fingerprint := nativeMetadataFingerprint(append([]string{}, rows...))
	if fingerprint != "a|2026-11-20T00:00:00Z|drwx------\nb|2026-11-20T00:00:00Z|-rw-------" {
		t.Fatal("original sorted fingerprint changed")
	}
	for _, changed := range [][]string{
		{rows[0]}, {rows[0], rows[1], "c|2026-11-20T00:00:00Z|-rw-------"},
		{rows[0], "renamed|2026-11-20T00:00:00Z|drwx------"},
		{rows[0], "a|2026-11-20T00:00:00.000000001Z|drwx------"},
		{rows[0], "a|2026-11-20T00:00:00Z|dr-x------"},
	} {
		if nativeMetadataFingerprint(changed) == fingerprint {
			t.Fatal("original fingerprint omitted a tracked entry or field")
		}
	}
}
