//go:build windows

package memory_test

import (
	"context"
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
	if _, err := e.CompilePrivateWiki(context.Background(), "case-a", ""); err == nil {
		t.Fatal("unqualified Windows publication admitted")
	}
	if page, err := e.ReadPrivateWiki("case-a", "", "index.md", 0, 64); err == nil || page.Content != "" {
		t.Fatal("unqualified Windows read admitted")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("unsupported calls initialized state")
	}
}
