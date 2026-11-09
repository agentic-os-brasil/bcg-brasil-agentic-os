//go:build windows

package memory_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

func TestPrivateWikiWindowsUnsafeBindingWithoutWrites(t *testing.T) {
	root := t.TempDir()
	policy, err := basememory.Policy()
	if err != nil {
		t.Fatal(err)
	}
	e := memory.Engine{Root: filepath.Join(root, "brain", "memory"), Policy: policy}
	if _, err := e.CompilePrivateWiki(context.Background(), "case-a", ""); !errors.Is(err, memory.ErrPrivateWikiNativeUnavailable) {
		t.Fatal("unadmitted Windows publication accepted")
	}
	if page, err := e.ReadPrivateWiki("case-a", "", "index.md", 0, 64); !errors.Is(err, memory.ErrPrivateWikiNativeUnavailable) || page.Content != "" {
		t.Fatal("unadmitted Windows read accepted")
	}
	if _, err := e.InspectPrivateWikiMaintenance("case-a"); !errors.Is(err, memory.ErrPrivateWikiNativeUnavailable) {
		t.Fatalf("wiki inspection accepted an unadmitted root: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("unadmitted calls initialized state")
	}
}
