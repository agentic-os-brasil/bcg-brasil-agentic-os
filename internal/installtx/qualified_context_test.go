package installtx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQualifiedPublicationCanceledBeforeLockKeepsForeignState(t *testing.T) {
	managed := t.TempDir()
	lock := filepath.Join(managed, ".activation.lock")
	expected := []byte("synthetic-foreign-owner\n")
	if err := os.WriteFile(lock, expected, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	published := false
	err := GuardQualifiedCurrentContext(ctx, managed, filepath.Join(managed, "state"), ReleaseBinding{}, time.Now(), func() error { published = true; return nil })
	if !errors.Is(err, context.Canceled) || published {
		t.Fatalf("canceled publication: called=%t err=%v", published, err)
	}
	got, err := os.ReadFile(lock)
	if err != nil || string(got) != string(expected) {
		t.Fatal("cancellation removed or changed another owner's lock")
	}
}
