package maintenance

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCanaryEnrollmentPersistsIANAZoneAndScopedActivations(t *testing.T) {
	root := t.TempDir()
	enrollment := CanaryEnrollment{SchemaVersion: EnrollmentSchemaVersion, WorkspaceID: "maestro-system", AgentID: "darwin", Home: filepath.Join(root, "home"), UID: "501", Timezone: "America/Sao_Paulo", LaunchAgentLabel: "com.bcg.maestro.maintenance", Mode: "filesystem_only", EnrolledAt: time.Date(2026, 8, 2, 7, 0, 0, 0, time.FixedZone("BRT", -3*60*60)), Activated: []Activation{{JobID: "darwin-housekeeping-daily", QualificationDigest: QualificationDigest("darwin-housekeeping-daily")}, {JobID: "darwin-deep-weekly", QualificationDigest: QualificationDigest("darwin-deep-weekly")}}}
	if err := SaveCanaryEnrollment(root, enrollment); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCanaryEnrollment(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Timezone != enrollment.Timezone || len(loaded.Activated) != 2 {
		t.Fatalf("loaded=%#v", loaded)
	}
	qualified, activated := ActivationMaps(loaded)
	if qualified["darwin-deep-weekly"] == "" || len(activated) != 2 {
		t.Fatalf("activation maps=%#v %#v", qualified, activated)
	}
	body, err := os.ReadFile(filepath.Join(root, "maintenance", "canary-enrollment.json"))
	if err != nil || len(body) == 0 {
		t.Fatalf("enrollment file err=%v", err)
	}
	if err := SaveCanaryEnrollment(root, loaded); err != nil {
		t.Fatal(err)
	}
	if err := DeleteCanaryEnrollment(root); err != nil {
		t.Fatal(err)
	}
}

func TestCanaryEnrollmentRejectsNonIANATimezone(t *testing.T) {
	enrollment := CanaryEnrollment{SchemaVersion: EnrollmentSchemaVersion, WorkspaceID: "maestro-system", AgentID: "darwin", Home: "/tmp/home", UID: "501", Timezone: "not-a-timezone", LaunchAgentLabel: "com.bcg.maestro.maintenance", Mode: "native", EnrolledAt: time.Now(), Activated: nil}
	if err := enrollment.Validate(); err == nil {
		t.Fatal("invalid timezone accepted")
	}
}
