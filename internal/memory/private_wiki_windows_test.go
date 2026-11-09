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

func TestPrivateWikiWindowsUnavailableWithoutWrites(t *testing.T) {
	root := t.TempDir()
	policy, err := basememory.Policy()
	if err != nil {
		t.Fatal(err)
	}
	e := memory.Engine{Root: filepath.Join(root, "brain", "memory"), Policy: policy}
	if _, err := e.CompilePrivateWiki(context.Background(), "case-a", ""); !errors.Is(err, memory.ErrPrivateWikiNativeUnavailable) {
		t.Fatal("unqualified Windows publication admitted")
	}
	if page, err := e.ReadPrivateWiki("case-a", "", "index.md", 0, 64); !errors.Is(err, memory.ErrPrivateWikiNativeUnavailable) || page.Content != "" {
		t.Fatal("unqualified Windows read admitted")
	}
	if _, err := e.InspectPrivateWikiMaintenance("case-a"); !errors.Is(err, memory.ErrPrivateWikiNativeUnavailable) {
		t.Fatalf("wiki inspection lost its independent native gate: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("unsupported calls initialized state")
	}
}
