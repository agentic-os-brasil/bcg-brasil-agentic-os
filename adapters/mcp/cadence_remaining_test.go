package product_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
