package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequestValidateAcceptsAllowlistedLocalSource(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "incoming")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(nested, "report.docx")
	if err := os.WriteFile(source, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}

	info, err := (Request{SourcePath: source, WorkspacePath: root, Policy: DefaultPolicy()}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if info.Size() != int64(len("fixture")) {
		t.Fatalf("size = %d", info.Size())
	}
}

func TestRequestValidateRejectsSourceOutsideWorkspace(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	outside := filepath.Join(parent, "outside")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(outside, "report.docx")
	if err := os.WriteFile(source, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, candidate := range map[string]string{
		"direct":    source,
		"traversal": filepath.Join(workspace, "..", "outside", "report.docx"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := (Request{SourcePath: candidate, WorkspacePath: workspace, Policy: DefaultPolicy()}).Validate()
			if err == nil || !strings.Contains(err.Error(), "outside the workspace scope") {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestRequestValidateRejectsSymlinkedParentEscape(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	outside := filepath.Join(parent, "outside")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(outside, "report.docx")
	if err := os.WriteFile(source, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(workspace, "linked")
	if err := os.Symlink(outside, linked); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := (Request{SourcePath: filepath.Join(linked, "report.docx"), WorkspacePath: workspace, Policy: DefaultPolicy()}).Validate()
	if err == nil || !strings.Contains(err.Error(), "symlink components") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRequestValidateRejectsSymlinkedParentWithinWorkspace(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real")
	linked := filepath.Join(root, "linked")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(target, "report.docx")
	if err := os.WriteFile(source, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, linked); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := (Request{SourcePath: filepath.Join(linked, "report.docx"), WorkspacePath: root, Policy: DefaultPolicy()}).Validate()
	if err == nil || !strings.Contains(err.Error(), "symlink components") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRequestValidateRejectsUnsafeInputs(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name   string
		source string
		mutate func(*Policy)
		want   string
	}{
		{name: "unsupported format", source: "report.zip", want: "not allowlisted"},
		{name: "network", source: "report.docx", mutate: func(policy *Policy) { policy.AllowNetwork = true }, want: "network ingestion"},
		{name: "plugin", source: "report.docx", mutate: func(policy *Policy) { policy.AllowPlugins = true }, want: "plugin ingestion"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := filepath.Join(root, tc.source)
			if err := os.WriteFile(source, []byte("fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
			policy := DefaultPolicy()
			if tc.mutate != nil {
				tc.mutate(&policy)
			}
			_, err := (Request{SourcePath: source, WorkspacePath: root, Policy: policy}).Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestRequestValidateRejectsSymlinkSource(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.docx")
	link := filepath.Join(root, "link.docx")
	if err := os.WriteFile(target, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := (Request{SourcePath: link, WorkspacePath: root, Policy: DefaultPolicy()}).Validate()
	if err == nil || !strings.Contains(err.Error(), "symlinks") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestFingerprintIsStableSHA256(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := Fingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint != "f16d05ec6b29248d2c61adb1e9263f78e4f7bace1b955014a2d17872cfe4064d" {
		t.Fatalf("fingerprint = %s", fingerprint)
	}
}
