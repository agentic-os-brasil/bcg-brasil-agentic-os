//go:build darwin || linux

package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
)

func TestInitializeRejectsFIFOWithoutWrites(t *testing.T) {
	base := t.TempDir()
	ws, data := filepath.Join(base, "workspace"), filepath.Join(base, "data")
	if err := os.MkdirAll(filepath.Join(ws, ".bcgos"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(ws, ".bcgos", "workspace.json"), 0600); err != nil {
		t.Fatal(err)
	}
	before := footprintSnapshot(t, base)
	if _, err := Initialize(Options{WorkspacePath: ws, DataRoot: data}); err == nil {
		t.Fatal("FIFO manifest admitted")
	}
	if after := footprintSnapshot(t, base); !reflect.DeepEqual(before, after) {
		t.Fatal("FIFO refusal wrote initialization state")
	}
}
