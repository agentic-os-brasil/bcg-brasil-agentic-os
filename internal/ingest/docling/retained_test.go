//go:build darwin || linux

package docling_test

import (
	"context"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func retainedRuntime(t *testing.T) (*docling.Runtime, string) {
	t.Helper()
	root := os.Getenv("MAESTRO_DOCLING_RETAINED_ROOT")
	seed := os.Getenv("MAESTRO_DOCLING_RETAINED_SEED")
	if root == "" || seed == "" {
		t.Skip("explicit retained engineering fixture and independent seed required")
	}
	return docling.New(docling.Options{DataRoot: filepath.Join(root, "state"), AuthorityRegistryPath: filepath.Join(root, "authority.json"), AuthorityRegistrySHA256: seed, CLIVersion: "0.1.0"}), root
}
func TestRetainedDoclingConversion(t *testing.T) {
	r, root := retainedRuntime(t)
	data := filepath.Join(root, "state")
	space := filepath.Join(root, "workspace")
	w, err := workspace.Inspect(space, data)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	got, err := r.Convert(context.Background(), ingest.Request{SourcePath: "sample.docx", WorkspacePath: space, Policy: ingest.DefaultPolicy()}, w.WorkspaceID)
	t.Logf("retained Convert=%s status=%s", time.Since(started), got.Result.Status)
	if err != nil || got.Result.Status != ingest.StatusUsable || !got.Provenance.NetworkDenied || len(got.Markdown) != 157 || len(got.Document) != 3553 {
		t.Fatal(got.Result, err)
	}
}
func TestRetainedDoclingVerifierAliases(t *testing.T) {
	r, root := retainedRuntime(t)
	status := r.Status(context.Background())
	if status.State != "ready" {
		t.Fatal(status)
	}
	installed := filepath.Join(root, "state/ingestion/docling/installed", status.PackSHA256)
	refused := func(t *testing.T) {
		t.Helper()
		if got := r.Status(context.Background()); got.State != "unavailable" || got.Reason != "installed_pack_changed" {
			t.Fatalf("changed installed tree admitted: %+v", got)
		}
	}
	t.Run("dependency bytes", func(t *testing.T) {
		name := filepath.Join(installed, "site-packages/docling/document_converter.py")
		body := read(t, name)
		_ = os.Chmod(name, 0600)
		changed := append([]byte(nil), body...)
		changed[0] ^= 1
		put(t, name, changed)
		_ = os.Chmod(name, 0444)
		defer func() { _ = os.Chmod(name, 0600); put(t, name, body); _ = os.Chmod(name, 0444) }()
		refused(t)
	})
	t.Run("extra", func(t *testing.T) {
		name := filepath.Join(installed, "surprise.pth")
		_ = os.Chmod(installed, 0700)
		put(t, name, []byte("import os"))
		_ = os.Chmod(installed, 0555)
		defer func() { _ = os.Chmod(installed, 0700); _ = os.Remove(name); _ = os.Chmod(installed, 0555) }()
		refused(t)
	})
	t.Run("hardlink", func(t *testing.T) {
		name := filepath.Join(root, "hardlink-test")
		if err := os.Link(filepath.Join(installed, "bridge.py"), name); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(name)
		refused(t)
	})
	for _, kind := range []string{"missing", "symlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			name := filepath.Join(installed, "site-packages/pydantic/py.typed")
			if info, err := os.Stat(name); err != nil || info.Size() != 0 {
				t.Fatalf("empty-file fixture: %v %v", info, err)
			}
			saved := filepath.Join(root, "saved-empty-test")
			parent := filepath.Dir(name)
			_ = os.Chmod(parent, 0700)
			if err := os.Rename(name, saved); err != nil {
				t.Fatal(err)
			}
			if kind == "symlink" {
				if err := os.Symlink(saved, name); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "fifo" {
				if err := unix.Mkfifo(name, 0444); err != nil {
					t.Fatal(err)
				}
			}
			_ = os.Chmod(parent, 0555)
			defer func() {
				_ = os.Chmod(parent, 0700)
				_ = os.Remove(name)
				_ = os.Rename(saved, name)
				_ = os.Chmod(parent, 0555)
			}()
			refused(t)
		})
	}
	t.Run("directory alias", func(t *testing.T) {
		name := filepath.Join(installed, "site-packages/docling")
		saved := filepath.Join(root, "saved-docling-test")
		parent := filepath.Dir(name)
		_ = os.Chmod(parent, 0700)
		_ = os.Chmod(name, 0700)
		if err := os.Rename(name, saved); err != nil {
			_ = os.Chmod(name, 0555)
			_ = os.Chmod(parent, 0555)
			t.Fatal(err)
		}
		_ = os.Chmod(saved, 0555)
		if err := os.Symlink(saved, name); err != nil {
			t.Fatal(err)
		}
		_ = os.Chmod(parent, 0555)
		defer func() {
			_ = os.Chmod(parent, 0700)
			_ = os.Remove(name)
			_ = os.Chmod(saved, 0700)
			_ = os.Rename(saved, name)
			_ = os.Chmod(name, 0555)
			_ = os.Chmod(parent, 0555)
		}()
		refused(t)
	})
	if got := r.Status(context.Background()); got.State != "ready" {
		t.Fatalf("fixture not restored: %+v", got)
	}
}
