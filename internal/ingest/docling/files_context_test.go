package docling

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type cancelAfterRead struct {
	reader io.Reader
	cancel context.CancelFunc
	reads  int
}

func (r *cancelAfterRead) Read(body []byte) (int, error) {
	r.reads++
	n, err := r.reader.Read(body)
	r.cancel()
	return n, err
}

func TestContextReaderStopsDuringRead(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source := &cancelAfterRead{reader: bytes.NewReader(bytes.Repeat([]byte("payload"), 1<<18)), cancel: cancel}
	_, err := io.ReadAll(contextReader{ctx, source})
	if !errors.Is(err, context.Canceled) || source.reads != 1 {
		t.Fatalf("cancelled read continued: error=%v; reads=%d", err, source.reads)
	}
	reader := contextReader{t.Context(), bytes.NewReader(make([]byte, 128<<10))}
	if n, err := reader.Read(make([]byte, 128<<10)); err != nil || n != 32<<10 {
		t.Fatalf("read lacks a cooperative block bound: bytes=%d; error=%v", n, err)
	}
}

func TestContextFileReadAndDigestPreserveBounds(t *testing.T) {
	name := filepath.Join(t.TempDir(), "fixture.bin")
	want := bytes.Repeat([]byte("authenticated fixture"), 1<<12)
	if err := os.WriteFile(name, want, 0600); err != nil {
		t.Fatal(err)
	}
	body, info, err := readFileContext(t.Context(), name, int64(len(want)))
	if err != nil || !bytes.Equal(body, want) || info == nil || info.Size() != int64(len(want)) {
		t.Fatalf("unchanged bounded file refused: %v", err)
	}
	if _, _, err := readFileContext(t.Context(), name, int64(len(want)-1)); err == nil {
		t.Fatal("oversized file admitted")
	}
	if got, err := digestContext(t.Context(), body); err != nil || got != digest(want) {
		t.Fatalf("cancellable digest changed bytes: %q %v", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := readFileContext(ctx, name, int64(len(want))); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled file read accepted: %v", err)
	}
	if _, err := digestContext(ctx, body); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled digest accepted: %v", err)
	}
	if actual, err := os.ReadFile(name); err != nil || !bytes.Equal(actual, want) {
		t.Fatalf("read-only cancellation changed the file: %v", err)
	}
}

func TestArchiveEntriesContextPreservesValidationAndCancellation(t *testing.T) {
	var body bytes.Buffer
	gz := gzip.NewWriter(&body)
	tarball := tar.NewWriter(gz)
	want := bytes.Repeat([]byte("fixture"), 1<<16)
	if err := tarball.WriteHeader(&tar.Header{Name: "fixture.txt", Mode: 0444, Size: int64(len(want)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarball.Write(want); err != nil {
		t.Fatal(err)
	}
	if err := tarball.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := archiveEntriesContext(t.Context(), body.Bytes())
	if err != nil || len(entries) != 1 || entries["fixture.txt"].mode != 0444 || !bytes.Equal(entries["fixture.txt"].body, want) {
		t.Fatalf("bounded archive bytes changed: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := archiveEntriesContext(ctx, body.Bytes()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled archive continued: %v", err)
	}
	if _, err := archiveEntriesContext(t.Context(), append(body.Bytes(), 'x')); err == nil {
		t.Fatal("trailing archive bytes admitted")
	}
}
