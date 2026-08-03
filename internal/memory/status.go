package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type StatusReport struct {
	WorkspaceID      string            `json:"workspace_id"`
	State            string            `json:"state"`
	CaptureFiles     int               `json:"capture_files"`
	TransactionID    string            `json:"transaction_id,omitempty"`
	CommittedAt      time.Time         `json:"committed_at,omitempty"`
	Layers           map[string]string `json:"layers"`
	ActivationLocked bool              `json:"activation_locked"`
}

func (engine *Engine) Status(workspaceID string) (StatusReport, error) {
	if strings.TrimSpace(engine.Root) == "" {
		return StatusReport{}, errors.New("memory root is required")
	}
	if err := validateWorkspaceID(workspaceID); err != nil {
		return StatusReport{}, err
	}
	report := StatusReport{WorkspaceID: workspaceID, State: "empty", Layers: make(map[string]string)}
	captures, err := filepath.Glob(filepath.Join(engine.workspaceRoot(workspaceID), "l1", "captures", "*.jsonl"))
	if err != nil {
		return StatusReport{}, err
	}
	report.CaptureFiles = len(captures)
	attested, err := filepath.Glob(filepath.Join(engine.workspaceRoot(workspaceID), "l1", "attested-captures", "*.jsonl"))
	if err != nil {
		return StatusReport{}, err
	}
	report.CaptureFiles += len(attested)
	if report.CaptureFiles > 0 {
		report.State = "captured"
	}
	lockPath := filepath.Join(engine.workspaceRoot(workspaceID), ".locks", "activation.lock")
	if _, err := os.Stat(lockPath); err == nil {
		report.ActivationLocked = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return StatusReport{}, err
	}
	manifest, _, err := engine.latestManifest(workspaceID)
	if errors.Is(err, os.ErrNotExist) {
		return report, nil
	}
	if errors.Is(err, ErrNoValidCommit) {
		report.State = "corrupt"
		return report, nil
	}
	if err != nil {
		return StatusReport{}, err
	}
	report.State = "ready"
	report.TransactionID = manifest.TransactionID
	report.CommittedAt = manifest.CommittedAt
	for key := range manifest.Artifacts {
		artifact, _, err := engine.readArtifactFromManifest(workspaceID, manifest, key)
		if err != nil {
			return StatusReport{}, err
		}
		if current := report.Layers[artifact.Layer]; artifact.Period > current {
			report.Layers[artifact.Layer] = artifact.Period
		}
	}
	return report, nil
}
