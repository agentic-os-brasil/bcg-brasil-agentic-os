package maintenance

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
)

// Each child starts from the actual test executable and replaces that pathname.
// Its permanent restart fence cannot poison the parent test process.
func TestProductExecutableReplacementFencesServicesAndPublication(t *testing.T) {
	if phase := os.Getenv("MAESTRO_IMAGE_FENCE_CHILD"); phase != "" {
		productImageFenceChild(t, phase)
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
	for _, phase := range []string{"services", "pointer", "diagnostic", "terminal"} {
		t.Run(phase, func(t *testing.T) {
			binary := filepath.Join(t.TempDir(), "retained-product-process")
			if err := os.WriteFile(binary, body, 0700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(binary, "-test.run=^TestProductExecutableReplacementFencesServicesAndPublication$", "-test.v")
			cmd.Env = append(os.Environ(), "MAESTRO_IMAGE_FENCE_CHILD="+phase)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("actual retained-image child: %s %v", output, err)
			}
		})
	}
}

func replaceActualProductTestImage() error {
	name, err := os.Executable()
	if err != nil {
		return err
	}
	body, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	// The separate MCP witness uses two real linked production executables.
	// Here different real file bytes isolate shared Service/publication checks.
	if err := os.WriteFile(name+".new", append(body, []byte("changed-installed-image")...), 0700); err != nil {
		return err
	}
	return os.Rename(name+".new", name)
}

func productImageFenceChild(t *testing.T, phase string) {
	ctx := context.Background()
	if phase == "services" {
		s := productFixture(t)
		before := productSnapshot(t, s.home)
		if err := replaceActualProductTestImage(); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Status(); !errors.Is(err, processidentity.ErrChanged) {
			t.Fatalf("existing Service adopted replacement: %v", err)
		}
		if _, err := NewService(s.options); !errors.Is(err, processidentity.ErrChanged) {
			t.Fatalf("new Service adopted replacement: %v", err)
		}
		guard := &productExecution{service: s, jobID: MemoryLightDreamJobID}
		if err := guard.guard(func() error { t.Fatal("changed process entered publication"); return nil }); !errors.Is(err, processidentity.ErrChanged) {
			t.Fatalf("changed process touched unprepared control authority: %v", err)
		}
		if _, err := s.engine.InspectPrivateWikiMaintenance(s.workspaceID); !errors.Is(err, processidentity.ErrChanged) {
			t.Fatalf("inspection invented current producer: %v", err)
		}
		if _, err := s.engine.CompilePrivateWiki(ctx, s.workspaceID, ""); !errors.Is(err, processidentity.ErrChanged) {
			t.Fatalf("changed image entered owning preparation: %v", err)
		}
		if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
			t.Fatal("identity refusal changed owner state")
		}
		return
	}
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
	if phase == "diagnostic" {
		s := productFixture(t)
		const job = "runtime-health-check"
		productApply(t, s, "qualify", job, "")
		active := productApply(t, s, "enroll", job, "UTC")
		store := DiagnosticStore{Root: s.receipts.Root}
		old, path, err := store.Load(s.workspaceID, job, "")
		if err != nil {
			t.Fatal(err)
		}
		registration := filepath.Join(s.options.WorkspacePath, ".bcgos", "workspace.json")
		body, err := os.ReadFile(registration)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(registration, append(body, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		report, err := s.diagnosticSource(job)
		if err != nil || report.SourceSHA256 == old.SourceSHA256 {
			t.Fatalf("actual diagnostic source did not change: %v", err)
		}
		report.ObservedAt = s.now()
		p := &productExecution{service: s, jobID: job, revision: active.Revision}
		if err := replaceActualProductTestImage(); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Publish(report, p.guard); !errors.Is(err, processidentity.ErrChanged) {
			t.Fatalf("changed image published diagnostic pointer: %v", err)
		}
		current, currentPath, err := store.Load(s.workspaceID, job, "")
		if err != nil || currentPath != path || current.SourceSHA256 != old.SourceSHA256 {
			t.Fatal("changed image replaced last diagnostic")
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
