package atlasops_test

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceFullPageReviewPreservesCollectionBudget(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "state")
	space := filepath.Join(root, "workspace")
	state, err := workspace.Initialize(workspace.Options{WorkspacePath: space, DataRoot: data})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := atlasops.OpenWorkspace(atlas.Options{DataRoot: data, WorkspacePath: space, WorkspaceID: state.WorkspaceID})
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
