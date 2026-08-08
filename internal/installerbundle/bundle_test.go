package installerbundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackAppendAndExtractSelfContainedPayload(t *testing.T) {
	root := t.TempDir()
	payloadRoot := filepath.Join(root, "package")
	if err := os.MkdirAll(filepath.Join(payloadRoot, "wizard", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"maestro-installer.exe":          "inner-bridge",
		"wizard/index.html":              "wizard",
		"wizard/assets/maestro-mark.svg": "mark",
		"release/release-manifest.json":  "manifest",
		"authority-registry.json":        "registry",
		"README-UNSIGNED.md":             "candidate",
	}
	for name, content := range files {
		path := filepath.Join(payloadRoot, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	payloadPath := filepath.Join(root, "payload.tar.gz")
	payloadFile, err := os.Create(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := PackDirectory(payloadRoot, payloadFile)
	if closeErr := payloadFile.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	if info.Size == 0 || info.SHA256 == "" {
		t.Fatalf("expected payload metadata, got %+v", info)
	}

	basePath := filepath.Join(root, "base.exe")
	if err := os.WriteFile(basePath, []byte("base-pe"), 0o755); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(root, "Maestro-Installer.exe")
	if _, err := AppendPayload(basePath, payloadPath, outputPath); err != nil {
		t.Fatal(err)
	}

	extracted, cleanup, err := ExtractExecutable(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for name, want := range files {
		got, readErr := os.ReadFile(filepath.Join(extracted, filepath.FromSlash(name)))
		if readErr != nil {
			t.Fatalf("read extracted %s: %v", name, readErr)
		}
		if string(got) != want {
			t.Fatalf("extracted %s = %q, want %q", name, got, want)
		}
	}
	if _, err := os.Stat(extracted); err != nil {
		t.Fatalf("extracted package vanished before cleanup: %v", err)
	}
	cleanup()
	if _, err := os.Stat(extracted); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup error = %v, want directory removal", err)
	}
}

func TestExtractRejectsTamperedPayload(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base.exe")
	payload := filepath.Join(root, "payload.bin")
	output := filepath.Join(root, "installer.exe")
	if err := os.WriteFile(base, []byte("base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(payload, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := AppendPayload(base, payload, output); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	contents[len(contents)-payloadFooterSize-1] ^= 0xff
	if err := os.WriteFile(output, contents, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ExtractExecutable(output); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("tampered payload error = %v, want digest failure", err)
	}
}

func TestExtractRejectsUnsafeArchiveEntries(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		entryType byte
	}{
		{name: "traversal", path: "../outside.txt", entryType: tar.TypeReg},
		{name: "backslash", path: `wizard\\..\\outside.txt`, entryType: tar.TypeReg},
		{name: "symlink", path: "wizard/link", entryType: tar.TypeSymlink},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			base := filepath.Join(root, "base.exe")
			payload := filepath.Join(root, "payload.tar.gz")
			output := filepath.Join(root, "installer.exe")
			if err := os.WriteFile(base, []byte("base"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := writeArchive(payload, test.path, test.entryType); err != nil {
				t.Fatal(err)
			}
			if _, err := AppendPayload(base, payload, output); err != nil {
				t.Fatal(err)
			}
			if _, _, err := ExtractExecutable(output); err == nil || !strings.Contains(err.Error(), "unsafe") {
				t.Fatalf("unsafe archive error = %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "outside.txt")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("archive escaped extraction root: %v", err)
			}
		})
	}
}

func TestExtractRejectsDuplicateArchiveEntries(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base.exe")
	payload := filepath.Join(root, "payload.tar.gz")
	output := filepath.Join(root, "installer.exe")
	if err := os.WriteFile(base, []byte("base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeDuplicateArchive(payload); err != nil {
		t.Fatal(err)
	}
	if _, err := AppendPayload(base, payload, output); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ExtractExecutable(output); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate archive error = %v", err)
	}
}

func writeArchive(path, entryPath string, entryType byte) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	header := &tar.Header{Name: entryPath, Typeflag: entryType, Mode: 0o644}
	if entryType == tar.TypeReg {
		header.Size = int64(len("content"))
	}
	if err := tarWriter.WriteHeader(header); err == nil && entryType == tar.TypeReg {
		_, err = io.WriteString(tarWriter, "content")
	}
	if closeErr := tarWriter.Close(); err == nil {
		err = closeErr
	}
	if closeErr := gzipWriter.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func writeDuplicateArchive(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	for range 2 {
		if err := tarWriter.WriteHeader(&tar.Header{Name: "same.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1}); err != nil {
			return err
		}
		if _, err := io.WriteString(tarWriter, "x"); err != nil {
			return err
		}
	}
	err = tarWriter.Close()
	if closeErr := gzipWriter.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func TestPackDirectoryRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "ok.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	var output bytes.Buffer
	if _, err := PackDirectory(root, &output); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink packaging error = %v", err)
	}
}
