package installer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type cancelClientDigestWriter struct{ cancel context.CancelFunc }

func (w cancelClientDigestWriter) Write(p []byte) (int, error) { w.cancel(); return len(p), nil }

func TestClientValidationHashHonorsMidReadCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	body := bytes.Repeat([]byte("current client bytes"), 1<<14)
	n, err := io.Copy(cancelClientDigestWriter{cancel}, installerReadContext{ctx, bytes.NewReader(body)})
	if !errors.Is(err, context.Canceled) || n <= 0 || n >= int64(len(body)) {
		t.Fatalf("client hash continued after cancellation or bypassed Read: n=%d err=%v", n, err)
	}
}

func TestClientDigestContextRetainsAuthenticationAndCallerCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client")
	body := []byte("exact client fixture")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ClientFileDigestContext(t.Context(), path)
	if err != nil || got != fmt.Sprintf("%x", sha256.Sum256(body)) {
		t.Fatalf("context digest changed exact bytes: %q %v", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ClientFileDigestContext(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("file digest ignored caller: %v", err)
	}
	if _, err := ClientArchiveDigestContext(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("archive digest ignored caller: %v", err)
	}
	if err := VerifyExperimentalInstalledBindingContext(ctx, SeededOperationOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("fixed-role validation ignored caller: %v", err)
	}
	if _, err := ReadClientsSeeded(ctx, ClientOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("client read ignored caller: %v", err)
	}
}
