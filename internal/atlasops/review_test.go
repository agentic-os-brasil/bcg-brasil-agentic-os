package atlasops_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
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
	assertRetainedReviewPreservesOrdinaryReadGuards(t)
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

func assertRetainedReviewPreservesOrdinaryReadGuards(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	options := registeredWorkspace(t, base, filepath.Join(base, "state"), "retained-review")
	engine, e := atlasops.OpenWorkspace(options)
	if e != nil {
		t.Fatal(e)
	}
	page, body := "projects/review.md", "# Reviewed project\n"
	if _, e = engine.CreatePage(atlasops.CreatePageRequest{Page: page, Body: body, Provenance: provenance("retained-reader-parity")}); e != nil {
		t.Fatal(e)
	}
	retained, e := scheduler.HoldDirectory(engine.Root(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer retained.Close()
	request := atlasops.CollectRequest{Purpose: "Read the same selected canonical page", Reader: atlasops.ReaderOwnerSession, Pages: []string{page}}
	ordinary, e := engine.ReviewPage(request)
	if e != nil || ordinary.Content != body {
		t.Fatal("ordinary valid page read failed", e)
	}
	held, e := engine.ReviewPageRetained(request, retained)
	if e != nil || held.Content != ordinary.Content || held.Revision != ordinary.Revision {
		t.Fatal("retained valid page read diverged", e)
	}
	path := filepath.Join(engine.Root(), filepath.FromSlash(page))
	parent := filepath.Dir(path)
	if runtime.GOOS != "windows" {
		if e = os.Chmod(parent, 0755); e != nil {
			t.Fatal(e)
		}
		if _, e = engine.ReviewPage(request); e == nil {
			t.Error("ordinary review admitted a nonprivate final parent")
		}
		if _, e = engine.ReviewPageRetained(request, retained); e == nil {
			t.Error("retained review lost private-parent gate")
		}
		if info, e := os.Stat(parent); e != nil || info.Mode().Perm() != 0755 {
			t.Fatal("read repaired directory mode", e)
		}
		if got, e := os.ReadFile(path); e != nil || string(got) != body {
			t.Fatal("read changed page bytes", e)
		}
		if e = os.Chmod(parent, 0700); e != nil {
			t.Fatal(e)
		}
	}
	malformed := append([]byte(body), 0xff)
	if e = os.WriteFile(path, malformed, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = engine.ReviewPage(request); e == nil {
		t.Error("ordinary review admitted malformed UTF8")
	}
	if _, e = engine.ReviewPageRetained(request, retained); e == nil {
		t.Error("retained review lost canonical UTF8 check")
	}
	if got, e := os.ReadFile(path); e != nil || string(got) != string(malformed) {
		t.Fatal("refused retained review repaired malformed bytes", e)
	}
}
