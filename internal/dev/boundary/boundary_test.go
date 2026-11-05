package boundary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAcceptsCleanDistributionSurface(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "bundles/base/manifest.yaml", "name: base\n")
	if err := Validate(root); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsDevelopmentLeak(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "bundles/base/README.md", "Run go run ./dev/harness validate\n")
	err := Validate(root)
	if err == nil || !strings.Contains(err.Error(), "references development-only path dev/harness") {
		t.Fatalf("Validate() error = %v, want development leak", err)
	}
}

func TestValidateRejectsContributorBootstrapLeak(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "bundles/base/README.md", "Run dev/bootstrap/windows.ps1\n")
	err := Validate(root)
	if err == nil || !strings.Contains(err.Error(), "references development-only path dev/bootstrap") {
		t.Fatalf("Validate() error = %v, want bootstrap leak", err)
	}
}

func TestValidateSeparatesAdapterGoTestsFromDistributedSource(t *testing.T) {
	for _, test := range []struct {
		path   string
		reject bool
	}{
		{"adapters/mcp/setup/setup_test.go", false},
		{"adapters/mcp/setup/setup.go", true},
		{"bundles/base/setup_test.go", true},
		{"adapters/mcp/setup/setup_test.go.md", true},
	} {
		t.Run(test.path, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, test.path, "package fixture\nimport _ \"example.com/internal/dev/releasepack\"\n")
			if err := Validate(root); (err != nil) != test.reject {
				t.Fatalf("Validate() error = %v, reject = %v", err, test.reject)
			}
		})
	}
}

func writeFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
