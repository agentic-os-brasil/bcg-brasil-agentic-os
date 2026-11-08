//go:build !windows

package ownerctx_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"golang.org/x/sys/unix"
)

func TestOwnerReviewRejectsUnsafeFilesWithoutDisclosureOrPublication(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "fifo", "oversize", "trailing-json", "duplicate-json", "registry-path", "proposal-id", "proposal-body"} {
		t.Run(kind, func(t *testing.T) {
			root := ownerRoot(t)
			r := proposeOwner(t, root, "personal-context", "## Current\nSynthetic reviewed answer.\n")
			review, err := ownerctx.ReviewRefinement(root, r.ID)
			if err != nil {
				t.Fatal(err)
			}
			proposal := filepath.Join(root, "owner", "refinement", "proposals", r.ID+".json")
			facet := filepath.Join(root, "owner", "self", "personal-context.md")
			before, err := os.ReadFile(facet)
			if err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(t.TempDir(), "private-marker")
			if err := os.WriteFile(outside, []byte("outside-private-marker"), 0600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "symlink", "hardlink", "fifo":
				if err := os.Remove(proposal); err != nil {
					t.Fatal(err)
				}
				if kind == "symlink" {
					err = os.Symlink(outside, proposal)
				} else if kind == "hardlink" {
					err = os.Link(outside, proposal)
				} else {
					err = unix.Mkfifo(proposal, 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
			case "oversize":
				err = os.WriteFile(proposal, []byte(strings.Repeat("x", 65537)), 0600)
			case "trailing-json":
				var body []byte
				body, err = os.ReadFile(proposal)
				if err == nil {
					err = os.WriteFile(proposal, append(body, []byte("{}")...), 0600)
				}
			case "duplicate-json":
				var body []byte
				body, err = os.ReadFile(proposal)
				if err == nil {
					err = os.WriteFile(proposal, append([]byte(`{"facet":"voice",`), body[1:]...), 0600)
				}
			case "registry-path":
				path := filepath.Join(root, "owner", "registry.json")
				var body []byte
				body, err = os.ReadFile(path)
				if err == nil {
					body = []byte(strings.Replace(string(body), "owner/self/personal-context.md", outside, 1))
					err = os.WriteFile(path, body, 0600)
				}
			case "proposal-id", "proposal-body":
				var body []byte
				body, err = os.ReadFile(proposal)
				if err != nil {
					t.Fatal(err)
				}
				var value map[string]any
				if err = json.Unmarshal(body, &value); err != nil {
					t.Fatal(err)
				}
				if kind == "proposal-id" {
					value["id"] = "different-proposal"
				} else {
					value["proposed_body"] = "Changed after trusted review"
				}
				body, err = json.Marshal(value)
				if err == nil {
					err = os.WriteFile(proposal, body, 0600)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if kind != "proposal-body" {
				value, err := ownerctx.ReviewRefinement(root, r.ID)
				if err == nil || value.ProposedBody != "" || value.CurrentBody != "" {
					t.Fatalf("unsafe review exposed text: %#v %v", value, err)
				}
			}
			if _, err := ownerctx.ApplyReviewedRefinement(root, r.ID, review.ReviewDigest, true); err == nil {
				t.Fatal("unsafe/stale review applied")
			}
			current, err := os.ReadFile(facet)
			if err != nil || string(current) != string(before) {
				t.Fatalf("canonical facet changed: %v", err)
			}
			marker, err := os.ReadFile(outside)
			if err != nil || string(marker) != "outside-private-marker" {
				t.Fatalf("outside file changed: %v", err)
			}
			if entries, _ := os.ReadDir(filepath.Join(root, "owner", "refinement", "audits")); len(entries) != 0 {
				t.Fatal("refused operation published an audit")
			}
		})
	}
}

func TestOwnerApplyRejectsUnsafeCanonicalFacet(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "fifo", "oversize", "ancestor"} {
		t.Run(kind, func(t *testing.T) {
			root := ownerRoot(t)
			r := proposeOwner(t, root, "owner-identity", "Synthetic replacement")
			review, err := ownerctx.ReviewRefinement(root, r.ID)
			if err != nil {
				t.Fatal(err)
			}
			facet := filepath.Join(root, "owner", "self", "owner-identity.md")
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(outside, []byte("outside-private-marker"), 0600); err != nil {
				t.Fatal(err)
			}
			if kind == "ancestor" {
				parent := filepath.Dir(facet)
				if err := os.Rename(parent, parent+"-held"); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(filepath.Dir(outside), parent)
			} else {
				if err := os.Remove(facet); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "symlink":
					err = os.Symlink(outside, facet)
				case "hardlink":
					err = os.Link(outside, facet)
				case "fifo":
					err = unix.Mkfifo(facet, 0600)
				case "oversize":
					err = os.WriteFile(facet, []byte(strings.Repeat("x", 12289)), 0600)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ownerctx.ApplyReviewedRefinement(root, r.ID, review.ReviewDigest, true); err == nil {
				t.Fatal("unsafe canonical facet accepted")
			}
			marker, err := os.ReadFile(outside)
			if err != nil || string(marker) != "outside-private-marker" {
				t.Fatalf("outside file changed: %v", err)
			}
		})
	}
}
