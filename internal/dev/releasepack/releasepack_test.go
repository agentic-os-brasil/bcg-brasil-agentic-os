package releasepack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestBuildBundleNormalizesExecutableModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX executable permissions")
	}
	for _, test := range []struct {
		name string
		mode os.FileMode
		want int64
	}{
		{"data", 0o666, 0o644},
		{"owner executable", 0o700, 0o755},
		{"group executable", 0o610, 0o755},
		{"other executable", 0o601, 0o755},
		{"privileged executable", os.ModeSetuid | os.ModeSetgid | os.ModeSticky | 0o777, 0o755},
		{"privileged data", os.ModeSetuid | os.ModeSetgid | os.ModeSticky | 0o666, 0o644},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			const source = "bundles/base/runtime/helper"
			writeFile(t, root, source, "nonempty fixture\n")
			if err := os.Chmod(filepath.Join(root, source), test.mode); err != nil {
				t.Fatal(err)
			}
			var archive bytes.Buffer
			if err := BuildBundle(root, Allowlist{SchemaVersion: 1, Files: []BundleFile{{Source: source, Path: "runtime/helper"}}}, &archive); err != nil {
				t.Fatal(err)
			}
			_, modes := archiveEntries(t, archive.Bytes())
			if len(modes) != 1 || modes[0] != test.want {
				t.Fatalf("archive modes = %o, want [%o]", modes, test.want)
			}
		})
	}
}

func TestBuildBundleIsDeterministicAndOrdered(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "bundles/base/skills/example/SKILL.md", "skill\n")
	writeFile(t, root, "bundles/base/runtime/capabilities.json", "{}\n")
	allowlist := Allowlist{
		SchemaVersion: 1,
		Files: []BundleFile{
			{Source: "bundles/base/skills/example/SKILL.md", Path: "skills/example/SKILL.md"},
			{Source: "bundles/base/runtime/capabilities.json", Path: "runtime/capabilities.json"},
		},
	}
	var first bytes.Buffer
	var second bytes.Buffer
	if err := BuildBundle(root, allowlist, &first); err != nil {
		t.Fatalf("BuildBundle(first) error = %v", err)
	}
	if err := BuildBundle(root, allowlist, &second); err != nil {
		t.Fatalf("BuildBundle(second) error = %v", err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("bundle bytes differ for identical inputs")
	}
	names, modes := archiveEntries(t, first.Bytes())
	if want := []string{"runtime/capabilities.json", "skills/example/SKILL.md"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("entries = %v, want %v", names, want)
	}
	if want := []int64{0o644, 0o644}; !reflect.DeepEqual(modes, want) {
		t.Fatalf("modes = %v, want %v", modes, want)
	}
}

func TestBuildBundleRejectsUnsafeSources(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "dev/secret.txt", "no\n")
	writeFile(t, root, "bundles/base/ok.txt", "ok\n")
	if err := os.MkdirAll(filepath.Join(root, "bundles/base/directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "bundles/base/ok.txt"), filepath.Join(root, "bundles/base/link.txt")); err != nil {
		t.Skipf("creating a symbolic link is unavailable here: %v", err)
	}
	tests := map[string]Allowlist{
		"development path": {SchemaVersion: 1, Files: []BundleFile{{Source: "dev/secret.txt", Path: "secret.txt"}}},
		"target traversal": {SchemaVersion: 1, Files: []BundleFile{{Source: "bundles/base/ok.txt", Path: "../ok.txt"}}},
		"source traversal": {SchemaVersion: 1, Files: []BundleFile{{Source: "bundles/base/../base/ok.txt", Path: "ok.txt"}}},
		"directory":        {SchemaVersion: 1, Files: []BundleFile{{Source: "bundles/base/directory", Path: "directory"}}},
		"symlink":          {SchemaVersion: 1, Files: []BundleFile{{Source: "bundles/base/link.txt", Path: "link.txt"}}},
		"duplicate target": {SchemaVersion: 1, Files: []BundleFile{
			{Source: "bundles/base/ok.txt", Path: "same.txt"},
			{Source: "bundles/base/ok.txt", Path: "same.txt"},
		}},
	}
	for name, allowlist := range tests {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			if err := BuildBundle(root, allowlist, &output); err == nil {
				t.Fatal("BuildBundle() accepted unsafe allowlist")
			}
		})
	}
}

func TestDetachedSignatureRejectsTampering(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("release bytes")
	signature := Sign(payload, privateKey)
	if err := Verify(payload, signature, publicKey); err != nil {
		t.Fatalf("Verify(valid) error = %v", err)
	}
	if err := Verify([]byte("tampered"), signature, publicKey); err == nil {
		t.Fatal("Verify() accepted tampered payload")
	}
}

func archiveEntries(t *testing.T, body []byte) ([]string, []int64) {
	t.Helper()
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	var names []string
	var modes []int64
	for {
		header, err := tarReader.Next()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatal(err)
		}
		names = append(names, header.Name)
		modes = append(modes, header.Mode)
	}
	return names, modes
}

func writeFile(t *testing.T, root, relative, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
