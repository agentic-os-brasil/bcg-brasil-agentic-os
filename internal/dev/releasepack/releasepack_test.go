package releasepack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

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
	if err := os.Symlink(filepath.Join(root, "bundles/base/ok.txt"), filepath.Join(root, "bundles/base/link.txt")); err != nil {
		t.Fatal(err)
	}
	tests := map[string]Allowlist{
		"development path": {SchemaVersion: 1, Files: []BundleFile{{Source: "dev/secret.txt", Path: "secret.txt"}}},
		"target traversal": {SchemaVersion: 1, Files: []BundleFile{{Source: "bundles/base/ok.txt", Path: "../ok.txt"}}},
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
