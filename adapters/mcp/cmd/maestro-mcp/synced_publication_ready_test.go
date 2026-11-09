package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

type pausedPublicationReady struct {
	entered chan<- struct{}
	release <-chan struct{}
}

func (snapshot pausedPublicationReady) MarshalJSON() ([]byte, error) {
	close(snapshot.entered)
	<-snapshot.release
	return []byte(`{"ready":"complete","generation":"N"}`), nil
}

func TestPublicationReadyIsAtomicAndExclusive(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "barrier")
	if err = scheduler.EnsurePrivateDirectory(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "ready.json")
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- publishPublicationReady(path, pausedPublicationReady{entered, release}) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("publisher did not reach the incomplete readiness control")
	}
	_, err = os.Stat(path)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ready destination became visible before complete JSON: %v", err)
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	want := []byte("{\"ready\":\"complete\",\"generation\":\"N\"}\n")
	body, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, want) {
		t.Fatalf("readiness was not published completely: %q %v", body, err)
	}
	if err = publishPublicationReady(path, map[string]string{"ready": "replacement"}); !errors.Is(err, os.ErrExist) {
		t.Fatalf("duplicate publisher did not preserve the original readiness: %v", err)
	}
	body, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(body, want) {
		t.Fatalf("duplicate publisher changed readiness: %q %v", body, err)
	}
}
