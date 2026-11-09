package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	maximumVisibilityIdentities      = 4096
	maximumVisibilityCheckpointBytes = 1 << 20
)

var errVisibilityCheckpointUnavailable = errors.New("required memory visibility checkpoint unavailable")

type visibilityCheckpointRequirement struct {
	SchemaVersion int    `json:"schema_version"`
	WorkspaceID   string `json:"workspace_id"`
}

func (engine *Engine) visibilityCheckpointRequired(workspaceID string) (bool, error) {
	body, err := memoryReadFile(filepath.Join(engine.workspaceRoot(workspaceID), ".visibility-checkpoint-required"), 512)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var requirement visibilityCheckpointRequirement
	if err := decodeMemoryJSON(body, &requirement); err != nil {
		return false, err
	}
	if requirement.SchemaVersion != 1 || requirement.WorkspaceID != workspaceID {
		return false, errors.New("invalid memory visibility checkpoint requirement")
	}
	return true, nil
}

func (engine *Engine) readVisibility(workspaceID string) (visibilityBarrier, error) {
	empty := visibilityBarrier{SchemaVersion: 1, WorkspaceID: workspaceID, Denied: map[string]uint64{}}
	if err := validateWorkspaceID(workspaceID); err != nil {
		return empty, err
	}
	required, err := engine.visibilityCheckpointRequired(workspaceID)
	if err != nil {
		return empty, err
	}
	if !required {
		return engine.readLegacyVisibility(workspaceID)
	}
	body, err := memoryReadFile(filepath.Join(engine.workspaceRoot(workspaceID), ".visibility", "checkpoint.json"), maximumVisibilityCheckpointBytes)
	if err != nil {
		return empty, fmt.Errorf("%w: %v", errVisibilityCheckpointUnavailable, err)
	}
	var state visibilityBarrier
	if err := decodeMemoryJSON(body, &state); err != nil {
		return empty, err
	}
	if err := validateVisibilityState(workspaceID, state); err != nil {
		return empty, err
	}
	return state, nil
}

func validateVisibilityState(workspaceID string, b visibilityBarrier) error {
	if b.SchemaVersion != 1 || b.WorkspaceID != workspaceID || b.Revision == 0 || b.Generation > b.Revision || b.LegacyThrough > 4096 || b.LegacyThrough > b.Revision || b.Denied == nil || len(b.Denied) > maximumVisibilityIdentities || (len(b.Denied) > 0 && b.Generation == 0) {
		return errors.New("invalid memory visibility checkpoint")
	}
	for id, revision := range b.Denied {
		if !ownedCaptureIDPattern.MatchString(id) || revision == 0 {
			return errors.New("invalid denied capture identity")
		}
	}
	return nil
}

func (engine *Engine) publishDenial(workspaceID string, denied map[string]uint64) (visibilityBarrier, error) {
	return engine.publishVisibility(workspaceID, denied, "")
}
func (engine *Engine) publishCaptureVisibility(workspaceID, id string) (visibilityBarrier, error) {
	return engine.publishVisibility(workspaceID, nil, id)
}

// Permanent tombstones reserve one slot per admitted identity, including live
// captures, so reaching the finite budget cannot consume removal capacity.
func (engine *Engine) reserveCaptureIdentity(workspaceID, id string, b visibilityBarrier) error {
	if b.Denied[id] > 0 {
		return ErrCaptureConflict
	}
	known := make(map[string]bool, len(b.Denied)+1)
	for denied := range b.Denied {
		known[denied] = true
	}
	journal, err := engine.journalCaptures(workspaceID)
	if err != nil {
		return err
	}
	owned, err := engine.ownedCaptures(workspaceID)
	if err != nil {
		return err
	}
	if len(journal)+len(owned) >= maximumVisibilityIdentities {
		return ErrCaptureCapacity
	}
	for _, c := range append(journal, owned...) {
		known[c.ID] = true
	}
	known[id] = true
	if len(known) > maximumVisibilityIdentities {
		return ErrCaptureCapacity
	}
	return nil
}

// The caller holds the existing workspace activation lock. The required marker
// precedes migration publication; a missing checkpoint can never reopen history.
func (engine *Engine) publishVisibility(workspaceID string, denied map[string]uint64, captureID string) (visibilityBarrier, error) {
	b, err := engine.readVisibility(workspaceID)
	if err != nil {
		return b, err
	}
	if b.Revision == ^uint64(0) {
		return b, ErrCaptureCapacity
	}
	if captureID != "" {
		if err := engine.reserveCaptureIdentity(workspaceID, captureID, b); err != nil {
			return b, err
		}
	}
	required, err := engine.visibilityCheckpointRequired(workspaceID)
	if err != nil {
		return b, err
	}
	if !required {
		b.LegacyThrough = b.Revision
	}
	b.Revision++
	if len(denied) > 0 {
		b.Generation = b.Revision
	}
	for id, revision := range denied {
		if revision > b.Denied[id] {
			b.Denied[id] = revision
		}
	}
	if len(b.Denied) > maximumVisibilityIdentities {
		return b, ErrCaptureCapacity
	}
	if err := validateVisibilityState(workspaceID, b); err != nil {
		return b, err
	}
	body, err := json.Marshal(b)
	if err != nil {
		return b, err
	}
	if len(body) > maximumVisibilityCheckpointBytes {
		return b, ErrCaptureCapacity
	}
	directory := filepath.Join(engine.workspaceRoot(workspaceID), ".visibility")
	if err := memoryMkdirAll(directory, 0700); err != nil {
		return b, err
	}
	if !required {
		requirement, _ := json.Marshal(visibilityCheckpointRequirement{SchemaVersion: 1, WorkspaceID: workspaceID})
		if err := memoryPublish(filepath.Join(engine.workspaceRoot(workspaceID), ".visibility-checkpoint-required"), requirement, true); err != nil {
			return b, err
		}
		if err := engine.fault("after_visibility_checkpoint_required"); err != nil {
			return b, err
		}
	}
	if err := engine.fault("before_visibility_checkpoint_publish"); err != nil {
		return b, err
	}
	if err := memoryPublish(filepath.Join(directory, "checkpoint.json"), body, !required); err != nil {
		return b, err
	}
	if err := engine.fault("after_visibility_checkpoint_publish"); err != nil {
		return b, err
	}
	if b.LegacyThrough > 0 {
		if err := memoryPruneVisibilityHistory(directory, b.LegacyThrough, func() error { return engine.fault("during_visibility_history_cleanup") }); err != nil {
			return b, err
		}
		b.LegacyThrough = 0
		body, err = json.Marshal(b)
		if err != nil {
			return b, err
		}
		if err := memoryPublish(filepath.Join(directory, "checkpoint.json"), body, false); err != nil {
			return b, err
		}
		if err := engine.fault("after_visibility_history_cleanup"); err != nil {
			return b, err
		}
	}
	return b, nil
}
