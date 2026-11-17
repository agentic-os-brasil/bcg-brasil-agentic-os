package acquisition

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
)

func TestVerifiedInputCacheResumesWithoutClobber(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	if err := privateDirectory(root, true); err != nil {
		t.Fatal(err)
	}
	if err := privateDirectory(filepath.Join(root, "blobs"), true); err != nil {
		t.Fatal(err)
	}
	attempt := filepath.Join(root, "attempt")
	if err := privateDirectory(attempt, true); err != nil {
		t.Fatal(err)
	}
	body := []byte("authenticated public input")
	input := docling.Input{Name: "fixture", Version: "1", Filename: "fixture.whl", Size: int64(len(body)), SHA256: digest(body)}
	pending := filepath.Join(attempt, "input.part")
	if err := writeInput(pending, input, bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	if err := publishBlob(root, input, pending); err != nil {
		t.Fatal(err)
	}
	if cached, err := cachedInput(root, input); err != nil || !cached {
		t.Fatalf("completed input not reusable: %v %v", cached, err)
	}
	if err := publishBlob(root, input, pending); err != nil {
		t.Fatalf("identical repeat should preserve cache: %v", err)
	}
	blob := filepath.Join(root, "blobs", input.SHA256)
	if err := os.WriteFile(blob, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := cachedInput(root, input); err == nil {
		t.Fatal("corrupt completed cache accepted")
	}
	if err := publishBlob(root, input, pending); err == nil {
		t.Fatal("corrupt occupied cache overwritten")
	}
	if got, err := os.ReadFile(blob); err != nil || string(got) != "corrupt" {
		t.Fatal("occupied cache was changed")
	}
}

func TestInputStreamBoundsPreserveExistingAndIncompleteFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := privateDirectory(root, true); err != nil {
		t.Fatal(err)
	}
	body := []byte("pinned")
	input := docling.Input{Filename: "fixed.whl", Size: int64(len(body)), SHA256: digest(body)}
	for _, test := range []struct {
		name string
		body []byte
	}{{"short", body[:2]}, {"oversize", append(append([]byte{}, body...), 'x')}, {"hash", []byte("wrong!")}} {
		t.Run(test.name, func(t *testing.T) {
			name := filepath.Join(root, test.name)
			if err := writeInput(name, input, bytes.NewReader(test.body)); err == nil {
				t.Fatal("invalid stream accepted")
			}
			if _, err := os.Lstat(name); err != nil {
				t.Fatal("interrupted private input not preserved")
			}
		})
	}
	name := filepath.Join(root, "occupied")
	if err := os.WriteFile(name, body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeInput(name, input, bytes.NewReader([]byte("wrong!"))); err == nil {
		t.Fatal("occupied input opened for overwrite")
	}
	if got, err := os.ReadFile(name); err != nil || !bytes.Equal(got, body) {
		t.Fatal("occupied input changed")
	}
}

func TestCacheReviewRejectsLinksAndRetainsInterruptedAttemptLimit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	if err := privateDirectory(root, true); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < maximumAttempts; n++ {
		if err := privateDirectory(filepath.Join(root, fmt.Sprintf("attempt-%d", n)), true); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := cacheState(root, nil); err == nil {
		t.Fatal("attempt bound ignored")
	}
	linked := filepath.Join(t.TempDir(), "cache-link")
	if err := os.Symlink(root, linked); err != nil {
		t.Skip("link fixture unavailable")
	}
	if _, _, err := cacheState(linked, nil); err == nil {
		t.Fatal("linked cache accepted")
	}
}
func TestReviewBindsCachedBytesAndDestination(t *testing.T) {
	f := fixture(t)
	p, err := Review(context.Background(), f.options)
	if err != nil {
		t.Fatal(err)
	}
	changed := f.options
	changed.CacheRoot = filepath.Join(filepath.Dir(f.options.CacheRoot), "other-cache")
	q, err := reviewAt(context.Background(), changed, mustTime(t, p.ReviewedAt), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if p.SHA256 == q.SHA256 {
		t.Fatal("destination not bound")
	}
	if err = privateDirectory(f.options.CacheRoot, true); err != nil {
		t.Fatal(err)
	}
	if err = privateDirectory(filepath.Join(f.options.CacheRoot, "blobs"), true); err != nil {
		t.Fatal(err)
	}
	// A corrupt occupied selected hash is preserved and makes review unavailable.
	writeFixture(t, filepath.Join(f.options.CacheRoot, "blobs", p.Inputs[0].SHA256), []byte("corrupt"))
	if _, err = Review(context.Background(), f.options); err == nil {
		t.Fatal("corrupt cache accepted")
	}
}
func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	stamp, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return stamp
}
