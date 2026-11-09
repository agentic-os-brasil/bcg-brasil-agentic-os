package maintenance

import (
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
	for _, phase := range []string{"services", "diagnostic"} {
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
		if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
			t.Fatal("identity refusal changed owner state")
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
	t.Fatalf("unknown image-fence phase: %s", phase)
}
