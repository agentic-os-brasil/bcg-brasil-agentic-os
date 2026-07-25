package pilotacceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCorporateRequiresThreeReceiptsBoundToOneRelease(t *testing.T) {
	root := t.TempDir()
	baselineDigest := strings.Repeat("a", 64)
	updateDigest := strings.Repeat("b", 64)
	bootstrapperDigest := strings.Repeat("c", 64)
	registryDigest := strings.Repeat("d", 64)
	deviceDigest := strings.Repeat("e", 64)
	activationDigest := strings.Repeat("f", 64)
	receipts := map[string]string{}
	for index, phase := range []string{"install", "update", "rollback"} {
		path := filepath.Join(root, phase+".json")
		fromVersion := ""
		toVersion := "0.1.0"
		manifestDigest := baselineDigest
		if phase == "update" {
			fromVersion, toVersion, manifestDigest = "0.1.0", "0.2.0", updateDigest
		}
		if phase == "rollback" {
			fromVersion, toVersion = "0.2.0", "0.1.0"
		}
		providerReleaseID := "98764"
		if phase == "update" {
			providerReleaseID = "98765"
		}
		activationReceiptDigest := ""
		if phase != "install" {
			activationReceiptDigest = activationDigest
		}
		writePhaseReceipt(t, path, PhaseReceipt{
			SchemaVersion: 1,
			RunID:         "corp-windows-1", DeviceIDHash: deviceDigest,
			Platform: "windows", Phase: phase,
			FromVersion: fromVersion, ToVersion: toVersion,
			ProviderReleaseID: providerReleaseID, ReleaseTag: "maestro-v" + toVersion,
			ManifestSHA256:          manifestDigest,
			BootstrapperSHA256:      bootstrapperDigest,
			AuthorityRegistrySHA256: registryDigest,
			NativeSignerID:          "0123456789abcdef0123456789abcdef01234567",
			ActivationReceiptSHA256: activationReceiptDigest,
			State:                   "pass",
			RecordedAt:              time.Date(2026, 7, 25, 12, index, 0, 0, time.UTC),
			Checks:                  expectedPhaseChecks(phase),
		})
		receipts[phase] = path
	}
	report, err := Corporate(CorporateOptions{
		Receipts: receipts,
		Attestation: Attestation{
			Operator: "release-operator", DeviceIDHash: deviceDigest,
			PolicyID:        "bcg-managed-standard-v1",
			ApprovedChannel: "canary", SupportOwner: "pilot-support",
		},
	})
	if err != nil {
		t.Fatalf("Corporate() error = %v", err)
	}
	if report.Release == nil ||
		report.Release.BaselineManifestSHA256 != baselineDigest ||
		report.Release.UpdateManifestSHA256 != updateDigest ||
		report.ReadinessClaim != "corporate_device_operator_attestation" {
		t.Fatalf("corporate report lost release binding: %#v", report.Release)
	}
	for _, scenario := range report.Scenarios {
		if !hashPattern.MatchString(scenario.ReceiptSHA256) {
			t.Fatalf("scenario %s lacks receipt digest: %#v", scenario.Name, scenario)
		}
	}

	update, _, err := ReadPhase(receipts["update"])
	if err != nil {
		t.Fatal(err)
	}
	update.NativeSignerID = "ffffffffffffffffffffffffffffffffffffffff"
	writePhaseReceipt(t, receipts["update"], update)
	if _, err := Corporate(CorporateOptions{
		Receipts:    receipts,
		Attestation: *report.Attestation,
	}); err == nil {
		t.Fatal("Corporate() accepted phase receipts from different native signers")
	}
}

func TestReadPhaseRejectsUnknownOrSensitiveFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receipt.json")
	validReceipt := PhaseReceipt{
		SchemaVersion: 1, RunID: "corp-macos-1", DeviceIDHash: strings.Repeat("a", 64),
		Platform: "macos", Phase: "install", ToVersion: "0.2.0",
		ProviderReleaseID: "12345", ReleaseTag: "maestro-v0.2.0",
		ManifestSHA256: strings.Repeat("b", 64), BootstrapperSHA256: strings.Repeat("c", 64),
		AuthorityRegistrySHA256: strings.Repeat("d", 64), NativeSignerID: "AB12CD34EF",
		State: "pass", RecordedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		Checks: expectedPhaseChecks("install"),
	}
	validBody, err := json.MarshalIndent(validReceipt, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimSuffix(string(validBody), "\n}") + ",\n  \"hostname\": \"must-not-enter\"\n}"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadPhase(path); err == nil {
		t.Fatal("ReadPhase() accepted an unknown device-identifying field")
	}
	valid := strings.Replace(body, ",\n  \"hostname\": \"must-not-enter\"", "", 1)
	duplicate := strings.Replace(
		valid,
		`"run_id": "corp-macos-1",`,
		`"run_id": "corp-macos-1", "run_id": "corp-macos-2",`,
		1,
	)
	if err := os.WriteFile(path, []byte(duplicate), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadPhase(path); err == nil {
		t.Fatal("ReadPhase() accepted duplicate JSON keys")
	}
}

func TestReadPhaseRejectsSymlinkedReceipt(t *testing.T) {
	root := t.TempDir()
	realPath := filepath.Join(root, "real.json")
	writePhaseReceipt(t, realPath, PhaseReceipt{
		SchemaVersion: 1,
		RunID:         "corp-macos-1", DeviceIDHash: strings.Repeat("a", 64),
		Platform: "macos", Phase: "install",
		ToVersion: "0.2.0", ProviderReleaseID: "12345", ReleaseTag: "maestro-v0.2.0",
		ManifestSHA256: strings.Repeat("b", 64), BootstrapperSHA256: strings.Repeat("c", 64),
		AuthorityRegistrySHA256: strings.Repeat("d", 64), NativeSignerID: "AB12CD34EF",
		State: "pass", RecordedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		Checks: expectedPhaseChecks("install"),
	})
	linkPath := filepath.Join(root, "link.json")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Skipf("symlink creation unavailable on this runner: %v", err)
	}
	if _, _, err := ReadPhase(linkPath); err == nil {
		t.Fatal("ReadPhase() accepted a symlinked receipt")
	}
}

func TestPhaseReceiptRejectsArbitraryChecks(t *testing.T) {
	receipt := PhaseReceipt{
		SchemaVersion: 1, RunID: "corp-macos-1", DeviceIDHash: strings.Repeat("a", 64),
		Platform: "macos", Phase: "install", ToVersion: "0.2.0",
		ProviderReleaseID: "12345", ReleaseTag: "maestro-v0.2.0",
		ManifestSHA256: strings.Repeat("b", 64), BootstrapperSHA256: strings.Repeat("c", 64),
		AuthorityRegistrySHA256: strings.Repeat("d", 64), NativeSignerID: "AB12CD34EF",
		State: "pass", RecordedAt: time.Now().UTC(),
		Checks: []string{"everything-is-fine"},
	}
	if err := receipt.Validate(); err == nil {
		t.Fatal("PhaseReceipt.Validate() accepted caller-defined checks")
	}
}

func writePhaseReceipt(t *testing.T, path string, receipt PhaseReceipt) {
	t.Helper()
	body, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
