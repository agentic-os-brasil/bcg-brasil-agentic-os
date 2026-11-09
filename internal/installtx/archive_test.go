package installtx

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

func TestFactoryBundlePrepareAndActivatePreservesRunnablePayload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("local shell fixture requires POSIX executable permissions")
	}
	root := t.TempDir()
	const helper = "#!/bin/sh\nprintf 'runnable fixture\\n'\n"
	const data = "ordinary data\n"
	writeTestFile(t, root, "bundles/base/runtime/helper", helper)
	writeTestFile(t, root, "bundles/base/runtime/data.sh", data)
	if err := os.Chmod(filepath.Join(root, "bundles/base/runtime/data.sh"), 0o644); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err := releasepack.BuildBundle(root, releasepack.Allowlist{SchemaVersion: 1, Files: []releasepack.BundleFile{
		{Source: "bundles/base/runtime/helper", Path: "runtime/helper"},
		{Source: "bundles/base/runtime/data.sh", Path: "runtime/data.sh"},
	}}, &archive); err != nil {
		t.Fatal(err)
	}
	releaseDirectory := t.TempDir()
	writeTestFile(t, releaseDirectory, "bcgos_0.1.0_darwin_arm64", "fixture CLI 0.1.0\n")
	writeTestFile(t, releaseDirectory, "maestro-base_0.1.0.tar.gz", archive.String())
	verified := authenticateFactoryFixture(t, releaseDirectory)
	options := PrepareOptions{Transition: "install", TargetOS: "darwin", TargetArch: "arm64",
		ManagedRoot: filepath.Join(t.TempDir(), "managed"), DataRoot: filepath.Join(t.TempDir(), "data")}
	planPath, err := Prepare(verified, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := Activate(planPath, verified, ActivateOptions{PrepareOptions: options, CheckCLI: func(path, _ string) error {
		body, err := os.ReadFile(path)
		if err == nil && string(body) != "fixture CLI 0.1.0\n" {
			return os.ErrInvalid
		}
		return err
	}}); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(options.ManagedRoot, "bundles", "0.1.0")
	assertArchiveFile(t, filepath.Join(bundle, "runtime/helper"), helper, 0o755)
	assertArchiveFile(t, filepath.Join(bundle, "runtime/data.sh"), data, 0o644)
	output, err := exec.Command(filepath.Join(bundle, "runtime/helper")).Output()
	if err != nil || string(output) != "runnable fixture\n" {
		t.Fatalf("installed helper execution = %q, %v", output, err)
	}
}

// Reuse the transaction manifest/identity fixture and the canonical verifier's
// engineering-key pattern; no seeded/native build or production key is needed.
func authenticateFactoryFixture(t *testing.T, directory string) releaseverify.VerifiedRelease {
	t.Helper()
	manifest := verifiedTestRelease(t, directory, "0.1.0").Manifest
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := range manifest.Artifacts {
		artifact := &manifest.Artifacts[i]
		artifact.SignatureRef = artifact.Name + ".sig"
		body, err := os.ReadFile(filepath.Join(directory, artifact.Name))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, directory, artifact.SignatureRef, string(ed25519.Sign(privateKey, body)))
	}
	const notesName = "release-notes-0.1.0.md"
	writeTestFile(t, directory, notesName, "engineering fixture\n")
	_, notesDigest := testFileIdentity(t, filepath.Join(directory, notesName))
	manifest.ReleaseNotes = releasecontract.ReleaseNotes{Name: notesName, SHA256: notesDigest}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, directory, releaseverify.ManifestName, string(body))
	writeTestFile(t, directory, releaseverify.ManifestSignatureName, string(ed25519.Sign(privateKey, body)))
	verified, err := releaseverify.VerifyDirectory(directory, releaseverify.StaticRegistry{
		"maestro/maestro-release/pilot-2026": publicKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	return verified
}

func TestExtractBundleNormalizesAuthenticatedModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX executable permissions")
	}
	for _, test := range []struct {
		mode int64
		want os.FileMode
	}{
		{0o644, 0o644}, {0o7777, 0o755}, {0o7666, 0o644},
		{0o700, 0o755}, {0o610, 0o755}, {0o601, 0o755},
	} {
		t.Run(fmt.Sprintf("%o", test.mode), func(t *testing.T) {
			directory := t.TempDir()
			archive := bundleHeaders(t, []*tar.Header{{Name: "content", Typeflag: tar.TypeReg, Mode: test.mode, Size: 7}}, []byte("fixture"))
			if err := extractBundle(bytes.NewReader(archive), directory); err != nil {
				t.Fatal(err)
			}
			assertArchiveFile(t, filepath.Join(directory, "content"), "fixture", test.want)
		})
	}
}

func TestExtractBundleRejectsSpecialEntries(t *testing.T) {
	for _, entryType := range []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo} {
		t.Run(fmt.Sprintf("%c", entryType), func(t *testing.T) {
			header := &tar.Header{Name: "special", Linkname: "target", Typeflag: entryType, Mode: 0o7777}
			archive := bundleHeaders(t, []*tar.Header{header}, nil)
			if err := extractBundle(bytes.NewReader(archive), t.TempDir()); err == nil || !strings.Contains(err.Error(), "unsupported") {
				t.Fatalf("special entry extraction error = %v", err)
			}
		})
	}
}

func TestExtractBundleDoesNotOverwrite(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%t", existing), func(t *testing.T) {
			directory := t.TempDir()
			header := &tar.Header{Name: "content", Typeflag: tar.TypeReg, Mode: 0o7777, Size: 7}
			headers := []*tar.Header{header}
			want := "fixture"
			if existing {
				writeTestFile(t, directory, "content", "preserved")
				want = "preserved"
			} else {
				headers = append(headers, header)
			}
			archive := bundleHeaders(t, headers, []byte("fixture"))
			if err := extractBundle(bytes.NewReader(archive), directory); !os.IsExist(err) {
				t.Fatalf("overwrite extraction error = %v, want already exists", err)
			}
			body, err := os.ReadFile(filepath.Join(directory, "content"))
			if err != nil || string(body) != want {
				t.Fatalf("existing bytes = %q, %v, want %q", body, err, want)
			}
		})
	}
}

func TestExtractBundleRetainsLimits(t *testing.T) {
	t.Run("entry count", func(t *testing.T) {
		headers := make([]*tar.Header, maximumBundleFiles+1)
		for i := range headers {
			headers[i] = &tar.Header{Name: "directory", Typeflag: tar.TypeDir, Mode: 0o7777}
		}
		archive := bundleHeaders(t, headers, nil)
		if err := extractBundle(bytes.NewReader(archive), t.TempDir()); err == nil || !strings.Contains(err.Error(), "limits exceeded") {
			t.Fatalf("entry count extraction error = %v", err)
		}
	})
	t.Run("byte count", func(t *testing.T) {
		// A header alone must trigger the bound before reading or allocating
		// the declared oversized body.
		var archive bytes.Buffer
		gz := gzip.NewWriter(&archive)
		tw := tar.NewWriter(gz)
		if err := tw.WriteHeader(&tar.Header{Name: "oversized", Typeflag: tar.TypeReg, Mode: 0o7777, Size: maximumBundleBytes + 1}); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		directory := t.TempDir()
		if err := extractBundle(bytes.NewReader(archive.Bytes()), directory); err == nil || !strings.Contains(err.Error(), "limits exceeded") {
			t.Fatalf("byte count extraction error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(directory, "oversized")); !os.IsNotExist(err) {
			t.Fatalf("oversized file created: %v", err)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if err := extractBundle(bytes.NewReader(bundleHeaders(t, nil, nil)), t.TempDir()); err == nil || !strings.Contains(err.Error(), "empty") {
			t.Fatalf("empty extraction error = %v", err)
		}
	})
}

func assertArchiveFile(t *testing.T, path, want string, mode os.FileMode) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil || string(body) != want {
		t.Fatalf("installed bytes = %q, %v; want %q", body, err, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != mode {
		t.Fatalf("installed mode = %v, want %v", info.Mode(), mode)
	}
}

func bundleHeaders(t *testing.T, headers []*tar.Header, body []byte) []byte {
	t.Helper()
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for _, header := range headers {
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA {
			if _, err := tw.Write(body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}
