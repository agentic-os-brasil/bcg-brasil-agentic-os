package docling_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWindowsImplementationEmbedsActualProductionDependencyClosure(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", ".."))
	command := exec.Command("go", "list", "-deps", "-json", "./internal/ingest/docling")
	command.Dir = root
	command.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0", "GOPROXY=off", "GOSUMDB=off")
	body, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(root, "internal", "docling_identity_windows.go"))
	if err != nil {
		t.Fatal(err)
	}
	embedded := map[string]bool{}
	for _, line := range strings.Split(string(manifest), "\n") {
		if strings.HasPrefix(line, "//go:embed ") {
			name := strings.TrimPrefix(line, "//go:embed ")
			if strings.ContainsAny(name, "*?") || strings.HasSuffix(name, "_test.go") || strings.Contains(name, "/testdata/") {
				t.Fatalf("identity embeds non-production or unbounded source %s", name)
			}
			embedded[name] = true
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	expected := map[string]bool{}
	for {
		var dependency struct {
			Dir                 string
			GoFiles, EmbedFiles []string
			Module              *struct{ Path string }
		}
		err := decoder.Decode(&dependency)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if dependency.Module != nil && dependency.Module.Path != "github.com/agentic-os-brasil/bcg-brasil-agentic-os" {
			if !strings.Contains(string(manifest), `dependency.Path != "`+dependency.Module.Path+`"`) {
				t.Errorf("actual external trust-boundary dependency absent from implementation identity: %s", dependency.Module.Path)
			}
			continue
		}
		if dependency.Module == nil {
			continue
		}
		files := dependency.GoFiles
		if dependency.Dir != filepath.Join(root, "internal") {
			files = append(files, dependency.EmbedFiles...)
		}
		for _, name := range files {
			relative, err := filepath.Rel(filepath.Join(root, "internal"), filepath.Join(dependency.Dir, name))
			if err != nil {
				t.Fatal(err)
			}
			expected[filepath.ToSlash(relative)] = true
		}
	}
	for name := range expected {
		if !embedded[name] {
			t.Errorf("actual Windows production dependency absent from implementation identity: %s", name)
		}
	}
	for name := range embedded {
		if !expected[name] {
			t.Errorf("identity includes source outside the actual Windows production closure: %s", name)
		}
	}
}
