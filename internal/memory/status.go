package memory

import (
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type StatusReport struct {
	WorkspaceID          string            `json:"workspace_id"`
	State                string            `json:"state"`
	CaptureFiles         int               `json:"capture_files"`
	AttestedCaptureFiles int               `json:"attested_capture_files"`
	TransactionID        string            `json:"transaction_id,omitempty"`
	CommittedAt          time.Time         `json:"committed_at,omitempty"`
	Layers               map[string]string `json:"layers"`
	ActivationLocked     bool              `json:"activation_locked"`
}

func (engine *Engine) Status(workspaceID string) (StatusReport, error) {
	if strings.TrimSpace(engine.Root) == "" {
		return StatusReport{}, errors.New("memory root is required")
	}
	if err := validateWorkspaceID(workspaceID); err != nil {
		return StatusReport{}, err
	}
	barrier, err := engine.readVisibility(workspaceID)
	if err != nil {
		return StatusReport{}, err
	}
	report := StatusReport{WorkspaceID: workspaceID, State: "empty", Layers: make(map[string]string)}
	for _, directory := range []string{"captures", "attested-captures"} {
		paths, err := engine.journalPaths(workspaceID, directory)
		if err != nil {
			return StatusReport{}, err
		}
		for _, path := range paths {
			body, err := memoryReadFile(path, 1<<20)
			if err != nil {
				return StatusReport{}, err
			}
			visible, _, err := engine.filterJournal(workspaceID, body, directory == "attested-captures", barrier)
			if err != nil {
				return StatusReport{}, err
			}
			if len(visible) > 0 {
				report.CaptureFiles++
				if directory == "attested-captures" {
					report.AttestedCaptureFiles++
				}
			}
		}
	}
	owned, err := engine.ownedCaptures(workspaceID)
	if err != nil {
		return StatusReport{}, err
	}
	for _, c := range owned {
		if c.Revision > barrier.Denied[c.ID] {
			report.CaptureFiles++
			report.AttestedCaptureFiles++
		}
	}
	if report.CaptureFiles > 0 {
		report.State = "captured"
	}
	lockPath := filepath.Join(engine.workspaceRoot(workspaceID), ".locks", "activation.lock")
	if err := scheduler.ValidateDirectory(lockPath); err == nil {
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
	if len(manifest.Artifacts) > 0 {
		report.State = "ready"
	}
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
	after, err := engine.readVisibility(workspaceID)
	if err != nil {
		return StatusReport{}, err
	}
	if after.Revision != barrier.Revision {
		return StatusReport{}, ErrCaptureConflict
	}
	return report, nil
}
