//go:build windows

package docling

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWindowsSharedImplementationIdentityAcrossEntryModules(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", ".."))
	var expected string
	for _, directory := range []string{root, filepath.Join(root, "adapters", "mcp")} {
		command := exec.Command("go", "run", "-mod=readonly", "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling/testdata/identity")
		command.Dir = directory
		command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOPROXY=off", "GOSUMDB=off")
		body, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("independent module identity: %v: %s", err, body)
		}
		identity := strings.TrimSpace(string(body))
		if len(identity) != 64 {
			t.Fatalf("invalid implementation identity %q", identity)
		}
		if expected != "" && identity != expected {
			t.Fatal("installer and MCP entry modules compute different shared converter identity")
		}
		expected = identity
	}
}
