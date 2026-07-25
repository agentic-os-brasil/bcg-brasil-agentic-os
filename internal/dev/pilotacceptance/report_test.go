package pilotacceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateDistinguishesIsolatedEngineeringFromCorporateAcceptance(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	isolated := Report{
		SchemaVersion: 2, RunID: "ci-windows-1", Mode: "isolated_ci", Platform: "windows",
		CandidateVersion: "0.1.0", ReadinessClaim: "engineering_evidence_only",
		StartedAt: now, FinishedAt: now.Add(time.Minute),
		Scenarios: passingScenarios(false),
	}
	if err := isolated.Validate(); err != nil {
		t.Fatalf("isolated Validate() error = %v", err)
	}
	isolated.ReadinessClaim = "corporate_device_operator_attestation"
	if err := isolated.Validate(); err == nil {
		t.Fatal("isolated CI claimed corporate acceptance")
	}

	corporate := Report{
		SchemaVersion: 2, RunID: "corp-macos-1", Mode: "corporate_device", Platform: "macos",
		CandidateVersion: "0.1.0", ReadinessClaim: "corporate_device_operator_attestation",
		StartedAt: now, FinishedAt: now.Add(5 * time.Minute),
		Scenarios: passingScenarios(true),
		Release: &ReleaseBinding{
			BaselineProviderReleaseID: "12345", BaselineReleaseTag: "maestro-v0.1.0",
			BaselineManifestSHA256:  strings.Repeat("b", 64),
			UpdateProviderReleaseID: "12346", UpdateReleaseTag: "maestro-v0.2.0",
			UpdateManifestSHA256:    strings.Repeat("c", 64),
			BootstrapperSHA256:      strings.Repeat("d", 64),
			AuthorityRegistrySHA256: strings.Repeat("e", 64),
			NativeSignerID:          "AB12CD34EF",
		},
		Attestation: &Attestation{
			Operator: "pilot-owner", DeviceIDHash: strings.Repeat("a", 64),
			PolicyID: "bcg-managed-standard-v1", ApprovedChannel: "canary",
			SupportOwner: "support-owner",
		},
	}
	if err := corporate.Validate(); err != nil {
		t.Fatalf("corporate Validate() error = %v", err)
	}
	corporate.Scenarios[0].Evidence = []string{"trial-smoke"}
	if err := corporate.Validate(); err == nil {
		t.Fatal("corporate report accepted substituted phase evidence")
	}
	corporate.Scenarios[0].Evidence = expectedPhaseChecks("install")
	corporate.Scenarios[1].FromVersion = "9.9.9"
	if err := corporate.Validate(); err == nil {
		t.Fatal("corporate report accepted discontinuous phase transitions")
	}
	corporate.Scenarios[1].FromVersion = "0.0.1"
	corporate.Attestation.PolicyID = "policy with free text"
	if err := corporate.Validate(); err == nil {
		t.Fatal("corporate report accepted an unrestricted policy context")
	}
}

func TestValidateRequiresEveryInstallUpdateRollbackScenario(t *testing.T) {
	report := Report{
		SchemaVersion: 2, RunID: "ci-macos-1", Mode: "isolated_ci", Platform: "macos",
		CandidateVersion: "0.1.0", ReadinessClaim: "engineering_evidence_only",
		StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC().Add(time.Second),
		Scenarios: []Scenario{{Name: "install", State: "pass"}},
	}
	if err := report.Validate(); err == nil {
		t.Fatal("Validate() accepted missing update and rollback evidence")
	}
}

func TestWriteNeverOverwritesAcceptanceEvidence(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	report := Isolated("ci-macos-1", "macos", "0.1.0", now, now.Add(time.Second))
	path := filepath.Join(t.TempDir(), "report.json")
	if err := Write(path, report); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(path, report); err == nil {
		t.Fatal("Write() replaced existing acceptance evidence")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("rejected overwrite changed existing acceptance evidence")
	}
}

func passingScenarios(withReceipts bool) []Scenario {
	scenarios := []Scenario{
		{Name: "install", State: "pass", Evidence: []string{"trial-smoke"}},
		{Name: "update", State: "pass", Evidence: []string{"transaction-tests"}},
		{Name: "rollback", State: "pass", Evidence: []string{"rollback-tests"}},
	}
	if withReceipts {
		for i := range scenarios {
			scenarios[i].ReceiptSHA256 = strings.Repeat("e", 64)
			scenarios[i].Evidence = expectedPhaseChecks(scenarios[i].Name)
		}
		scenarios[0].ToVersion = "0.0.1"
		scenarios[1].FromVersion, scenarios[1].ToVersion = "0.0.1", "0.1.0"
		scenarios[2].FromVersion, scenarios[2].ToVersion = "0.1.0", "0.0.1"
	}
	return scenarios
}
