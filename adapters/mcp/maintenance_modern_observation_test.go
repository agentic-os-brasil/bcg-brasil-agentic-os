package product

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
)

func TestMaintenanceStatusObservesModernInstallationWithoutLegacyProjection(t *testing.T) {
	f := newProductSeedFixture(t)
	b, err := Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	value, err := b.MaintenanceStatusContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		Jobs []struct {
			JobID              string                      `json:"job_id"`
			State              string                      `json:"state"`
			Revision           uint64                      `json:"revision"`
			QualificationState string                      `json:"qualification_state"`
			Schedule           maintenance.ProductSchedule `json:"schedule"`
			Diagnosis          *maintenance.DarwinSource   `json:"diagnosis"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{f.options.ProductRoot, f.options.UserHome, f.options.WorkspacePath, "experimental-installed-origin", "PRIVATE_PAYLOAD"} {
		if strings.Contains(string(body), private) {
			t.Fatalf("modern status leaked private source %q", private)
		}
	}
	observed := false
	for _, job := range status.Jobs {
		if job.State != "inactive" || job.Revision != 0 || job.QualificationState != "unqualified" || job.Schedule.Cadence != "" {
			t.Fatalf("read-only diagnostic changed fresh control: %+v", job)
		}
		if job.JobID == maintenance.DarwinDailyJobID && job.Diagnosis != nil {
			if job.Diagnosis.Counts["capabilities_unavailable"] != 13 || job.Diagnosis.Parent.Present || job.Diagnosis.CacheState != "absent" {
				t.Fatalf("configuration observation promoted native/job evidence: %+v", job.Diagnosis)
			}
			for _, source := range job.Diagnosis.Coverage {
				if source.Source == "modern_installation_state" && source.State == "observed" {
					observed = true
				}
			}
		}
	}
	if !observed {
		t.Fatalf("admitted modern installation omitted without legacy home/projection files: %s", body)
	}
}

func installedMaintenanceFixture(t *testing.T, f *productSeedFixture, selection maintenance.InstalledSelection) *maintenance.Service {
	t.Helper()
	catalog, err := baseruntime.Maintenance()
	if err != nil {
		t.Fatal(err)
	}
	s, err := maintenance.NewService(maintenance.ServiceOptions{ProductRoot: f.options.ProductRoot, UserHome: f.options.UserHome, WorkspacePath: f.options.WorkspacePath, SourceAdmission: f.options.ExperimentalSeed, AuthorityRegistrySHA256: f.options.AuthorityRegistrySHA256, CLIVersion: f.options.CLIVersion, Installed: selection, Catalog: catalog, Adapter: "local"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func installedSelection(f *productSeedFixture) maintenance.InstalledSelection {
	return maintenance.InstalledSelection{ManifestSHA256: f.options.InstalledManifestSHA256, ClientInputsSHA256: f.options.InstalledClientInputsSHA256}
}

func maintenanceTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := fmt.Sprint(info.Mode())
		if !entry.IsDir() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += contentDigest(body)
		}
		out[path] = value
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestModernMaintenanceDiagnosisIsReadOnlyAndRefusesChangedInputs(t *testing.T) {
	for _, kind := range []string{"unchanged", "wrong-manifest", "partial-selection", "tampered-state", "legacy-migration-shape", "unsafe-state", "missing-carrier"} {
		t.Run(kind, func(t *testing.T) {
			f := newProductSeedFixture(t)
			selection := installedSelection(f)
			if kind == "wrong-manifest" {
				selection.ManifestSHA256 = strings.Repeat("1", 64)
			}
			if kind == "partial-selection" {
				selection.ClientInputsSHA256 = ""
			}
			if kind == "missing-carrier" {
				selection.CarrierID, selection.InstallerVersion = strings.Repeat("a", 32), "0.2.0"
			}
			s := installedMaintenanceFixture(t, f, selection)
			path := filepath.Join(f.options.UserHome, ".maestro-install", "config", "install-state.json")
			if kind == "tampered-state" {
				if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "legacy-migration-shape" {
				if err := os.WriteFile(path, []byte(`{"schema_version":1}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "unsafe-state" {
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				outside := filepath.Join(t.TempDir(), "state.json")
				if err := os.WriteFile(outside, body, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, path); err != nil {
					t.Skip("symlink fixture unavailable", err)
				}
			}
			home, err := datahome.Resolve(datahome.Options{ProductRoot: f.options.ProductRoot, UserHome: f.options.UserHome})
			if err != nil {
				t.Fatal(err)
			}
			before := maintenanceTree(t, home.Home)
			stateBefore, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "unchanged" {
				review, err := s.Review(maintenance.Operation{Kind: "qualify", JobID: "runtime-health-check"})
				if err != nil {
					t.Fatal(err)
				}
				if review.SourceState == "" {
					t.Fatal("modern diagnosis has no genuine source identity")
				}
			} else if _, err := s.Review(maintenance.Operation{Kind: "qualify", JobID: "runtime-health-check"}); err == nil {
				t.Fatal("unsafe or mismatched modern input supplied a diagnostic review")
			}
			if !reflect.DeepEqual(before, maintenanceTree(t, home.Home)) {
				t.Fatal("read-only modern diagnosis wrote owner state")
			}
			stateAfter, err := os.ReadFile(path)
			if err != nil || string(stateBefore) != string(stateAfter) {
				t.Fatal("diagnostic migrated or rewrote installed state", err)
			}
		})
	}
}

func TestModernMaintenanceObservesCanonicalClientAndRejectsPacketTamper(t *testing.T) {
	f := newProductSeedFixtureWithInstaller(t, true, true)
	home, err := datahome.Resolve(datahome.Options{ProductRoot: f.options.ProductRoot, UserHome: f.options.UserHome})
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(filepath.Dir(f.options.ProductRoot), "clients")
	code := filepath.Join(parent, "code")
	desktop := filepath.Join(parent, "desktop.mcpb")
	if err := os.MkdirAll(code, 0700); err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(code, "synthetic.txt")
	if err := os.WriteFile(payload, []byte("synthetic public packet"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(desktop, []byte("synthetic private packet inventory; no host acceptance"), 0600); err != nil {
		t.Fatal(err)
	}
	codeSHA, err := installer.ClientTreeDigest(code)
	if err != nil {
		t.Fatal(err)
	}
	desktopSHA, err := installer.ClientArchiveDigest(desktop)
	if err != nil {
		t.Fatal(err)
	}
	carrier := strings.Repeat("a", 32)
	b, err := Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	m := installer.ClientMaterialization{CodeRoot: code, DesktopPath: desktop, Release: f.state.Release, ManifestSHA256: f.state.Binding.ManifestSHA256, ClientInputsSHA256: f.state.Binding.ClientInputsSHA256, ServerSHA256: f.state.Server.SHA256, EmbeddedInputsSHA256: f.state.Server.EmbeddedInputsSHA256, BundleSHA256: f.state.Binding.BundleSHA256, CodeTreeSHA256: codeSHA, DesktopSHA256: desktopSHA, DesktopNativeContractSHA256: strings.Repeat("b", 64), BootstrapServerSHA256: f.state.Server.SHA256, BootstrapEmbeddedInputsSHA256: f.state.Server.EmbeddedInputsSHA256}
	r := installer.ClientRecord{SchemaVersion: 1, Profile: installer.StableClientProfile, Protocol: installer.StableClientProtocol, CarrierVersion: installer.StableCarrierVersion, CarrierID: carrier, Binding: installer.ClientBinding{ManagedRoot: f.options.ProductRoot, UserHome: f.options.UserHome, Home: home.Home, Workspace: f.options.WorkspacePath, WorkspaceID: b.workspaceID, RegistrySHA256: f.options.AuthorityRegistrySHA256}, Materialization: m, Code: installer.CodeObservation{Scope: "user", State: "registration_pending", EvidenceClass: "not_observed"}}
	body, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	clients := filepath.Join(f.options.UserHome, ".maestro-install", "clients")
	if err := os.MkdirAll(clients, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clients, carrier+".json"), body, 0600); err != nil {
		t.Fatal(err)
	}
	selection := installedSelection(f)
	selection.CarrierID, selection.InstallerVersion = carrier, "0.2.0"
	s := installedMaintenanceFixture(t, f, selection)
	// The original canonical client reader hashes these real fixture packets.
	// This is materialization evidence, not installation or host qualification.
	if _, err := s.Review(maintenance.Operation{Kind: "qualify", JobID: "runtime-health-check"}); err != nil {
		t.Fatal("valid canonical current client omitted", err)
	}
	if err := os.WriteFile(payload, []byte("changed packet"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Review(maintenance.Operation{Kind: "qualify", JobID: "runtime-health-check"}); err == nil {
		t.Fatal("tampered current packet supplied an observed diagnostic")
	}
	if err := os.WriteFile(payload, []byte("synthetic public packet"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"wrong-workspace", "stale-release"} {
		t.Run(kind, func(t *testing.T) {
			changed := r
			if kind == "wrong-workspace" {
				changed.Binding.WorkspaceID = strings.Repeat("c", 32)
			} else {
				changed.Materialization.Release = "0.2.1"
			}
			changedBody, err := json.Marshal(changed)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(clients, carrier+".json"), changedBody, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Review(maintenance.Operation{Kind: "qualify", JobID: "runtime-health-check"}); err == nil {
				t.Fatal("mismatched current client supplied an observed diagnostic")
			}
		})
	}
}

func TestModernMaintenanceDiagnosisHonorsRequestCancellation(t *testing.T) {
	f := newProductSeedFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	b, err := Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	s, err := b.maintenanceServiceContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(f.identityCounter)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := s.Review(maintenance.Operation{Kind: "qualify", JobID: "runtime-health-check"}); err == nil {
		t.Fatal("cancelled original request began installed diagnostic reads")
	}
	after, err := os.ReadFile(f.identityCounter)
	if err != nil || string(before) != string(after) {
		t.Fatal("cancelled request launched another identity helper", err)
	}
}
