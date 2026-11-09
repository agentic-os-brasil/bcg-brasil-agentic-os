package atlasops_test

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceFullPageReviewPreservesCollectionBudget(t *testing.T) {
	root := t.TempDir()
	options := registeredWorkspace(t, root, filepath.Join(root, "state"), "workspace")
	engine, err := atlasops.OpenWorkspace(options)
	if err != nil {
		t.Fatal(err)
	}
	body := "# Full page\n" + strings.Repeat("x", (256<<10)-12)
	written, err := engine.CreatePage(atlasops.CreatePageRequest{Page: "knowledge/large.md", Body: body, Provenance: atlasops.Provenance{Origin: atlasops.OriginAttended, SessionID: "review-test", IdempotencyKey: "large-page"}})
	if err != nil {
		t.Fatal(err)
	}
	request := atlasops.CollectRequest{Purpose: "Review the full selected page", Reader: atlasops.ReaderOwnerSession, Pages: []string{"knowledge/large.md"}}
	ordinary, err := engine.Collect(request)
	if err != nil || len(ordinary.Pages) != 0 || len(ordinary.Omissions) != 1 {
		t.Fatal("collection budget widened", ordinary, err)
	}
	reviewed, err := engine.ReviewPage(request)
	if err != nil || reviewed.Content != body || reviewed.Revision != written.Revision {
		t.Fatal("full review lost content/revision", err)
	}
	request.Reader = atlasops.ReaderDelegate
	request.Authorized = true
	if _, err := engine.ReviewPage(request); err == nil {
		t.Fatal("delegated reader received full review")
	}
	request.Reader = atlasops.ReaderOwnerSession
	request.Purpose = ""
	if _, err := engine.ReviewPage(request); err == nil {
		t.Fatal("unpurposed review admitted")
	}
}

func TestWorkspaceFullPageReviewPreservesTextAndBindingGuards(t *testing.T) {
	base := t.TempDir()
	options := registeredWorkspace(t, base, filepath.Join(base, "state"), "case")
	engine, err := atlasops.OpenWorkspace(options)
	if err != nil {
		t.Fatal(err)
	}
	page, body := "knowledge/review.md", "# Reviewed page\n"
	if _, err := engine.CreatePage(atlasops.CreatePageRequest{Page: page, Body: body, Provenance: provenance("review-guards")}); err != nil {
		t.Fatal(err)
	}
	request := atlasops.CollectRequest{Purpose: string([]byte{0xff}), Reader: atlasops.ReaderOwnerSession, Pages: []string{page}}
	if _, err := engine.ReviewPage(request); err == nil {
		t.Fatal("malformed purpose accepted for full private review")
	}
	path := filepath.Join(engine.Root(), filepath.FromSlash(page))
	malformed := []byte(body + string([]byte{0xfe}))
	if err := os.WriteFile(path, malformed, 0600); err != nil {
		t.Fatal(err)
	}
	request.Purpose = "Review the exact selected page"
	if _, err := engine.ReviewPage(request); err == nil {
		t.Fatal("malformed existing page normalized for full review")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(malformed) {
		t.Fatal("full review changed malformed page bytes", err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(options.DataRoot, "workspaces", options.WorkspaceID, "binding.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ReviewPage(request); err == nil {
		t.Fatal("full review bypassed removed workspace registration")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != body {
		t.Fatal("refused full review changed page bytes", err)
	}
}
