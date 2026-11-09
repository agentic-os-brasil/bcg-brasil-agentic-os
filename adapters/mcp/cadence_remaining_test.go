package product_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimeprojection"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func maintenanceReportCall(t *testing.T, cs *mcp.ClientSession, id string, fail bool) map[string]any {
	t.Helper()
	return cadenceCall(t, cs, "report", map[string]any{"job_id": id}, fail)
}
func TestMaintenanceRemainingPublicOutputs(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		t.Run(fmt.Sprint(stdio), func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			prepareMemoryFixture(t, f)
			if _, err := runtimeprojection.Install("codex", f.options.WorkspacePath); err != nil {
				t.Fatal(err)
			}
			cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			})
			status := cadenceCall(t, cs, "status", nil, false)
			workspace := status["workspace_id"].(string)
			home := filepath.Dir(f.root)
			memoryCall(t, cs, "memory.capture", map[string]any{"text": "private-maintenance-text: preserve the actual accepted evidence"}, false)
			cadenceCall(t, cs, "qualify", map[string]any{"job_id": maintenance.MemoryLightDreamJobID, "expected_revision": 0}, false)
			for _, job := range []string{maintenance.MemoryDeepDreamJobID, "memory-retention-check", "skills-index-refresh", "runtime-health-check", "capability-recheck", "runtime-drift-check"} {
				qualified := cadenceCall(t, cs, "qualify", map[string]any{"job_id": job, "expected_revision": 0}, false)
				if qualified["qualification_state"] != "qualified" || qualified["state"] != "inactive" {
					t.Fatalf("%s: %+v", job, qualified)
				}
				report := maintenanceReportCall(t, cs, job, false)
				if report["job_id"] != job || report["workspace_id"] != workspace || report["finding"] == "" {
					t.Fatalf("missing domain result: %+v", report)
				}
				switch job {
				case maintenance.MemoryDeepDreamJobID:
					engine, err := maintenance.NewProductWeeklyMemoryEngine(home)
					if err != nil {
						t.Fatal(err)
					}
					year, week := time.Now().UTC().ISOWeek()
					if _, err = engine.WeeklyCommitEvidence(workspace, fmt.Sprintf("%04d-W%02d", year, week), ""); err != nil {
						t.Fatal(err)
					}
					if report["finding"] != "weekly_layers_published" {
						t.Fatal(report)
					}
				case "skills-index-refresh":
					root := filepath.Join(home, "brain", "runtime", "workspaces", workspace, "skills-index")
					index, err := skillsindex.LoadRuntimeIndex(root, "")
					if err != nil {
						t.Fatal(err)
					}
					if len(index.Metadata.Source.Pointers) == 0 || len(report["skills"].([]any)) != len(index.Metadata.Source.Pointers) {
						t.Fatal("index navigation absent")
					}
					for _, pointer := range report["skills"].([]any) {
						rel := pointer.(map[string]any)["pointer"].(string)
						if filepath.IsAbs(rel) || strings.Contains(rel, "..") {
							t.Fatal("unsafe navigation pointer")
						}
						if _, err := os.Stat(filepath.Join(f.options.WorkspacePath, rel)); err != nil {
							t.Fatal(err)
						}
					}
				default:
					durable, _, err := (maintenance.DiagnosticStore{Root: filepath.Join(home, "state", "maintenance")}).Load(workspace, job, "")
					if err != nil || durable.Finding != report["finding"] {
						t.Fatalf("report readback: %+v %v", durable, err)
					}
					if job != "memory-retention-check" && report["native_observation"] != "not_observed" {
						t.Fatal("native health manufactured")
					}
				}
			}
			// Actual projected-byte tampering must block index use and must be
			// diagnosed as drift, without granting native capability.
			_, _, installed, err := runtimeprojection.RoutingInputs("codex", f.options.WorkspacePath)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(f.options.WorkspacePath, installed[0].Pointer), []byte("tampered method"), 0600); err != nil {
				t.Fatal(err)
			}
			maintenanceReportCall(t, cs, "skills-index-refresh", true)
			status = cadenceCall(t, cs, "status", nil, false)
			drift := cadenceJob(t, status, "runtime-drift-check")
			cadenceCall(t, cs, "qualify", map[string]any{"job_id": "runtime-drift-check", "expected_revision": drift["revision"]}, false)
			report := maintenanceReportCall(t, cs, "runtime-drift-check", false)
			if report["finding"] != "drift_detected" {
				t.Fatal(report)
			}
		})
	}
}

func TestMaintenanceIndexRetentionChangedExecutableRecovery(t *testing.T) {
	f := newOwnerFixture(t, true)
	prepareMemoryFixture(t, f)
	if _, err := runtimeprojection.Install("codex", f.options.WorkspacePath); err != nil {
		t.Fatal(err)
	}
	snapshot := func() map[string][32]byte {
		files := map[string][32]byte{}
		if err := filepath.WalkDir(f.options.WorkspacePath, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
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
		}); err != nil {
			t.Fatal(err)
		}
		return files
	}
	installedBefore := snapshot()
	cs := f.connect(t, cadenceApprove)
	memoryCall(t, cs, "memory.capture", map[string]any{"text": "private-maintenance-text: stable retention input across real rebuild"}, false)
	status := cadenceCall(t, cs, "status", nil, false)
	workspace := status["workspace_id"].(string)
	home := filepath.Dir(f.root)
	indexRoot := filepath.Join(home, "brain", "runtime", "workspaces", workspace, "skills-index")
	store := maintenance.DiagnosticStore{Root: filepath.Join(home, "state", "maintenance")}
	jobs := []string{"skills-index-refresh", "memory-retention-check"}
	for _, job := range jobs {
		q := cadenceCall(t, cs, "qualify", map[string]any{"job_id": job, "expected_revision": 0}, false)
		active := cadenceCall(t, cs, "enroll", map[string]any{"job_id": job, "expected_revision": q["revision"], "timezone": "UTC"}, false)
		cadenceCall(t, cs, "pause", map[string]any{"job_id": job, "expected_revision": active["revision"]}, false)
	}
	oldIndex, err := skillsindex.LoadRuntimeIndex(indexRoot, "")
	if err != nil {
		t.Fatal(err)
	}
	oldRetention, oldRetentionPath, err := store.Load(workspace, "memory-retention-check", "")
	if err != nil {
		t.Fatal(err)
	}
	cs.Close()
	original, err := os.ReadFile(f.binary)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(filepath.Dir(f.binary), "maestro-mcp-changed-index-retention")
	build := exec.Command("go", "build", "-ldflags", "-X main.AuthorityRegistrySHA256=changed-index-retention-build", "-o", replacement, "./cmd/maestro-mcp")
	build.Env = os.Environ()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("rebuild: %s %v", out, err)
	}
	rebuilt, err := os.ReadFile(replacement)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(original) == sha256.Sum256(rebuilt) {
		t.Fatal("rebuild did not change executable bytes")
	}
	f.binary = replacement
	cs = f.connect(t, cadenceApprove)
	for _, job := range jobs {
		t.Run(job, func(t *testing.T) {
			status := cadenceJob(t, cadenceCall(t, cs, "status", nil, false), job)
			if status["qualification_state"] != "stale" {
				t.Fatalf("changed executable retained qualification: %+v", status)
			}
			cadenceCall(t, cs, "resume", map[string]any{"job_id": job, "expected_revision": status["revision"]}, true)
			q := cadenceCall(t, cs, "qualify", map[string]any{"job_id": job, "expected_revision": status["revision"]}, false)
			if q["qualification_state"] != "qualified" {
				t.Fatal(q)
			}
			active := cadenceCall(t, cs, "resume", map[string]any{"job_id": job, "expected_revision": q["revision"]}, false)
			if active["state"] != "active" {
				t.Fatal(active)
			}
			cadenceCall(t, cs, "qualify", map[string]any{"job_id": job, "expected_revision": active["revision"]}, true)
		})
	}
	currentIndex, err := skillsindex.LoadRuntimeIndex(indexRoot, "")
	if err != nil {
		t.Fatal(err)
	}
	currentRetention, currentRetentionPath, err := store.Load(workspace, "memory-retention-check", "")
	if err != nil {
		t.Fatal(err)
	}
	originalDigest := fmt.Sprintf("%x", sha256.Sum256(original))
	rebuiltDigest := fmt.Sprintf("%x", sha256.Sum256(rebuilt))
	if oldIndex.Metadata.Source.Implementation != originalDigest || oldRetention.Implementation != originalDigest ||
		currentIndex.Metadata.Source.Implementation != rebuiltDigest || currentRetention.Implementation != rebuiltDigest {
		t.Fatal("durable execution provenance does not match actual executable bytes")
	}
	if oldIndex.Metadata.Source.Configuration != currentIndex.Metadata.Source.Configuration ||
		oldRetention.Configuration != currentRetention.Configuration {
		t.Fatal("executable-only recovery changed configuration provenance")
	}
	if currentIndex.Generation == oldIndex.Generation || currentRetentionPath == oldRetentionPath {
		t.Error("changed executable did not publish new durable execution evidence")
	}
	if !reflect.DeepEqual(oldIndex.Metadata.Source.Catalog, currentIndex.Metadata.Source.Catalog) ||
		!reflect.DeepEqual(oldIndex.Metadata.Source.Pointers, currentIndex.Metadata.Source.Pointers) ||
		!reflect.DeepEqual(oldRetention.Retention, currentRetention.Retention) ||
		!reflect.DeepEqual(installedBefore, snapshot()) {
		t.Fatal("requalification changed actual installed/canonical input")
	}
}
