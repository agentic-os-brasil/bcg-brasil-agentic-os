package pilotacceptance

import (
	"strings"
	"testing"
	"time"
)

func TestValidateDistinguishesIsolatedEngineeringFromCorporateAcceptance(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	isolated := Report{
		SchemaVersion: 1, RunID: "ci-windows-1", Mode: "isolated_ci", Platform: "windows",
		CandidateVersion: "0.1.0", ReadinessClaim: "engineering_evidence_only",
		StartedAt: now, FinishedAt: now.Add(time.Minute),
		Scenarios: passingScenarios(),
	}
	if err := isolated.Validate(); err != nil {
		t.Fatalf("isolated Validate() error = %v", err)
	}
	isolated.ReadinessClaim = "corporate_device_acceptance"
	if err := isolated.Validate(); err == nil {
		t.Fatal("isolated CI claimed corporate acceptance")
	}

	corporate := Report{
		SchemaVersion: 1, RunID: "corp-macos-1", Mode: "corporate_device", Platform: "macos",
		CandidateVersion: "0.1.0", ReadinessClaim: "corporate_device_acceptance",
		StartedAt: now, FinishedAt: now.Add(5 * time.Minute),
		Scenarios: passingScenarios(),
		Attestation: &Attestation{
			Operator: "pilot-owner", DeviceIDHash: strings.Repeat("a", 64),
			PolicyContext: "managed BCG macOS pilot device", ApprovedChannel: "canary",
			SignedManifest: true, NativeCodeSigning: true, AuthenticatedProvider: true,
		},
	}
	if err := corporate.Validate(); err != nil {
		t.Fatalf("corporate Validate() error = %v", err)
	}
	corporate.Attestation.NativeCodeSigning = false
	if err := corporate.Validate(); err == nil {
		t.Fatal("corporate report accepted missing native signing")
	}
}

func TestValidateRequiresEveryInstallUpdateRollbackScenario(t *testing.T) {
	report := Report{
		SchemaVersion: 1, RunID: "ci-macos-1", Mode: "isolated_ci", Platform: "macos",
		CandidateVersion: "0.1.0", ReadinessClaim: "engineering_evidence_only",
		StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC().Add(time.Second),
		Scenarios: []Scenario{{Name: "install", State: "pass"}},
	}
	if err := report.Validate(); err == nil {
		t.Fatal("Validate() accepted missing update and rollback evidence")
	}
}

func passingScenarios() []Scenario {
	return []Scenario{
		{Name: "install", State: "pass", Evidence: []string{"trial-smoke"}},
		{Name: "update", State: "pass", Evidence: []string{"transaction-tests"}},
		{Name: "rollback", State: "pass", Evidence: []string{"rollback-tests"}},
	}
}
