//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package datahome_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"golang.org/x/sys/unix"
)

func TestInitializeFreshPartialFailureNeverGrantsAuthorityOrErasesForeignState(t *testing.T) {
	opts := fixture(t)
	sentinel := filepath.Join(opts.UserHome, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("foreign original"), 0600); err != nil {
		t.Fatal(err)
	}
	// Force newly created directories to be unusable, exercising a real OS error
	// after mkdir rather than replacing the bootstrap implementation with a mock.
	mask := unix.Umask(0777)
	got, err := datahome.InitializeFresh(opts)
	unix.Umask(mask)
	home := filepath.Join(opts.UserHome, ".maestro")
	defer os.Chmod(home, 0700) // allow the test runner to remove its own fixture
	if err == nil || got.Writable || got.Diagnostic != "bootstrap-partial" || len(err.Error()) > 300 {
		t.Fatalf("partial bootstrap: %+v %v", got, err)
	}
	if !strings.Contains(err.Error(), "inspect read-only") {
		t.Fatalf("missing safe recovery: %v", err)
	}
	reopened, resolveErr := datahome.Resolve(opts)
	if resolveErr == nil || reopened.Writable {
		t.Fatalf("partial state admitted: %+v %v", reopened, resolveErr)
	}
	body, readErr := os.ReadFile(sentinel)
	if readErr != nil || string(body) != "foreign original" {
		t.Fatalf("foreign data modified: %q %v", body, readErr)
	}
}
