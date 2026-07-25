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
	"time"
)

var (
	identifierPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
	versionPattern    = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	hashPattern       = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type Report struct {
	SchemaVersion    int          `json:"schema_version"`
	RunID            string       `json:"run_id"`
	Mode             string       `json:"mode"`
	Platform         string       `json:"platform"`
	CandidateVersion string       `json:"candidate_version"`
	ReadinessClaim   string       `json:"readiness_claim"`
	StartedAt        time.Time    `json:"started_at"`
	FinishedAt       time.Time    `json:"finished_at"`
	Scenarios        []Scenario   `json:"scenarios"`
	Attestation      *Attestation `json:"attestation,omitempty"`
}

type Scenario struct {
	Name     string   `json:"name"`
	State    string   `json:"state"`
	Evidence []string `json:"evidence"`
}

type Attestation struct {
	Operator              string `json:"operator"`
	DeviceIDHash          string `json:"device_id_hash"`
	PolicyContext         string `json:"policy_context"`
	ApprovedChannel       string `json:"approved_channel"`
	SignedManifest        bool   `json:"signed_manifest"`
	NativeCodeSigning     bool   `json:"native_code_signing"`
	AuthenticatedProvider bool   `json:"authenticated_provider"`
}

func (report Report) Validate() error {
	if report.SchemaVersion != 1 || !identifierPattern.MatchString(report.RunID) {
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
		if report.ReadinessClaim != "engineering_evidence_only" || report.Attestation != nil {
			return errors.New("isolated CI may claim engineering evidence only")
		}
	case "corporate_device":
		if report.ReadinessClaim != "corporate_device_acceptance" || report.Attestation == nil {
			return errors.New("corporate-device acceptance requires its attestation")
		}
		if err := report.Attestation.validate(); err != nil {
			return err
		}
	default:
		return errors.New("acceptance mode must be isolated_ci or corporate_device")
	}
	return nil
}

func (attestation Attestation) validate() error {
	if !identifierPattern.MatchString(attestation.Operator) || !hashPattern.MatchString(attestation.DeviceIDHash) {
		return errors.New("corporate-device attestation identity is invalid")
	}
	if attestation.PolicyContext == "" || len(attestation.PolicyContext) > 512 {
		return errors.New("corporate-device policy context is invalid")
	}
	if attestation.ApprovedChannel != "canary" && attestation.ApprovedChannel != "beta" && attestation.ApprovedChannel != "stable" {
		return errors.New("corporate-device approved channel is invalid")
	}
	if !attestation.SignedManifest || !attestation.NativeCodeSigning || !attestation.AuthenticatedProvider {
		return errors.New("corporate-device acceptance requires every release authority")
	}
	return nil
}

func Isolated(runID, platform, version string, started, finished time.Time) Report {
	return Report{
		SchemaVersion: 1, RunID: runID, Mode: "isolated_ci", Platform: platform,
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
	body, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}
	if len(body) > 1<<20 {
		return Report{}, errors.New("acceptance report exceeds 1 MiB")
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
	file, err := os.CreateTemp(filepath.Dir(path), ".acceptance-")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
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
	return os.Rename(temp, path)
}
