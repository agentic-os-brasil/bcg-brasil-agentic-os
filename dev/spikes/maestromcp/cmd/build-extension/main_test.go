package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"debug/macho"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/spike"
)

func extensionFixture(t *testing.T) (base, server, state string) {
	t.Helper()
	base = t.TempDir()
	server = filepath.Join(base, "native server")
	if err := os.WriteFile(server, machoFixture(), 0700); err != nil {
		t.Fatal(err)
	}
	state = filepath.Join(base, "synthetic state with spaces")
	if _, err := spike.Seed(state); err != nil {
		t.Fatal(err)
	}
	return
}

// A parser fixture, deliberately never executed. Native launch is exercised by
// the separate rehearsal against the compiled server included in each package.
func machoFixture() []byte {
	var body bytes.Buffer
	for _, word := range []uint32{macho.Magic64, uint32(macho.CpuArm64), 0, uint32(macho.TypeExec), 0, 0, 0, 0} {
		_ = binary.Write(&body, binary.LittleEndian, word)
	}
	return body.Bytes()
}

func treeDigest(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	digest := map[string][32]byte{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest[path] = sha256.Sum256(body)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestBuildExtensionPackagesOnlyManifestAndExecutableWithoutChangingState(t *testing.T) {
	base, server, state := extensionFixture(t)
	before := treeDigest(t, state)
	destination := filepath.Join(base, "desktop connector with spaces.mcpb")
	args := []string{"--destination", destination, "--server", server, "--state-root", state, "--workspace", "case-a", "--version", "0.0.2"}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d: %s", code, &stderr)
	}
	reader, err := zip.OpenReader(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if len(reader.File) != 2 {
		t.Fatalf("archive must contain exactly manifest and binary: %d entries", len(reader.File))
	}
	entries := map[string][]byte{}
	for _, entry := range reader.File {
		stream, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		entries[entry.Name], err = io.ReadAll(stream)
		closeErr := stream.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("read archive: %v / %v", err, closeErr)
		}
		if entry.Name == "server/maestro-mcp-spike" && entry.Mode().Perm() != 0700 {
			t.Fatalf("binary permissions=%v", entry.Mode())
		}
	}
	if !bytes.Equal(entries["server/maestro-mcp-spike"], machoFixture()) {
		t.Fatal("native binary not copied exactly")
	}
	var manifest struct {
		ManifestVersion string `json:"manifest_version"`
		Name            string `json:"name"`
		Version         string `json:"version"`
		Server          struct {
			Type       string `json:"type"`
			EntryPoint string `json:"entry_point"`
			MCPConfig  struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
			} `json:"mcp_config"`
		} `json:"server"`
		Compatibility struct {
			Platforms []string `json:"platforms"`
		} `json:"compatibility"`
	}
	if err := json.Unmarshal(entries["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ManifestVersion != "0.3" || manifest.Name != "maestro-architecture-spike" || manifest.Version != "0.0.2" {
		t.Fatalf("wrong identity: %+v", manifest)
	}
	if manifest.Server.Type != "binary" || manifest.Server.EntryPoint != "server/maestro-mcp-spike" || manifest.Server.MCPConfig.Command != "${__dirname}/server/maestro-mcp-spike" {
		t.Fatalf("wrong native launch: %+v", manifest.Server)
	}
	if !reflect.DeepEqual(manifest.Server.MCPConfig.Args, []string{"serve", "--root", state, "--workspace", "case-a"}) || !reflect.DeepEqual(manifest.Compatibility.Platforms, []string{"darwin"}) {
		t.Fatalf("wrong scope: %+v", manifest)
	}
	if !reflect.DeepEqual(before, treeDigest(t, state)) {
		t.Fatal("packaging changed owner state")
	}
	archiveBefore, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if code := run(args, &stdout, &stderr); code == 0 {
		t.Fatal("existing artifact overwritten")
	}
	archiveAfter, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(archiveBefore, archiveAfter) {
		t.Fatal("existing artifact changed")
	}
}

func TestBuildExtensionRejectsInvalidInputsBeforeCreatingArtifact(t *testing.T) {
	base, server, state := extensionFixture(t)
	symlink := filepath.Join(base, "server link")
	if err := os.Symlink(server, symlink); err != nil {
		t.Fatal(err)
	}
	invalidRoot := filepath.Join(base, "not a fixture")
	if err := os.Mkdir(invalidRoot, 0700); err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	escape := filepath.Join(base, "outside Developer")
	if err := os.Symlink(home, escape); err != nil {
		t.Fatal(err)
	}
	stateAlias := filepath.Join(base, "state alias")
	if err := os.Symlink(state, stateAlias); err != nil {
		t.Fatal(err)
	}
	nonDarwin := filepath.Join(base, "not a Mach-O binary")
	if err := os.WriteFile(nonDarwin, []byte("unqualified executable input"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, binary, root, workspace, version, destination string }{
		{"relative-root", server, "relative", "case-a", "0.0.1", ""},
		{"unmarked-root", server, invalidRoot, "case-a", "0.0.1", ""},
		{"unknown-case", server, state, "case-c", "0.0.1", ""},
		{"unsafe-version", server, state, "case-a", "../wrong", ""},
		{"state-overlap", server, state, "case-a", "0.0.1", filepath.Join(state, "connector.mcpb")},
		{"binary-directory", base, state, "case-a", "0.0.1", ""},
		{"binary-symlink", symlink, state, "case-a", "0.0.1", ""},
		{"missing-binary", filepath.Join(base, "absent"), state, "case-a", "0.0.1", ""},
		{"non-darwin-binary", nonDarwin, state, "case-a", "0.0.1", ""},
		{"wrong-extension", server, state, "case-a", "0.0.1", filepath.Join(base, "connector.zip")},
		{"outside-Developer", server, state, "case-a", "0.0.1", filepath.Join(home, filepath.Base(base)+"-must-not-exist.mcpb")},
		{"escaped-parent", server, state, "case-a", "0.0.1", filepath.Join(escape, filepath.Base(base)+"-must-not-exist.mcpb")},
		{"state-parent-alias", server, state, "case-a", "0.0.1", filepath.Join(stateAlias, "must-not-exist.mcpb")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			destination := tc.destination
			if destination == "" {
				destination = filepath.Join(base, tc.name+".mcpb")
			}
			var out bytes.Buffer
			if code := run([]string{"--destination", destination, "--server", tc.binary, "--state-root", tc.root, "--workspace", tc.workspace, "--version", tc.version}, &out, &out); code == 0 {
				t.Fatal("invalid configuration accepted")
			}
			if _, err := os.Lstat(destination); !os.IsNotExist(err) {
				t.Fatalf("invalid build created artifact: %v", err)
			}
		})
	}
	// O_EXCL must also preserve a pre-existing symlink and its target.
	destination := filepath.Join(base, "existing.mcpb")
	if err := os.Symlink(server, destination); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := run([]string{"--destination", destination, "--server", server, "--state-root", state, "--workspace", "case-b"}, &out, &out); code == 0 {
		t.Fatal("existing symlink accepted")
	}
	if target, err := os.Readlink(destination); err != nil || target != server {
		t.Fatal("existing symlink changed")
	}
}
