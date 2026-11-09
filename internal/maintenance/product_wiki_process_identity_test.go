//go:build !windows

package maintenance

import (
	"context"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The wiki phases require qualified Unix publication. Ordinary service and
// diagnostic phases remain in their existing platform-independent test.
func TestProductWikiExecutableReplacementFencesPublication(t *testing.T) {
	if phase := os.Getenv("MAESTRO_WIKI_IMAGE_FENCE_CHILD"); phase != "" {
		productWikiImageFenceChild(t, phase)
		return
	}
	image, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(image)
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"pointer", "terminal"} {
		t.Run(phase, func(t *testing.T) {
			binary := filepath.Join(t.TempDir(), "retained-wiki-process")
			if err := os.WriteFile(binary, body, 0700); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(binary, "-test.run=^TestProductWikiExecutableReplacementFencesPublication$", "-test.v")
			command.Env = append(os.Environ(), "MAESTRO_WIKI_IMAGE_FENCE_CHILD="+phase)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("actual retained wiki image: %s %v", output, err)
			}
		})
	}
}
func productWikiImageFenceChild(t *testing.T, phase string) {
	ctx := context.Background()
	if phase == "pointer" {
		s := productFixture(t)
		productWikiCommit(t, s, "private-wiki-body: BEFORE_IMAGE_REPLACEMENT")
		s.engine.FaultPoint = func(point string) error {
			if point == "wiki_before_pointer" {
				return replaceActualProductTestImage()
			}
			return nil
		}
		if _, err := s.engine.CompilePrivateWiki(ctx, s.workspaceID, ""); !errors.Is(err, processidentity.ErrChanged) {
			t.Fatalf("replacement at pointer was accepted: %v", err)
		}
		pointer := filepath.Join(s.engine.Root, "workspaces", s.workspaceID, ".private-wiki", "current.json")
		if _, err := os.Lstat(pointer); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("replacement activated a private graph")
		}
		return
	}
	s, active := activeProductWiki(t)
	pending, err := s.engine.InspectPrivateWikiMaintenance(s.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	p := &productExecution{service: s, jobID: WikiIncrementalJobID, revision: active.Revision, source: pending.SourceIdentity}
	worker := s.worker(p)
	worker.Jobs = nil
	worker.LocalQualification = map[string]string{WikiIncrementalJobID: active.Qualification.ID}
	worker.ActivatedJobs = []string{WikiIncrementalJobID}
	actual := worker.Handlers[WikiIncrementalJobID].(Handler)
	worker.Handlers[WikiIncrementalJobID] = HandlerFunc(func(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
		out, err := actual.ExecuteAuthorized(ctx, command, grant)
		if err != nil || out.State != ReceiptSucceeded {
			return out, err
		}
		return out, replaceActualProductTestImage()
	})
	out, err := worker.Run(ctx, WakeRequest{WorkspaceID: s.workspaceID, Trigger: TriggerEvent, EventID: pending.EventID, Now: s.now(), OwnerID: "changed-image-terminal", Preauthorized: true, IdleState: IdleConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	for _, receipt := range out.Receipts {
		if receipt.JobID == WikiIncrementalJobID && receipt.State == ReceiptSucceeded {
			t.Fatal("changed image published terminal success")
		}
	}
	receipts, err := s.receipts.Receipts(s.workspaceID, WikiIncrementalJobID)
	if err != nil {
		t.Fatal(err)
	}
	for _, receipt := range receipts {
		if receipt.EventID == pending.EventID && receipt.State == ReceiptSucceeded {
			t.Fatal("changed image persisted terminal success")
		}
	}
}
