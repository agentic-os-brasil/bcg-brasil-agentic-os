package installtx

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

// TestSyncedSourceCrashReconciliation consumes only a real synthetic installation
// produced by the source-aware end-to-end factory tracer. No healthy executable,
// saved effect receipt or fallback fixture substitutes for the transaction.
func TestSyncedSourceCrashReconciliation(t *testing.T) {
	input := os.Getenv("MAESTRO_SYNCED_CRASH_FIXTURE")
	if input == "" {
		t.Skip("real source-aware factory fixture not supplied")
	}
	var fixture struct{ UserHome, ManagedRoot, DataRoot, PlanID, Registry, Output string }
	body, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	var pending struct {
		VerifiedDirectory  string `json:"verified_directory"`
		ActivationPlanPath string `json:"activation_plan_path"`
	}
	body, err = os.ReadFile(filepath.Join(fixture.DataRoot, "config", "pending-update.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(body, &pending); err != nil {
		t.Fatal(err)
	}
	plan, err := ReadPlan(pending.ActivationPlanPath)
	if err != nil {
		t.Fatal(err)
	}
	if plan.SchemaVersion != 4 || plan.ConfirmationPlanID != fixture.PlanID {
		t.Fatal("real confirmed source-aware activation plan required")
	}
	registry, err := releaseverify.LoadAuthorityRegistry(fixture.Registry, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := releaseverify.VerifyDirectory(pending.VerifiedDirectory, registry)
	if err != nil {
		t.Fatal(err)
	}
	options := PrepareOptions{UserHome: fixture.UserHome, Transition: "update", ConfirmationPlanID: fixture.PlanID, FromRelease: plan.FromRelease, FromChannel: plan.FromChannel, FromCLIVersion: plan.FromCLIVersion, FromBundleVersion: plan.FromBundleVersion, TargetOS: plan.TargetOS, TargetArch: plan.TargetArch, ManagedRoot: fixture.ManagedRoot, DataRoot: fixture.DataRoot, Source: plan.Source, SourceStateSHA256: plan.SourceStateSHA256}
	if _, err := ValidatePrepared(pending.ActivationPlanPath, verified, options); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("MAESTRO_SYNCED_CRASH_PHASE") == "child" {
		err := Activate(pending.ActivationPlanPath, verified, ActivateOptions{PrepareOptions: options, afterPayload: func() { os.Exit(73) }})
		t.Fatalf("expected controlled process death after real payload/self-check, got %v", err)
	}
	before, err := ReadSourceState(fixture.DataRoot, fixture.ManagedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if before.Source == nil {
		t.Fatal("real enrolled source required")
	}
	command := exec.Command(os.Args[0], "-test.run=^TestSyncedSourceCrashReconciliation$", "-test.v")
	command.Env = append(os.Environ(), "MAESTRO_SYNCED_CRASH_PHASE=child")
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 73 {
		t.Fatalf("controlled crash did not occur after payload: %v %s", err, output)
	}
	interrupted, err := ReadSourceState(fixture.DataRoot, fixture.ManagedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if StateDigest(interrupted) != StateDigest(before) {
		t.Fatal("crash unexpectedly committed target state")
	}
	if err := verifyActivatedPayload(plan); err != nil {
		t.Fatalf("real target payload absent at crash boundary: %v", err)
	}
	if _, err := readActivationReceipt(pending.ActivationPlanPath, plan); err != nil {
		t.Fatalf("real activation intent absent at crash boundary: %v", err)
	}
	applied, err := ReconcileActivated(pending.ActivationPlanPath, ActivateOptions{PrepareOptions: options})
	if err != nil || !applied {
		t.Fatalf("canonical reconciliation did not complete real crash: %t %v", applied, err)
	}
	after, err := ValidateQualifiedLaunch(fixture.ManagedRoot, fixture.DataRoot, plan.Binding, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if after.Release != plan.Release || after.Previous == nil || after.Previous.Release != before.Release || !reflect.DeepEqual(before.Source, after.Source) {
		t.Fatal("crash reconciliation lost actual release/previous/source custody")
	}
	report := map[string]any{"crash_exit": 73, "actual_payload_verified_before_reconcile": true, "real_receipt_schema": 3, "source_before_and_after": before.Source, "from_release": before.Release, "reconciled_release": after.Release, "server_sha256": after.Server.SHA256, "manifest_sha256": after.Binding.ManifestSHA256, "state": "actual source-aware process death after payload and before state commit reconciled; qualified launch readback passed"}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if fixture.Output != "" {
		if err := os.WriteFile(fixture.Output, append(encoded, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Log(string(encoded))
}
