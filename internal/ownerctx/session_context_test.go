package ownerctx_test

import (
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

func acceptedSessionRoot(t *testing.T) string {
	t.Helper()
	root := ownerRoot(t)
	if _, err := ownerctx.SelectOnboardingTrack(root, "quick"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"owner-identity", "personal-context", "professional-role", "communication-style", "preferences", "quality-bar"} {
		proposal := proposeOwner(t, root, id, "# Accepted synthetic owner\n\nAnswer for "+id+".\n")
		review, err := ownerctx.ReviewRefinement(root, proposal.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = ownerctx.ApplyReviewedRefinement(root, proposal.ID, review.ReviewDigest, true); err != nil {
			t.Fatal(err)
		}
	}
	review, err := ownerctx.ReviewOnboarding(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ownerctx.ConfirmOnboarding(root, review.ReviewDigest); err != nil {
		t.Fatal(err)
	}
	return root
}

func sessionOwnerFiles(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	files := map[string][32]byte{}
	err := filepath.WalkDir(filepath.Join(root, "owner"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[path] = sha256.Sum256(body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestAcceptedSessionContextUsesExistingLockWithoutWrites(t *testing.T) {
	root := acceptedSessionRoot(t)
	lock := filepath.Join(root, "owner", "interview", ".transition.lock")
	before, err := os.Stat(lock)
	if err != nil {
		t.Fatal(err)
	}
	filesBefore := sessionOwnerFiles(t, root)
	first, err := ownerctx.ProjectAcceptedSessionContext(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ownerctx.ProjectAcceptedSessionContext(root)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(lock)
	if err != nil {
		t.Fatal(err)
	}
	if first.Reader != "session" || len(first.Facets) != 6 || first.Generation != second.Generation || first.ConfirmationSHA256 == "" || !os.SameFile(before, after) || before.ModTime() != after.ModTime() {
		t.Fatal("projection changed authority or existing native lock")
	}
	if !reflect.DeepEqual(filesBefore, sessionOwnerFiles(t, root)) {
		t.Fatal("read modified canonical owner files")
	}
	if err = os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if _, err = ownerctx.ProjectAcceptedSessionContext(root); err == nil {
		t.Fatal("missing lock admitted")
	}
	if _, err = os.Lstat(lock); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read created missing lock: %v", err)
	}
}

func TestAcceptedSessionContextRefusesUnconfirmedAndUnsafeFacet(t *testing.T) {
	t.Run("unconfirmed", func(t *testing.T) {
		root := ownerRoot(t)
		if _, err := ownerctx.ProjectAcceptedSessionContext(root); !errors.Is(err, ownerctx.ErrSessionContextUnconfirmed) {
			t.Fatalf("unconfirmed profile: %v", err)
		}
	})
	for _, kind := range []string{"directory", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			root := acceptedSessionRoot(t)
			path := filepath.Join(root, "owner", "self", "communication-style.md")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if kind == "directory" {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(path, []byte(strings.Repeat("x", (32<<10)+1)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if view, err := ownerctx.ProjectAcceptedSessionContext(root); err == nil || len(view.Facets) != 0 {
				t.Fatal("unsafe facet delivered")
			}
		})
	}
}
