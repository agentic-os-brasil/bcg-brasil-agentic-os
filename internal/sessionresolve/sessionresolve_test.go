package sessionresolve

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionctx"
)

func TestResolveReadsOnlyPacketAuthorizedPointerWithinBudget(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "owner", "self", "voice.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("concise"), 0o600); err != nil {
		t.Fatal(err)
	}
	packet := sessionctx.Packet{Owner: sessionctx.Owner{Facets: map[string]sessionctx.Pointer{"voice": {Path: "owner/self/voice.md", Available: true}}}}
	result, err := Resolve(root, "owner/self/voice.md", "session", packet, 64)
	if err != nil || result.State != "available" || result.Body != "concise" {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}

func TestResolveRejectsUnexposedAndOversizedPointers(t *testing.T) {
	root := t.TempDir()
	packet := sessionctx.Packet{}
	if _, err := Resolve(root, "owner/self/psychological-profile.md", "session", packet, 64); err == nil {
		t.Fatal("resolved unexposed pointer")
	}
	if _, err := Resolve(root, "../secret", "session", packet, 64); err == nil {
		t.Fatal("resolved traversal pointer")
	}
}

func TestResolveRejectsAuthorizedPointerWhenItIsSymlinkedOutsideRoot(t *testing.T) {
	root, outside := t.TempDir(), filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "owner", "self", "voice.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	packet := sessionctx.Packet{Owner: sessionctx.Owner{Facets: map[string]sessionctx.Pointer{"voice": {Path: "owner/self/voice.md", Available: true}}}}
	if _, err := Resolve(root, "owner/self/voice.md", "session", packet, 64); err == nil {
		t.Fatal("resolved symlink outside root")
	}
}
