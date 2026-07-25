package mermaiddoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAcceptsSupportedMermaidBlocks(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, root, "README.md", `# Architecture

`+"```mermaid"+`
flowchart LR
    A --> B
`+"```"+`

`+"```mermaid"+`
sequenceDiagram
    A->>B: bounded packet
`+"```"+`
`)

	if err := Validate(root); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsMermaidBlockWithoutDiagramType(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, root, "docs/broken.md", `# Broken

`+"```mermaid"+`
    A --> B
`+"```"+`
`)

	err := Validate(root)
	if err == nil || !strings.Contains(err.Error(), "supported diagram type") {
		t.Fatalf("Validate() error = %v, want supported diagram type failure", err)
	}
}

func TestValidateRejectsUnclosedMermaidBlock(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, root, "README.md", `# Broken

`+"```mermaid"+`
flowchart LR
    A --> B
`)

	err := Validate(root)
	if err == nil || !strings.Contains(err.Error(), "unclosed Mermaid block") {
		t.Fatalf("Validate() error = %v, want unclosed block failure", err)
	}
}

func TestValidateIgnoresGeneratedAndDependencyTrees(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, root, "README.md", "# Valid\n")
	writeMarkdown(t, root, ".git/broken.md", "```mermaid\n")
	writeMarkdown(t, root, "node_modules/package/broken.md", "```mermaid\n")
	writeMarkdown(t, root, ".claude/worktrees/generated/broken.md", "```mermaid\n")

	if err := Validate(root); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func writeMarkdown(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
