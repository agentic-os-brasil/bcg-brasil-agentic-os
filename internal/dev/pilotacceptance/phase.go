package pilotacceptance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"
)

type PhaseReceipt struct {
	SchemaVersion           int       `json:"schema_version"`
	RunID                   string    `json:"run_id"`
	DeviceIDHash            string    `json:"device_id_hash"`
	Platform                string    `json:"platform"`
	Phase                   string    `json:"phase"`
	FromVersion             string    `json:"from_version"`
	ToVersion               string    `json:"to_version"`
	ProviderReleaseID       string    `json:"provider_release_id"`
	ReleaseTag              string    `json:"release_tag"`
	ManifestSHA256          string    `json:"manifest_sha256"`
	BootstrapperSHA256      string    `json:"bootstrapper_sha256"`
	AuthorityRegistrySHA256 string    `json:"authority_registry_sha256"`
	NativeSignerID          string    `json:"native_signer_id"`
	ActivationReceiptSHA256 string    `json:"activation_receipt_sha256,omitempty"`
	State                   string    `json:"state"`
	RecordedAt              time.Time `json:"recorded_at"`
	Checks                  []string  `json:"checks"`
}

type CorporateOptions struct {
	Receipts    map[string]string
	Attestation Attestation
}

func (receipt PhaseReceipt) Validate() error {
	if receipt.SchemaVersion != 1 ||
		!identifierPattern.MatchString(receipt.RunID) ||
		!hashPattern.MatchString(receipt.DeviceIDHash) ||
		!versionPattern.MatchString(receipt.ToVersion) ||
		!providerIDPattern.MatchString(receipt.ProviderReleaseID) ||
		receipt.ReleaseTag != "maestro-v"+receipt.ToVersion ||
		!hashPattern.MatchString(receipt.ManifestSHA256) ||
		!hashPattern.MatchString(receipt.BootstrapperSHA256) ||
		!hashPattern.MatchString(receipt.AuthorityRegistrySHA256) ||
		!identifierPattern.MatchString(receipt.NativeSignerID) ||
		receipt.RecordedAt.IsZero() ||
		receipt.State != "pass" {
		return errors.New("clean-device phase receipt identity is invalid")
	}
	if receipt.Platform != "windows" && receipt.Platform != "macos" {
		return errors.New("clean-device phase platform must be windows or macos")
	}
	if receipt.Phase != "install" && receipt.Phase != "update" && receipt.Phase != "rollback" {
		return errors.New("clean-device phase must be install, update or rollback")
	}
	if receipt.Phase == "install" {
		if receipt.FromVersion != "" {
			return errors.New("clean-device install receipt cannot have a source version")
		}
		if receipt.ActivationReceiptSHA256 != "" {
			return errors.New("clean-device install receipt cannot bind an update activation receipt")
		}
	} else {
		if !versionPattern.MatchString(receipt.FromVersion) {
			return errors.New("clean-device update and rollback receipts require a source version")
		}
		if !hashPattern.MatchString(receipt.ActivationReceiptSHA256) {
			return errors.New("clean-device update and rollback receipts require an activation receipt digest")
		}
	}
	expected := expectedPhaseChecks(receipt.Phase)
	actual := append([]string(nil), receipt.Checks...)
	sort.Strings(actual)
	if len(actual) != len(expected) {
		return fmt.Errorf("clean-device %s receipt has an incomplete check contract", receipt.Phase)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			return fmt.Errorf("clean-device %s receipt has an unexpected check contract", receipt.Phase)
		}
	}
	return nil
}

func expectedPhaseChecks(phase string) []string {
	common := []string{
		"active-cli-self-check",
		"authority-seed-bound",
		"doctor-verified",
		"native-bootstrapper-signature",
		"native-cli-signature",
		"owner-data-sentinel-preserved",
		"status-verified",
	}
	switch phase {
	case "install":
		common = append(common,
			"first-install-activated",
			"provider-release-operator-asserted",
			"signed-release-verified",
		)
	case "update":
		common = append(common,
			"activation-receipt-bound",
			"exact-plan-confirmed",
			"pending-provider-bound",
			"signed-update-activated",
		)
	case "rollback":
		common = append(common,
			"activation-receipt-bound",
			"last-known-good-restored",
			"provider-release-operator-asserted",
		)
	}
	sort.Strings(common)
	return common
}

func ReadPhase(path string) (PhaseReceipt, string, error) {
	body, err := readEvidenceFile(path)
	if err != nil {
		return PhaseReceipt{}, "", err
	}
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return PhaseReceipt{}, "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var receipt PhaseReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return PhaseReceipt{}, "", err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return PhaseReceipt{}, "", errors.New("clean-device phase receipt contains multiple JSON values")
		}
		return PhaseReceipt{}, "", err
	}
	if err := receipt.Validate(); err != nil {
		return PhaseReceipt{}, "", err
	}
	sum := sha256.Sum256(body)
	return receipt, hex.EncodeToString(sum[:]), nil
}

func Corporate(options CorporateOptions) (Report, error) {
	required := []string{"install", "update", "rollback"}
	receipts := make([]PhaseReceipt, 0, len(required))
	digests := map[string]string{}
	for _, phase := range required {
		path := options.Receipts[phase]
		if path == "" {
			return Report{}, fmt.Errorf("corporate-device phase receipt %s is required", phase)
		}
		receipt, digest, err := ReadPhase(path)
		if err != nil {
			return Report{}, fmt.Errorf("read %s receipt: %w", phase, err)
		}
		if receipt.Phase != phase {
			return Report{}, fmt.Errorf("%s receipt declares phase %s", phase, receipt.Phase)
		}
		receipts = append(receipts, receipt)
		digests[phase] = digest
	}
	installReceipt, updateReceipt, rollbackReceipt := receipts[0], receipts[1], receipts[2]
	if updateReceipt.FromVersion != installReceipt.ToVersion ||
		updateReceipt.ToVersion == installReceipt.ToVersion ||
		rollbackReceipt.FromVersion != updateReceipt.ToVersion ||
		rollbackReceipt.ToVersion != installReceipt.ToVersion {
		return Report{}, errors.New("corporate-device receipts do not prove install, update and rollback continuity")
	}
	if updateReceipt.RecordedAt.Before(installReceipt.RecordedAt) ||
		rollbackReceipt.RecordedAt.Before(updateReceipt.RecordedAt) {
		return Report{}, errors.New("corporate-device phase receipts are not chronologically ordered")
	}
	if rollbackReceipt.ManifestSHA256 != installReceipt.ManifestSHA256 {
		return Report{}, errors.New("corporate-device receipts do not match the baseline and update manifests")
	}
	if rollbackReceipt.ProviderReleaseID != installReceipt.ProviderReleaseID ||
		rollbackReceipt.ReleaseTag != installReceipt.ReleaseTag {
		return Report{}, errors.New("corporate-device receipts do not match the baseline and update provider identities")
	}
	if updateReceipt.ActivationReceiptSHA256 != rollbackReceipt.ActivationReceiptSHA256 {
		return Report{}, errors.New("corporate-device update and rollback do not bind one activation receipt")
	}
	for _, receipt := range receipts {
		if receipt.BootstrapperSHA256 != installReceipt.BootstrapperSHA256 ||
			receipt.AuthorityRegistrySHA256 != installReceipt.AuthorityRegistrySHA256 ||
			receipt.NativeSignerID != installReceipt.NativeSignerID {
			return Report{}, errors.New("corporate-device receipts do not bind one bootstrapper authority and native signer")
		}
	}
	first := installReceipt
	if options.Attestation.DeviceIDHash != first.DeviceIDHash {
		return Report{}, errors.New("corporate-device operator attestation does not match the receipt device identity")
	}
	started, finished := first.RecordedAt, first.RecordedAt
	scenarios := make([]Scenario, 0, len(receipts))
	for _, receipt := range receipts {
		if receipt.RunID != first.RunID ||
			receipt.DeviceIDHash != first.DeviceIDHash ||
			receipt.Platform != first.Platform {
			return Report{}, errors.New("corporate-device phase receipts are not one device run")
		}
		if receipt.RecordedAt.Before(started) {
			started = receipt.RecordedAt
		}
		if receipt.RecordedAt.After(finished) {
			finished = receipt.RecordedAt
		}
		checks := append([]string(nil), receipt.Checks...)
		sort.Strings(checks)
		scenarios = append(scenarios, Scenario{
			Name: receipt.Phase, State: receipt.State,
			FromVersion: receipt.FromVersion, ToVersion: receipt.ToVersion,
			Evidence: checks, ReceiptSHA256: digests[receipt.Phase],
		})
	}
	report := Report{
		SchemaVersion: 2,
		RunID:         first.RunID, Mode: "corporate_device", Platform: first.Platform,
		CandidateVersion: updateReceipt.ToVersion,
		ReadinessClaim:   "corporate_device_operator_attestation",
		StartedAt:        started.UTC(), FinishedAt: finished.UTC(),
		Scenarios: scenarios,
		Release: &ReleaseBinding{
			BaselineProviderReleaseID: installReceipt.ProviderReleaseID,
			BaselineReleaseTag:        installReceipt.ReleaseTag,
			BaselineManifestSHA256:    installReceipt.ManifestSHA256,
			UpdateProviderReleaseID:   updateReceipt.ProviderReleaseID,
			UpdateReleaseTag:          updateReceipt.ReleaseTag,
			UpdateManifestSHA256:      updateReceipt.ManifestSHA256,
			BootstrapperSHA256:        installReceipt.BootstrapperSHA256,
			AuthorityRegistrySHA256:   installReceipt.AuthorityRegistrySHA256,
			NativeSignerID:            installReceipt.NativeSignerID,
		},
		Attestation: &options.Attestation,
	}
	if err := report.Validate(); err != nil {
		return Report{}, err
	}
	return report, nil
}
