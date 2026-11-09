package docling_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestRevalidateSourceCanonicalBoundary(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		if err := docling.New(docling.Options{}).RevalidateSource(context.Background(), ingest.Request{}, "workspace", "digest"); err == nil {
			t.Fatal("unsupported source validation platform admitted")
		}
		return
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, space := filepath.Join(root, "state"), filepath.Join(root, "workspace")
	state, err := workspace.Initialize(workspace.Options{WorkspacePath: space, DataRoot: data})
	if err != nil {
		t.Fatal(err)
	}
	engine := docling.New(docling.Options{DataRoot: data})
	sourcePath := filepath.Join(space, "selected.docx")
	body := []byte("synthetic source reader fixture; not conversion evidence")
	if err := os.WriteFile(sourcePath, body, 0600); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	request := ingest.Request{SourcePath: "selected.docx", WorkspacePath: space}
	for _, mode := range []os.FileMode{0600, 0644, 0444} {
		if err := os.Chmod(sourcePath, mode); err != nil {
			t.Fatal(err)
		}
		if err := engine.RevalidateSource(context.Background(), request, state.WorkspaceID, digest); err != nil {
			t.Fatalf("ordinary readable mode %04o refused: %v", mode, err)
		}
		info, err := os.Stat(sourcePath)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatal("revalidation changed source permissions", err)
		}
	}
	if err := os.Chmod(sourcePath, 0600); err != nil {
		t.Fatal(err)
	}
	refuse := func(t *testing.T, req ingest.Request, id, sha string) {
		t.Helper()
		if err := engine.RevalidateSource(context.Background(), req, id, sha); err == nil {
			t.Fatal("unsafe or changed source admitted")
		}
	}
	t.Run("hardlink", func(t *testing.T) {
		alias := filepath.Join(root, "source-alias.docx")
		if err := os.Link(sourcePath, alias); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(alias)
		refuse(t, request, state.WorkspaceID, digest)
	})
	t.Run("symlink", func(t *testing.T) {
		saved := filepath.Join(space, "saved.docx")
		if err := os.Rename(sourcePath, saved); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Remove(sourcePath); _ = os.Rename(saved, sourcePath) }()
		if err := os.Symlink(saved, sourcePath); err != nil {
			t.Fatal(err)
		}
		refuse(t, request, state.WorkspaceID, digest)
	})
	t.Run("changed bytes", func(t *testing.T) {
		if err := os.WriteFile(sourcePath, []byte("changed"), 0600); err != nil {
			t.Fatal(err)
		}
		defer os.WriteFile(sourcePath, body, 0600)
		refuse(t, request, state.WorkspaceID, digest)
	})
	t.Run("canonical input budget", func(t *testing.T) {
		file, err := os.OpenFile(sourcePath, os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		err = file.Truncate(ingest.DefaultPolicy().MaxInputBytes + 1)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		defer os.WriteFile(sourcePath, body, 0600)
		widened := request
		widened.Policy.MaxInputBytes = 1 << 40
		refuse(t, widened, state.WorkspaceID, digest)
	})
	t.Run("scope", func(t *testing.T) {
		refuse(t, request, "foreign-workspace", digest)
		for _, name := range []string{"../selected.docx", sourcePath} {
			outside := request
			outside.SourcePath = name
			refuse(t, outside, state.WorkspaceID, digest)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := engine.RevalidateSource(ctx, request, state.WorkspaceID, digest); !errors.Is(err, context.Canceled) {
			t.Fatal("caller cancellation lost", err)
		}
	})
	if err := engine.RevalidateSource(context.Background(), request, state.WorkspaceID, digest); err != nil {
		t.Fatal("restored source unavailable", err)
	}
}
