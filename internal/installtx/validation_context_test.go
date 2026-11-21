package installtx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

type cancelInstallDigestWriter struct {
	cancel context.CancelFunc
	bytes  int
}

func (w *cancelInstallDigestWriter) Write(p []byte) (int, error) {
	w.bytes += len(p)
	w.cancel()
	return len(p), nil
}

func TestInstalledValidationHashHonorsMidReadCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	body := bytes.Repeat([]byte("authenticated input"), 1<<14)
	sink := &cancelInstallDigestWriter{cancel: cancel}
	n, err := io.Copy(sink, installReadContext{ctx, bytes.NewReader(body)})
	if !errors.Is(err, context.Canceled) || n <= 0 || n >= int64(len(body)) {
		t.Fatalf("hash continued after cancellation or bypassed Read: n=%d err=%v", n, err)
	}
}

func TestCurrentValidationHonorsAlreadyCancelledCaller(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ValidateQualifiedLaunchContext(ctx, "not-opened", "not-opened", ReleaseBinding{}, time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("qualified validation ignored caller: %v", err)
	}
	if _, err := ValidateActivatedCurrentContext(ctx, "not-opened", "not-opened", ReleaseBinding{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("activated validation ignored caller: %v", err)
	}
	if err := CheckActivatedServerContext(ctx, "not-opened", State{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("identity validation ignored caller: %v", err)
	}
}
