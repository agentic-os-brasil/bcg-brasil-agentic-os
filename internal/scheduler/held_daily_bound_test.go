package scheduler_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestHeldPrivateUpdateDailyBound(t *testing.T) {
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	held, e := scheduler.HoldDirectory(root, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer held.Close()
	body := bytes.Repeat([]byte("d"), 2<<20)
	if e = held.UpdatePrivateFile("daily.md", 4<<20, func(current []byte) ([]byte, error) {
		if len(current) != 0 {
			t.Fatal("unexpected existing page")
		}
		return body, nil
	}); e != nil {
		t.Fatal(e)
	}
	got, e := held.ReadPrivateFile("daily.md", 4<<20)
	if e != nil || !bytes.Equal(got, body) {
		t.Fatal("bounded held write/read failed", e)
	}
	called := false
	if e = scheduler.UpdatePrivateFile(filepath.Join(root, "ordinary.json"), 2<<20, func([]byte) ([]byte, error) { called = true; return body, nil }); e == nil || called {
		t.Fatal("path-based 1MiB limit widened", e)
	}
	if e = held.UpdatePrivateFile("oversize.md", (4<<20)+1, func([]byte) ([]byte, error) { t.Fatal("oversized bound reached callback"); return nil, nil }); e == nil {
		t.Fatal("held bound became unbounded")
	}
}
