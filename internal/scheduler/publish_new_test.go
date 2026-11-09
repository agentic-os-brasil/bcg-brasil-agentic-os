//go:build darwin || linux

package scheduler_test

import (
	"bytes"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishNewPrivateFileAtomicAndExclusive(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.EnsurePrivateDirectory(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "artifact")
	body := bytes.Repeat([]byte("complete immutable content\n"), 50000)
	done := make(chan error, 1)
	go func() { done <- scheduler.PublishNewPrivateFile(path, body) }()
	for {
		got, err := os.ReadFile(path)
		if err == nil && !bytes.Equal(got, body) {
			t.Fatal("partially published artifact visible")
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			goto published
		default:
		}
	}
published:
	if err := scheduler.PublishNewPrivateFile(path, []byte("replace")); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing artifact not preserved: %v", err)
	}
	got, err := scheduler.ReadPrivateFile(path, int64(len(body)))
	if err != nil || !bytes.Equal(got, body) {
		t.Fatal("immutable content changed", err)
	}
	link := filepath.Join(root, "alias")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.PublishNewPrivateFile(filepath.Join(link, "escaped"), body); err == nil {
		t.Fatal("aliased parent admitted")
	}
	if err := os.Symlink(path, filepath.Join(root, "leaf")); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.PublishNewPrivateFile(filepath.Join(root, "leaf"), body); err == nil {
		t.Fatal("aliased leaf admitted")
	}
}
