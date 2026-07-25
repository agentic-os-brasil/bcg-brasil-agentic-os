// Package pilotacceptance classifies distribution evidence without allowing
// isolated CI to masquerade as corporate-device pilot acceptance.
package pilotacceptance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

var (
	identifierPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
	versionPattern    = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	hashPattern       = regexp.MustCompile(`^[a-f0-9]{64}$`)
	providerIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
)

type Report struct {
	SchemaVersion    int             `json:"schema_version"`
	RunID            string          `json:"run_id"`
	Mode             string          `json:"mode"`
	Platform         string          `json:"platform"`
	CandidateVersion string          `json:"candidate_version"`
	ReadinessClaim   string          `json:"readiness_claim"`
	StartedAt        time.Time       `json:"started_at"`
	FinishedAt       time.Time       `json:"finished_at"`
	Scenarios        []Scenario      `json:"scenarios"`
	Release          *ReleaseBinding `json:"release,omitempty"`
	Attestation      *Attestation    `json:"attestation,omitempty"`
}

type Scenario struct {
	Name          string   `json:"name"`
	State         string   `json:"state"`
	FromVersion   string   `json:"from_version,omitempty"`
	ToVersion     string   `json:"to_version,omitempty"`
	Evidence      []string `json:"evidence"`
	ReceiptSHA256 string   `json:"receipt_sha256,omitempty"`
}

type ReleaseBinding struct {
	BaselineProviderReleaseID string `json:"baseline_provider_release_id"`
	BaselineReleaseTag        string `json:"baseline_release_tag"`
	BaselineManifestSHA256    string `json:"baseline_manifest_sha256"`
	UpdateProviderReleaseID   string `json:"update_provider_release_id"`
	UpdateReleaseTag          string `json:"update_release_tag"`
	UpdateManifestSHA256      string `json:"update_manifest_sha256"`
	BootstrapperSHA256        string `json:"bootstrapper_sha256"`
	AuthorityRegistrySHA256   string `json:"authority_registry_sha256"`
	NativeSignerID            string `json:"native_signer_id"`
}

type Attestation struct {
	Operator        string `json:"operator"`
	DeviceIDHash    string `json:"device_id_hash"`
	PolicyID        string `json:"policy_id"`
	ApprovedChannel string `json:"approved_channel"`
	SupportOwner    string `json:"support_owner"`
}

func (report Report) Validate() error {
	if report.SchemaVersion != 2 || !identifierPattern.MatchString(report.RunID) {
		return errors.New("acceptance report identity is invalid")
	}
	if report.Platform != "windows" && report.Platform != "macos" {
		return errors.New("acceptance platform must be windows or macos")
	}
	if !versionPattern.MatchString(report.CandidateVersion) {
		return errors.New("acceptance candidate version is invalid")
	}
	if report.StartedAt.IsZero() || report.FinishedAt.Before(report.StartedAt) {
		return errors.New("acceptance timestamps are invalid")
	}
	seen := map[string]bool{}
	for _, scenario := range report.Scenarios {
		if scenario.Name != "install" && scenario.Name != "update" && scenario.Name != "rollback" {
			return fmt.Errorf("unknown acceptance scenario %q", scenario.Name)
		}
		if seen[scenario.Name] || scenario.State != "pass" || len(scenario.Evidence) == 0 {
			return fmt.Errorf("acceptance scenario %s is duplicate, failed or lacks evidence", scenario.Name)
		}
		for _, evidence := range scenario.Evidence {
			if !identifierPattern.MatchString(evidence) {
				return fmt.Errorf("acceptance evidence label %q is invalid", evidence)
			}
		}
		seen[scenario.Name] = true
	}
	for _, required := range []string{"install", "update", "rollback"} {
		if !seen[required] {
			return fmt.Errorf("acceptance scenario %s is missing", required)
		}
	}
	switch report.Mode {
	case "isolated_ci":
		if report.ReadinessClaim != "engineering_evidence_only" ||
			report.Attestation != nil || report.Release != nil {
			return errors.New("isolated CI may claim engineering evidence only")
		}
		for _, scenario := range report.Scenarios {
			if scenario.ReceiptSHA256 != "" ||
				scenario.FromVersion != "" || scenario.ToVersion != "" {
				return errors.New("isolated CI cannot claim corporate phase transitions or receipts")
			}
		}
	case "corporate_device":
		if report.ReadinessClaim != "corporate_device_operator_attestation" ||
			report.Attestation == nil || report.Release == nil {
			return errors.New("corporate-device evidence requires release binding and an operator attestation")
		}
		if err := report.Attestation.validate(); err != nil {
			return err
		}
		if err := report.Release.validate(); err != nil {
			return err
		}
		for _, scenario := range report.Scenarios {
			if !hashPattern.MatchString(scenario.ReceiptSHA256) {
				return fmt.Errorf("corporate scenario %s requires an exact phase receipt digest", scenario.Name)
			}
			actual := append([]string(nil), scenario.Evidence...)
			sort.Strings(actual)
			expected := expectedPhaseChecks(scenario.Name)
			if len(actual) != len(expected) {
				return fmt.Errorf("corporate scenario %s has an incomplete evidence contract", scenario.Name)
			}
			for index := range expected {
				if actual[index] != expected[index] {
					return fmt.Errorf("corporate scenario %s has an unexpected evidence contract", scenario.Name)
				}
			}
		}
		byName := map[string]Scenario{}
		for _, scenario := range report.Scenarios {
			byName[scenario.Name] = scenario
		}
		install, update, rollback := byName["install"], byName["update"], byName["rollback"]
		if install.FromVersion != "" ||
			!versionPattern.MatchString(install.ToVersion) ||
			update.FromVersion != install.ToVersion ||
			!versionPattern.MatchString(update.ToVersion) ||
			update.ToVersion == install.ToVersion ||
			rollback.FromVersion != update.ToVersion ||
			rollback.ToVersion != install.ToVersion ||
			report.CandidateVersion != update.ToVersion {
			return errors.New("corporate report does not prove none-to-baseline-to-update-to-baseline continuity")
		}
	default:
		return errors.New("acceptance mode must be isolated_ci or corporate_device")
	}
	return nil
}

func (attestation Attestation) validate() error {
	if !identifierPattern.MatchString(attestation.Operator) ||
		!identifierPattern.MatchString(attestation.SupportOwner) ||
		!identifierPattern.MatchString(attestation.PolicyID) ||
		!hashPattern.MatchString(attestation.DeviceIDHash) {
		return errors.New("corporate-device attestation identity is invalid")
	}
	if attestation.ApprovedChannel != "canary" && attestation.ApprovedChannel != "beta" && attestation.ApprovedChannel != "stable" {
		return errors.New("corporate-device approved channel is invalid")
	}
	return nil
}

func (binding ReleaseBinding) validate() error {
	if !providerIDPattern.MatchString(binding.BaselineProviderReleaseID) ||
		!identifierPattern.MatchString(binding.BaselineReleaseTag) ||
		!hashPattern.MatchString(binding.BaselineManifestSHA256) ||
		!providerIDPattern.MatchString(binding.UpdateProviderReleaseID) ||
		!identifierPattern.MatchString(binding.UpdateReleaseTag) ||
		!hashPattern.MatchString(binding.UpdateManifestSHA256) ||
		!hashPattern.MatchString(binding.BootstrapperSHA256) ||
		!hashPattern.MatchString(binding.AuthorityRegistrySHA256) ||
		!identifierPattern.MatchString(binding.NativeSignerID) {
		return errors.New("corporate-device release binding is invalid")
	}
	if binding.BaselineProviderReleaseID == binding.UpdateProviderReleaseID ||
		binding.BaselineReleaseTag == binding.UpdateReleaseTag ||
		binding.BaselineManifestSHA256 == binding.UpdateManifestSHA256 {
		return errors.New("corporate-device baseline and update releases must be distinct")
	}
	return nil
}

func Isolated(runID, platform, version string, started, finished time.Time) Report {
	return Report{
		SchemaVersion: 2, RunID: runID, Mode: "isolated_ci", Platform: platform,
		CandidateVersion: version, ReadinessClaim: "engineering_evidence_only",
		StartedAt: started.UTC(), FinishedAt: finished.UTC(),
		Scenarios: []Scenario{
			{Name: "install", State: "pass", Evidence: []string{"trial-smoke"}},
			{Name: "update", State: "pass", Evidence: []string{"transaction-tests"}},
			{Name: "rollback", State: "pass", Evidence: []string{"rollback-tests"}},
		},
	}
}

func Read(path string) (Report, error) {
	body, err := readEvidenceFile(path)
	if err != nil {
		return Report{}, err
	}
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return Report{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var report Report
	if err := decoder.Decode(&report); err != nil {
		return Report{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Report{}, errors.New("acceptance report contains multiple JSON values")
		}
		return Report{}, err
	}
	if err := report.Validate(); err != nil {
		return Report{}, err
	}
	return report, nil
}

func Write(path string, report Report) error {
	if err := report.Validate(); err != nil {
		return err
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}
